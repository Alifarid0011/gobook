package main

import (
	"context"
	"crypto/subtle"
	"encoding/json"
	"errors"
	"io"
	"net/http"
)

func (s *Server) releaseExpiredNative() {
	s.nativeMu.Lock()
	defer s.nativeMu.Unlock()
	if s.nativeActive && !s.nativeStarting && !s.nativeStopping && s.Native != nil && !s.Native.State().Active {
		s.nativeActive = false
		s.mu.Lock()
		s.busy = false
		s.mu.Unlock()
	}
}

func (s *Server) writeNative(w http.ResponseWriter, snapshot NativeSnapshot) {
	s.nativeMu.Lock()
	notebook := s.nativeNotebook
	s.nativeMu.Unlock()
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(struct {
		NativeSnapshot
		Notebook *Notebook `json:"notebook,omitempty"`
	}{snapshot, notebook})
}

func (s *Server) nativeHTTP(w http.ResponseWriter, r *http.Request) {
	if subtle.ConstantTimeCompare([]byte(r.Header.Get("Authorization")), []byte("Bearer "+s.Token)) != 1 {
		http.Error(w, "unauthorized", 401)
		return
	}
	if s.Native == nil {
		http.Error(w, "native memory kernel unavailable", 503)
		return
	}
	method := http.MethodPost
	switch r.URL.Path {
	case "/api/native/state":
		method = http.MethodGet
	case "/api/native/start", "/api/native/execute", "/api/native/stop":
	default:
		http.NotFound(w, r)
		return
	}
	if r.Method != method {
		w.Header().Set("Allow", method)
		http.Error(w, method+" required", 405)
		return
	}
	if method == http.MethodGet {
		s.releaseExpiredNative()
		s.writeNative(w, s.Native.State())
		return
	}
	r.Body = http.MaxBytesReader(w, r.Body, 256<<10)
	defer r.Body.Close()
	var request struct {
		Notebook      Notebook `json:"notebook"`
		Cell          int      `json:"cell"`
		SessionID     string   `json:"sessionId"`
		HistoryLength *int     `json:"historyLength"`
	}
	decoder := json.NewDecoder(r.Body)
	decoder.DisallowUnknownFields()
	err := decoder.Decode(&request)
	if err == nil {
		var extra any
		if next := decoder.Decode(&extra); next != io.EOF {
			err = errors.New("expected one request object")
		}
	}
	if err != nil {
		var limit *http.MaxBytesError
		if errors.As(err, &limit) {
			http.Error(w, "request exceeds 256 KiB", 413)
		} else {
			http.Error(w, err.Error(), 400)
		}
		return
	}
	switch r.URL.Path {
	case "/api/native/stop":
		s.nativeMu.Lock()
		if s.nativeStopping {
			s.nativeMu.Unlock()
			http.Error(w, "kernel stop already in progress", 409)
			return
		}
		if request.SessionID != "" && request.SessionID != s.Native.State().SessionID {
			s.nativeMu.Unlock()
			http.Error(w, "kernel changed; refresh status before stopping it", 409)
			return
		}
		owned := s.nativeActive
		if owned {
			s.nativeStopping = true
		}
		generation := s.nativeGeneration
		cancel := s.nativeCancel
		s.nativeMu.Unlock()
		if owned {
			if cancel != nil {
				cancel()
			}
			if err := s.Native.Stop(); err != nil {
				s.nativeMu.Lock()
				s.nativeStopping = false
				s.nativeMu.Unlock()
				http.Error(w, err.Error(), 500)
				return
			}
			s.nativeMu.Lock()
			s.nativeStopping = false
			if s.nativeGeneration == generation && s.nativeActive {
				s.nativeActive = false
				s.nativeStarting = false
				s.nativeGeneration++
				s.mu.Lock()
				s.busy = false
				s.mu.Unlock()
			}
			s.nativeMu.Unlock()
		}
		s.writeNative(w, s.Native.State())
	case "/api/native/start":
		s.releaseExpiredDebug(r.Context())
		s.releaseExpiredNative()
		s.nativeMu.Lock()
		s.mu.Lock()
		if s.busy || s.nativeActive || s.nativeStopping {
			s.mu.Unlock()
			s.nativeMu.Unlock()
			http.Error(w, "stop the current run or kernel before starting a memory session", 409)
			return
		}
		s.busy = true
		s.nativeActive = true
		s.nativeStarting = true
		s.nativeGeneration++
		generation := s.nativeGeneration
		ctx, cancel := context.WithCancel(r.Context())
		s.nativeCancel = cancel
		s.nativeNotebook = &request.Notebook
		s.mu.Unlock()
		s.nativeMu.Unlock()
		snapshot, err := s.Native.Start(ctx, request.Notebook)
		cancel()
		s.nativeMu.Lock()
		if s.nativeGeneration == generation {
			s.nativeStarting = false
			s.nativeCancel = nil
			if err != nil && !s.nativeStopping {
				s.nativeActive = false
				s.mu.Lock()
				s.busy = false
				s.mu.Unlock()
			}
		}
		s.nativeMu.Unlock()
		if err != nil {
			snapshot.Error = err.Error()
		}
		s.writeNative(w, snapshot)
	case "/api/native/execute":
		s.nativeMu.Lock()
		owned := s.nativeActive && !s.nativeStarting && !s.nativeStopping
		s.nativeMu.Unlock()
		if !owned {
			http.Error(w, "start a memory kernel first", 409)
			return
		}
		if request.HistoryLength == nil || *request.HistoryLength < 0 || request.SessionID == "" {
			http.Error(w, "sessionId and historyLength are required; refresh kernel status", 400)
			return
		}
		snapshot, err := s.Native.ExecuteExpected(r.Context(), request.Notebook, request.Cell, request.SessionID, *request.HistoryLength)
		if err != nil {
			snapshot.Error = err.Error()
		}
		s.releaseExpiredNative()
		s.writeNative(w, snapshot)
	}
}
