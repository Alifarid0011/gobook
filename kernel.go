package main

import (
	"bytes"
	"context"
	"debug/buildinfo"
	"encoding/base64"
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"runtime"
	"sort"
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
	Version      int               `json:"version"`
	Title        string            `json:"title"`
	Note         string            `json:"note"`
	Imports      []string          `json:"imports"`
	Cells        []Cell            `json:"cells"`
	Dependencies map[string]string `json:"dependencies,omitempty"`
	Files        []InputFile       `json:"files,omitempty"`
	RunThrough   *int              `json:"runThrough,omitempty"`
}
type InputFile struct {
	Name string `json:"name"`
	Data string `json:"data"`
}

type Provenance struct {
	RunThrough      *int          `json:"runThrough,omitempty"`
	GoFlags         string        `json:"goflags"`
	GoEnv           string        `json:"goenv"`
	GoWork          string        `json:"gowork"`
	GoVersionSource string        `json:"goVersionSource"`
	Started         time.Time     `json:"started"`
	Finished        time.Time     `json:"finished"`
	GoVersion       string        `json:"goVersion"`
	GOOS            string        `json:"goos"`
	GOARCH          string        `json:"goarch"`
	GoMod           string        `json:"goMod"`
	GoSum           string        `json:"goSum"`
	BuildDurationNS time.Duration `json:"buildDurationNS"`
	RunDurationNS   time.Duration `json:"runDurationNS"`
	BuildTimeoutNS  time.Duration `json:"buildTimeoutNS"`
	RunTimeoutNS    time.Duration `json:"runTimeoutNS"`
	BuildArgs       []string      `json:"buildArgs"`
	ModuleMode      string        `json:"moduleMode"`
	GOMAXPROCS      int           `json:"gomaxprocs"`
}

type Artifact struct {
	Name string `json:"name"`
	MIME string `json:"mime"`
	Data string `json:"data"`
}
type Result struct {
	Stdout     string     `json:"stdout"`
	Stderr     string     `json:"stderr"`
	Error      string     `json:"error,omitempty"`
	Truncated  bool       `json:"truncated"`
	Artifacts  []Artifact `json:"artifacts"`
	Provenance Provenance `json:"provenance"`
}
type Kernel struct {
	Chartframe               string
	BuildTimeout, RunTimeout time.Duration
}

var importPath = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._/-]*$`)

// Pins use canonical Go semantic versions, including prereleases and +incompatible.
var moduleVersion = regexp.MustCompile(`^v(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)(-[0-9A-Za-z-]+(\.[0-9A-Za-z-]+)*)?(\+incompatible)?$`)
var reservedInputName = regexp.MustCompile(`^(COM|LPT)[1-9]$`)
var inputName = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9_-]{0,58}\.(json|csv)$`)

func validateNotebook(n Notebook) (map[string][]byte, error) {
	if n.RunThrough != nil && (*n.RunThrough < 1 || *n.RunThrough > len(n.Cells)) {
		return nil, fmt.Errorf("runThrough must be between 1 and %d", len(n.Cells))
	}
	if len(n.Dependencies) > 64 {
		return nil, fmt.Errorf("dependency limit exceeded (64 modules)")
	}
	for path, version := range n.Dependencies {
		if err := validatePin(path, version); err != nil {
			return nil, err
		}
	}
	if len(n.Files) > 8 {
		return nil, fmt.Errorf("input limit exceeded (8 files / 128 KiB)")
	}
	files := make(map[string][]byte, len(n.Files))
	seen := map[string]bool{}
	total := 0
	for _, f := range n.Files {
		key := strings.ToLower(f.Name)
		stem := strings.ToUpper(strings.SplitN(f.Name, ".", 2)[0])
		reserved := stem == "CON" || stem == "PRN" || stem == "AUX" || stem == "NUL" || reservedInputName.MatchString(stem)
		if !inputName.MatchString(f.Name) || reserved || seen[key] {
			return nil, fmt.Errorf("invalid or duplicate input filename %q", f.Name)
		}
		seen[key] = true
		if len(f.Data) > base64.StdEncoding.EncodedLen(128*1024-total) {
			return nil, fmt.Errorf("input limit exceeded (128 KiB)")
		}
		data, err := base64.StdEncoding.Strict().DecodeString(f.Data)
		if err != nil {
			return nil, fmt.Errorf("input %q: invalid base64", f.Name)
		}
		total += len(data)
		if total > 128*1024 {
			return nil, fmt.Errorf("input limit exceeded (128 KiB)")
		}
		files[f.Name] = data
	}
	return files, nil
}

func validatePin(path, version string) error {
	invalid := func() error { return fmt.Errorf("invalid exact dependency pin %q %q", path, version) }
	if len(path) > 256 || len(version) > 128 || !importPath.MatchString(path) || strings.Contains(path, "..") || strings.Contains(path, "//") || path == "gobook.local/notebook" {
		return invalid()
	}
	parts := strings.Split(path, "/")
	if !strings.Contains(parts[0], ".") || strings.HasPrefix(parts[0], ".") || strings.HasSuffix(parts[0], ".") {
		return invalid()
	}
	for _, part := range parts {
		if part == "" || strings.HasPrefix(part, ".") || strings.HasSuffix(part, ".") || strings.HasPrefix(part, "-") || strings.HasSuffix(part, "~") {
			return invalid()
		}
	}
	v := moduleVersion.FindStringSubmatch(version)
	if v == nil {
		return invalid()
	}
	if v[4] != "" {
		for _, part := range strings.Split(v[4][1:], ".") {
			if len(part) > 1 && part[0] == '0' && strings.Trim(part, "0123456789") == "" {
				return invalid()
			}
		}
	}
	major := v[1]
	last := parts[len(parts)-1]
	suffix := ""
	if strings.HasPrefix(last, "v") && strings.TrimPrefix(last, "v") != "" && strings.Trim(strings.TrimPrefix(last, "v"), "0123456789") == "" {
		suffix = last[1:]
		if suffix == "0" || suffix == "1" || suffix[0] == '0' {
			return invalid()
		}
	}
	if parts[0] == "gopkg.in" {
		dot := strings.LastIndex(last, ".v")
		if dot < 0 {
			return invalid()
		}
		suffix = last[dot+2:]
	}
	if suffix != "" {
		if suffix != major || v[6] != "" {
			return invalid()
		}
	} else if major != "0" && major != "1" && v[6] == "" {
		return invalid()
	}
	if v[6] != "" && (major == "0" || major == "1") {
		return invalid()
	}
	return nil
}

func (k Kernel) moduleFile(n Notebook) string {
	deps := map[string]string{}
	for _, p := range n.Imports {
		if p == "github.com/Alifarid0011/chartframe" || strings.HasPrefix(p, "github.com/Alifarid0011/chartframe/") {
			deps["github.com/Alifarid0011/chartframe"] = "v0.1.0"
		}
		if p == "gonum.org/v1/gonum" || strings.HasPrefix(p, "gonum.org/v1/gonum/") {
			deps["gonum.org/v1/gonum"] = "v0.17.0"
		}
	}
	for p, v := range n.Dependencies {
		deps[p] = v
	}
	keys := make([]string, 0, len(deps))
	for p := range deps {
		keys = append(keys, p)
	}
	sort.Strings(keys)
	mod := "module gobook.local/notebook\n\ngo 1.25.0\n"
	for _, p := range keys {
		mod += "require " + p + " " + deps[p] + "\n"
	}
	// An explicit chartframe pin always selects the published module.
	if _, explicit := n.Dependencies["github.com/Alifarid0011/chartframe"]; !explicit && deps["github.com/Alifarid0011/chartframe"] != "" && k.Chartframe != "" {
		mod += "replace github.com/Alifarid0011/chartframe => " + strconv.Quote(k.Chartframe) + "\n"
	}
	return mod
}

// A partial run needs a statement boundary at every cell. Shared lexical
// blocks and goto can otherwise jump around the injected return. Full runs
// retain the original concatenation semantics.
func validatePartialCells(cells []Cell) error {
	for i, cell := range cells {
		name := fmt.Sprintf("cell%d.go", i+1)
		wrapped := "package main\nfunc main(){\n//line " + name + ":1\n" + cell.Code + "\n}\n"
		parsed, err := parser.ParseFile(token.NewFileSet(), name, wrapped, 0)
		if err != nil {
			return fmt.Errorf("Run to here requires complete statements in each cell: %w; use Run all for cross-cell statements", err)
		}
		if len(parsed.Decls) != 1 {
			return fmt.Errorf("Run to here requires complete statements in each cell (%s); use Run all for cross-cell statements", name)
		}
		hasGoto := false
		ast.Inspect(parsed, func(node ast.Node) bool {
			if branch, ok := node.(*ast.BranchStmt); ok && branch.Tok == token.GOTO {
				hasGoto = true
			}
			return true
		})
		if hasGoto {
			return fmt.Errorf("Run to here does not support goto; use Run all (%s)", name)
		}
	}
	return nil
}

func source(n Notebook) (string, error) {
	if _, err := validateNotebook(n); err != nil {
		return "", err
	}
	if n.Version != 1 {
		return "", fmt.Errorf("unsupported notebook version %d", n.Version)
	}
	if n.RunThrough != nil && *n.RunThrough < len(n.Cells) {
		if err := validatePartialCells(n.Cells); err != nil {
			return "", err
		}
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
		// Keep the entire program available to the compiler so later imports and
		// variable uses remain valid, but stop execution after the selected cell.
		if n.RunThrough != nil && i+1 == *n.RunThrough && i+1 < len(n.Cells) {
			b.WriteString("//line gobook_run.go:1\nif true { return }\n")
		}
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
func (k Kernel) Run(ctx context.Context, n Notebook) (r Result) {
	r.Provenance = Provenance{GoEnv: "off", GoWork: "off", Started: time.Now().UTC(), GoVersionSource: "server runtime", GoVersion: runtime.Version(), GOOS: runtime.GOOS, GOARCH: runtime.GOARCH, ModuleMode: "mod", GOMAXPROCS: 2}
	if n.RunThrough != nil {
		selected := *n.RunThrough
		r.Provenance.RunThrough = &selected
	}
	defer func() { r.Provenance.Finished = time.Now().UTC() }()
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
	mod := k.moduleFile(n)
	r.Provenance.GoMod = mod
	inputs, _ := validateNotebook(n)
	for name, data := range inputs {
		if err = os.WriteFile(filepath.Join(dir, name), data, 0600); err != nil {
			r.Error = err.Error()
			return r
		}
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
	r.Provenance.BuildTimeoutNS = buildTimeout
	r.Provenance.RunTimeoutNS = runTimeout
	r.Provenance.BuildArgs = []string{"build", "-p", "2", "-buildvcs=false", "-mod=mod", "-o", "notebook", "."}
	binary := filepath.Join(dir, "notebook")
	if runtime.GOOS == "windows" {
		binary += ".exe"
	}
	buildCtx, cancel := context.WithTimeout(ctx, buildTimeout)
	buildStarted := time.Now()
	err = runProcess(buildCtx, dir, c, "go", "build", "-p", "2", "-buildvcs=false", "-mod=mod", "-o", binary, ".")
	r.Provenance.BuildDurationNS = time.Since(buildStarted)
	cancel()
	buildErr := err
	if lockErr := captureModules(dir, &r.Provenance); lockErr != nil {
		if buildErr != nil {
			r.Error = fmt.Sprintf("build: %v; %v", buildErr, lockErr)
		} else {
			r.Error = lockErr.Error()
		}
		return r
	}
	if err == nil {
		err = verifyExactPins(n.Dependencies, r.Provenance.GoMod)
	}
	if info, e := buildinfo.ReadFile(binary); e == nil {
		r.Provenance.GoVersion = info.GoVersion
		r.Provenance.GoVersionSource = "notebook binary"
		for _, setting := range info.Settings {
			if setting.Key == "GOOS" {
				r.Provenance.GOOS = setting.Value
			}
			if setting.Key == "GOARCH" {
				r.Provenance.GOARCH = setting.Value
			}
		}
	}
	if err != nil {
		r.Error = fmt.Sprintf("build: %v", err)
	} else {
		runCtx, stop := context.WithTimeout(ctx, runTimeout)
		runStarted := time.Now()
		err = runProcess(runCtx, dir, c, binary)
		r.Provenance.RunDurationNS = time.Since(runStarted)
		stop()
		if err != nil {
			r.Error = fmt.Sprintf("run: %v", err)
		}
	}
	r.Stdout = c.out.String()
	r.Stderr = c.err.String()
	r.Truncated = c.truncated
	r.Artifacts, err = collectArtifactsExcluding(dir, inputs)
	if err != nil {
		if r.Error != "" {
			r.Error += "; "
		}
		r.Error += err.Error()
	}
	return r
}

// Go's minimum-version selection may upgrade a direct requirement. An exact
// notebook pin must fail clearly rather than silently run that newer version.
func verifyExactPins(pins map[string]string, mod string) error {
	resolved := map[string]string{}
	for _, line := range strings.Split(mod, "\n") {
		fields := strings.Fields(line)
		if len(fields) > 0 && fields[0] == "require" {
			fields = fields[1:]
		}
		if len(fields) >= 2 {
			if _, ok := pins[fields[0]]; ok {
				resolved[fields[0]] = fields[1]
			}
		}
	}
	for path, want := range pins {
		if got := resolved[path]; got != want {
			return fmt.Errorf("exact pin %s %s conflicts with resolved version %q", path, want, got)
		}
	}
	return nil
}

// Snapshot compiler-produced module locks before user code can rewrite them.
func captureModules(dir string, p *Provenance) error {
	root, err := os.OpenRoot(dir)
	if err != nil {
		return err
	}
	defer root.Close()
	for _, item := range []struct {
		name   string
		target *string
	}{{"go.mod", &p.GoMod}, {"go.sum", &p.GoSum}} {
		f, err := openArtifact(root, item.name)
		if os.IsNotExist(err) && item.name == "go.sum" {
			continue
		}
		if err != nil {
			return fmt.Errorf("module provenance %s: %w", item.name, err)
		}
		info, err := f.Stat()
		if err != nil || !info.Mode().IsRegular() {
			f.Close()
			return fmt.Errorf("module provenance %s is not a regular file", item.name)
		}
		data, err := io.ReadAll(io.LimitReader(f, 256*1024+1))
		f.Close()
		if err != nil {
			return fmt.Errorf("module provenance %s: %w", item.name, err)
		}
		if len(data) > 256*1024 {
			return fmt.Errorf("module provenance %s exceeds 256 KiB", item.name)
		}
		*item.target = string(data)
	}
	return nil
}

func runProcess(ctx context.Context, dir string, c *capture, name string, args ...string) error {
	cmd := exec.CommandContext(ctx, name, args...)
	cmd.Dir = dir
	// Retain environment cache/proxy settings, but compile only the generated
	// module using explicit flags. Saved go env settings can also supply GOFLAGS,
	// so disable that file as well as clearing inherited -modfile/-overlay flags.
	cmd.Env = append(os.Environ(), "GOMAXPROCS=2", "GOWORK=off", "GOFLAGS=", "GOENV=off")
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
	return collectArtifactsExcluding(dir, nil)
}

// Unchanged supplied inputs are not generated outputs; changed inputs are exported.
func collectArtifactsExcluding(dir string, inputs map[string][]byte) ([]Artifact, error) {
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
		if ext == ".json" {
			mime = "application/json"
		}
		if ext == ".csv" {
			mime = "text/csv"
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
		original, isInput := inputs[entry.Name()]
		readLimit := artifactLimit - total
		if isInput && len(original) > readLimit {
			readLimit = len(original)
		}
		if info.Size() > int64(readLimit) {
			return artifacts, fmt.Errorf("artifact limit exceeded (16 files / 8 MiB)")
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
		data, err := io.ReadAll(io.LimitReader(f, int64(readLimit)+1))
		f.Close()
		if err != nil {
			return artifacts, err
		}
		if isInput && bytes.Equal(original, data) {
			continue
		}
		if len(artifacts) >= 16 || len(data) > artifactLimit-total {
			return artifacts, fmt.Errorf("artifact limit exceeded (16 files / 8 MiB)")
		}
		total += len(data)
		artifacts = append(artifacts, Artifact{entry.Name(), mime, base64.StdEncoding.EncodeToString(data)})
	}
	return artifacts, nil
}
