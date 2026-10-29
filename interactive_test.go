package main

import (
	"bytes"
	"context"
	"os"
	"strings"
	"testing"
)

func TestInteractiveEditsRetainValues(t *testing.T) {
	var out bytes.Buffer
	e := newInteractiveEvaluator(&out, &out)
	run := func(code string) {
		t.Helper()
		if err := e.eval([]string{"fmt"}, code); err != nil {
			t.Fatal(err)
		}
	}
	run(`data := []int{2, 4}; original := &data[0]; calls := 0; sum := func() int { calls++; return data[0]+data[1] }`)
	run(`v := sum(); fmt.Println(v, calls, original==&data[0])`)
	run(`v := sum()+10; fmt.Println(v, calls, original==&data[0])`)
	run(`fmt.Println(v); data[0]=8`)
	run(`func doubled(x int) int { return x*2 }`)
	run(`fmt.Println(doubled(sum()))`)
	if got := out.String(); got != "6 1 true\n16 2 true\n16\n24\n" {
		t.Fatal(got)
	}
	if err := e.eval(nil, `this is not valid Go`); err == nil {
		t.Fatal("invalid code accepted")
	}
	run(`fmt.Println(data[0], calls)`)
	if !strings.HasSuffix(out.String(), "8 3\n") {
		t.Fatal(out.String())
	}
}
func TestInteractiveUnsupportedImports(t *testing.T) {
	var out bytes.Buffer
	e := newInteractiveEvaluator(&out, &out)
	if err := e.eval([]string{"github.com/Alifarid0011/sheetframe"}, `println(1)`); err == nil || !strings.Contains(err.Error(), "native") {
		t.Fatalf("missing native guidance: %v", err)
	}
	if out.Len() != 0 {
		t.Fatal("unsupported import executed cell")
	}
}

func TestInteractiveWorkerProcess(t *testing.T) {
	if os.Getenv("GOBOOK_INTERACTIVE_MARKER") == "" {
		return
	}
	if err := interactiveWorker(); err != nil {
		os.Exit(2)
	}
	os.Exit(0)
}
func TestInteractiveIntegrationEditableWorker(t *testing.T) {
	nativeIntegration(t)
	exe, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	m := &NativeManager{WorkerCommand: []string{exe, "-test.run=^TestInteractiveWorkerProcess$"}}
	t.Cleanup(func() { _ = m.Stop() })
	n := Notebook{Version: 1, Imports: []string{"fmt"}, Cells: []Cell{{Code: `data:=[]int{4,6}; calls:=0; sum:=func()int{calls++;return data[0]+data[1]}`}, {Code: `value:=sum();fmt.Println(value,calls)`}}}
	s, err := m.StartEngine(context.Background(), n, "interactive")
	if err != nil {
		t.Fatal(err)
	}
	id := s.SessionID
	run := func(cell int, want string, wantError bool) {
		t.Helper()
		s, err = m.ExecuteExpected(context.Background(), n, cell, id, len(s.History))
		if (err != nil) != wantError || !s.Active || !s.Ready || s.SessionID != id || !s.CommandAccepted || s.Result.Stdout != want {
			t.Fatalf("result err=%v snapshot=%+v result=%+v", err, s, s.Result)
		}
	}
	run(1, "", false)
	run(2, "10 1\n", false)
	n.Cells[1].Code = `value:=sum()+20;fmt.Println(value,calls)`
	run(2, "30 2\n", false)
	n.Cells = append(n.Cells, Cell{Code: `fmt.Println(value, data[0]);data[0]=9`})
	run(3, "30 4\n", false)
	n.Cells[1].Code = `invalid %% syntax`
	run(2, "", true)
	n.Cells[1].Code = `fmt.Println(sum(),calls)`
	run(2, "15 3\n", false)
	before := len(s.History)
	n.Files = []InputFile{{Name: "new.csv", Data: "eA=="}}
	// Configuration edits must not silently replace resources in the existing RAM.
	if _, err = m.ExecuteExpected(context.Background(), n, 2, id, before); err == nil {
		t.Fatal("input replacement accepted")
	}
}

func TestInteractiveIntegrationLongFrames(t *testing.T) {
	nativeIntegration(t)
	exe, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	m := &NativeManager{WorkerCommand: []string{exe, "-test.run=^TestInteractiveWorkerProcess$"}}
	t.Cleanup(func() { _ = m.Stop() })
	n := Notebook{Version: 1, Imports: []string{"fmt"}, Cells: []Cell{{Code: `x:=7`}}}
	s, err := m.StartEngine(context.Background(), n, "interactive")
	if err != nil {
		t.Fatal(err)
	}
	s, err = m.Execute(context.Background(), n, 1)
	if err != nil {
		t.Fatal(err)
	}
	n.Cells[0].Code = `huge:="` + strings.Repeat("<", 100000) + `";fmt.Println(len(huge),x)`
	s, err = m.Execute(context.Background(), n, 1)
	if err != nil || !s.Active || s.Result.Stdout != "100000 7\n" {
		t.Fatalf("large valid cell lost RAM: %v %+v", err, s)
	}
	n.Cells[0].Code = `panic("` + strings.Repeat("<", 1500) + `")`
	s, err = m.Execute(context.Background(), n, 1)
	if err == nil || !s.Active || !s.Ready {
		t.Fatalf("recoverable long error lost RAM: %v %+v", err, s)
	}
	n.Cells[0].Code = `fmt.Println(x)`
	s, err = m.Execute(context.Background(), n, 1)
	if err != nil || s.Result.Stdout != "7\n" {
		t.Fatalf("post-error state lost: %v %+v", err, s)
	}
	if s.Notebook == nil || s.Notebook.Cells[0].Code != n.Cells[0].Code || s.Fingerprint != nativeFingerprint(*s.Notebook) {
		t.Fatal("snapshot paired with wrong source")
	}
	s.Notebook.Cells[0].Code = "mutated view"
	if m.State().Notebook.Cells[0].Code != n.Cells[0].Code {
		t.Fatal("mutable snapshot escaped")
	}
	n.Cells = make([]Cell, 1001)
	n.Cells[0].Code = "fmt.Println(x)"
	before := len(m.State().History)
	rejected, e := m.Execute(context.Background(), n, 1)
	if e == nil || !rejected.Active || !rejected.Ready || len(rejected.History) != before {
		t.Fatal("oversized edited notebook damaged session")
	}
}
