package main

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestAPIGuards(t *testing.T) {
	s := &Server{Token: "secret", Host: "127.0.0.1:8080"}
	for _, tt := range []struct {
		name, host, token, origin, body string
		want                            int
	}{
		{"unauthorized", "127.0.0.1:8080", "", "", "{}", 401},
		{"rebinding", "evil.example", "secret", "", "{}", 403},
		{"csrf", "127.0.0.1:8080", "secret", "https://evil.example", "{}", 403},
		{"oversized", "127.0.0.1:8080", "secret", "", strings.Repeat(" ", 256*1024+1), 413},
		{"invalid notebook", "127.0.0.1:8080", "secret", "", "{}", 400},
	} {
		t.Run(tt.name, func(t *testing.T) {
			r := httptest.NewRequest(http.MethodPost, "http://"+tt.host+"/api/run", strings.NewReader(tt.body))
			r.Host = tt.host
			r.Header.Set("Authorization", "Bearer "+tt.token)
			if tt.origin != "" {
				r.Header.Set("Origin", tt.origin)
			}
			w := httptest.NewRecorder()
			s.ServeHTTP(w, r)
			if w.Code != tt.want {
				t.Fatalf("got %d %s", w.Code, w.Body.String())
			}
		})
	}
}

func TestPageReload(t *testing.T) {
	s := &Server{Token: "secret", Host: "127.0.0.1:8080"}
	r := httptest.NewRequest(http.MethodGet, "http://127.0.0.1:8080/", nil)
	w := httptest.NewRecorder()
	s.ServeHTTP(w, r)
	if w.Code != 200 {
		t.Fatalf("page shell must reload after URL token removal: %d", w.Code)
	}
}

func TestPublicOrigin(t *testing.T) {
	s := &Server{Token: "secret", Host: "127.0.0.1:8080", PublicOrigin: "https://book.example"}
	for _, tt := range []struct {
		host, origin string
		want         int
	}{
		{"book.example", "https://book.example", 200},
		{"book.example", "http://book.example", 403},
		{"127.0.0.1:8080", "https://book.example", 403},
		{"evil.example", "https://book.example", 403},
	} {
		r := httptest.NewRequest(http.MethodPost, "https://"+tt.host+"/api/run", strings.NewReader(`{"version":1,"imports":["fmt"],"cells":[{"code":"fmt.Println(42)"}]}`))
		r.Host = tt.host
		r.Header.Set("Origin", tt.origin)
		r.Header.Set("Authorization", "Bearer secret")
		w := httptest.NewRecorder()
		s.ServeHTTP(w, r)
		if w.Code != tt.want {
			t.Errorf("%s %s: got %d want %d", tt.host, tt.origin, w.Code, tt.want)
		}
		if tt.want == 200 && !strings.Contains(w.Body.String(), "42") {
			t.Fatal(w.Body.String())
		}
	}
	for _, origin := range []string{"http://book.example", "https://book.example/path", "https://user@book.example", "https://book.example?token=abc", "https://book.example#x"} {
		if _, err := publicOrigin(origin); err == nil {
			t.Errorf("accepted %q", origin)
		}
	}
}

func TestBusy(t *testing.T) {
	s := &Server{Token: "secret", Host: "127.0.0.1:8080", busy: true}
	r := httptest.NewRequest(http.MethodPost, "http://127.0.0.1:8080/api/run", strings.NewReader(`{"version":1}`))
	r.Header.Set("Authorization", "Bearer secret")
	w := httptest.NewRecorder()
	s.ServeHTTP(w, r)
	if w.Code != 409 {
		t.Fatalf("got %d %s", w.Code, w.Body.String())
	}
}

func TestOriginCanonicalPort(t *testing.T) {
	got, err := publicOrigin("https://book.example:443/")
	if err != nil || got != "https://book.example" {
		t.Fatalf("got %q, %v", got, err)
	}
}
