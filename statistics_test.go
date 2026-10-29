package main

import (
	"context"
	"encoding/json"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestStatisticsExamples(t *testing.T) {
	chart, err := filepath.Abs("../chartframe")
	if err != nil {
		t.Fatal(err)
	}
	for _, tt := range []struct{ name, want string }{
		{"summary", "Mean: 5.00"},
		{"regression", "Slope: 2.00"},
		{"distribution", "Observations: 20"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			b, err := os.ReadFile("examples/" + tt.name + ".gobook.json")
			if err != nil {
				t.Fatal(err)
			}
			var n Notebook
			if err := json.Unmarshal(b, &n); err != nil {
				t.Fatal(err)
			}
			if _, err := source(n); err != nil {
				t.Fatal(err)
			}
			if _, err := os.Stat(filepath.Join(chart, "go.mod")); err != nil {
				t.Skip("optional adjacent chartframe unavailable")
			}
			r := (Kernel{Chartframe: chart}).Run(context.Background(), n)
			if r.Error != "" || !strings.Contains(r.Stdout, tt.want) {
				t.Fatalf("result: %+v", r)
			}
			if len(r.Artifacts) != 1 || r.Artifacts[0].MIME != "image/svg+xml" {
				t.Fatal("expected one SVG figure")
			}
		})
	}
}

func TestNotebookAssets(t *testing.T) {
	s := &Server{Host: "127.0.0.1:8080"}
	for _, tt := range []struct {
		path, mime string
		status     int
	}{
		{"/ui.css", "text/css", 200},
		{"/editor.js", "text/javascript", 200},
		{"/debug_ui.js", "text/javascript", 200},
		{"/graph_ui.js", "text/javascript", 200},
		{"/native_ui.js", "text/javascript", 200},
		{"/desktop_ui.js", "text/javascript", 200},
		{"/examples/penguins.gobook.json", "application/json", 200},
		{"/examples/memory.gobook.json", "application/json", 200},
		{"/examples/functions.gobook.json", "application/json", 200},
		{"/examples/summary.gobook.json", "application/json", 200},
		{"/examples/regression.gobook.json", "application/json", 200},
		{"/examples/distribution.gobook.json", "application/json", 200},
		{"/examples/missing.gobook.json", "text/plain", 404},
		{"/examples/../kernel.go", "text/plain", 404},
	} {
		t.Run(tt.path, func(t *testing.T) {
			r := httptest.NewRequest("GET", "http://"+s.Host+tt.path, nil)
			w := httptest.NewRecorder()
			s.ServeHTTP(w, r)
			if w.Code != tt.status || !strings.HasPrefix(w.Header().Get("Content-Type"), tt.mime) {
				t.Fatalf("%d %s", w.Code, w.Header().Get("Content-Type"))
			}
			if tt.status == 200 && strings.HasSuffix(tt.path, ".json") {
				var n Notebook
				if err := json.Unmarshal(w.Body.Bytes(), &n); err != nil {
					t.Fatal(err)
				}
				if _, err := source(n); err != nil {
					t.Fatal(err)
				}
			}
		})
	}
}
