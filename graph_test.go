package main

import (
	"encoding/json"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestNotebookGraph(t *testing.T) {
	n := Notebook{Version: 1, Imports: []string{"fmt"}, Cells: []Cell{{Code: `x := 1`}, {Code: `y := x+2; fmt.Println(y)`}, {Code: `x = 3`}, {Code: `fmt.Println(x)`}}}
	g, err := analyzeNotebook(n)
	if err != nil {
		t.Fatal(err)
	}
	if len(g.Nodes) != 4 {
		t.Fatal(g)
	}
	edges := map[[2]int]bool{}
	for _, e := range g.Edges {
		edges[[2]int{e.From, e.To}] = true
	}
	for _, e := range [][2]int{{1, 2}, {1, 3}, {2, 3}, {3, 4}} {
		if !edges[e] {
			t.Errorf("missing %v in %+v", e, g.Edges)
		}
	}
	if len(g.Warnings) == 0 {
		t.Fatal("must explain alias and call limitations")
	}
}

func TestNotebookGraphLocalShadows(t *testing.T) {
	n := Notebook{Version: 1, Cells: []Cell{{Code: `{ x:=1; _=x }`}, {Code: `{ x:=2; _=x }`}}}
	g, err := analyzeNotebook(n)
	if err != nil {
		t.Fatal(err)
	}
	if len(g.Edges) != 0 {
		t.Fatal(g.Edges)
	}
	n.Cells[1].Code = `broken(`
	if _, err := analyzeNotebook(n); err == nil {
		t.Fatal("invalid Go accepted")
	}
}

func TestGraphHTTPGuards(t *testing.T) {
	s := &Server{Host: "127.0.0.1:8080", Token: "secret"}
	body := `{"version":1,"imports":["fmt"],"cells":[{"code":"x:=1"},{"code":"fmt.Println(x)"}]}`
	for _, tt := range []struct {
		token string
		want  int
	}{{"", 401}, {"secret", 200}} {
		r := httptest.NewRequest("POST", "http://"+s.Host+"/api/analyze", strings.NewReader(body))
		r.Header.Set("Authorization", "Bearer "+tt.token)
		w := httptest.NewRecorder()
		s.ServeHTTP(w, r)
		if w.Code != tt.want {
			t.Fatalf("status %d body %s", w.Code, w.Body.String())
		}
		if tt.want == 200 {
			var graph NotebookGraph
			if err := json.Unmarshal(w.Body.Bytes(), &graph); err != nil || len(graph.Edges) != 1 {
				t.Fatalf("graph %s", w.Body.String())
			}
		}
	}
}
