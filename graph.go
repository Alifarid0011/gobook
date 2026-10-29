package main

import (
	"go/ast"
	"go/parser"
	"go/token"
	"sort"
	"strconv"
	"strings"
)

type GraphNode struct {
	Cell    int      `json:"cell"`
	Defines []string `json:"defines"`
	Reads   []string `json:"reads"`
	Writes  []string `json:"writes"`
}
type GraphEdge struct {
	From  int      `json:"from"`
	To    int      `json:"to"`
	Names []string `json:"names"`
}
type NotebookGraph struct {
	Nodes    []GraphNode `json:"nodes"`
	Edges    []GraphEdge `json:"edges"`
	Warnings []string    `json:"warnings"`
}

// This is a lexical dependency guide, never a proof that arbitrary Go cells
// are independent. Calls, pointers, globals and external resources can alias.
func analyzeNotebook(n Notebook) (NotebookGraph, error) {
	g := NotebookGraph{Nodes: []GraphNode{}, Edges: []GraphEdge{}, Warnings: []string{"Static variable flow only. Calls, closures, pointers, goroutines and external side effects can introduce dependencies not shown here. This graph does not authorize automatic parallel execution."}}
	src, err := source(n)
	if err != nil {
		return g, err
	}
	fs := token.NewFileSet()
	file, err := parser.ParseFile(fs, "notebook.go", src, 0)
	if err != nil {
		return g, err
	}
	cellAt := func(pos token.Pos) int {
		name := fs.Position(pos).Filename
		if !strings.HasPrefix(name, "cell") || !strings.HasSuffix(name, ".go") {
			return 0
		}
		id, _ := strconv.Atoi(strings.TrimSuffix(strings.TrimPrefix(name, "cell"), ".go"))
		if id < 1 || id > len(n.Cells) {
			return 0
		}
		return id
	}
	reads := make([]map[*ast.Object]bool, len(n.Cells))
	writes := make([]map[*ast.Object]bool, len(n.Cells))
	defines := make([]map[*ast.Object]bool, len(n.Cells))
	for i := range reads {
		reads[i] = map[*ast.Object]bool{}
		writes[i] = map[*ast.Object]bool{}
		defines[i] = map[*ast.Object]bool{}
	}
	var rootObject func(ast.Expr) *ast.Object
	rootObject = func(expr ast.Expr) *ast.Object {
		switch x := expr.(type) {
		case *ast.Ident:
			return x.Obj
		case *ast.IndexExpr:
			return rootObject(x.X)
		case *ast.SelectorExpr:
			return rootObject(x.X)
		case *ast.StarExpr:
			return rootObject(x.X)
		case *ast.ParenExpr:
			return rootObject(x.X)
		}
		return nil
	}
	ast.Inspect(file, func(node ast.Node) bool {
		if node == nil {
			return true
		}
		cell := cellAt(node.Pos())
		if cell == 0 {
			return true
		}
		i := cell - 1
		switch x := node.(type) {
		case *ast.Ident:
			if x.Obj != nil && x.Obj.Kind == ast.Var {
				reads[i][x.Obj] = true
				if cellAt(x.Obj.Pos()) == cell {
					defines[i][x.Obj] = true
					writes[i][x.Obj] = true
				}
			}
		case *ast.AssignStmt:
			for _, expr := range x.Lhs {
				if object := rootObject(expr); object != nil && object.Kind == ast.Var {
					writes[i][object] = true
				}
			}
		case *ast.IncDecStmt:
			if object := rootObject(x.X); object != nil && object.Kind == ast.Var {
				writes[i][object] = true
			}
		}
		return true
	})
	names := func(objects map[*ast.Object]bool) []string {
		set := map[string]bool{}
		for object := range objects {
			if object.Name != "_" {
				set[object.Name] = true
			}
		}
		out := make([]string, 0, len(set))
		for name := range set {
			out = append(out, name)
		}
		sort.Strings(out)
		return out
	}
	edges := map[[2]int]map[string]bool{}
	add := func(from, to int, name string) {
		if from == 0 || from == to {
			return
		}
		key := [2]int{from, to}
		if edges[key] == nil {
			edges[key] = map[string]bool{}
		}
		edges[key][name] = true
	}
	lastWriter := map[*ast.Object]int{}
	readers := map[*ast.Object]map[int]bool{}
	for i := range n.Cells {
		cell := i + 1
		g.Nodes = append(g.Nodes, GraphNode{Cell: cell, Defines: names(defines[i]), Reads: names(reads[i]), Writes: names(writes[i])})
		for object := range reads[i] {
			add(lastWriter[object], cell, object.Name)
		}
		for object := range writes[i] {
			add(lastWriter[object], cell, object.Name)
			for reader := range readers[object] {
				add(reader, cell, object.Name)
			}
			lastWriter[object] = cell
			readers[object] = map[int]bool{}
		}
		for object := range reads[i] {
			if readers[object] == nil {
				readers[object] = map[int]bool{}
			}
			readers[object][cell] = true
		}
	}
	for key, labels := range edges {
		edge := GraphEdge{From: key[0], To: key[1], Names: []string{}}
		for name := range labels {
			edge.Names = append(edge.Names, name)
		}
		sort.Strings(edge.Names)
		g.Edges = append(g.Edges, edge)
	}
	sort.Slice(g.Edges, func(i, j int) bool {
		if g.Edges[i].To != g.Edges[j].To {
			return g.Edges[i].To < g.Edges[j].To
		}
		return g.Edges[i].From < g.Edges[j].From
	})
	return g, nil
}
