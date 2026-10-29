package main

import (
	"context"
	"net/http/httptest"
	"strings"
	"testing"
)

func debugRequest(s *Server, method, path, body string) *httptest.ResponseRecorder {
	r := httptest.NewRequest(method, "http://"+s.Host+path, strings.NewReader(body))
	r.Header.Set("Authorization", "Bearer "+s.Token)
	w := httptest.NewRecorder()
	s.ServeHTTP(w, r)
	return w
}
func TestDebugHTTPGuards(t *testing.T) {
	for _, tc := range []struct {
		name, path, method, body, token, origin, host, site string
		want                                                int
	}{
		{name: "auth", path: "start", method: "POST", body: "{}", want: 401},
		{name: "method", path: "start", method: "GET", token: "secret", want: 405},
		{name: "csrf", path: "state", method: "GET", token: "secret", origin: "https://evil.example", want: 403},
		{name: "host", path: "state", method: "GET", token: "secret", host: "evil.example", want: 403},
		{name: "fetch metadata", path: "state", method: "GET", token: "secret", site: "cross-site", want: 403},
		{name: "trailing", path: "start", method: "POST", token: "secret", body: "{} {}", want: 400},
		{name: "unknown", path: "command", method: "POST", token: "secret", body: `{"shell":"rm"}`, want: 400},
		{name: "oversize", path: "start", method: "POST", token: "secret", body: strings.Repeat(" ", 256*1024+1), want: 413},
		{name: "empty", path: "start", method: "POST", token: "secret", want: 400},
		{name: "stop trailing", path: "stop", method: "POST", token: "secret", body: "{} {}", want: 400},
		{name: "unknown route", path: "shell", method: "POST", token: "secret", want: 404},
	} {
		t.Run(tc.name, func(t *testing.T) {
			s := &Server{Token: "secret", Host: "127.0.0.1:8080", Debugger: &DebugManager{}}
			r := httptest.NewRequest(tc.method, "http://"+s.Host+"/api/debug/"+tc.path, strings.NewReader(tc.body))
			r.Header.Set("Authorization", "Bearer "+tc.token)
			r.Header.Set("Origin", tc.origin)
			r.Header.Set("Sec-Fetch-Site", tc.site)
			if tc.host != "" {
				r.Host = tc.host
			}
			w := httptest.NewRecorder()
			s.ServeHTTP(w, r)
			if w.Code != tc.want {
				t.Fatalf("got %d: %s", w.Code, w.Body.String())
			}
		})
	}
}
func TestDebugHTTPBusyOwnership(t *testing.T) {
	s := &Server{Token: "secret", Host: "127.0.0.1:8080", Debugger: &DebugManager{}, busy: true}
	for _, path := range []string{"state", "stop"} {
		method := "POST"
		if path == "state" {
			method = "GET"
		}
		w := debugRequest(s, method, "/api/debug/"+path, "")
		if w.Code != 200 {
			t.Fatalf("%s: %d %s", path, w.Code, w.Body.String())
		}
		if !s.busy {
			t.Fatalf("%s released an ordinary run", path)
		}
	}
	w := debugRequest(s, "POST", "/api/debug/start", `{"notebook":{"version":1,"cells":[{"code":"println(1)"}]}}`)
	if w.Code != 409 || !s.busy {
		t.Fatalf("busy start: %d %s", w.Code, w.Body.String())
	}
	s.busy = false
	w = debugRequest(s, "POST", "/api/debug/start", `{"notebook":{"version":0}}`)
	if w.Code != 400 || s.busy || s.debugActive {
		t.Fatalf("failed start retained busy: %d %s", w.Code, w.Body.String())
	}
}
func TestDebugHTTPUnavailable(t *testing.T) {
	s := &Server{Token: "secret", Host: "127.0.0.1:8080"}
	w := debugRequest(s, "POST", "/api/debug/start", "{}")
	if w.Code != 501 {
		t.Fatalf("got %d: %s", w.Code, w.Body.String())
	}
}

func TestReleaseExpiredDebug(t *testing.T) {
	t.Run("preserves ordinary run", func(t *testing.T) {
		s := &Server{Debugger: &DebugManager{}, busy: true}
		s.releaseExpiredDebug(context.Background())
		if !s.busy {
			t.Fatal("released an ordinary run")
		}
	})
	t.Run("absent debugger session", func(t *testing.T) {
		s := &Server{Debugger: &DebugManager{}, busy: true, debugActive: true}
		s.releaseExpiredDebug(context.Background())
		if s.busy || s.debugActive {
			t.Fatal("retained absent debugger reservation")
		}
	})
	t.Run("expired session without browser polling", func(t *testing.T) {
		dir := t.TempDir()
		manager := &DebugManager{session: &debugSession{dir: dir, cancel: func() {}, output: &capture{}, snapshot: DebugSnapshot{Exited: true}}}
		s := &Server{Debugger: manager, busy: true, debugActive: true}
		s.releaseExpiredDebug(context.Background())
		if s.busy || s.debugActive {
			t.Fatal("retained expired debugger reservation")
		}
		if _, err := manager.current(); err == nil {
			t.Fatal("retained manager session")
		}
	})
	t.Run("preserves paused live session", func(t *testing.T) {
		manager := &DebugManager{session: &debugSession{output: &capture{}}}
		s := &Server{Debugger: manager, busy: true, debugActive: true}
		s.releaseExpiredDebug(context.Background())
		if !s.busy || !s.debugActive {
			t.Fatal("released a live debugger")
		}
	})
}
