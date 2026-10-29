package main

import (
	"bytes"
	"context"
	"encoding/base64"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"time"
)

const outputLimit = 1 << 20
const artifactLimit = 8 << 20

type Cell struct {
	Code string `json:"code"`
}
type Notebook struct {
	Version int      `json:"version"`
	Title   string   `json:"title"`
	Note    string   `json:"note"`
	Imports []string `json:"imports"`
	Cells   []Cell   `json:"cells"`
}
type Artifact struct {
	Name string `json:"name"`
	MIME string `json:"mime"`
	Data string `json:"data"`
}
type Result struct {
	Stdout    string     `json:"stdout"`
	Stderr    string     `json:"stderr"`
	Error     string     `json:"error,omitempty"`
	Truncated bool       `json:"truncated"`
	Artifacts []Artifact `json:"artifacts"`
}
type Kernel struct {
	Chartframe               string
	BuildTimeout, RunTimeout time.Duration
}

var importPath = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._/-]*$`)

func source(n Notebook) (string, error) {
	if n.Version != 1 {
		return "", fmt.Errorf("unsupported notebook version %d", n.Version)
	}
	var b strings.Builder
	b.WriteString("package main\nimport (\n")
	for _, p := range n.Imports {
		if !importPath.MatchString(p) || strings.Contains(p, "..") || strings.HasSuffix(p, "/") {
			return "", fmt.Errorf("invalid import path %q", p)
		}
		b.WriteString(strconv.Quote(p) + "\n")
	}
	b.WriteString(")\nfunc main(){\n")
	for i, c := range n.Cells {
		fmt.Fprintf(&b, "//line cell%d.go:1\n%s\n", i+1, c.Code)
	}
	b.WriteString("}\n")
	return b.String(), nil
}

// Both streams share one budget and lock: subprocess copying occurs concurrently.
type capture struct {
	mu        sync.Mutex
	out, err  bytes.Buffer
	remaining int
	truncated bool
}
type stream struct {
	c      *capture
	stderr bool
}

func (w stream) Write(p []byte) (int, error) {
	w.c.mu.Lock()
	defer w.c.mu.Unlock()
	n := len(p)
	if len(p) > w.c.remaining {
		p = p[:w.c.remaining]
		w.c.truncated = true
	}
	w.c.remaining -= len(p)
	if w.stderr {
		w.c.err.Write(p)
	} else {
		w.c.out.Write(p)
	}
	return n, nil
}
func (k Kernel) Run(ctx context.Context, n Notebook) Result {
	var r Result
	src, err := source(n)
	if err != nil {
		r.Error = err.Error()
		return r
	}
	dir, err := os.MkdirTemp("", "gobook-")
	if err != nil {
		r.Error = err.Error()
		return r
	}
	defer os.RemoveAll(dir)
	mod := "module gobook.local/notebook\n\ngo 1.25.0\n"
	chartRequired, gonumRequired := false, false
	for _, p := range n.Imports {
		if p == "github.com/Alifarid0011/chartframe" {
			chartRequired = true
		}
		if strings.HasPrefix(p, "gonum.org/v1/gonum/") {
			gonumRequired = true
		}
	}
	if chartRequired {
		mod += "require github.com/Alifarid0011/chartframe v0.1.0\n"
		if k.Chartframe != "" {
			mod += "replace github.com/Alifarid0011/chartframe => " + strconv.Quote(k.Chartframe) + "\n"
		}
	}
	if gonumRequired {
		mod += "require gonum.org/v1/gonum v0.17.0\n"
	}

	for name, data := range map[string]string{"go.mod": mod, "main.go": src} {
		if err = os.WriteFile(filepath.Join(dir, name), []byte(data), 0600); err != nil {
			r.Error = err.Error()
			return r
		}
	}
	c := &capture{remaining: outputLimit}
	buildTimeout := k.BuildTimeout
	if buildTimeout == 0 {
		buildTimeout = 60 * time.Second
	}
	runTimeout := k.RunTimeout
	if runTimeout == 0 {
		runTimeout = 30 * time.Second
	}
	binary := filepath.Join(dir, "notebook")
	if runtime.GOOS == "windows" {
		binary += ".exe"
	}
	buildCtx, cancel := context.WithTimeout(ctx, buildTimeout)
	err = runProcess(buildCtx, dir, c, "go", "build", "-p", "2", "-buildvcs=false", "-mod=mod", "-o", binary, ".")
	cancel()
	if err != nil {
		r.Error = fmt.Sprintf("build: %v", err)
	} else {
		runCtx, stop := context.WithTimeout(ctx, runTimeout)
		err = runProcess(runCtx, dir, c, binary)
		stop()
		if err != nil {
			r.Error = fmt.Sprintf("run: %v", err)
		}
	}
	r.Stdout = c.out.String()
	r.Stderr = c.err.String()
	r.Truncated = c.truncated
	r.Artifacts, err = collectArtifacts(dir)
	if err != nil {
		if r.Error != "" {
			r.Error += "; "
		}
		r.Error += err.Error()
	}
	return r
}
func runProcess(ctx context.Context, dir string, c *capture, name string, args ...string) error {
	cmd := exec.CommandContext(ctx, name, args...)
	cmd.Dir = dir
	// Keep the host module/build cache configuration; only constrain Go parallelism.
	cmd.Env = append(os.Environ(), "GOMAXPROCS=2", "GOWORK=off")
	cmd.Stdout = stream{c: c}
	cmd.Stderr = stream{c: c, stderr: true}
	configureProcess(cmd)
	err := cmd.Run()
	cleanupProcess(cmd)
	if ctx.Err() != nil {
		return fmt.Errorf("%w: %v", ctx.Err(), err)
	}
	return err
}
func collectArtifacts(dir string) ([]Artifact, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, err
	}
	var artifacts []Artifact
	total := 0
	for _, entry := range entries {
		ext := strings.ToLower(filepath.Ext(entry.Name()))
		mime := ""
		if ext == ".svg" {
			mime = "image/svg+xml"
		}
		if ext == ".png" {
			mime = "image/png"
		}
		if mime == "" {
			continue
		}
		info, err := entry.Info()
		if err != nil {
			return artifacts, err
		}
		if !info.Mode().IsRegular() {
			continue
		}
		if len(artifacts) >= 16 || info.Size() > int64(artifactLimit-total) {
			return artifacts, fmt.Errorf("artifact limit exceeded (16 images / 8 MiB)")
		}
		// OpenRoot prevents a symlink swap from escaping the run directory.
		root, err := os.OpenRoot(dir)
		if err != nil {
			return artifacts, err
		}
		f, err := openArtifact(root, entry.Name())
		root.Close()
		if err != nil {
			return artifacts, err
		}
		openedInfo, statErr := f.Stat()
		if statErr != nil {
			f.Close()
			return artifacts, statErr
		}
		if !openedInfo.Mode().IsRegular() {
			f.Close()
			continue
		}
		data, err := io.ReadAll(io.LimitReader(f, int64(artifactLimit-total)+1))
		f.Close()
		if err != nil {
			return artifacts, err
		}
		if len(data) > artifactLimit-total {
			return artifacts, fmt.Errorf("artifact limit exceeded")
		}
		total += len(data)
		artifacts = append(artifacts, Artifact{entry.Name(), mime, base64.StdEncoding.EncodeToString(data)})
	}
	return artifacts, nil
}
