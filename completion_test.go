package main

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"net/http/httptest"
	"os"
	"os/exec"
	"strings"
	"testing"
	"time"
	"unicode/utf16"
)

func TestCompletionSourceUTF16(t *testing.T) {
	n := Notebook{Version: 1, Imports: []string{"fmt"}, Cells: []Cell{{Code: `name := "علی😀"`}, {Code: "fmt.Pr"}}}
	source, line, char, err := completionSource(n, 2, 6)
	if err != nil || !strings.Contains(source, "name :=") || !strings.Contains(source, "fmt.Pr") || line < 4 || char != 6 {
		t.Fatalf("%d %d %v %s", line, char, err, source)
	}
	if _, _, _, err = completionSource(n, 2, 99); err == nil {
		t.Fatal("invalid offset")
	}
}
func TestLocalCompletion(t *testing.T) {
	n := Notebook{Version: 1, Imports: []string{"fmt"}, Cells: []Cell{{Code: `count := 1`}, {Code: "fmt.Pr"}}}
	items := localCompletions(n, 2, 6)
	found := false
	for _, item := range items {
		if item.Label == "Println" && strings.Contains(item.Detail, "func(") {
			found = true
		}
	}
	if !found {
		t.Fatalf("missing fmt function completion: %+v", items)
	}
	n.Cells[1].Code = "cou"
	items = localCompletions(n, 2, 3)
	if len(items) != 1 || items[0].Label != "count" {
		t.Fatalf("missing notebook symbol: %+v", items)
	}
}
func TestGoplsIntegrationCompletion(t *testing.T) {
	path := os.Getenv("GOBOOK_GOPLS")
	if path == "" {
		t.Skip("set GOBOOK_GOPLS to test semantic completion")
	}
	c := &CompletionManager{Path: path}
	t.Cleanup(c.Close)
	n := Notebook{Version: 1, Imports: []string{"fmt"}, Cells: []Cell{{Code: "name := \"Ada\""}, {Code: "fmt.Pr"}}}
	items, err := c.Complete(context.Background(), n, 2, 6)
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for _, item := range items {
		if item.Label == "Println" {
			found = true
		}
	}
	if !found {
		t.Fatalf("missing semantic completion: %+v", items)
	}
	n.Cells[1].Code = "fmt.Println"
	items, err = c.Complete(context.Background(), n, 2, 6)
	if err != nil {
		t.Fatal(err)
	}
	found = false
	for _, item := range items {
		if item.Label == "Println" && item.Start == 4 && item.End == 11 {
			found = true
		}
	}
	if !found {
		t.Fatalf("mid-word replacement range missing: %+v", items)
	}
	n.Cells[1].Code = "na"
	items, err = c.Complete(context.Background(), n, 2, 2)
	if err != nil {
		t.Fatal(err)
	}
	found = false
	for _, item := range items {
		if item.Label == "name" && strings.Contains(item.Detail, "string") {
			found = true
		}
	}
	if !found {
		t.Fatalf("previous cell type completion missing: %+v", items)
	}
}

func TestCompletionHTTPValidationAndOfflineFallback(t *testing.T) {
	s := &Server{Host: "127.0.0.1:8080", Token: "test"}
	request := func(body, auth string) *httptest.ResponseRecorder {
		t.Helper()
		r := httptest.NewRequest("POST", "http://"+s.Host+"/api/complete", strings.NewReader(body))
		r.Header.Set("Authorization", auth)
		w := httptest.NewRecorder()
		s.ServeHTTP(w, r)
		return w
	}
	body := `{"notebook":{"version":1,"imports":["fmt"],"cells":[{"code":"fmt.Pr"}]},"cell":1,"offset":6}`
	if w := request(body, ""); w.Code != 401 {
		t.Fatalf("unauthorized %d", w.Code)
	}
	w := request(body, "Bearer test")
	if w.Code != 200 || !strings.Contains(w.Body.String(), "Println") || !strings.Contains(w.Body.String(), `"provider":"local"`) {
		t.Fatal(w.Body.String())
	}
	if w := request(strings.Replace(body, `"offset":6`, `"offset":99`, 1), "Bearer test"); w.Code != 400 {
		t.Fatal("invalid cursor accepted")
	}
}
func TestLSPBoundedFraming(t *testing.T) {
	message := `{"id":1,"result":{"ok":true}}`
	framed := fmt.Sprintf("Content-Length: %d\r\n\r\n%s", len(message), message)
	reply, err := readLSP(bufio.NewReader(strings.NewReader(framed)))
	if err != nil || reply.ID != 1 {
		t.Fatal(reply, err)
	}
	for _, input := range []string{"Content-Length: 999999999\r\n\r\n", "Content-Length: -1\r\n\r\n", "Content-Length: 10\r\n\r\n{}"} {
		if _, err = readLSP(bufio.NewReader(strings.NewReader(input))); err == nil {
			t.Fatal("bad frame accepted")
		}
	}
}

func TestCompletionCancellationUnblocksPendingWrite(t *testing.T) {
	reader, writer := io.Pipe()
	defer reader.Close()
	p := &completionProcess{cmd: &exec.Cmd{Cancel: func() error { return nil }}, in: writer}
	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()
	stop := p.bindContext(ctx)
	defer stop()
	done := make(chan error, 1)
	go func() { done <- p.send(map[string]any{"text": strings.Repeat("x", 200000)}) }()
	select {
	case err := <-done:
		if err == nil {
			t.Fatal("blocked write unexpectedly succeeded")
		}
	case <-time.After(time.Second):
		t.Fatal("cancellation did not unblock language-server write")
	}
}
func TestCompletionRangeMapping(t *testing.T) {
	code := "// علی😀\nfmt.Println"
	prefix := "// علی😀\nfmt.Pr"
	offset := len(utf16.Encode([]rune(prefix)))
	start, end, ok := mapCompletionRange(code, completionRange{Start: completionPosition{11, 4}, End: completionPosition{11, 11}}, 10, offset)
	if !ok || end-start != 7 || start != offset-2 {
		t.Fatalf("range %d %d %t", start, end, ok)
	}
	if _, _, ok = mapCompletionRange(code, completionRange{Start: completionPosition{0, 0}, End: completionPosition{0, 1}}, 10, offset); ok {
		t.Fatal("edit outside cell accepted")
	}
}
