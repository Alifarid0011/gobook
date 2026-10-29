package main

import (
	"bufio"
	"context"
	"crypto/subtle"
	"encoding/json"
	"errors"
	"fmt"
	"go/scanner"
	"go/token"
	"io"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"
	"unicode"
	"unicode/utf16"
	"unicode/utf8"

	"github.com/traefik/yaegi/stdlib"
)

type CompletionItem struct {
	Label      string `json:"label"`
	InsertText string `json:"insertText"`
	Detail     string `json:"detail"`
	Start      int    `json:"start"`
	End        int    `json:"end"`
}

func utf16Prefix(text string, offset int) (string, error) {
	units := 0
	for index, r := range text {
		if units == offset {
			return text[:index], nil
		}
		units++
		if r > 0xffff {
			units++
		}
		if units > offset {
			return "", errors.New("cursor splits a Unicode character")
		}
	}
	if units == offset {
		return text, nil
	}
	return "", errors.New("cursor outside cell")
}
func completionSource(n Notebook, cell, offset int) (string, int, int, error) {
	if n.Version != 1 || cell < 1 || cell > len(n.Cells) {
		return "", 0, 0, errors.New("invalid notebook cell")
	}
	current := n.Cells[cell-1].Code
	prefix, err := utf16Prefix(current, offset)
	if err != nil {
		return "", 0, 0, err
	}
	var b strings.Builder
	b.WriteString("package main\nimport (\n")
	for _, p := range n.Imports {
		if !importPath.MatchString(p) || strings.Contains(p, "..") {
			return "", 0, 0, errors.New("invalid import")
		}
		b.WriteString(strconv.Quote(p) + "\n")
	}
	b.WriteString(")\nfunc main(){\n")
	for _, c := range n.Cells[:cell-1] {
		b.WriteString(c.Code + "\n")
	}
	before := b.String() + prefix
	line := strings.Count(before, "\n")
	last := before[strings.LastIndex(before, "\n")+1:]
	character := len(utf16.Encode([]rune(last)))
	b.WriteString(current)
	b.WriteString("\n}\n")
	return b.String(), line, character, nil
}
func completionPrefix(code string, offset int) (string, string, error) {
	text, err := utf16Prefix(code, offset)
	if err != nil {
		return "", "", err
	}
	end := len(text)
	start := end
	for start > 0 {
		r, size := utf8LastRune(text[:start])
		if !unicode.IsLetter(r) && !unicode.IsDigit(r) && r != '_' {
			break
		}
		start -= size
	}
	prefix := text[start:]
	qualifier := ""
	if start > 0 && text[start-1] == '.' {
		end = start - 1
		start = end
		for start > 0 {
			r, size := utf8LastRune(text[:start])
			if !unicode.IsLetter(r) && !unicode.IsDigit(r) && r != '_' {
				break
			}
			start -= size
		}
		qualifier = text[start:end]
	}
	return prefix, qualifier, nil
}
func utf8LastRune(s string) (rune, int) { return utf8.DecodeLastRuneInString(s) }
func localCompletions(n Notebook, cell, offset int) []CompletionItem {
	if cell < 1 || cell > len(n.Cells) {
		return nil
	}
	prefix, qualifier, err := completionPrefix(n.Cells[cell-1].Code, offset)
	if err != nil {
		return nil
	}
	found := map[string]CompletionItem{}
	if qualifier != "" {
		for _, p := range n.Imports {
			for path, symbols := range stdlib.Symbols {
				if strings.HasPrefix(path, p+"/") && path[strings.LastIndex(path, "/")+1:] == qualifier {
					for name, value := range symbols {
						if strings.HasPrefix(name, prefix) && token.IsExported(name) {
							detail := "standard library"
							if value.IsValid() {
								detail = value.Type().String()
							}
							found[name] = CompletionItem{Label: name, InsertText: name, Detail: detail}
						}
					}
				}
			}
		}
	} else {
		for _, word := range strings.Fields("append cap clear close copy delete len make new panic recover min max println func for range if else return go defer select switch case var const type struct map chan interface error string int float64 bool true false nil") {
			if strings.HasPrefix(word, prefix) {
				found[word] = CompletionItem{Label: word, InsertText: word, Detail: "Go keyword or builtin"}
			}
		}
		for _, c := range n.Cells {
			fset := token.NewFileSet()
			file := fset.AddFile("cell.go", -1, len(c.Code))
			var scan scanner.Scanner
			scan.Init(file, []byte(c.Code), func(token.Position, string) {}, 0)
			for {
				_, tok, literal := scan.Scan()
				if tok == token.EOF {
					break
				}
				if tok == token.IDENT && strings.HasPrefix(literal, prefix) && literal != prefix {
					found[literal] = CompletionItem{Label: literal, InsertText: literal, Detail: "Notebook identifier · lexical suggestion"}
				}
			}
		}
	}
	names := make([]string, 0, len(found))
	for name := range found {
		names = append(names, name)
	}
	sort.Strings(names)
	items := make([]CompletionItem, 0, min(40, len(names)))
	for _, name := range names {
		item := found[name]
		item.Start, item.End = identifierRange(n.Cells[cell-1].Code, offset)
		items = append(items, item)
		if len(items) == 40 {
			break
		}
	}
	return items
}

type lspReply struct {
	ID     int             `json:"id"`
	Method string          `json:"method"`
	Params json.RawMessage `json:"params"`
	Result json.RawMessage `json:"result"`
	Error  *struct {
		Message string `json:"message"`
	} `json:"error"`
}
type completionProcess struct {
	cmd          *exec.Cmd
	in           io.WriteCloser
	replies      chan lspReply
	done         chan struct{}
	writeMu      sync.Mutex
	diagnosticMu sync.Mutex
	diagnostic   string
	stderr       *capture
}

func (p *completionProcess) bindContext(ctx context.Context) func() bool {
	return context.AfterFunc(ctx, func() {
		if p.cmd.Cancel != nil {
			_ = p.cmd.Cancel()
		}
		_ = p.in.Close()
	})
}
func (p *completionProcess) send(value any) error {
	data, err := json.Marshal(value)
	if err != nil {
		return err
	}
	p.writeMu.Lock()
	defer p.writeMu.Unlock()
	_, err = fmt.Fprintf(p.in, "Content-Length: %d\r\n\r\n%s", len(data), data)
	return err
}
func readLSP(reader *bufio.Reader) (lspReply, error) {
	size := -1
	for {
		line, err := reader.ReadString('\n')
		if err != nil {
			return lspReply{}, err
		}
		if len(line) > 1024 {
			return lspReply{}, errors.New("LSP header too long")
		}
		line = strings.TrimSpace(line)
		if line == "" {
			break
		}
		if strings.HasPrefix(strings.ToLower(line), "content-length:") {
			size, err = strconv.Atoi(strings.TrimSpace(strings.SplitN(line, ":", 2)[1]))
			if err != nil {
				return lspReply{}, err
			}
		}
	}
	if size < 0 || size > 4<<20 {
		return lspReply{}, errors.New("invalid LSP message size")
	}
	data := make([]byte, size)
	if _, err := io.ReadFull(reader, data); err != nil {
		return lspReply{}, err
	}
	var reply lspReply
	err := json.Unmarshal(data, &reply)
	return reply, err
}
func (p *completionProcess) stop() {
	if p.cmd.Cancel != nil {
		_ = p.cmd.Cancel()
	}
	_ = p.in.Close()
	<-p.done
}
func (p *completionProcess) rpc(ctx context.Context, id int, method string, params any) (json.RawMessage, error) {
	if err := p.send(map[string]any{"jsonrpc": "2.0", "id": id, "method": method, "params": params}); err != nil {
		return nil, err
	}
	for {
		select {
		case reply := <-p.replies:
			if reply.ID == id {
				if reply.Error != nil {
					return nil, p.protocolError(reply.Error.Message)
				}
				return reply.Result, nil
			}
		case <-p.done:
			return nil, errors.New("Go language server exited")
		case <-ctx.Done():
			return nil, ctx.Err()
		}
	}
}

// One language-server workspace is reused per local host; requests are serialized
// and do not execute notebook code. Native execution remains independent of it.
type CompletionManager struct {
	Path        string
	Kernel      Kernel
	mu          sync.Mutex
	p           *completionProcess
	dir         string
	version, id int
	module      string
}

func (c *CompletionManager) Close() { c.mu.Lock(); defer c.mu.Unlock(); c.closeLocked() }
func (c *CompletionManager) closeLocked() {
	if c.p != nil {
		c.p.stop()
		c.p = nil
	}
	if c.dir != "" {
		_ = os.RemoveAll(c.dir)
		c.dir = ""
	}
	c.module = ""
	c.version = 0
}
func (c *CompletionManager) start(ctx context.Context, module string) error {
	path := c.Path
	if path == "" {
		var err error
		path, err = exec.LookPath("gopls")
		if err != nil {
			exe, _ := os.Executable()
			path = filepath.Join(filepath.Dir(exe), "gopls")
			if runtime.GOOS == "windows" {
				path += ".exe"
			}
			if _, err = os.Stat(path); err != nil {
				return errors.New("gopls is not installed; showing local suggestions")
			}
		}
	}
	var err error
	c.dir, err = os.MkdirTemp("", "gobook-completion-")
	if err != nil {
		return err
	}
	if err = os.WriteFile(filepath.Join(c.dir, "go.mod"), []byte(module), 0600); err != nil {
		c.closeLocked()
		return err
	}
	cmd := exec.CommandContext(context.Background(), path, "serve")
	cmd.Dir = c.dir
	cmd.Env = append(os.Environ(), "GOMAXPROCS=2", "GOMEMLIMIT=256MiB", "GOWORK=off", "GOENV=off", "GOFLAGS=", "GOPROXY=off", "GOSUMDB=off")
	in, err := cmd.StdinPipe()
	if err != nil {
		return err
	}
	out, err := cmd.StdoutPipe()
	if err != nil {
		in.Close()
		return err
	}
	diagnostics := &capture{remaining: 8192}
	cmd.Stderr = stream{c: diagnostics, stderr: true}
	configureProcess(cmd)
	if err = cmd.Start(); err != nil {
		in.Close()
		out.Close()
		c.closeLocked()
		return err
	}
	p := &completionProcess{cmd: cmd, in: in, replies: make(chan lspReply, 8), done: make(chan struct{}), stderr: diagnostics}
	c.p = p
	c.module = module
	stopWrite := p.bindContext(ctx)
	defer stopWrite()
	go func() {
		reader := bufio.NewReader(out)
		for {
			reply, err := readLSP(reader)
			if err != nil {
				break
			}
			if reply.Method == "window/showMessage" || reply.Method == "window/logMessage" {
				var message struct {
					Type    int    `json:"type"`
					Message string `json:"message"`
				}
				if json.Unmarshal(reply.Params, &message) == nil && message.Type <= 2 {
					p.diagnosticMu.Lock()
					p.diagnostic += message.Message + "\n"
					if len(p.diagnostic) > 8192 {
						p.diagnostic = p.diagnostic[len(p.diagnostic)-8192:]
					}
					p.diagnosticMu.Unlock()
				}
			}
			if reply.Method != "" && reply.ID != 0 {
				_ = p.send(map[string]any{"jsonrpc": "2.0", "id": reply.ID, "result": nil})
				continue
			}
			if reply.ID != 0 {
				select {
				case p.replies <- reply:
				case <-p.done:
					return
				}
			}
		}
	}()
	go func() { _ = cmd.Wait(); cleanupProcess(cmd); close(p.done) }()
	c.id++
	_, err = p.rpc(ctx, c.id, "initialize", map[string]any{"processId": os.Getpid(), "rootUri": fileURI(c.dir), "capabilities": map[string]any{"textDocument": map[string]any{"completion": map[string]any{"completionItem": map[string]any{"snippetSupport": false}}}}, "initializationOptions": map[string]any{"completeUnimported": false, "usePlaceholders": false, "completionBudget": "100ms", "diagnosticsTrigger": "Save", "analyses": map[string]bool{"unusedparams": false}}})
	if err != nil {
		c.closeLocked()
		return err
	}
	return p.send(map[string]any{"jsonrpc": "2.0", "method": "initialized", "params": map[string]any{}})
}
func fileURI(path string) string {
	return (&url.URL{Scheme: "file", Path: func() string {
		s := filepath.ToSlash(path)
		if !strings.HasPrefix(s, "/") {
			s = "/" + s
		}
		return s
	}()}).String()
}
func (c *CompletionManager) Complete(ctx context.Context, n Notebook, cell, offset int) ([]CompletionItem, error) {
	if !c.mu.TryLock() {
		return nil, errors.New("language server is preparing another suggestion")
	}
	defer c.mu.Unlock()
	source, line, char, err := completionSource(n, cell, offset)
	if err != nil {
		return nil, err
	}
	ctx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	module := c.Kernel.moduleFile(n)
	if c.p != nil && c.module != module {
		c.closeLocked()
	}
	if c.p == nil {
		if err = c.start(ctx, module); err != nil {
			return nil, err
		}
	}
	stopWrite := c.p.bindContext(ctx)
	defer stopWrite()
	uri := fileURI(filepath.Join(c.dir, "notebook.go"))
	c.version++
	if c.version == 1 {
		err = c.p.send(map[string]any{"jsonrpc": "2.0", "method": "textDocument/didOpen", "params": map[string]any{"textDocument": map[string]any{"uri": uri, "languageId": "go", "version": c.version, "text": source}}})
	} else {
		err = c.p.send(map[string]any{"jsonrpc": "2.0", "method": "textDocument/didChange", "params": map[string]any{"textDocument": map[string]any{"uri": uri, "version": c.version}, "contentChanges": []any{map[string]any{"text": source}}}})
	}
	if err != nil {
		c.closeLocked()
		return nil, err
	}
	c.id++
	raw, err := c.p.rpc(ctx, c.id, "textDocument/completion", map[string]any{"textDocument": map[string]any{"uri": uri}, "position": map[string]int{"line": line, "character": char}})
	if err != nil {
		c.closeLocked()
		return nil, err
	}
	var result struct {
		Items []struct {
			Label      string `json:"label"`
			Detail     string `json:"detail"`
			Start      int    `json:"start"`
			End        int    `json:"end"`
			InsertText string `json:"insertText"`
			TextEdit   struct {
				NewText string           `json:"newText"`
				Range   *completionRange `json:"range"`
				Replace *completionRange `json:"replace"`
			} `json:"textEdit"`
			Additional []any `json:"additionalTextEdits"`
		} `json:"items"`
	}
	if err = json.Unmarshal(raw, &result); err != nil {
		return nil, err
	}
	items := make([]CompletionItem, 0, min(40, len(result.Items)))
	for _, item := range result.Items {
		if len(item.Additional) > 0 {
			continue
		}
		text := item.TextEdit.NewText
		if text == "" {
			text = item.InsertText
		}
		if text == "" {
			text = item.Label
		}
		start, end := identifierRange(n.Cells[cell-1].Code, offset)
		editRange := item.TextEdit.Range
		if item.TextEdit.Replace != nil {
			editRange = item.TextEdit.Replace
		}
		if editRange != nil {
			prefix, _ := utf16Prefix(n.Cells[cell-1].Code, offset)
			baseLine := line - strings.Count(prefix, "\n")
			var valid bool
			start, end, valid = mapCompletionRange(n.Cells[cell-1].Code, *editRange, baseLine, offset)
			if !valid {
				continue
			}
		}
		items = append(items, CompletionItem{Label: item.Label, InsertText: text, Detail: item.Detail, Start: start, End: end})
		if len(items) == 40 {
			break
		}
	}
	return items, nil
}
func (s *Server) completionHTTP(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		w.Header().Set("Allow", "POST")
		http.Error(w, "POST required", 405)
		return
	}
	if subtle.ConstantTimeCompare([]byte(r.Header.Get("Authorization")), []byte("Bearer "+s.Token)) != 1 {
		http.Error(w, "unauthorized", 401)
		return
	}
	r.Body = http.MaxBytesReader(w, r.Body, 256<<10)
	defer r.Body.Close()
	var request struct {
		Notebook Notebook `json:"notebook"`
		Cell     int      `json:"cell"`
		Offset   int      `json:"offset"`
	}
	d := json.NewDecoder(r.Body)
	d.DisallowUnknownFields()
	if err := d.Decode(&request); err != nil {
		http.Error(w, "invalid completion request", 400)
		return
	}
	var extra any
	if err := d.Decode(&extra); err != io.EOF {
		http.Error(w, "expected one completion request", 400)
		return
	}
	if _, _, _, err := completionSource(request.Notebook, request.Cell, request.Offset); err != nil {
		http.Error(w, err.Error(), 400)
		return
	}
	items := localCompletions(request.Notebook, request.Cell, request.Offset)
	provider, message := "local", "Local symbols and bundled standard-library signatures"
	if s.Completion != nil {
		if semantic, err := s.Completion.Complete(r.Context(), request.Notebook, request.Cell, request.Offset); err == nil {
			items = semantic
			provider = "gopls"
			message = "Go language server · source-based completion"
		} else {
			message = err.Error()
		}
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]any{"items": items, "provider": provider, "message": message})
}

type completionPosition struct {
	Line      int `json:"line"`
	Character int `json:"character"`
}
type completionRange struct {
	Start completionPosition `json:"start"`
	End   completionPosition `json:"end"`
}

func identifierRange(code string, offset int) (int, int) {
	prefix, _, err := completionPrefix(code, offset)
	if err != nil {
		return offset, offset
	}
	start := offset - len(utf16.Encode([]rune(prefix)))
	before, _ := utf16Prefix(code, offset)
	end := offset
	for _, r := range code[len(before):] {
		if !unicode.IsLetter(r) && !unicode.IsDigit(r) && r != '_' {
			break
		}
		end += len(utf16.Encode([]rune{r}))
	}
	return start, end
}
func mapCompletionRange(code string, edit completionRange, baseLine, cursor int) (int, int, bool) {
	lines := strings.Split(code, "\n")
	convert := func(pos completionPosition) (int, bool) {
		line := pos.Line - baseLine
		if line < 0 || line >= len(lines) || pos.Character < 0 {
			return 0, false
		}
		if _, err := utf16Prefix(lines[line], pos.Character); err != nil {
			return 0, false
		}
		offset := pos.Character
		for _, text := range lines[:line] {
			offset += len(utf16.Encode([]rune(text))) + 1
		}
		return offset, true
	}
	start, ok := convert(edit.Start)
	if !ok {
		return 0, 0, false
	}
	end, ok := convert(edit.End)
	return start, end, ok && start <= cursor && end >= cursor && start <= end
}

func (p *completionProcess) protocolError(message string) error {
	p.diagnosticMu.Lock()
	diagnostic := p.diagnostic
	p.diagnosticMu.Unlock()
	if p.stderr != nil {
		p.stderr.mu.Lock()
		diagnostic += p.stderr.err.String()
		p.stderr.mu.Unlock()
	}
	if diagnostic != "" {
		return fmt.Errorf("%s: %s", message, diagnostic)
	}
	return errors.New(message)
}
