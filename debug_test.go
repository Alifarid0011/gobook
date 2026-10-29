package main

import (
	"context"
	"encoding/json"
	"fmt"
	"net"
	"net/rpc/jsonrpc"
	"os"
	"os/exec"
	"runtime"
	"strconv"
	"strings"
	"testing"
	"time"
)

func TestDebugRPCProtocol(t *testing.T) {
	clientConn, serverConn := net.Pipe()
	defer serverConn.Close()
	client := jsonrpc.NewClient(clientConn)
	defer client.Close()
	served := make(chan error, 1)
	go func() {
		var request struct {
			Method string
			Params []struct{ Loc string }
			ID     int
		}
		if err := json.NewDecoder(serverConn).Decode(&request); err != nil {
			served <- err
			return
		}
		if request.Method != "RPCServer.FindLocation" || len(request.Params) != 1 || request.Params[0].Loc != "cell2.go:3" {
			served <- fmt.Errorf("bad RPC request: %+v", request)
			return
		}
		_, err := fmt.Fprintf(serverConn, `{"id":%d,"result":{"Locations":[{"pc":123,"file":"/tmp/cell2.go","line":3,"function":{"name":"main.main"}}]},"error":null}`+"\n", request.ID)
		served <- err
	}()
	var reply struct{ Locations []delveLocation }
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	if err := debugRPC(ctx, client, "FindLocation", map[string]any{"Loc": "cell2.go:3"}, &reply); err != nil {
		t.Fatal(err)
	}
	if err := <-served; err != nil {
		t.Fatal(err)
	}
	if len(reply.Locations) != 1 {
		t.Fatalf("locations: %+v", reply)
	}
	frame := debugFrame(reply.Locations[0])
	if frame.Cell != 2 || frame.Line != 3 || frame.Function != "main.main" {
		t.Fatalf("frame: %+v", frame)
	}
}
func TestDebugRPCTimeout(t *testing.T) {
	a, b := net.Pipe()
	defer b.Close()
	client := jsonrpc.NewClient(a)
	defer client.Close()
	go func() { var request any; _ = json.NewDecoder(b).Decode(&request) }()
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Millisecond)
	defer cancel()
	if err := debugRPC(ctx, client, "State", struct{}{}, &struct{}{}); err != context.DeadlineExceeded {
		t.Fatalf("got %v", err)
	}
}
func TestDebugValidation(t *testing.T) {
	m := &DebugManager{}
	if _, err := m.Command(context.Background(), "shell"); err == nil {
		t.Fatal("unsupported command accepted")
	}
	if _, err := m.Start(context.Background(), Notebook{Version: 1, Cells: []Cell{{Code: "x := 1"}}}, []DebugBreakpoint{{Cell: 1, Line: 2}}); err == nil || !strings.Contains(err.Error(), "invalid breakpoint") {
		t.Fatalf("got %v", err)
	}
	if err := m.Stop(); err != nil {
		t.Fatal(err)
	}
}
func waitDebug(t *testing.T, m *DebugManager) DebugSnapshot {
	t.Helper()
	deadline := time.After(15 * time.Second)
	tick := time.NewTicker(10 * time.Millisecond)
	defer tick.Stop()
	for {
		snap, err := m.State(context.Background())
		if err != nil {
			t.Fatal(err)
		}
		if !snap.Running {
			if snap.Error != "" {
				t.Fatalf("debugger: %s (stderr %s)", snap.Error, snap.Stderr)
			}
			return snap
		}
		select {
		case <-deadline:
			t.Fatal("debugger command timed out")
		case <-tick.C:
		}
	}
}

// A real ptrace-capable Delve is optional in ordinary CI. Set GOBOOK_DLV to opt
// in explicitly, or put dlv on PATH. Failures after discovery are never skipped.
func TestDelveIntegration(t *testing.T) {
	delve := os.Getenv("GOBOOK_DLV")
	if delve == "" {
		var err error
		delve, err = exec.LookPath("dlv")
		if err != nil {
			t.Skip("dlv unavailable (set GOBOOK_DLV to run the real debugger test)")
		}
	}
	m := &DebugManager{Delve: delve, Kernel: Kernel{BuildTimeout: 60 * time.Second}}
	defer m.Stop()
	n := Notebook{Version: 1, Imports: []string{"fmt"}, Cells: []Cell{{Code: "x := 40\nx += 2\nfmt.Println(x)"}, {Code: "fmt.Println(x + 1)"}}}
	if _, err := m.Start(context.Background(), n, []DebugBreakpoint{{Cell: 1, Line: 2}}); err != nil {
		t.Fatal(err)
	}
	snap := waitDebug(t, m)
	if snap.Cell != 1 || snap.Line != 2 || len(snap.Stack) == 0 {
		t.Fatalf("breakpoint: %+v", snap)
	}
	assertX := func(want string) {
		t.Helper()
		for _, v := range snap.Locals {
			if v.Name == "x" && v.Value == want {
				return
			}
		}
		t.Fatalf("want x=%s, locals %+v", want, snap.Locals)
	}
	assertX("40")
	if _, err := m.Command(context.Background(), "next"); err != nil {
		t.Fatal(err)
	}
	snap = waitDebug(t, m)
	assertX("42")
	if snap.Cell != 1 || snap.Line != 3 {
		t.Fatalf("next: %+v", snap)
	}
	if _, err := m.Command(context.Background(), "continue"); err != nil {
		t.Fatal(err)
	}
	snap = waitDebug(t, m)
	if !snap.Exited || !strings.Contains(snap.Stdout, "42\n43\n") {
		t.Fatalf("exit: %+v", snap)
	}
}

func TestDelveStepAndHalt(t *testing.T) {
	delve := os.Getenv("GOBOOK_DLV")
	if delve == "" {
		var err error
		delve, err = exec.LookPath("dlv")
		if err != nil {
			t.Skip("dlv unavailable")
		}
	}
	m := &DebugManager{Delve: delve}
	defer m.Stop()
	n := Notebook{Version: 1, Imports: []string{"fmt"}, Cells: []Cell{{Code: "double := func(n int) int {\nreturn n * 2\n}\nx := 21\nx = double(x)\nfmt.Println(x)"}}}
	if _, err := m.Start(context.Background(), n, []DebugBreakpoint{{Cell: 1, Line: 5}}); err != nil {
		t.Fatal(err)
	}
	snap := waitDebug(t, m)
	if snap.Line != 5 {
		t.Fatalf("before step: %+v", snap)
	}
	if _, err := m.Command(context.Background(), "step"); err != nil {
		t.Fatal(err)
	}
	snap = waitDebug(t, m)
	if !strings.Contains(snap.Function, "func1") || len(snap.Stack) < 2 {
		t.Fatalf("step into: %+v", snap)
	}
	if _, err := m.Command(context.Background(), "stepOut"); err != nil {
		t.Fatal(err)
	}
	snap = waitDebug(t, m)
	if snap.Function != "main.main" {
		t.Fatalf("step out: %+v", snap)
	}
	// Restart replaces the previous process and its working directory.
	n = Notebook{Version: 1, Cells: []Cell{{Code: "x := 0\nfor {\nx++\n}"}}}
	if _, err := m.Start(context.Background(), n, nil); err != nil {
		t.Fatal(err)
	}
	snap = waitDebug(t, m)
	t.Logf("default breakpoint: cell=%d line=%d", snap.Cell, snap.Line)
	if _, err := m.Command(context.Background(), "continue"); err != nil {
		t.Fatal(err)
	}
	if _, err := m.Command(context.Background(), "halt"); err != nil {
		t.Fatal(err)
	}
	snap = waitDebug(t, m)
	if snap.Exited {
		t.Fatalf("halt exited: %+v", snap)
	}
	session, err := m.current()
	if err != nil {
		t.Fatal(err)
	}
	dir := session.dir
	targetPID := session.target.Pid
	if err := m.Stop(); err != nil {
		t.Fatal(err)
	}
	if runtime.GOOS == "linux" {
		if _, err := os.Stat("/proc/" + strconv.Itoa(targetPID)); !os.IsNotExist(err) {
			t.Fatalf("target PID %d still exists: %v", targetPID, err)
		}
	}
	if _, err := os.Stat(dir); !os.IsNotExist(err) {
		t.Fatalf("debug directory retained: %v", err)
	}
	if err := m.Stop(); err != nil {
		t.Fatal(err)
	}
}

func TestDebugRPCError(t *testing.T) {
	a, b := net.Pipe()
	defer b.Close()
	client := jsonrpc.NewClient(a)
	defer client.Close()
	go func() {
		var request struct{ ID int }
		if json.NewDecoder(b).Decode(&request) == nil {
			_, _ = fmt.Fprintf(b, `{"id":%d,"result":null,"error":"no executable location"}`+"\n", request.ID)
		}
	}()
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	err := debugRPC(ctx, client, "FindLocation", struct{}{}, &struct{}{})
	if err == nil || err.Error() != "no executable location" {
		t.Fatalf("got %v", err)
	}
}
