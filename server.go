package main

import (
	"crypto/subtle"
	"embed"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strings"
	"sync"
)

//go:embed ui.html ui.css editor.js debug_ui.js examples/*.gobook.json
var ui embed.FS

type Server struct {
	Token, Host  string
	PublicOrigin string
	Kernel       Kernel
	Debugger     *DebugManager
	debugMu      sync.Mutex
	debugActive  bool
	mu           sync.Mutex
	busy         bool
}

func (s *Server) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("X-Content-Type-Options", "nosniff")
	w.Header().Set("Referrer-Policy", "no-referrer")
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("Content-Security-Policy", "default-src 'none'; script-src 'self'; style-src 'self' 'unsafe-inline'; img-src 'self' data:; manifest-src 'self'; connect-src 'self'; base-uri 'none'; frame-ancestors 'none'; form-action 'none'")
	expectedOrigin := "http://" + s.Host
	if r.Host != s.Host {
		if s.PublicOrigin == "" || r.Host != strings.TrimPrefix(s.PublicOrigin, "https://") {
			http.Error(w, "invalid host", 403)
			return
		}
		expectedOrigin = s.PublicOrigin
	}
	if origin := r.Header.Get("Origin"); origin != "" && origin != expectedOrigin {
		http.Error(w, "invalid origin", 403)
		return
	}

	if r.Header.Get("Sec-Fetch-Site") == "cross-site" {
		http.Error(w, "cross-site request refused", http.StatusForbidden)
		return
	}
	if r.URL.Path == "/" && r.Method == http.MethodGet {
		if supplied := r.URL.Query().Get("token"); supplied != "" && subtle.ConstantTimeCompare([]byte(supplied), []byte(s.Token)) != 1 {
			http.Error(w, "open the capability URL printed by gobook", 401)
			return
		}
		b, _ := ui.ReadFile("ui.html")
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		w.Write(b)
		return
	}
	if r.Method == http.MethodGet && r.URL.Path == "/manifest.webmanifest" {
		w.Header().Set("Content-Type", "application/manifest+json")
		w.Write([]byte(manifest))
		return
	}
	if r.Method == http.MethodGet && r.URL.Path == "/icon.svg" {
		w.Header().Set("Content-Type", "image/svg+xml")
		w.Write([]byte(iconSVG))
		return
	}
	if r.URL.Path == "/app.js" && r.Method == http.MethodGet {
		w.Header().Set("Content-Type", "text/javascript; charset=utf-8")
		w.Write([]byte(appJS))
		return
	}
	if r.Method == http.MethodGet && (r.URL.Path == "/ui.css" || r.URL.Path == "/editor.js" || r.URL.Path == "/debug_ui.js" || strings.HasPrefix(r.URL.Path, "/examples/")) {
		b, err := ui.ReadFile(strings.TrimPrefix(r.URL.Path, "/"))
		if err != nil {
			http.NotFound(w, r)
			return
		}
		mime := "application/json; charset=utf-8"
		if r.URL.Path == "/ui.css" {
			mime = "text/css; charset=utf-8"
		}
		if r.URL.Path == "/editor.js" || r.URL.Path == "/debug_ui.js" {
			mime = "text/javascript; charset=utf-8"
		}
		w.Header().Set("Content-Type", mime)
		w.Write(b)
		return
	}
	if strings.HasPrefix(r.URL.Path, "/api/debug/") {
		s.debugHTTP(w, r)
		return
	}
	if r.URL.Path != "/api/run" {
		http.NotFound(w, r)
		return
	}
	if subtle.ConstantTimeCompare([]byte(r.Header.Get("Authorization")), []byte("Bearer "+s.Token)) != 1 {
		http.Error(w, "unauthorized", 401)
		return
	}
	if r.Method != http.MethodPost {
		w.Header().Set("Allow", "POST")
		http.Error(w, "POST required", 405)
		return
	}
	r.Body = http.MaxBytesReader(w, r.Body, 256<<10)
	defer r.Body.Close()
	var n Notebook
	dec := json.NewDecoder(r.Body)
	dec.DisallowUnknownFields()
	err := dec.Decode(&n)
	if err == nil {
		var extra any
		err = dec.Decode(&extra)
		if err == io.EOF {
			err = nil
		} else if err == nil {
			err = errors.New("expected a single notebook")
		}
	}
	if err != nil {
		var limit *http.MaxBytesError
		if errors.As(err, &limit) {
			http.Error(w, "notebook exceeds 256 KiB", 413)
		} else {
			http.Error(w, err.Error(), 400)
		}
		return
	}
	if _, err = source(n); err != nil {
		http.Error(w, err.Error(), 400)
		return
	}
	s.releaseExpiredDebug(r.Context())
	s.mu.Lock()
	if s.busy {
		s.mu.Unlock()
		http.Error(w, "a notebook is already running", 409)
		return
	}
	s.busy = true
	s.mu.Unlock()
	defer func() { s.mu.Lock(); s.busy = false; s.mu.Unlock() }()
	result := s.Kernel.Run(r.Context(), n)
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(result)
}
