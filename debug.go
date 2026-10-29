package main

import (
	"context"
	"errors"
	"fmt"
	"net"
	"net/rpc"
	"net/rpc/jsonrpc"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"
)

type DebugManager struct {
	Kernel  Kernel
	Delve   string
	startMu sync.Mutex
	mu      sync.Mutex
	session *debugSession
}
type DebugBreakpoint struct {
	Cell int `json:"cell"`
	Line int `json:"line"`
}
type DebugVariable struct {
	Name       string          `json:"name"`
	Type       string          `json:"type"`
	Value      string          `json:"value"`
	Unreadable string          `json:"unreadable,omitempty"`
	Children   []DebugVariable `json:"children,omitempty"`
}
type DebugFrame struct {
	Cell     int    `json:"cell"`
	Line     int    `json:"line"`
	File     string `json:"file"`
	Function string `json:"function"`
}
type DebugSnapshot struct {
	Running     bool              `json:"running"`
	Exited      bool              `json:"exited"`
	Cell        int               `json:"cell"`
	Line        int               `json:"line"`
	Function    string            `json:"function"`
	Locals      []DebugVariable   `json:"locals"`
	Arguments   []DebugVariable   `json:"arguments"`
	Stack       []DebugFrame      `json:"stack"`
	Breakpoints []DebugBreakpoint `json:"breakpoints"`
	Stdout      string            `json:"stdout"`
	Stderr      string            `json:"stderr"`
	Truncated   bool              `json:"truncated"`
	Error       string            `json:"error,omitempty"`
}
type debugSession struct {
	mu       sync.Mutex
	client   *rpc.Client
	cancel   context.CancelFunc
	cmd      *exec.Cmd
	target   *os.Process
	done     chan struct{}
	dir      string
	output   *capture
	snapshot DebugSnapshot
	closed   bool
	stopOnce sync.Once
	stopErr  error
}
type delveLocation struct {
	PC       uint64 `json:"pc"`
	File     string `json:"file"`
	Line     int    `json:"line"`
	Function *struct {
		Name string `json:"name"`
	} `json:"function"`
}
type delveState struct {
	Running       bool
	Exited        bool
	ExitStatus    int
	CurrentThread *struct {
		delveLocation
		GoroutineID int64
	}
	SelectedGoroutine *struct {
		ID         int64
		CurrentLoc delveLocation
	}
}

func debugFrame(loc delveLocation) DebugFrame {
	frame := DebugFrame{Line: loc.Line, File: loc.File}
	if loc.Function != nil {
		frame.Function = loc.Function.Name
	}
	name := filepath.Base(loc.File)
	if strings.HasPrefix(name, "cell") && strings.HasSuffix(name, ".go") {
		frame.Cell, _ = strconv.Atoi(strings.TrimSuffix(strings.TrimPrefix(name, "cell"), ".go"))
	}
	return frame
}

// Closing a timed-out connection releases pending RPC commands as well.
func debugRPC(ctx context.Context, client *rpc.Client, method string, args, reply any) error {
	call := client.Go("RPCServer."+method, args, reply, make(chan *rpc.Call, 1))
	select {
	case result := <-call.Done:
		return result.Error
	case <-ctx.Done():
		client.Close()
		return ctx.Err()
	}
}
func (s *debugSession) call(ctx context.Context, method string, args, reply any) error {
	return debugRPC(ctx, s.client, method, args, reply)
}

// Delve chooses its loopback port; parse only its bounded startup announcement.
type debugStartupWriter struct {
	c       *capture
	mu      sync.Mutex
	line    string
	address chan string
}

func (w *debugStartupWriter) Write(p []byte) (int, error) {
	n, err := (stream{c: w.c}).Write(p)
	w.mu.Lock()
	defer w.mu.Unlock()
	if len(w.line) < 4096 {
		w.line += string(p[:min(len(p), 4096-len(w.line))])
	}
	for {
		i := strings.IndexByte(w.line, '\n')
		if i < 0 {
			break
		}
		line := strings.TrimSpace(w.line[:i])
		w.line = w.line[i+1:]
		const prefix = "API server listening at: "
		if strings.HasPrefix(line, prefix) {
			select {
			case w.address <- strings.TrimPrefix(line, prefix):
			default:
			}
		}
	}
	return n, err
}
func (m *DebugManager) Start(ctx context.Context, n Notebook, bps []DebugBreakpoint) (snap DebugSnapshot, err error) {
	m.startMu.Lock()
	defer m.startMu.Unlock()
	if err = m.Stop(); err != nil {
		return snap, err
	}
	n.RunThrough = nil
	src, err := source(n)
	if err != nil {
		return snap, err
	}
	if len(bps) > 128 {
		return snap, errors.New("breakpoint limit exceeded (128)")
	}
	for _, bp := range bps {
		if bp.Cell < 1 || bp.Cell > len(n.Cells) || bp.Line < 1 || bp.Line > strings.Count(n.Cells[bp.Cell-1].Code, "\n")+1 {
			return snap, fmt.Errorf("invalid breakpoint cell %d line %d", bp.Cell, bp.Line)
		}
	}
	delve := m.Delve
	if delve == "" {
		delve = "dlv"
	}
	delve, err = exec.LookPath(delve)
	if err != nil {
		return snap, fmt.Errorf("Delve unavailable: install dlv or set -delve: %w", err)
	}
	if !filepath.IsAbs(delve) {
		delve, err = filepath.Abs(delve)
		if err != nil {
			return snap, err
		}
	}
	dir, err := os.MkdirTemp("", "gobook-debug-")
	if err != nil {
		return snap, err
	}
	s := &debugSession{dir: dir, output: &capture{remaining: outputLimit}, snapshot: DebugSnapshot{Locals: []DebugVariable{}, Arguments: []DebugVariable{}, Stack: []DebugFrame{}, Breakpoints: []DebugBreakpoint{}}}
	life, cancel := context.WithTimeout(context.Background(), 15*time.Minute)
	processCtx, cancelProcess := context.WithCancel(context.Background())
	s.cancel = func() { cancel(); cancelProcess() }
	m.mu.Lock()
	m.session = s
	m.mu.Unlock()
	go func() { <-life.Done(); _ = s.stop() }()
	defer func() {
		if err != nil {
			m.Stop()
		}
	}()
	inputs, _ := validateNotebook(n)
	for name, data := range inputs {
		if err = os.WriteFile(filepath.Join(dir, name), data, 0600); err != nil {
			return snap, err
		}
	}
	for name, data := range map[string]string{"main.go": src, "go.mod": m.Kernel.moduleFile(n)} {
		if err = os.WriteFile(filepath.Join(dir, name), []byte(data), 0600); err != nil {
			return snap, err
		}
	}
	timeout := m.Kernel.BuildTimeout
	if timeout == 0 {
		timeout = 60 * time.Second
	}
	buildCtx, buildCancel := context.WithTimeout(ctx, timeout)
	stopBuild := context.AfterFunc(life, buildCancel)
	err = runProcess(buildCtx, dir, s.output, "go", "build", "-p", "2", "-buildvcs=false", "-mod=mod", "-gcflags=all=-N -l", "-o", filepath.Join(dir, "notebook"), "main.go")
	stopBuild()
	buildCancel()
	if err != nil {
		snap = s.view()
		return snap, fmt.Errorf("debug build: %w: %s", err, snap.Stderr)
	}
	var provenance Provenance
	if err = captureModules(dir, &provenance); err != nil {
		return snap, err
	}
	if err = verifyExactPins(n.Dependencies, provenance.GoMod); err != nil {
		return snap, err
	}
	// These statement fragments are for source lookup, never compilation.
	for i, cell := range n.Cells {
		if err = os.WriteFile(filepath.Join(dir, fmt.Sprintf("cell%d.go", i+1)), []byte(cell.Code), 0600); err != nil {
			return snap, err
		}
	}
	address := make(chan string, 1)
	cmd := exec.CommandContext(processCtx, delve, "exec", filepath.Join(dir, "notebook"), "--headless", "--api-version=2", "--listen=127.0.0.1:0", "--accept-multiclient=false", "--check-go-version=true")
	cmd.Dir = dir
	cmd.Env = append(os.Environ(), "GOFLAGS=", "GOENV=off", "GOWORK=off")
	cmd.Stdout = &debugStartupWriter{c: s.output, address: address}
	cmd.Stderr = stream{c: s.output, stderr: true}
	configureProcess(cmd)
	s.mu.Lock()
	if s.closed {
		s.mu.Unlock()
		return snap, errors.New("debugger stopped")
	}
	err = cmd.Start()
	if err == nil {
		s.cmd = cmd
		s.done = make(chan struct{})
	}
	s.mu.Unlock()
	if err != nil {
		return snap, err
	}
	go func() {
		waitErr := cmd.Wait()
		cleanupProcess(cmd)
		s.mu.Lock()
		if !s.closed && waitErr != nil {
			s.snapshot.Error = fmt.Sprintf("Delve: %v", waitErr)
		}
		s.snapshot.Running = false
		s.snapshot.Exited = true
		close(s.done)
		s.mu.Unlock()
	}()
	startupCtx, startCancel := context.WithTimeout(ctx, 15*time.Second)
	defer startCancel()
	var addr string
	select {
	case addr = <-address:
	case <-s.done:
		snap = s.view()
		return snap, fmt.Errorf("Delve startup failed: %s", snap.Stderr)
	case <-startupCtx.Done():
		return snap, fmt.Errorf("Delve startup: %w", startupCtx.Err())
	case <-life.Done():
		return snap, life.Err()
	}
	host, _, e := net.SplitHostPort(addr)
	if e != nil || host != "127.0.0.1" {
		return snap, fmt.Errorf("invalid Delve loopback address %q", addr)
	}
	conn, e := (&net.Dialer{}).DialContext(startupCtx, "tcp", addr)
	if e != nil {
		return snap, e
	}
	s.mu.Lock()
	if s.closed {
		s.mu.Unlock()
		conn.Close()
		return snap, errors.New("debugger stopped")
	}
	s.client = jsonrpc.NewClient(conn)
	s.mu.Unlock()
	var pid struct{ Pid int }
	if err = s.call(startupCtx, "ProcessPid", struct{}{}, &pid); err != nil {
		return snap, err
	}
	if pid.Pid <= 0 {
		return snap, errors.New("Delve returned an invalid target PID")
	}
	target, findErr := os.FindProcess(pid.Pid)
	if findErr != nil {
		return snap, findErr
	}
	s.mu.Lock()
	if s.closed {
		s.mu.Unlock()
		cleanupProcess(&exec.Cmd{Process: target})
		return snap, errors.New("debugger stopped")
	}
	s.target = target
	s.mu.Unlock()
	if len(bps) == 0 {
		var found struct{ Locations []delveLocation }
		if err = s.call(startupCtx, "FindLocation", map[string]any{"Scope": map[string]any{"GoroutineID": -1}, "Loc": "main.main"}, &found); err != nil {
			return snap, err
		}
		if len(found.Locations) == 0 {
			return snap, errors.New("Delve could not locate main.main")
		}
		loc := found.Locations[0]
		bp := debugFrame(loc)
		bps = []DebugBreakpoint{{Cell: bp.Cell, Line: bp.Line}}
		if err = s.breakAt(startupCtx, loc.PC); err != nil {
			return snap, err
		}
	} else {
		for _, bp := range bps {
			var found struct{ Locations []delveLocation }
			location := fmt.Sprintf("cell%d.go:%d", bp.Cell, bp.Line)
			if err = s.call(startupCtx, "FindLocation", map[string]any{"Scope": map[string]any{"GoroutineID": -1}, "Loc": location}, &found); err != nil {
				return snap, fmt.Errorf("breakpoint %s: %w", location, err)
			}
			if len(found.Locations) == 0 {
				return snap, fmt.Errorf("no executable location at %s", location)
			}
			if err = s.breakAt(startupCtx, found.Locations[0].PC); err != nil {
				return snap, err
			}
		}
	}
	s.mu.Lock()
	s.snapshot.Breakpoints = append([]DebugBreakpoint(nil), bps...)
	s.mu.Unlock()
	return m.Command(ctx, "continue")
}
func (s *debugSession) breakAt(ctx context.Context, pc uint64) error {
	var reply struct{}
	return s.call(ctx, "CreateBreakpoint", map[string]any{"Breakpoint": map[string]any{"addr": pc}}, &reply)
}
func (s *debugSession) view() DebugSnapshot {
	s.mu.Lock()
	snap := s.snapshot
	s.mu.Unlock()
	s.output.mu.Lock()
	snap.Stdout = s.output.out.String()
	snap.Stderr = s.output.err.String()
	snap.Truncated = s.output.truncated
	s.output.mu.Unlock()
	lines := strings.Split(snap.Stdout, "\n")
	filtered := lines[:0]
	for _, line := range lines {
		if !strings.HasPrefix(line, "API server listening at: ") {
			filtered = append(filtered, line)
		}
	}
	snap.Stdout = strings.Join(filtered, "\n")
	return snap
}
func (m *DebugManager) current() (*debugSession, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.session == nil {
		return nil, errors.New("no debugger session")
	}
	return m.session, nil
}
func (m *DebugManager) State(ctx context.Context) (DebugSnapshot, error) {
	s, err := m.current()
	if err != nil {
		return DebugSnapshot{}, err
	}
	return s.view(), nil
}
func (m *DebugManager) Command(ctx context.Context, command string) (DebugSnapshot, error) {
	switch command {
	case "continue", "next", "step", "stepOut", "halt":
	default:
		return DebugSnapshot{}, fmt.Errorf("unsupported debugger command %q", command)
	}
	s, err := m.current()
	if err != nil {
		return DebugSnapshot{}, err
	}
	s.mu.Lock()
	if s.closed || s.client == nil || s.snapshot.Exited {
		s.mu.Unlock()
		return s.view(), errors.New("debugger is not active")
	}
	if command != "halt" && s.snapshot.Running {
		s.mu.Unlock()
		return s.view(), errors.New("debugger is running; halt before stepping")
	}
	if command == "halt" && !s.snapshot.Running {
		s.mu.Unlock()
		return s.view(), nil
	}
	if command != "halt" {
		s.snapshot.Running = true
		s.snapshot.Error = ""
		s.snapshot.Locals = []DebugVariable{}
		s.snapshot.Arguments = []DebugVariable{}
		s.snapshot.Stack = []DebugFrame{}
	}
	s.mu.Unlock()
	if command == "halt" {
		callCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
		defer cancel()
		var result struct{ State delveState }
		err = s.call(callCtx, "Command", map[string]any{"name": "halt"}, &result)
		return s.view(), err
	}
	var result struct{ State delveState }
	// Send before returning so an immediate Halt cannot overtake Continue.
	call := s.client.Go("RPCServer.Command", map[string]any{"name": command}, &result, make(chan *rpc.Call, 1))
	go func() { completed := <-call.Done; s.update(result.State, completed.Error) }()
	return s.view(), nil
}
func (s *debugSession) update(state delveState, commandErr error) {
	snap := s.view()
	snap.Running = false
	snap.Exited = state.Exited
	snap.Cell = 0
	snap.Line = 0
	snap.Function = ""
	if commandErr != nil {
		snap.Error = commandErr.Error()
	}
	if state.Exited && state.ExitStatus != 0 {
		snap.Error = fmt.Sprintf("notebook exited with status %d", state.ExitStatus)
	}
	if !state.Exited && commandErr == nil {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		loc := delveLocation{}
		goroutine := int64(-1)
		if state.CurrentThread != nil {
			loc = state.CurrentThread.delveLocation
			goroutine = state.CurrentThread.GoroutineID
		}
		if state.SelectedGoroutine != nil {
			goroutine = state.SelectedGoroutine.ID
		}
		frame := debugFrame(loc)
		snap.Cell = frame.Cell
		snap.Line = frame.Line
		snap.Function = frame.Function
		scope := map[string]any{"GoroutineID": goroutine, "Frame": 0}
		cfg := map[string]any{"FollowPointers": true, "MaxVariableRecurse": 1, "MaxStringLen": 256, "MaxArrayValues": 32, "MaxStructFields": 32}
		var locals struct{ Variables []DebugVariable }
		if err := s.call(ctx, "ListLocalVars", map[string]any{"Scope": scope, "Cfg": cfg}, &locals); err != nil {
			snap.Error = err.Error()
		} else {
			snap.Locals = locals.Variables
		}
		var args struct{ Args []DebugVariable }
		if err := s.call(ctx, "ListFunctionArgs", map[string]any{"Scope": scope, "Cfg": cfg}, &args); err == nil {
			snap.Arguments = args.Args
		}
		var stack struct{ Locations []delveLocation }
		if err := s.call(ctx, "Stacktrace", map[string]any{"Id": goroutine, "Depth": 32, "Full": false}, &stack); err != nil {
			snap.Error = err.Error()
		} else {
			snap.Stack = []DebugFrame{}
			for _, loc := range stack.Locations {
				snap.Stack = append(snap.Stack, debugFrame(loc))
			}
		}
	}
	s.mu.Lock()
	if !s.closed {
		s.snapshot = snap
	}
	s.mu.Unlock()
}
func (m *DebugManager) Stop() error {
	m.mu.Lock()
	s := m.session
	m.session = nil
	m.mu.Unlock()
	if s == nil {
		return nil
	}
	return s.stop()
}

func (s *debugSession) stop() error {
	s.stopOnce.Do(func() {
		s.mu.Lock()
		s.closed = true
		running := s.snapshot.Running
		s.snapshot.Running = false
		s.snapshot.Exited = true
		client, cmd, done, target := s.client, s.cmd, s.done, s.target
		s.mu.Unlock()
		// Delve's inferior has its own process group. Let Delve halt, kill and
		// reap it before terminating the debugger; keep a group-kill fallback.
		if client != nil {
			ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
			if running {
				var out struct{}
				_ = debugRPC(ctx, client, "Command", map[string]any{"name": "halt"}, &out)
			}
			var out struct{}
			_ = debugRPC(ctx, client, "Detach", map[string]any{"Kill": true}, &out)
			cancel()
			_ = client.Close()
		} else if cmd != nil {
			_ = cmd.Process.Signal(os.Interrupt)
		}
		if target != nil {
			cleanupProcess(&exec.Cmd{Process: target})
			_ = target.Release()
		}
		if cmd != nil {
			select {
			case <-done:
			case <-time.After(time.Second):
				cleanupProcess(cmd)
			}
		}
		s.cancel()
		if cmd != nil {
			cleanupProcess(cmd)
			select {
			case <-done:
			case <-time.After(3 * time.Second):
				s.stopErr = errors.New("Delve did not stop within 3 seconds")
			}
		}
		if err := os.RemoveAll(s.dir); s.stopErr == nil {
			s.stopErr = err
		}
	})
	return s.stopErr
}
