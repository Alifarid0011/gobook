package main

import (
	"context"
	"strings"
	"testing"
)

func TestCellOutputs(t *testing.T) {
	n := Notebook{Version: 1, Imports: []string{"fmt", "os"}, Cells: []Cell{
		{Code: `x := 40; fmt.Print("first"); fmt.Fprint(os.Stderr,"warning"); if err := os.WriteFile("first.csv",[]byte("x\n40\n"),0600); err != nil { panic(err) }`},
		{Code: `x += 2; fmt.Printf("second:%d",x); if err := os.WriteFile("second.json",[]byte("{}"),0600); err != nil { panic(err) }`},
	}}
	r := (Kernel{}).Run(context.Background(), n)
	if r.Error != "" {
		t.Fatal(r.Error, r.Stderr)
	}
	if len(r.Cells) != 2 {
		t.Fatalf("cell outputs missing: %+v", r)
	}
	if r.Stdout != "firstsecond:42" || r.Stderr != "warning" {
		t.Fatalf("aggregate changed: %+v", r)
	}
	if r.Cells[0].Stdout != "first" || r.Cells[0].Stderr != "warning" || r.Cells[1].Stdout != "second:42" {
		t.Fatalf("wrong attribution: %+v", r.Cells)
	}
	if len(r.Cells[0].Artifacts) != 1 || r.Cells[0].Artifacts[0].Name != "first.csv" || len(r.Cells[1].Artifacts) != 1 || r.Cells[1].Artifacts[0].Name != "second.json" {
		t.Fatalf("artifacts: %+v", r.Cells)
	}
	through := 1
	n.RunThrough = &through
	r = (Kernel{}).Run(context.Background(), n)
	if r.Error != "" || len(r.Cells) != 1 || r.Stdout != "first" {
		t.Fatalf("prefix: %+v", r)
	}
}

func TestCellFailureAndOverwrite(t *testing.T) {
	n := Notebook{Version: 1, Imports: []string{"fmt", "os"}, Cells: []Cell{
		{Code: `os.WriteFile("data.csv",[]byte("old"),0600); fmt.Println("before")`},
		{Code: `os.WriteFile("data.csv",[]byte("new"),0600); panic("broken")`},
		{Code: `fmt.Println("never")`},
	}}
	r := (Kernel{}).Run(context.Background(), n)
	if r.Error == "" || len(r.Cells) != 2 || len(r.Cells[0].Artifacts) != 0 || len(r.Cells[1].Artifacts) != 1 || !strings.Contains(r.Cells[1].Stderr, "panic: broken") {
		t.Fatalf("failure attribution: %+v", r)
	}
}

func TestCellOutputCompatibility(t *testing.T) {
	t.Run("cross-cell blocks retain aggregate semantics", func(t *testing.T) {
		n := Notebook{Version: 1, Imports: []string{"fmt"}, Cells: []Cell{{Code: `for i:=0;i<2;i++ {`}, {Code: `fmt.Print(i) }`}}}
		r := (Kernel{}).Run(context.Background(), n)
		if r.Error != "" || r.Stdout != "01" || len(r.Cells) != 0 {
			t.Fatalf("legacy: %+v", r)
		}
	})
	t.Run("runtime exit preserves captured output", func(t *testing.T) {
		n := Notebook{Version: 1, Imports: []string{"fmt", "os"}, Cells: []Cell{{Code: `fmt.Print("exit"); os.Exit(3)`}, {Code: `fmt.Print("never")`}}}
		r := (Kernel{}).Run(context.Background(), n)
		if r.Error == "" || r.Stdout != "exit" || len(r.Cells) != 1 {
			t.Fatalf("exit: %+v", r)
		}
	})
	t.Run("deferred output and builtin stderr", func(t *testing.T) {
		n := Notebook{Version: 1, Imports: []string{"fmt"}, Cells: []Cell{{Code: `defer fmt.Print("deferred"); fmt.Print("first")`}, {Code: `println("stderr"); fmt.Print("last")`}}}
		r := (Kernel{}).Run(context.Background(), n)
		if r.Error != "" || r.Stdout != "firstlastdeferred" || r.Cells[1].Stdout != "lastdeferred" || r.Cells[1].Stderr != "stderr\n" {
			t.Fatalf("deferred: %+v", r)
		}
	})
	t.Run("output budget", func(t *testing.T) {
		n := Notebook{Version: 1, Imports: []string{"fmt", "strings"}, Cells: []Cell{{Code: `fmt.Print(strings.Repeat("x",1100000))`}, {Code: `fmt.Print("tail")`}}}
		r := (Kernel{}).Run(context.Background(), n)
		if r.Error != "" || !r.Truncated || len(r.Stdout)+len(r.Stderr) > outputLimit {
			t.Fatalf("budget: error=%s truncated=%v bytes=%d", r.Error, r.Truncated, len(r.Stdout)+len(r.Stderr))
		}
		total := 0
		for _, c := range r.Cells {
			total += len(c.Stdout) + len(c.Stderr)
		}
		if total > outputLimit {
			t.Fatal(total)
		}
	})
}

func TestSplitCellStream(t *testing.T) {
	marker := "\x1etest:"
	all, cells := splitCellStream("build"+marker+"1\x1fa"+marker+"2\x1fb"+marker+"bad\x1fc", marker, 2)
	if all != "buildab"+marker+"bad\x1fc" || cells[1] != "a" || cells[2] != "b"+marker+"bad\x1fc" {
		t.Fatalf("%q %+v", all, cells)
	}
	all, cells = splitCellStream(marker+"1\x1fno newline", marker, 1)
	if all != "no newline" || cells[1] != "no newline" {
		t.Fatal(all, cells)
	}
}

func TestCellBuildDiagnostics(t *testing.T) {
	r := (Kernel{}).Run(context.Background(), Notebook{Version: 1, Cells: []Cell{{Code: `missing()`}}})
	if r.Error == "" || len(r.Cells) != 0 || !strings.Contains(r.Stderr, "cell1.go:1") || strings.Contains(r.Stderr, "gobookTrace") {
		t.Fatalf("build diagnostics: %+v", r)
	}
}

func TestCellBudgetAfterInitializationOutput(t *testing.T) {
	marker := "\x1etest:"
	r := Result{Stdout: strings.Repeat("a", outputLimit-3) + marker + "1\x1f12345", Stderr: marker + "1\x1ferror"}
	attributeCells(&r, t.TempDir(), marker, 1)
	if len(r.Cells) != 1 || r.Cells[0].Stdout != "123" || r.Cells[0].Stderr != "" || !r.Truncated {
		t.Fatalf("cell budget: %+v", r.Cells)
	}
}
