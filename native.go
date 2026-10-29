package main

import (
	"bufio"
	"bytes"
	"context"
	"crypto/rand"
	"crypto/sha256"
	"debug/buildinfo"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"time"
)

const nativeHistoryLimit = 1000

type NativeSnapshot struct {
	Notebook *Notebook `json:"notebook,omitempty"`
	Engine   string    `json:"engine,omitempty"`
	// CommandAccepted describes only the execute response, never stored state.
	CommandAccepted bool       `json:"commandAccepted,omitempty"`
	SessionID       string     `json:"sessionId"`
	Files           []Artifact `json:"files"`
	Active          bool       `json:"active"`
	Ready           bool       `json:"ready"`
	CurrentCell     int        `json:"currentCell"`
	NextCell        int        `json:"nextCell"`
	HeapAlloc       uint64     `json:"heapAlloc"`
	Goroutines      int        `json:"goroutines"`
	Fingerprint     string     `json:"fingerprint"`
	History         []int      `json:"history"`
	Result          *Result    `json:"result,omitempty"`
	Error           string     `json:"error,omitempty"`
}

type NativeManager struct {
	Kernel        Kernel
	WorkerCommand []string // Optional worker executable override for process integration tests.
	operation     sync.Mutex
	mu            sync.Mutex
	session       *nativeSession
}

type nativeSession struct {
	mu             sync.Mutex
	snapshot       NativeSnapshot
	dir, marker    string
	interactive    bool
	configuration  string
	output         *capture
	boundaries     [2]int
	changed        chan struct{}
	cmd            *exec.Cmd
	cancel         context.CancelFunc
	conn           net.Conn
	reader         *bufio.Reader
	done           chan struct{}
	prepared       chan struct{}
	closed         bool
	stopOnce       sync.Once
	stopErr        error
	provenance     Provenance
	inputs         map[string][]byte
	artifactHashes map[string][32]byte
}

type nativeEvent struct {
	Error      string `json:"error,omitempty"`
	Cell       int    `json:"cell"`
	Sequence   int    `json:"sequence"`
	HeapAlloc  uint64 `json:"heapAlloc"`
	Goroutines int    `json:"goroutines"`
}

func nativeFingerprint(n Notebook) string {
	n.Title = ""
	n.Note = ""
	n.RunThrough = nil
	data, _ := json.Marshal(n)
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:])
}

func newNativeSession(dir, marker string) *nativeSession {
	return &nativeSession{dir: dir, marker: marker, output: &capture{remaining: outputLimit}, changed: make(chan struct{}, 1), artifactHashes: make(map[string][32]byte)}
}

// Each pipe keeps only a possible marker prefix plus at most six sequence bytes.
// Parsing continues after the shared console budget is exhausted. A ready control
// message alone is insufficient: both pipe markers must also have been drained.
// Background goroutine output is captured when drained and may cross commands;
// notebook code must synchronize its own goroutines when output order matters.
type nativeStream struct {
	s       *nativeSession
	stderr  bool
	pending []byte
}

func (w *nativeStream) Write(p []byte) (int, error) {
	n := len(p)
	w.s.mu.Lock()
	defer w.s.mu.Unlock()
	// exec supplies small chunks, but this loop also bounds direct large writes.
	for len(p) > 0 {
		size := min(len(p), 4096)
		w.pending = append(w.pending, p[:size]...)
		p = p[size:]
		w.parse()
	}
	return n, nil
}
func (w *nativeStream) emit(p []byte) { _, _ = (stream{c: w.s.output, stderr: w.stderr}).Write(p) }
func (w *nativeStream) parse() {
	marker := []byte(w.s.marker)
	for len(w.pending) > 0 {
		start := bytes.Index(w.pending, marker)
		if start < 0 {
			keep := min(len(w.pending), len(marker)-1)
			for keep > 0 && !bytes.Equal(w.pending[len(w.pending)-keep:], marker[:keep]) {
				keep--
			}
			w.emit(w.pending[:len(w.pending)-keep])
			w.pending = append(w.pending[:0], w.pending[len(w.pending)-keep:]...)
			return
		}
		w.emit(w.pending[:start])
		w.pending = w.pending[start:]
		suffix := w.pending[len(marker):]
		end := bytes.IndexByte(suffix, '\x1f')
		if end < 0 && len(suffix) <= 6 {
			return
		}
		seq, err := strconv.Atoi(string(suffix[:max(0, end)]))
		index := 0
		if w.stderr {
			index = 1
		}
		if end < 0 || end > 6 || err != nil || seq != w.s.boundaries[index] || seq > nativeHistoryLimit {
			w.emit(w.pending[:1])
			w.pending = w.pending[1:]
			continue
		}
		w.pending = w.pending[len(marker)+end+1:]
		w.s.boundaries[index]++
		select {
		case w.s.changed <- struct{}{}:
		default:
		}
	}
}
func (w *nativeStream) flush() {
	w.s.mu.Lock()
	defer w.s.mu.Unlock()
	w.emit(w.pending)
	w.pending = nil
}

func nativeSource(n Notebook) (string, string, string, error) {
	n.RunThrough = nil
	if _, err := source(n); err != nil {
		return "", "", "", err
	}
	if len(n.Cells) < 1 || len(n.Cells) > 1000 {
		return "", "", "", errors.New("native sessions require 1..1000 cells")
	}
	if err := validatePartialCells(n.Cells); err != nil {
		return "", "", "", fmt.Errorf("native sessions require complete statements without goto: %w", err)
	}
	nonce := make([]byte, 16)
	if _, err := rand.Read(nonce); err != nil {
		return "", "", "", err
	}
	id := "gobookNative" + hex.EncodeToString(nonce)
	marker := "\x1e" + id + ":"
	var src strings.Builder
	src.WriteString("package main\nimport (\n")
	for _, p := range n.Imports {
		src.WriteString(strconv.Quote(p) + "\n")
	}
	fmt.Fprintf(&src, ")\nfunc main(){\nif %sGate(0)!=1{return}\n", id)
	for i, cell := range n.Cells {
		fmt.Fprintf(&src, "%sCell%d: ;\n//line cell%d.go:1\n%s\n//line gobook_native.go:1\nswitch %sGate(%d){\n", id, i+1, i+1, cell.Code, id, i+1)
		for target := 1; target <= i+1; target++ {
			fmt.Fprintf(&src, "case %d:goto %sCell%d\n", target, id, target)
		}
		if i == len(n.Cells)-1 {
			src.WriteString("default:return\n")
		}
		src.WriteString("}\n")
	}
	src.WriteString("}\n")
	helper := strings.NewReplacer("NATIVEID", id, "MARKER", strconv.Quote(marker), "CELLCOUNT", strconv.Itoa(len(n.Cells))).Replace(nativeRuntime)
	return src.String(), helper, marker, nil
}

const nativeRuntime = `package main
import("bufio";"encoding/json";"fmt";"net";"os";"runtime";"strconv";"strings")
var NATIVEIDConn net.Conn
var NATIVEIDReader *bufio.Reader
var NATIVEIDSequence int
func NATIVEIDGate(cell int) int {
 if NATIVEIDConn==nil {
  address,token:=os.Getenv("GOBOOK_NATIVE_ADDRESS"),os.Getenv("GOBOOK_NATIVE_TOKEN")
  os.Unsetenv("GOBOOK_NATIVE_ADDRESS");os.Unsetenv("GOBOOK_NATIVE_TOKEN")
  conn,err:=net.Dial("tcp",address);if err!=nil {panic(err)}
  NATIVEIDConn=conn;NATIVEIDReader=bufio.NewReaderSize(conn,128)
  if _,err=fmt.Fprintln(conn,token);err!=nil {panic(err)}
 }
 fmt.Fprintf(os.Stdout,MARKER+"%d\x1f",NATIVEIDSequence)
 fmt.Fprintf(os.Stderr,MARKER+"%d\x1f",NATIVEIDSequence)
 var stats runtime.MemStats;runtime.ReadMemStats(&stats)
 event:=struct{Cell int ` + "`json:\"cell\"`" + `;Sequence int ` + "`json:\"sequence\"`" + `;HeapAlloc uint64 ` + "`json:\"heapAlloc\"`" + `;Goroutines int ` + "`json:\"goroutines\"`" + `}{cell,NATIVEIDSequence,stats.HeapAlloc,runtime.NumGoroutine()}
 if err:=json.NewEncoder(NATIVEIDConn).Encode(event);err!=nil {panic(err)}
 command,err:=NATIVEIDReader.ReadSlice('\n');if err!=nil || len(command)>8 {os.Exit(125)}
 target,err:=strconv.Atoi(strings.TrimSpace(string(command)))
 if err!=nil || target<1 || target>min(cell+1,CELLCOUNT) {os.Exit(125)}
 NATIVEIDSequence++
 return target
}
`

func (m *NativeManager) State() NativeSnapshot {
	m.mu.Lock()
	s := m.session
	m.mu.Unlock()
	if s == nil {
		return NativeSnapshot{History: []int{}}
	}
	return s.view()
}
func (s *nativeSession) view() NativeSnapshot {
	s.mu.Lock()
	defer s.mu.Unlock()
	snap := s.snapshot
	if snap.Notebook != nil {
		snap.Notebook = copyNotebook(*snap.Notebook)
	}
	snap.History = append([]int{}, snap.History...)
	if snap.Files != nil {
		snap.Files = append([]Artifact{}, snap.Files...)
	}
	// JSON cloning keeps callers from mutating the resident session's result.
	if snap.Result != nil {
		data, _ := json.Marshal(snap.Result)
		var r Result
		_ = json.Unmarshal(data, &r)
		snap.Result = &r
	}
	return snap
}
func (m *NativeManager) Start(ctx context.Context, n Notebook) (NativeSnapshot, error) {
	return m.StartEngine(ctx, n, "native")
}
func (m *NativeManager) StartEngine(ctx context.Context, n Notebook, engine string) (snap NativeSnapshot, err error) {
	if engine != "native" && engine != "interactive" {
		return snap, errors.New("unknown kernel engine")
	}
	if engine == "interactive" {
		if n.Version != 1 || len(n.Cells) < 1 || len(n.Cells) > 1000 {
			return snap, errors.New("interactive sessions require version 1 and 1..1000 cells")
		}
		if len(n.Dependencies) > 0 {
			return snap, errors.New("interactive kernel does not support module pins; select the native kernel")
		}
		if err = supportedInteractiveImports(n.Imports); err != nil {
			return snap, err
		}
	}

	if !m.operation.TryLock() {
		return m.State(), errors.New("native session is busy")
	}
	defer m.operation.Unlock()
	m.mu.Lock()
	old := m.session
	m.mu.Unlock()
	if old != nil {
		old.mu.Lock()
		active := !old.closed
		old.mu.Unlock()
		if active {
			return old.view(), errors.New("native session already active; stop or restart explicitly")
		}
	}
	n.RunThrough = nil
	var src, helper, marker string
	if engine == "native" {
		src, helper, marker, err = nativeSource(n)
	} else {
		nonce := make([]byte, 16)
		_, err = rand.Read(nonce)
		marker = "\x1egobookInteractive" + hex.EncodeToString(nonce) + ":"
	}
	if err != nil {
		return snap, err
	}
	inputs, err := validateNotebook(n)
	if err != nil {
		return snap, err
	}
	dir, err := os.MkdirTemp("", "gobook-native-")
	if err != nil {
		return snap, err
	}
	s := newNativeSession(dir, marker)
	s.interactive = engine == "interactive"
	s.configuration = interactiveConfiguration(n)
	s.prepared = make(chan struct{})
	var preparedOnce sync.Once
	preparationDone := func() { preparedOnce.Do(func() { close(s.prepared) }) }
	s.inputs = inputs
	life, cancel := context.WithCancel(context.Background())
	s.cancel = cancel
	id := sha256.Sum256([]byte(marker))
	s.snapshot = NativeSnapshot{Notebook: copyNotebook(n), Engine: engine, SessionID: hex.EncodeToString(id[:16]), Fingerprint: nativeFingerprint(n), History: []int{}, Files: []Artifact{}}
	buildTimeout, runTimeout := m.Kernel.BuildTimeout, m.Kernel.RunTimeout
	if buildTimeout == 0 {
		buildTimeout = 60 * time.Second
	}
	if runTimeout == 0 {
		runTimeout = 30 * time.Second
	}
	s.provenance = Provenance{GoEnv: "off", GoWork: "off", GoVersionSource: "server runtime", Started: time.Now().UTC(), GoVersion: runtime.Version(), GOOS: runtime.GOOS, GOARCH: runtime.GOARCH, ModuleMode: "mod", GOMAXPROCS: 2, BuildTimeoutNS: buildTimeout, RunTimeoutNS: runTimeout, BuildArgs: []string{"build", "-p", "2", "-buildvcs=false", "-mod=mod", "-o", "notebook", "."}}
	m.mu.Lock()
	m.session = s
	m.mu.Unlock()
	defer func() {
		preparationDone()
		if err != nil {
			s.fail(err)
			snap = s.view()
		}
	}()
	for name, data := range inputs {
		if err = os.WriteFile(filepath.Join(dir, name), data, 0600); err != nil {
			return snap, err
		}
		s.artifactHashes[name] = sha256.Sum256(data)
	}
	var binary string
	if !s.interactive {
		for name, data := range map[string]string{"main.go": src, "gobook_native.go": helper, "go.mod": m.Kernel.moduleFile(n)} {
			if err = os.WriteFile(filepath.Join(dir, name), []byte(data), 0600); err != nil {
				return snap, err
			}
		}
		binary = filepath.Join(dir, "notebook")
		if runtime.GOOS == "windows" {
			binary += ".exe"
		}
		buildCtx, buildCancel := context.WithTimeout(ctx, buildTimeout)
		stopBuild := context.AfterFunc(life, buildCancel)
		started := time.Now()
		err = runProcess(buildCtx, dir, s.output, "go", "build", "-p", "2", "-buildvcs=false", "-mod=mod", "-o", binary, ".")
		s.provenance.BuildDurationNS = time.Since(started)
		stopBuild()
		buildCancel()
		buildErr := err
		if err = captureModules(dir, &s.provenance); err != nil {
			if buildErr != nil {
				err = fmt.Errorf("build: %v; %w", buildErr, err)
			}
			return snap, err
		}
		if buildErr != nil {
			err = fmt.Errorf("native build: %w", buildErr)
			return snap, err
		}
		if err = verifyExactPins(n.Dependencies, s.provenance.GoMod); err != nil {
			return snap, err
		}
		if info, e := buildinfo.ReadFile(binary); e == nil {
			s.provenance.GoVersion = info.GoVersion
			s.provenance.GoVersionSource = "notebook binary"
			for _, setting := range info.Settings {
				if setting.Key == "GOOS" {
					s.provenance.GOOS = setting.Value
				}
				if setting.Key == "GOARCH" {
					s.provenance.GOARCH = setting.Value
				}
			}
		}
	} else {
		binary, err = os.Executable()
		if err != nil {
			return snap, err
		}
		s.provenance.BuildArgs = nil
		s.provenance.GoVersionSource = "GoBook interactive worker (Yaegi v0.16.1)"
		s.provenance.ModuleMode = "bundled standard library"
	}

	// Windows cannot remove files while startup reads or writes still hold them.
	preparationDone()
	listener, err := net.Listen("tcp4", "127.0.0.1:0")
	if err != nil {
		return snap, err
	}
	defer listener.Close()
	tokenBytes := make([]byte, 32)
	if _, err = rand.Read(tokenBytes); err != nil {
		return snap, err
	}
	auth := hex.EncodeToString(tokenBytes)
	args := []string{}
	if s.interactive {
		args = []string{"-interactive-worker"}
		if len(m.WorkerCommand) > 0 {
			binary = m.WorkerCommand[0]
			args = m.WorkerCommand[1:]
		}
	}
	cmd := exec.CommandContext(life, binary, args...)
	cmd.Dir = dir
	cmd.Env = append(os.Environ(), "GOMAXPROCS=2", "GOWORK=off", "GOFLAGS=", "GOENV=off", "GOBOOK_NATIVE_ADDRESS="+listener.Addr().String(), "GOBOOK_NATIVE_TOKEN="+auth, "GOBOOK_INTERACTIVE_MARKER="+marker)
	stdout, stderr := &nativeStream{s: s}, &nativeStream{s: s, stderr: true}
	cmd.Stdout = stdout
	cmd.Stderr = stderr
	configureProcess(cmd)
	s.mu.Lock()
	if s.closed {
		s.mu.Unlock()
		return snap, errors.New("native session stopped")
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
		stdout.flush()
		stderr.flush()
		s.mu.Lock()
		if !s.closed {
			s.snapshot.Active = false
			s.snapshot.Ready = false
			s.snapshot.Error = "native worker exited"
			if waitErr != nil {
				s.snapshot.Error += ": " + waitErr.Error()
			}
			cell := 0
			if len(s.snapshot.History) > 0 {
				cell = s.snapshot.History[len(s.snapshot.History)-1]
			}
			previous := s.snapshot.Result
			s.snapshot.Result = s.resultLocked(cell, 0)
			s.snapshot.Result.Error = s.snapshot.Error
			if previous != nil {
				s.snapshot.Result.Artifacts = previous.Artifacts
				if len(s.snapshot.Result.Cells) == 1 {
					s.snapshot.Result.Cells[0].Artifacts = previous.Artifacts
				}
			}
		}
		close(s.done)
		s.mu.Unlock()
		_ = s.stop()
	}()
	startupCtx, startupCancel := context.WithTimeout(ctx, runTimeout)
	defer startupCancel()
	stopAccept := context.AfterFunc(startupCtx, func() { _ = listener.Close() })
	defer stopAccept()
	stopAcceptLife := context.AfterFunc(life, func() { _ = listener.Close() })
	defer stopAcceptLife()
	var conn net.Conn
	var reader *bufio.Reader
	for {
		conn, err = listener.Accept()
		if err != nil {
			return snap, fmt.Errorf("native control connection: %w", err)
		}
		deadline := time.Now().Add(time.Second)
		if d, ok := startupCtx.Deadline(); ok && d.Before(deadline) {
			deadline = d
		}
		_ = conn.SetReadDeadline(deadline)
		reader = bufio.NewReaderSize(conn, 16384)
		var line []byte
		line, err = reader.ReadSlice('\n')
		if err == nil && string(line) == auth+"\n" {
			break
		}
		conn.Close()
	}
	_ = conn.SetReadDeadline(time.Time{})
	s.mu.Lock()
	if s.closed {
		s.mu.Unlock()
		conn.Close()
		return snap, errors.New("native session stopped")
	}
	s.conn = conn
	s.reader = reader
	s.mu.Unlock()
	if err = s.waitReady(startupCtx, 0, 0); err != nil {
		return snap, err
	}
	s.mu.Lock()
	if s.closed {
		s.mu.Unlock()
		return snap, errors.New("native session stopped during startup")
	}
	s.snapshot.Active = true
	s.snapshot.Ready = true
	s.snapshot.NextCell = 1
	s.snapshot.Result = s.resultLocked(0, 0)
	s.mu.Unlock()
	return s.view(), nil
}

func (s *nativeSession) waitReady(ctx context.Context, cell, sequence int) error {
	s.mu.Lock()
	conn, reader, done := s.conn, s.reader, s.done
	s.mu.Unlock()
	stop := context.AfterFunc(ctx, func() { _ = conn.SetDeadline(time.Now()) })
	defer stop()
	if deadline, ok := ctx.Deadline(); ok {
		_ = conn.SetDeadline(deadline)
	}
	line, err := reader.ReadSlice('\n')
	if err != nil {
		if ctx.Err() != nil {
			return fmt.Errorf("native run: %w", ctx.Err())
		}
		return fmt.Errorf("native worker control: %w", err)
	}
	var event nativeEvent
	if len(line) > 16384 || json.Unmarshal(line, &event) != nil || event.Cell != cell || event.Sequence != sequence || event.Goroutines < 1 {
		return errors.New("invalid native worker ready message")
	}
	for {
		s.mu.Lock()
		drained := s.boundaries[0] > sequence && s.boundaries[1] > sequence
		s.mu.Unlock()
		if drained {
			break
		}
		select {
		case <-s.changed:
		case <-done:
			return errors.New("native worker exited before console drain")
		case <-ctx.Done():
			return fmt.Errorf("native console drain: %w", ctx.Err())
		}
	}
	_ = conn.SetDeadline(time.Time{})
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.closed {
		return errors.New("native session stopped")
	}
	s.snapshot.HeapAlloc = event.HeapAlloc
	s.snapshot.Goroutines = event.Goroutines
	s.snapshot.Error = event.Error
	return nil
}

func (s *nativeSession) resultLocked(cell int, duration time.Duration) *Result {
	r := &Result{Provenance: s.provenance}
	r.Provenance.RunDurationNS = duration
	r.Provenance.Finished = time.Now().UTC()
	s.output.mu.Lock()
	r.Stdout = s.output.out.String()
	r.Stderr = s.output.err.String()
	r.Truncated = s.output.truncated
	s.output.mu.Unlock()
	if cell > 0 {
		r.Cells = []CellResult{{Cell: cell, Stdout: r.Stdout, Stderr: r.Stderr}}
	}
	return r
}
func (m *NativeManager) Execute(ctx context.Context, n Notebook, cell int) (NativeSnapshot, error) {
	return m.execute(ctx, n, cell, "", -1)
}

// ExecuteExpected rejects stale browser commands atomically with acquiring the
// command slot, so another tab cannot silently change a command's input state.
func (m *NativeManager) ExecuteExpected(ctx context.Context, n Notebook, cell int, sessionID string, historyLength int) (NativeSnapshot, error) {
	if sessionID == "" || historyLength < 0 {
		return m.State(), errors.New("session id and nonnegative history length are required")
	}
	return m.execute(ctx, n, cell, sessionID, historyLength)
}
func (m *NativeManager) execute(ctx context.Context, n Notebook, cell int, sessionID string, historyLength int) (NativeSnapshot, error) {
	if !m.operation.TryLock() {
		return m.State(), errors.New("native session is busy")
	}
	defer m.operation.Unlock()
	m.mu.Lock()
	s := m.session
	m.mu.Unlock()
	if s == nil {
		return m.State(), errors.New("no native session; start or restart explicitly")
	}
	s.mu.Lock()
	var err error
	switch {
	case s.closed || !s.snapshot.Active || !s.snapshot.Ready:
		err = errors.New("native session is not ready; restart explicitly")
	case historyLength >= 0 && (sessionID != s.snapshot.SessionID || historyLength != len(s.snapshot.History)):
		err = errors.New("native session changed in another page; refresh status and restore the kernel notebook")
	case !s.interactive && nativeFingerprint(n) != s.snapshot.Fingerprint:
		err = errors.New("notebook source, imports, pins or inputs changed; restart required (live memory will be lost)")
	case s.interactive && len(n.Cells) > 1000:
		err = errors.New("interactive sessions support at most 1000 cells")
	case s.interactive && interactiveConfiguration(n) != s.configuration:
		err = errors.New("input files or dependency configuration changed; restart required")
	case cell < 1 || cell > len(n.Cells):
		err = errors.New("cell is outside notebook")
	case !s.interactive && cell > min(s.snapshot.NextCell, len(n.Cells)):
		err = fmt.Errorf("cell %d is unavailable; execute a cell between 1 and %d (later cells must run sequentially)", cell, min(s.snapshot.NextCell, len(n.Cells)))
	case len(s.snapshot.History) >= nativeHistoryLimit:
		err = errors.New("native execution history limit reached (1000 commands); restart required")
	}
	if err == nil && s.interactive {
		_, err = validateNotebook(n)
		if err == nil {
			err = supportedInteractiveImports(n.Imports)
		}
	}
	if err != nil {
		s.mu.Unlock()
		return s.view(), err
	}
	var command []byte
	if s.interactive {
		var wire bytes.Buffer
		encoder := json.NewEncoder(&wire)
		encoder.SetEscapeHTML(false)
		err = encoder.Encode(interactiveCommand{Cell: cell, Code: n.Cells[cell-1].Code, Imports: n.Imports})
		if err != nil || wire.Len() > 512<<10 {
			s.mu.Unlock()
			return s.view(), errors.New("interactive command exceeds 512 KiB encoded limit")
		}
		command = wire.Bytes()
	}
	s.snapshot.Ready = false
	s.snapshot.Error = ""
	s.output = &capture{remaining: outputLimit}
	conn := s.conn
	sequence := len(s.snapshot.History) + 1
	timeout := s.provenance.RunTimeoutNS
	s.mu.Unlock()
	runCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	stop := context.AfterFunc(runCtx, func() { _ = conn.SetDeadline(time.Now()) })
	if deadline, ok := runCtx.Deadline(); ok {
		_ = conn.SetDeadline(deadline)
	}
	if s.interactive {
		_, err = conn.Write(command)
	} else {
		_, err = fmt.Fprintf(conn, "%d\n", cell)
	}
	stop()
	started := time.Now()
	accepted := err == nil
	if err == nil {
		s.mu.Lock()
		s.snapshot.History = append(s.snapshot.History, cell)
		if s.interactive {
			s.snapshot.Fingerprint = nativeFingerprint(n)
			s.snapshot.Notebook = copyNotebook(n)
		}
		s.mu.Unlock()
		err = s.waitReady(runCtx, cell, sequence)
	}
	if err != nil {
		if runCtx.Err() != nil {
			err = fmt.Errorf("native run: %w", runCtx.Err())
		}
		s.fail(err)
		snapshot := s.view()
		snapshot.CommandAccepted = accepted
		return snapshot, err
	}
	artifacts, artifactErr := collectArtifactsExcluding(s.dir, s.inputs)
	changed := make([]Artifact, 0, len(artifacts))
	currentHashes := make(map[string][32]byte, len(artifacts))
	for _, artifact := range artifacts {
		// collectArtifactsExcluding has already bounded and rooted the file reads.
		sum := sha256.Sum256([]byte(artifact.Data))
		if previous, ok := s.artifactHashes[artifact.Name]; !ok || previous != sum {
			changed = append(changed, artifact)
		}
		currentHashes[artifact.Name] = sum
	}
	if artifactErr == nil {
		s.artifactHashes = currentHashes
	}
	s.mu.Lock()
	if artifactErr == nil {
		s.snapshot.Files = append([]Artifact{}, artifacts...)
	} else {
		s.snapshot.Files = nil
	}
	s.snapshot.CurrentCell = cell
	s.snapshot.NextCell = min(cell+1, len(n.Cells))
	if s.interactive {
		s.snapshot.NextCell = len(n.Cells)
	}
	s.snapshot.Ready = !s.closed
	s.snapshot.Result = s.resultLocked(cell, time.Since(started))
	s.snapshot.Result.Artifacts = changed
	s.snapshot.Result.Cells[0].Artifacts = changed
	if s.snapshot.Error != "" {
		s.snapshot.Result.Error = s.snapshot.Error
		err = errors.New(s.snapshot.Error)
	}
	if artifactErr != nil {
		s.snapshot.Result.Error = artifactErr.Error()
		s.snapshot.Error = artifactErr.Error()
	}
	s.mu.Unlock()
	snapshot := s.view()
	snapshot.CommandAccepted = true
	if artifactErr != nil {
		return snapshot, artifactErr
	}
	return snapshot, err
}

func (s *nativeSession) fail(err error) {
	_ = s.stop()
	s.mu.Lock()
	defer s.mu.Unlock()
	s.snapshot.Active = false
	s.snapshot.Ready = false
	s.snapshot.Error = err.Error()
	cell := 0
	if len(s.snapshot.History) > 0 {
		cell = s.snapshot.History[len(s.snapshot.History)-1]
	}
	s.snapshot.Result = s.resultLocked(cell, 0)
	s.snapshot.Result.Error = err.Error()
}
func (m *NativeManager) Stop() error {
	m.mu.Lock()
	s := m.session
	m.mu.Unlock()
	if s == nil {
		return nil
	}
	return s.stop()
}
func (s *nativeSession) stop() error {
	s.stopOnce.Do(func() {
		s.mu.Lock()
		s.closed = true
		s.snapshot.Active = false
		s.snapshot.Ready = false
		conn, cmd, done, cancel, prepared := s.conn, s.cmd, s.done, s.cancel, s.prepared
		s.mu.Unlock()
		if conn != nil {
			_ = conn.Close()
		}
		if cancel != nil {
			cancel()
		}
		if cmd != nil {
			if cmd.Cancel != nil {
				_ = cmd.Cancel()
			}
			cleanupProcess(cmd)
			<-done
		}
		if prepared != nil {
			<-prepared
		}
		s.stopErr = os.RemoveAll(s.dir)
	})
	return s.stopErr
}
