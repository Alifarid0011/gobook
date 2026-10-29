package main

import (
	"bufio"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"os"
	"runtime"
	"strconv"
	"strings"

	"github.com/traefik/yaegi/interp"
	"github.com/traefik/yaegi/stdlib"
)

// The interpreter lives only in a disposable child process. Notebook code has
// the user's permissions; process isolation protects host lifetime, not files.
type interactiveEvaluator struct {
	i       *interp.Interpreter
	imports map[string]bool
}

func newInteractiveEvaluator(stdout, stderr io.Writer) *interactiveEvaluator {
	i := interp.New(interp.Options{Stdout: stdout, Stderr: stderr, Stdin: strings.NewReader("")})
	if err := i.Use(stdlib.Symbols); err != nil {
		panic(err)
	}
	return &interactiveEvaluator{i: i, imports: make(map[string]bool)}
}
func supportedInteractiveImports(imports []string) error {
	for _, p := range imports {
		if _, ok := stdlib.Symbols[p+"/"+p[strings.LastIndex(p, "/")+1:]]; !ok {
			return fmt.Errorf("interactive kernel does not bundle import %q; use the native kernel for third-party packages or unsupported standard packages", p)
		}
	}
	return nil
}
func (e *interactiveEvaluator) eval(imports []string, code string) (err error) {
	if err = supportedInteractiveImports(imports); err != nil {
		return err
	}
	defer func() {
		if v := recover(); v != nil {
			err = fmt.Errorf("interactive panic: %v", v)
		}
	}()
	for _, p := range imports {
		if e.imports[p] {
			continue
		}
		if _, err = e.i.Eval("import " + strconv.Quote(p)); err != nil {
			return err
		}
		e.imports[p] = true
	}
	_, err = e.i.Eval(code)
	return err
}

type interactiveCommand struct {
	Cell    int      `json:"cell"`
	Code    string   `json:"code"`
	Imports []string `json:"imports"`
}

func interactiveWorker() error {
	address, token, marker := os.Getenv("GOBOOK_NATIVE_ADDRESS"), os.Getenv("GOBOOK_NATIVE_TOKEN"), os.Getenv("GOBOOK_INTERACTIVE_MARKER")
	os.Unsetenv("GOBOOK_NATIVE_ADDRESS")
	os.Unsetenv("GOBOOK_NATIVE_TOKEN")
	os.Unsetenv("GOBOOK_INTERACTIVE_MARKER")
	if address == "" || token == "" || marker == "" {
		return fmt.Errorf("interactive worker requires private parent control")
	}
	conn, err := net.Dial("tcp4", address)
	if err != nil {
		return err
	}
	defer conn.Close()
	if _, err = fmt.Fprintln(conn, token); err != nil {
		return err
	}
	e := newInteractiveEvaluator(os.Stdout, os.Stderr)
	reader := bufio.NewReaderSize(conn, 512<<10)
	encoder := json.NewEncoder(conn)
	cell, seq, message := 0, 0, ""
	for {
		fmt.Fprintf(os.Stdout, "%s%d\x1f", marker, seq)
		fmt.Fprintf(os.Stderr, "%s%d\x1f", marker, seq)
		var stats runtime.MemStats
		runtime.ReadMemStats(&stats)
		if err = encoder.Encode(nativeEvent{Cell: cell, Sequence: seq, HeapAlloc: stats.HeapAlloc, Goroutines: runtime.NumGoroutine(), Error: message}); err != nil {
			return err
		}
		line, readErr := reader.ReadSlice('\n')
		if readErr != nil {
			return readErr
		}
		var command interactiveCommand
		if len(line) > 512<<10 || json.Unmarshal(line, &command) != nil || command.Cell < 1 || command.Cell > 1000 {
			return fmt.Errorf("invalid interactive command")
		}
		cell = command.Cell
		seq++
		message = ""
		if err = e.eval(command.Imports, command.Code); err != nil {
			message = err.Error()
			if len(message) > 2048 {
				message = message[:2048]
			}
		}
	}
}

// Code and imports may change in a live session. Replacing input files or module
// configuration is explicit because it can invalidate resources already in use.
func interactiveConfiguration(n Notebook) string {
	n.Title = ""
	n.Note = ""
	n.RunThrough = nil
	n.Cells = nil
	n.Imports = nil
	return nativeFingerprint(n)
}

func copyNotebook(n Notebook) *Notebook {
	data, _ := json.Marshal(n)
	var copied Notebook
	_ = json.Unmarshal(data, &copied)
	return &copied
}
