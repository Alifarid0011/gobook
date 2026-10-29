package main

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"runtime"
	"strings"
	"testing"
	"time"
)

func TestNativeFingerprint(t *testing.T) {
	n := Notebook{Version: 1, Title: "one", Cells: []Cell{{Code: "println(1)"}}}
	original := nativeFingerprint(n)
	n.Title = "two"
	n.Note = "annotation"
	i := 999
	n.RunThrough = &i
	if nativeFingerprint(n) != original {
		t.Fatal("annotations or run selection changed execution fingerprint")
	}
	n.Cells[0].Code = "println(2)"
	if nativeFingerprint(n) == original {
		t.Fatal("source edit retained execution fingerprint")
	}
}

func TestNativeStreamingBoundaries(t *testing.T) {
	s := newNativeSession("", "\x1eprivate:")
	for _, stderr := range []bool{false, true} {
		w := &nativeStream{s: s, stderr: stderr}
		for _, chunk := range []string{"pre", strings.Repeat("x", outputLimit+15), "\x1epri", "vate:", "0", "\x1f"} {
			if _, err := w.Write([]byte(chunk)); err != nil {
				t.Fatal(err)
			}
		}
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if !s.output.truncated || s.output.out.Len()+s.output.err.Len() != outputLimit {
		t.Fatal("shared output bound missing")
	}
	if s.boundaries[0] != 1 || s.boundaries[1] != 1 {
		t.Fatalf("completion lost after truncation: %v", s.boundaries)
	}
}

func nativeIntegration(t *testing.T) {
	t.Helper()
	if os.Getenv("GOBOOK_NATIVE_INTEGRATION") != "1" {
		t.Skip("set GOBOOK_NATIVE_INTEGRATION=1 for real compiled worker / loopback tests")
	}
}
func nativeStartTest(t *testing.T, n Notebook, k Kernel) *NativeManager {
	t.Helper()
	nativeIntegration(t)
	m := &NativeManager{Kernel: k}
	t.Cleanup(func() {
		if err := m.Stop(); err != nil {
			t.Error(err)
		}
	})
	snap, err := m.Start(context.Background(), n)
	if err != nil {
		t.Fatalf("start: %v; %+v", err, snap.Result)
	}
	if !snap.Active || !snap.Ready || snap.CurrentCell != 0 || snap.NextCell != 1 {
		t.Fatalf("start snapshot: %+v", snap)
	}
	return m
}
func nativeRunTest(t *testing.T, m *NativeManager, n Notebook, cell int) NativeSnapshot {
	t.Helper()
	s, err := m.Execute(context.Background(), n, cell)
	if err != nil {
		t.Fatalf("cell %d: %v; %+v", cell, err, s.Result)
	}
	return s
}

func TestNativeIntegrationMemoryAndFrontier(t *testing.T) {
	n := Notebook{Version: 1, Imports: []string{"fmt", "os"}, Cells: []Cell{
		{Code: `m:=map[string]int{"n":1}; s:=[]int{1}; p:=&s[0]; f:=func()int{return m["n"]}; fmt.Print("once");os.WriteFile("model.json",[]byte("1"),0600)`},
		{Code: `m["n"]++;s[0]++;fmt.Printf("%d/%d/%v",f(),s[0],p==&s[0])`},
		{Code: `fmt.Printf("end:%d",m["n"]);os.WriteFile("model.json",[]byte("3"),0600)`},
	}}
	m := nativeStartTest(t, n, Kernel{})
	if s, err := m.Execute(context.Background(), n, 2); err == nil || s.CurrentCell != 0 {
		t.Fatal("skipped unexecuted prerequisite")
	}
	if s := nativeRunTest(t, m, n, 1); s.Result.Stdout != "once" {
		t.Fatalf("cell1: %+v", s)
	}
	if s := nativeRunTest(t, m, n, 2); s.Result.Stdout != "2/2/true" {
		t.Fatalf("cell2: %+v", s)
	}
	if s := nativeRunTest(t, m, n, 2); s.Result.Stdout != "3/3/true" {
		t.Fatalf("repeat cell2: %+v", s)
	}
	if s := nativeRunTest(t, m, n, 3); s.Result.Stdout != "end:3" || len(s.Result.Cells) != 1 || len(s.Result.Cells[0].Artifacts) != 1 {
		t.Fatalf("cell3 artifacts: %+v", s.Result)
	}
	nativeRunTest(t, m, n, 1)
	if _, err := m.Execute(context.Background(), n, 3); err == nil {
		t.Fatal("earlier rerun did not invalidate later cells")
	}
	s := nativeRunTest(t, m, n, 2)
	if !reflect.DeepEqual(s.History, []int{1, 2, 2, 3, 1, 2}) {
		t.Fatalf("history: %v", s.History)
	}
	n.Title = "new title"
	nativeRunTest(t, m, n, 2)
	n.Cells[0].Code += ";println(9)"
	if s, err := m.Execute(context.Background(), n, 2); err == nil || !s.Active {
		t.Fatal("edit must require restart without dropping memory")
	}
}

func TestNativeIntegrationGoroutinesGenericsAndStdin(t *testing.T) {
	n := Notebook{Version: 1, Imports: []string{"fmt", "slices", "os"}, Cells: []Cell{
		{Code: `ch:=make(chan int);go func(){for i:=1;;i++{ch<-i}}(); var input [1]byte;os.Stdin.Read(input[:]);fmt.Print("ready")`},
		{Code: `v:=[]int{3,1,2};slices.Sort(v);fmt.Printf("%v/%d",v,<-ch)`},
	}}
	m := nativeStartTest(t, n, Kernel{})
	nativeRunTest(t, m, n, 1)
	if s := nativeRunTest(t, m, n, 2); s.Result.Stdout != "[1 2 3]/1" || s.Goroutines < 2 || s.HeapAlloc == 0 {
		t.Fatalf("native runtime: %+v", s)
	}
	if s := nativeRunTest(t, m, n, 2); s.Result.Stdout != "[1 2 3]/2" {
		t.Fatalf("channel restarted: %+v", s)
	}
}
func TestNativeIntegrationHugePartialOutput(t *testing.T) {
	n := Notebook{Version: 1, Imports: []string{"fmt", "os", "strings"}, Cells: []Cell{{Code: `fmt.Print(strings.Repeat("x",(1<<20)+123));fmt.Fprint(os.Stderr,"tail")`}}}
	m := nativeStartTest(t, n, Kernel{})
	s := nativeRunTest(t, m, n, 1)
	if !s.Ready || !s.Result.Truncated || len(s.Result.Stdout)+len(s.Result.Stderr) != outputLimit {
		t.Fatalf("large output did not finish bounded: %+v", s)
	}
}
func TestNativeIntegrationCrashAndTimeout(t *testing.T) {
	for _, tc := range []struct {
		name, code string
		timeout    time.Duration
	}{{"panic", `panic("boom")`, time.Second}, {"exit", `os.Exit(0)`, time.Second}, {"timeout", `for {time.Sleep(time.Second)}`, 80 * time.Millisecond}} {
		t.Run(tc.name, func(t *testing.T) {
			n := Notebook{Version: 1, Imports: []string{"os", "time"}, Cells: []Cell{{Code: `_=os.Args;_=time.Second;` + tc.code}}}
			m := nativeStartTest(t, n, Kernel{RunTimeout: tc.timeout})
			m.mu.Lock()
			dir := m.session.dir
			m.mu.Unlock()
			s, err := m.Execute(context.Background(), n, 1)
			if err == nil || s.Active || s.Ready || s.Error == "" {
				t.Fatalf("worker failure not reported: %+v %v", s, err)
			}
			if _, err := os.Stat(dir); !os.IsNotExist(err) {
				t.Fatalf("failed worker directory remains: %s (%v)", dir, err)
			}
			if err := m.Stop(); err != nil {
				t.Fatal(err)
			}
		})
	}
}
func TestNativeIntegrationWholeNotebookBuild(t *testing.T) {
	nativeIntegration(t)
	for _, n := range []Notebook{
		{Version: 1, Cells: []Cell{{Code: `println("never")`}, {Code: `unknownName()`}}},
		{Version: 1, Cells: []Cell{{Code: `if true {`}, {Code: `}`}}},
		{Version: 1, Cells: []Cell{{Code: `goto end; end: println(1)`}}},
		{Version: 1, Dependencies: map[string]string{"example.com/unused": "latest"}, Cells: []Cell{{Code: `println(1)`}}},
	} {
		m := &NativeManager{}
		s, err := m.Start(context.Background(), n)
		_ = m.Stop()
		if err == nil || s.Active {
			t.Fatalf("invalid whole notebook accepted: %+v", s)
		}
	}
}
func TestNativeIntegrationStopInterruptsAndCleans(t *testing.T) {
	n := Notebook{Version: 1, Imports: []string{"os", "time"}, Cells: []Cell{{Code: `os.WriteFile("running.json",[]byte("1"),0600);for{time.Sleep(time.Second)}`}}}
	m := nativeStartTest(t, n, Kernel{RunTimeout: 10 * time.Second})
	m.mu.Lock()
	dir := m.session.dir
	m.mu.Unlock()
	done := make(chan error, 1)
	go func() { _, err := m.Execute(context.Background(), n, 1); done <- err }()
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		if _, err := os.Stat(filepath.Join(dir, "running.json")); err == nil {
			break
		}
		time.Sleep(5 * time.Millisecond)
	}
	if state := m.State(); !state.Active || state.Ready {
		t.Fatalf("running command snapshot: %+v", state)
	}
	if _, err := m.Execute(context.Background(), n, 1); err == nil {
		t.Fatal("concurrent command accepted")
	}
	if err := m.Stop(); err != nil {
		t.Fatal(err)
	}
	select {
	case err := <-done:
		if err == nil {
			t.Fatal("interrupted command succeeded")
		}
	case <-time.After(time.Second):
		t.Fatal("stop blocked command")
	}
	if _, err := os.Stat(dir); !os.IsNotExist(err) {
		t.Fatal("stop left directory")
	}
}

func TestNativeRejectsChangedExecutionInputsAndHistoryOverflow(t *testing.T) {
	n := Notebook{Version: 1, Imports: []string{"fmt"}, Cells: []Cell{{Code: `fmt.Print("one")`}}, Dependencies: map[string]string{"example.com/module": "v1.0.0"}, Files: []InputFile{{Name: "a.json", Data: "e30="}}}
	for _, change := range []func(*Notebook){
		func(n *Notebook) { n.Imports = []string{"os"} },
		func(n *Notebook) { n.Dependencies = map[string]string{"example.com/module": "v1.1.0"} },
		func(n *Notebook) { n.Files = []InputFile{{Name: "a.json", Data: "W10="}} },
	} {
		s := newNativeSession("", "marker")
		s.snapshot = NativeSnapshot{Active: true, Ready: true, NextCell: 1, Fingerprint: nativeFingerprint(n)}
		m := &NativeManager{session: s}
		edited := n
		change(&edited)
		if snap, err := m.Execute(context.Background(), edited, 1); err == nil || !snap.Active || len(snap.History) != 0 {
			t.Fatal("changed inputs were accepted or lost memory")
		}
	}
	s := newNativeSession("", "marker")
	s.snapshot = NativeSnapshot{Active: true, Ready: true, NextCell: 1, Fingerprint: nativeFingerprint(n), History: make([]int, nativeHistoryLimit)}
	m := &NativeManager{session: s}
	if snap, err := m.Execute(context.Background(), n, 1); err == nil || len(snap.History) != nativeHistoryLimit || !snap.Active {
		t.Fatal("history limit must reject further work while retaining provenance and memory")
	}
}

func TestNativeIntegrationIdleCrashUpdatesResult(t *testing.T) {
	n := Notebook{Version: 1, Imports: []string{"fmt", "os", "time"}, Cells: []Cell{{Code: `go func(){time.Sleep(80*time.Millisecond);fmt.Fprint(os.Stderr,"async crash");os.Exit(7)}()`}}}
	m := nativeStartTest(t, n, Kernel{})
	nativeRunTest(t, m, n, 1)
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		s := m.State()
		if !s.Active {
			if s.Error == "" || s.Result == nil || s.Result.Error == "" || !strings.Contains(s.Result.Stderr, "async crash") {
				t.Fatalf("asynchronous exit lost final console/error: %+v", s)
			}
			return
		}
		time.Sleep(5 * time.Millisecond)
	}
	t.Fatal("idle worker exit went undetected")
}

func TestNativeIntegrationExternalGenericsAndExactPins(t *testing.T) {
	const version = "v0.0.0-20250620022241-b7579e27df2b"
	n := Notebook{Version: 1, Imports: []string{"fmt", "golang.org/x/exp/slices"}, Dependencies: map[string]string{"golang.org/x/exp": version}, Cells: []Cell{{Code: `values:=[]int{3,2,1};slices.Sort(values);fmt.Print(values)`}}}
	m := nativeStartTest(t, n, Kernel{})
	s := nativeRunTest(t, m, n, 1)
	if s.Result.Stdout != "[1 2 3]" {
		t.Fatalf("external generics: %+v", s.Result)
	}
	p := s.Result.Provenance
	if p.GoVersionSource != "notebook binary" || !strings.Contains(p.GoMod, "golang.org/x/exp "+version) || !strings.Contains(p.GoSum, "golang.org/x/exp "+version) {
		t.Fatalf("compiler/module provenance missing: %+v", p)
	}
	if p.BuildTimeoutNS != 60*time.Second || p.RunTimeoutNS != 30*time.Second {
		t.Fatalf("timeouts not recorded: %+v", p)
	}
	s.History[0] = 999
	s.Result.Stdout = "mutated"
	if state := m.State(); state.History[0] != 1 || state.Result.Stdout != "[1 2 3]" {
		t.Fatal("caller mutated resident provenance/result")
	}
}

func TestNativeIntegrationIdleDoesNotExpire(t *testing.T) {
	n := Notebook{Version: 1, Cells: []Cell{{Code: `println("resident")`}}}
	m := nativeStartTest(t, n, Kernel{RunTimeout: 100 * time.Millisecond})
	time.Sleep(220 * time.Millisecond)
	if !m.State().Active {
		t.Fatal("per-command timeout expired idle resident memory")
	}
	nativeRunTest(t, m, n, 1)
	time.Sleep(220 * time.Millisecond)
	if !m.State().Active {
		t.Fatal("completed command timeout expired resident memory")
	}
}

func TestNativeStreamingPreservesNonProtocolBytes(t *testing.T) {
	s := newNativeSession("", "\x1eprivate:")
	w := &nativeStream{s: s}
	want := "before\x1eprivate:not-a-command\x1fafter"
	for _, b := range []byte(want + "\x1eprivate:0\x1f") {
		_, _ = w.Write([]byte{b})
	}
	w.flush()
	if got := s.output.out.String(); got != want {
		t.Fatalf("console altered: %q", got)
	}
	if len(w.pending) > 0 || s.boundaries[0] != 1 {
		t.Fatalf("boundary parse: %v", s.boundaries)
	}
}

func TestNativeIntegrationDefersAccumulate(t *testing.T) {
	n := Notebook{Version: 1, Imports: []string{"fmt"}, Cells: []Cell{{Code: `defer fmt.Print("deferred")`}, {Code: `fmt.Print("live")`}, {Code: `return`}}}
	m := nativeStartTest(t, n, Kernel{})
	nativeRunTest(t, m, n, 1)
	nativeRunTest(t, m, n, 1)
	nativeRunTest(t, m, n, 2)
	s, err := m.Execute(context.Background(), n, 3)
	if err == nil || s.Result.Stdout != "deferreddeferred" {
		t.Fatalf("native function defers changed semantics: %+v %v", s.Result, err)
	}
}

func TestNativeIntegrationTimeoutKillsProcessGroup(t *testing.T) {
	nativeIntegration(t)
	if runtime.GOOS == "windows" {
		t.Skip("Unix process-group behavior; Windows runtime needs native CI")
	}
	marker := filepath.Join(t.TempDir(), "orphan.json")
	shell := "sleep 0.3; printf orphan > " + nativeShellQuote(marker)
	code := fmt.Sprintf(`child:=exec.Command("sh","-c",%q);if err:=child.Start();err!=nil{panic(err)};for{time.Sleep(time.Second)}`, shell)
	n := Notebook{Version: 1, Imports: []string{"os/exec", "time"}, Cells: []Cell{{Code: code}}}
	m := nativeStartTest(t, n, Kernel{RunTimeout: 80 * time.Millisecond})
	if _, err := m.Execute(context.Background(), n, 1); err == nil {
		t.Fatal("infinite cell succeeded")
	}
	time.Sleep(400 * time.Millisecond)
	if _, err := os.Stat(marker); !os.IsNotExist(err) {
		t.Fatal("worker left a running child process after timeout")
	}
}
func nativeShellQuote(s string) string { return "'" + strings.ReplaceAll(s, "'", "'\"'\"'") + "'" }

func TestNativeIntegrationStopDuringStartup(t *testing.T) {
	nativeIntegration(t)
	n := Notebook{Version: 1, Cells: []Cell{{Code: `println("must not run")`}}}
	m := &NativeManager{}
	t.Cleanup(func() { _ = m.Stop() })
	done := make(chan error, 1)
	go func() { _, err := m.Start(context.Background(), n); done <- err }()
	var dir string
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		m.mu.Lock()
		if m.session != nil {
			dir = m.session.dir
		}
		m.mu.Unlock()
		if dir != "" {
			break
		}
		time.Sleep(time.Millisecond)
	}
	if dir == "" {
		t.Fatal("startup did not register cancellable session")
	}
	if err := m.Stop(); err != nil {
		t.Fatal(err)
	}
	select {
	case err := <-done:
		if err == nil {
			t.Fatal("cancelled startup succeeded")
		}
	case <-time.After(3 * time.Second):
		t.Fatal("startup did not stop")
	}
	if s := m.State(); s.Active || s.Ready {
		t.Fatal("cancelled startup left an active session")
	}
	if _, err := os.Stat(dir); !os.IsNotExist(err) {
		t.Fatal("cancelled startup left build directory")
	}
}

func TestNativeIntegrationCanonicalFilesAndCommandRevision(t *testing.T) {
	n := Notebook{Version: 1, Imports: []string{"os"}, Cells: []Cell{{Code: `if err:=os.WriteFile("retained.csv",[]byte("x\n1\n"),0600);err!=nil{panic(err)}`}, {Code: `if err:=os.Remove("retained.csv");err!=nil{panic(err)}`}}}
	m := nativeStartTest(t, n, Kernel{})
	first := nativeRunTest(t, m, n, 1)
	if !first.CommandAccepted || first.SessionID == "" || len(first.Files) != 1 || len(first.Result.Artifacts) != 1 {
		t.Fatalf("first files: %+v", first)
	}
	if _, err := m.ExecuteExpected(context.Background(), n, 1, "wrong", -1); err == nil {
		t.Fatal("negative revision bypassed CAS")
	}
	if rejected, err := m.ExecuteExpected(context.Background(), n, 1, first.SessionID, 0); err == nil || rejected.CommandAccepted {
		t.Fatal("stale history command accepted")
	}
	if got := m.State(); len(got.History) != 1 {
		t.Fatal("rejected stale command executed")
	}
	repeated, err := m.ExecuteExpected(context.Background(), n, 1, first.SessionID, 1)
	if err != nil || len(repeated.Files) != 1 || len(repeated.Result.Artifacts) != 0 {
		t.Fatalf("unchanged file disappeared: %+v %v", repeated, err)
	}
	repeated.Files[0].Data = "mutated caller"
	if m.State().Files[0].Data == "mutated caller" {
		t.Fatal("state aliases canonical files")
	}
	removed := nativeRunTest(t, m, n, 2)
	if removed.Files == nil || len(removed.Files) != 0 {
		t.Fatal("deleted file retained in canonical list")
	}
	recreated := nativeRunTest(t, m, n, 1)
	if len(recreated.Files) != 1 || len(recreated.Result.Artifacts) != 1 {
		t.Fatal("recreated file not reported")
	}
	if err := m.Stop(); err != nil {
		t.Fatal(err)
	}
	next, err := m.Start(context.Background(), n)
	if err != nil {
		t.Fatal(err)
	}
	if next.SessionID == first.SessionID {
		t.Fatal("replacement kernel reused instance id")
	}
	if _, err := m.ExecuteExpected(context.Background(), n, 1, "wrong", -1); err == nil {
		t.Fatal("negative revision bypassed CAS")
	}
	if rejected, err := m.ExecuteExpected(context.Background(), n, 1, first.SessionID, 0); err == nil || rejected.CommandAccepted {
		t.Fatal("old kernel command accepted")
	}
}

func TestNativeStopWaitsForPreparation(t *testing.T) {
	dir := t.TempDir()
	s := newNativeSession(dir, "test")
	s.prepared = make(chan struct{})
	s.cancel = func() {}
	stopped := make(chan error, 1)
	go func() { stopped <- s.stop() }()
	// Wait until Stop has taken ownership, then finish the simulated build write.
	deadline := time.Now().Add(3 * time.Second)
	for {
		s.mu.Lock()
		closed := s.closed
		s.mu.Unlock()
		if closed {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("stop did not begin")
		}
		time.Sleep(time.Millisecond)
	}
	select {
	case err := <-stopped:
		close(s.prepared)
		t.Fatalf("cleanup returned before preparation completed: %v", err)
	default:
	}
	if err := os.WriteFile(filepath.Join(dir, "notebook.exe"), []byte("build completed"), 0600); err != nil {
		close(s.prepared)
		t.Fatal(err)
	}
	close(s.prepared)
	select {
	case err := <-stopped:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("stop did not finish")
	}
	if _, err := os.Stat(dir); !os.IsNotExist(err) {
		t.Fatal("preparation directory retained")
	}
}

func TestNativeIntegrationSharedFunctionExample(t *testing.T) {
	data, err := os.ReadFile("examples/functions.gobook.json")
	if err != nil {
		t.Fatal(err)
	}
	var n Notebook
	if err = json.Unmarshal(data, &n); err != nil {
		t.Fatal(err)
	}
	m := nativeStartTest(t, n, Kernel{})
	for _, step := range []struct {
		cell int
		want string
	}{
		{1, "setup once\n"},
		{2, "mean=16.5 calls=1 setup=1 same=true\n"},
		{2, "mean=16.5 calls=2 setup=1 same=true\n"},
		{3, "previous=16.5\nUpdated the existing slice. Run cell 2 again to compute its new mean.\n"},
		{2, "mean=19.5 calls=3 setup=1 same=true\n"},
	} {
		got := nativeRunTest(t, m, n, step.cell)
		if got.Result.Stdout != step.want {
			t.Fatalf("cell %d: %q; want %q", step.cell, got.Result.Stdout, step.want)
		}
	}
	if got := m.State().History; !reflect.DeepEqual(got, []int{1, 2, 2, 3, 2}) {
		t.Fatalf("unexpected replay: %v", got)
	}
}
