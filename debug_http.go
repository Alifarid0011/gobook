package main

import (
	"context"
	"crypto/subtle"
	"encoding/json"
	"errors"
	"io"
	"net/http"
)

// ServeHTTP checks Host, Origin and Fetch Metadata before dispatching here.
func (s *Server) debugHTTP(w http.ResponseWriter, r *http.Request) {
	if subtle.ConstantTimeCompare([]byte(r.Header.Get("Authorization")), []byte("Bearer "+s.Token)) != 1 {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}
	method := http.MethodPost
	switch r.URL.Path {
	case "/api/debug/state":
		method = http.MethodGet
	case "/api/debug/start", "/api/debug/command", "/api/debug/stop":
	default:
		http.NotFound(w, r)
		return
	}
	if r.Method != method {
		w.Header().Set("Allow", method)
		http.Error(w, method+" required", http.StatusMethodNotAllowed)
		return
	}
	if s.Debugger == nil {
		http.Error(w, "debugger unavailable", http.StatusNotImplemented)
		return
	}
	if r.URL.Path == "/api/debug/start" {
		s.releaseExpiredDebug(r.Context())
	}
	s.debugMu.Lock()
	defer s.debugMu.Unlock()
	write := func(snap DebugSnapshot, err error) {
		w.Header().Set("Content-Type", "application/json")
		if err != nil {
			snap.Error = err.Error()
			w.WriteHeader(http.StatusBadRequest)
		}
		_ = json.NewEncoder(w).Encode(snap)
	}
	idle := DebugSnapshot{Exited: true, Locals: []DebugVariable{}, Arguments: []DebugVariable{}, Stack: []DebugFrame{}, Breakpoints: []DebugBreakpoint{}}
	release := func() {
		if s.debugActive {
			s.mu.Lock()
			s.busy = false
			s.mu.Unlock()
			s.debugActive = false
		}
	}
	decode := func(v any) bool {
		r.Body = http.MaxBytesReader(w, r.Body, 256<<10)
		defer r.Body.Close()
		dec := json.NewDecoder(r.Body)
		dec.DisallowUnknownFields()
		err := dec.Decode(v)
		if err == nil {
			var extra any
			err = dec.Decode(&extra)
			if err == io.EOF {
				err = nil
			} else if err == nil {
				err = errors.New("expected a single JSON object")
			}
		}
		if err != nil {
			var limit *http.MaxBytesError
			if errors.As(err, &limit) {
				http.Error(w, "debug request exceeds 256 KiB", http.StatusRequestEntityTooLarge)
			} else {
				http.Error(w, err.Error(), http.StatusBadRequest)
			}
			return false
		}
		return true
	}
	switch r.URL.Path {
	case "/api/debug/start":
		var request struct {
			Notebook    Notebook          `json:"notebook"`
			Breakpoints []DebugBreakpoint `json:"breakpoints"`
		}
		if !decode(&request) {
			return
		}
		s.mu.Lock()
		if s.busy {
			s.mu.Unlock()
			http.Error(w, "a notebook is already running or debugging", http.StatusConflict)
			return
		}
		s.busy = true
		s.mu.Unlock()
		snap, err := s.Debugger.Start(r.Context(), request.Notebook, request.Breakpoints)
		if err != nil {
			s.mu.Lock()
			s.busy = false
			s.mu.Unlock()
		} else {
			s.debugActive = true
		}
		write(snap, err)
	case "/api/debug/command":
		var request struct {
			Command string `json:"command"`
		}
		if !decode(&request) {
			return
		}
		if !s.debugActive {
			write(idle, errors.New("no debugger session"))
			return
		}
		snap, err := s.Debugger.Command(r.Context(), request.Command)
		write(snap, err)
	case "/api/debug/state":
		if !s.debugActive {
			write(idle, nil)
			return
		}
		snap, err := s.Debugger.State(r.Context())
		if err == nil && snap.Exited {
			err = s.Debugger.Stop()
			release()
		}
		write(snap, err)
	case "/api/debug/stop":
		// Stop accepts an empty body or {} only, preserving strict JSON boundaries.
		if r.ContentLength != 0 {
			var request struct{}
			if !decode(&request) {
				return
			}
		}
		if !s.debugActive {
			write(idle, nil)
			return
		}
		err := s.Debugger.Stop()
		release()
		write(idle, err)
	}
}

// Reclaim an expired session even when its browser stopped polling. Call this
// before reserving the kernel for a normal run, with no Server locks held.
func (s *Server) releaseExpiredDebug(ctx context.Context) {
	s.debugMu.Lock()
	defer s.debugMu.Unlock()
	if !s.debugActive || s.Debugger == nil {
		return
	}
	snap, err := s.Debugger.State(ctx)
	if err == nil && !snap.Exited {
		return
	}
	// An absent manager session also releases this debugger-owned reservation.
	_ = s.Debugger.Stop()
	s.debugActive = false
	s.mu.Lock()
	s.busy = false
	s.mu.Unlock()
}
