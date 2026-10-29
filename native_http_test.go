package main

import (
	"encoding/json"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func nativeRequest(s *Server, path, body string) *httptest.ResponseRecorder {
	r := httptest.NewRequest("POST", "http://"+s.Host+path, strings.NewReader(body))
	r.Header.Set("Authorization", "Bearer "+s.Token)
	w := httptest.NewRecorder()
	s.ServeHTTP(w, r)
	return w
}
func TestNativeHTTPGuards(t *testing.T) {
	s := &Server{Host: "127.0.0.1:8080", Token: "secret", Native: &NativeManager{}}
	r := httptest.NewRequest("GET", "http://"+s.Host+"/api/native/state", nil)
	w := httptest.NewRecorder()
	s.ServeHTTP(w, r)
	if w.Code != 401 {
		t.Fatal(w.Code)
	}
	for _, body := range []string{`{} {}`, `{"unknown":true}`, `{"notebook":`, strings.Repeat("x", (256<<10)+1)} {
		if w := nativeRequest(s, "/api/native/start", body); w.Code < 400 {
			t.Fatalf("accepted invalid request %d", w.Code)
		}
	}
	s.busy = true
	if w := nativeRequest(s, "/api/native/stop", `{}`); w.Code != 200 || !s.busy {
		t.Fatal("stop stole ordinary run reservation")
	}
	s.busy = false
	if w := nativeRequest(s, "/api/native/start", `{"notebook":{"version":999}}`); w.Code != 200 || s.busy || s.nativeActive || !strings.Contains(w.Body.String(), "error") {
		t.Fatalf("failed start retained reservation: %d %s", w.Code, w.Body.String())
	}
}
func TestNativeHTTPIntegration(t *testing.T) {
	nativeIntegration(t)
	s := &Server{Host: "127.0.0.1:8080", Token: "secret", Native: &NativeManager{}}
	t.Cleanup(func() { s.Native.Stop() })
	n := Notebook{Version: 1, Imports: []string{"fmt"}, Cells: []Cell{{Code: `x:=1`}, {Code: `fmt.Println(x)`}}}
	body, _ := json.Marshal(struct {
		Notebook Notebook `json:"notebook"`
	}{n})
	start := nativeRequest(s, "/api/native/start", string(body))
	var snap NativeSnapshot
	if err := json.Unmarshal(start.Body.Bytes(), &snap); err != nil || !snap.Active || !snap.Ready {
		t.Fatalf("start: %s", start.Body.String())
	}
	if w := nativeRequest(s, "/api/run", `{"version":1,"cells":[]}`); w.Code != 409 {
		t.Fatalf("fresh run overlapped native kernel: %d", w.Code)
	}
	for cell := 1; cell <= 2; cell++ {
		payload, _ := json.Marshal(struct {
			Notebook      Notebook `json:"notebook"`
			Cell          int      `json:"cell"`
			SessionID     string   `json:"sessionId"`
			HistoryLength int      `json:"historyLength"`
		}{n, cell, snap.SessionID, len(snap.History)})
		w := nativeRequest(s, "/api/native/execute", string(payload))
		if err := json.Unmarshal(w.Body.Bytes(), &snap); err != nil || !snap.Active || snap.Error != "" || snap.CurrentCell != cell {
			t.Fatalf("execute: %s", w.Body.String())
		}
	}
	if snap.Result == nil || snap.Result.Stdout != "1\n" {
		t.Fatal(snap)
	}
	if w := nativeRequest(s, "/api/native/stop", `{}`); w.Code != 200 || s.busy {
		t.Fatalf("stop: %s", w.Body.String())
	}
	if w := nativeRequest(s, "/api/run", `{"version":1,"imports":["fmt"],"cells":[{"code":"fmt.Print(42)"}]}`); w.Code != 200 || !strings.Contains(w.Body.String(), `"stdout":"42"`) {
		t.Fatalf("fresh after stop: %s", w.Body.String())
	}
}
func TestNativeHTTPStopDuringStartup(t *testing.T) {
	nativeIntegration(t)
	s := &Server{Host: "127.0.0.1:8080", Token: "secret", Native: &NativeManager{}}
	t.Cleanup(func() { s.Native.Stop() })
	finished := make(chan struct{})
	go func() {
		defer close(finished)
		nativeRequest(s, "/api/native/start", `{"notebook":{"version":1,"imports":["fmt"],"cells":[{"code":"fmt.Print(1)"}]}}`)
	}()
	deadline := time.Now().Add(5 * time.Second)
	for {
		s.nativeMu.Lock()
		started := s.nativeStarting
		s.nativeMu.Unlock()
		if started {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("startup not observed")
		}
		time.Sleep(time.Millisecond)
	}
	if w := nativeRequest(s, "/api/native/stop", `{}`); w.Code != 200 {
		t.Fatal(w.Body.String())
	}
	select {
	case <-finished:
	case <-time.After(10 * time.Second):
		t.Fatal("startup was not interrupted")
	}
	s.nativeMu.Lock()
	defer s.nativeMu.Unlock()
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.nativeActive || s.busy {
		t.Fatal("startup cancellation leaked reservation")
	}
}

func TestNativeHTTPStoppingRejectsReplacement(t *testing.T) {
	s := &Server{Host: "127.0.0.1:8080", Token: "secret", Native: &NativeManager{}, nativeStopping: true}
	for _, path := range []string{"/api/native/start", "/api/native/stop"} {
		w := nativeRequest(s, path, `{}`)
		if w.Code != 409 {
			t.Fatalf("%s accepted while stop in progress: %d %s", path, w.Code, w.Body.String())
		}
	}
	if !s.nativeStopping {
		t.Fatal("rejected request changed stop ownership")
	}
}
