package main

import (
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func TestDesktopCapabilityValidation(t *testing.T) {
	valid := "http://127.0.0.1:43856/?token=" + strings.Repeat("a", 64)
	for _, address := range []string{valid, "https://127.0.0.1:43856/?token=" + strings.Repeat("a", 64), "http://evil.test:43856/?token=" + strings.Repeat("a", 64), "http://user@127.0.0.1:43856/?token=" + strings.Repeat("a", 64), valid + "&extra=1", "http://127.0.0.1:43856/?token=short"} {
		_, err := desktopCapability(address, "127.0.0.1:43856")
		if (address == valid) != (err == nil) {
			t.Errorf("accepted=%v for %q: %v", err == nil, address, err)
		}
	}
}

func TestDesktopHTTPGuards(t *testing.T) {
	quit := 0
	s := &Server{Host: "127.0.0.1:43856", Token: strings.Repeat("a", 64), DesktopQuit: func() { quit++ }}
	for _, tt := range []struct {
		method, path, auth, origin string
		status                     int
	}{
		{"GET", "/api/desktop/status", "", "", 401},
		{"GET", "/api/desktop/status", s.Token, "", 200},
		{"GET", "/api/desktop/quit", s.Token, "", 405},
		{"POST", "/api/desktop/quit", s.Token, "http://evil.test", 403},
		{"POST", "/api/desktop/quit", s.Token, "", 202},
	} {
		r := httptest.NewRequest(tt.method, "http://"+s.Host+tt.path, nil)
		r.Header.Set("Authorization", "Bearer "+tt.auth)
		if tt.origin != "" {
			r.Header.Set("Origin", tt.origin)
		}
		w := httptest.NewRecorder()
		s.ServeHTTP(w, r)
		if w.Code != tt.status {
			t.Fatalf("%s %s: %d", tt.method, tt.path, w.Code)
		}
	}
	if quit != 1 {
		t.Fatal("unauthenticated request changed host lifetime")
	}
	s.DesktopQuit = nil
	r := httptest.NewRequest("GET", "http://"+s.Host+"/api/desktop/status", nil)
	w := httptest.NewRecorder()
	s.ServeHTTP(w, r)
	if w.Code != 404 {
		t.Fatal("ordinary server exposes desktop controls")
	}
}

func TestDesktopRecordAndProbe(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer "+strings.Repeat("a", 64) {
			http.Error(w, "unauthorized", 401)
			return
		}
		io.WriteString(w, `{"app":"gobook"}`)
	}))
	defer server.Close()
	u, _ := url.Parse(server.URL)
	dir, err := desktopStateDir(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	address := server.URL + "/?token=" + strings.Repeat("a", 64)
	if err = saveDesktopURL(dir, address); err != nil {
		t.Fatal(err)
	}
	if got, err := existingDesktopURL(dir, u.Host); err != nil || got != address {
		t.Fatalf("cannot reopen: %q %v", got, err)
	}
	info, err := os.Stat(filepath.Join(dir, "desktop.url"))
	if err != nil {
		t.Fatal(err)
	}
	if runtime.GOOS != "windows" && info.Mode().Perm() != 0600 {
		t.Fatal("capability file is not private")
	}
	redirect := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { http.Redirect(w, r, server.URL, 302) }))
	defer redirect.Close()
	ru, _ := url.Parse(redirect.URL)
	if err = saveDesktopURL(dir, redirect.URL+"/?token="+strings.Repeat("a", 64)); err != nil {
		t.Fatal(err)
	}
	if _, err = existingDesktopURL(dir, ru.Host); err == nil {
		t.Fatal("followed redirect from occupied port")
	}
}

func TestDesktopTempUsesUserCache(t *testing.T) {
	for _, name := range []string{"TMPDIR", "TMP", "TEMP", "GOTMPDIR"} {
		t.Setenv(name, os.Getenv(name))
	}
	base := t.TempDir()
	if err := configureDesktopTemp(base); err != nil {
		t.Fatal(err)
	}
	want := filepath.Join(base, "gobook", "tmp")
	for _, name := range []string{"TMPDIR", "TMP", "TEMP", "GOTMPDIR"} {
		if got := os.Getenv(name); got != want {
			t.Fatalf("%s=%q; want %q", name, got, want)
		}
	}
	dir, err := os.MkdirTemp("", "desktop-test-")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(dir)
	if filepath.Dir(dir) != want {
		t.Fatal("notebook work still depends on system temporary filesystem")
	}
}
