package main

import (
	"archive/zip"
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func notebookJSON(t *testing.T, extra string) Notebook {
	t.Helper()
	var n Notebook
	if err := json.Unmarshal([]byte(`{"version":1,`+extra+`}`), &n); err != nil {
		t.Fatal(err)
	}
	return n
}

func TestDependencyPins(t *testing.T) {
	for _, pin := range []struct{ path, version string }{
		{"example.com/model", "latest"}, {"example.com/model", "main"}, {"example.com/model", "../local"},
		{"example.com/model", "v1.2.3\nreplace evil"}, {"../evil", "v1.2.3"}, {"example.com/model/v2", "v1.2.3"},
		{"example.com/model", "v2.0.0"}, {"example.com/model", "v01.2.3"}, {"example.com/model", "v1.2"},
		{"example.com/model//bad", "v1.0.0"}, {"example.com/model/v1", "v1.0.0"},
	} {
		t.Run(pin.path+pin.version, func(t *testing.T) {
			deps, _ := json.Marshal(map[string]string{pin.path: pin.version})
			if _, err := source(notebookJSON(t, `"dependencies":`+string(deps))); err == nil {
				t.Fatal("accepted invalid unused pin")
			}
		})
	}
	for _, deps := range []string{`{"example.com/model":"v1.2.3"}`, `{"example.com/model/v2":"v2.1.0"}`, `{"gonum.org/v1/gonum":"v0.17.0"}`, `{"gopkg.in/yaml.v3":"v3.0.1"}`} {
		if _, err := source(notebookJSON(t, `"dependencies":`+deps)); err != nil {
			t.Fatal(err)
		}
	}
}

func TestInputFileValidation(t *testing.T) {
	for _, name := range []string{"../a.json", "a/b.csv", "go.mod", ".json", "a.JSON", "a b.csv", "a\\b.csv", "CON.json", strings.Repeat("a", 65) + ".csv"} {
		data, _ := json.Marshal([]map[string]string{{"name": name, "data": "e30="}})
		if _, err := source(notebookJSON(t, `"files":`+string(data))); err == nil {
			t.Errorf("accepted %q", name)
		}
	}
	for _, files := range []string{`[{"name":"a.json","data":"!!!"}]`, `[{"name":"a.csv","data":""},{"name":"A.csv","data":""}]`} {
		if _, err := source(notebookJSON(t, `"files":`+files)); err == nil {
			t.Fatal("accepted invalid files")
		}
	}
	for _, sizes := range [][]int{{128*1024 + 1}, {65536, 65537}, {0, 0, 0, 0, 0, 0, 0, 0, 0}} {
		var files []map[string]string
		for i, size := range sizes {
			files = append(files, map[string]string{"name": fmt.Sprintf("f%d.json", i), "data": base64.StdEncoding.EncodeToString(make([]byte, size))})
		}
		data, _ := json.Marshal(files)
		if _, err := source(notebookJSON(t, `"files":`+string(data))); err == nil {
			t.Fatal("accepted oversized inputs")
		}
	}
}

func TestJSONCSVArtifacts(t *testing.T) {
	dir := t.TempDir()
	for _, name := range []string{"model.json", "predictions.csv"} {
		if err := os.WriteFile(filepath.Join(dir, name), []byte("data"), 0600); err != nil {
			t.Fatal(err)
		}
	}
	got, err := collectArtifacts(dir)
	if err != nil || len(got) != 2 {
		t.Fatalf("artifacts: %+v %v", got, err)
	}
	if got[0].MIME != "application/json" || got[1].MIME != "text/csv" {
		t.Fatalf("MIME: %+v", got)
	}
}

func TestReproducibleFileRoundTrip(t *testing.T) {
	n := notebookJSON(t, `"imports":["fmt","os","encoding/json"],"dependencies":{"example.com/unused":"v1.2.3"},"files":[{"name":"model.json","data":"eyJjb2VmZmljaWVudCI6Mn0="}],"cells":[{"code":"b,e:=os.ReadFile(\"model.json\");if e!=nil {panic(e)};var m map[string]int;if e=json.Unmarshal(b,&m);e!=nil {panic(e)};fmt.Println(m[\"coefficient\"]*3);if e=os.WriteFile(\"saved.json\",b,0600);e!=nil {panic(e)}"}]`)
	r := (Kernel{}).Run(context.Background(), n)
	if r.Error != "" || r.Stdout != "6\n" {
		t.Fatalf("run: %+v", r)
	}
	if len(r.Artifacts) != 1 || r.Artifacts[0].Name != "saved.json" || r.Artifacts[0].Data != "eyJjb2VmZmljaWVudCI6Mn0=" {
		t.Fatalf("roundtrip: %+v", r.Artifacts)
	}
	data, _ := json.Marshal(r)
	// Inspect JSON field names directly to protect the exported bundle contract.
	var fields map[string]json.RawMessage
	json.Unmarshal(data, &fields)
	if len(fields["provenance"]) == 0 {
		t.Fatal("missing provenance")
	}
	var p map[string]interface{}
	json.Unmarshal(fields["provenance"], &p)
	for _, key := range []string{"started", "finished", "goVersion", "goos", "goarch", "goMod", "goSum", "buildDurationNS", "runDurationNS", "buildTimeoutNS", "runTimeoutNS", "buildArgs"} {
		if _, ok := p[key]; !ok {
			t.Errorf("missing %s", key)
		}
	}
	if mod, _ := p["goMod"].(string); !strings.Contains(mod, "example.com/unused v1.2.3") {
		t.Fatalf("pin missing: %s", mod)
	}
}

func TestDependencyOverrides(t *testing.T) {
	n := Notebook{Version: 1, Imports: []string{"github.com/Alifarid0011/chartframe", "gonum.org/v1/gonum/stat"}, Dependencies: map[string]string{"github.com/Alifarid0011/chartframe": "v0.2.0", "gonum.org/v1/gonum": "v0.16.0"}}
	mod := (Kernel{Chartframe: "/local/chartframe"}).moduleFile(n)
	if strings.Count(mod, "github.com/Alifarid0011/chartframe") != 1 || strings.Contains(mod, "replace") || strings.Contains(mod, "v0.17.0") || !strings.Contains(mod, "v0.2.0") {
		t.Fatal(mod)
	}
}

func TestProvenanceBeforeRun(t *testing.T) {
	n := Notebook{Version: 1, Imports: []string{"os"}, Cells: []Cell{{Code: `if err:=os.WriteFile("go.mod",[]byte("changed by notebook"),0600);err!=nil {panic(err)}`}}}
	r := (Kernel{}).Run(context.Background(), n)
	if r.Error != "" {
		t.Fatalf("%s: %s", r.Error, r.Stderr)
	}
	if !strings.HasPrefix(r.Provenance.GoMod, "module gobook.local/notebook\n") {
		t.Fatal("provenance captured notebook-edited module")
	}
}

func TestFailureProvenance(t *testing.T) {
	r := (Kernel{}).Run(context.Background(), Notebook{Version: 1, Cells: []Cell{{Code: "invalid go"}}})
	if r.Error == "" || r.Provenance.Started.IsZero() || r.Provenance.Finished.Before(r.Provenance.Started) || r.Provenance.BuildDurationNS <= 0 || r.Provenance.GoMod == "" || r.Provenance.GoVersion == "" {
		t.Fatalf("missing failure provenance: %+v", r)
	}
	r = (Kernel{}).Run(context.Background(), Notebook{Version: 2})
	if r.Error == "" || r.Provenance.Finished.IsZero() {
		t.Fatalf("missing validation failure provenance: %+v", r)
	}
}

func TestInputPermissionsAndChangedExport(t *testing.T) {
	n := Notebook{Version: 1, Imports: []string{"os", "fmt", "runtime"}, Files: []InputFile{{Name: "data.csv", Data: base64.StdEncoding.EncodeToString([]byte("a\n1\n"))}}, Cells: []Cell{{Code: `s,e:=os.Stat("data.csv");if e!=nil {panic(e)};if runtime.GOOS!="windows" && s.Mode().Perm()!=0600 {panic("input permissions")};s,e=os.Stat(".");if e!=nil {panic(e)};if runtime.GOOS!="windows" && s.Mode().Perm()!=0700 {panic("directory permissions")};fmt.Println("private");if e=os.WriteFile("data.csv",[]byte("a\n2\n"),0600);e!=nil {panic(e)}`}}}
	r := (Kernel{}).Run(context.Background(), n)
	if r.Error != "" || r.Stdout != "private\n" || len(r.Artifacts) != 1 || r.Artifacts[0].Data != base64.StdEncoding.EncodeToString([]byte("a\n2\n")) {
		t.Fatalf("changed input: %+v", r)
	}
}

func TestUnchangedInputsDoNotCountTowardArtifactLimit(t *testing.T) {
	dir := t.TempDir()
	for i := 0; i < 16; i++ {
		if err := os.WriteFile(filepath.Join(dir, fmt.Sprintf("a%02d.csv", i)), []byte("x"), 0600); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.WriteFile(filepath.Join(dir, "z.json"), []byte("{}"), 0600); err != nil {
		t.Fatal(err)
	}
	got, err := collectArtifactsExcluding(dir, map[string][]byte{"z.json": []byte("{}")})
	if err != nil || len(got) != 16 {
		t.Fatalf("got %d artifacts: %v", len(got), err)
	}
}

func TestExplicitDependencyRun(t *testing.T) {
	proxy := t.TempDir()
	writeProxyModule(t, proxy, "example.com/model", "v1.2.3", "module example.com/model\n\ngo 1.25.0\n", "package model\nconst Value = 42\n")
	t.Setenv("GOPROXY", fileProxyURL(proxy))
	t.Setenv("GOSUMDB", "off")
	isolatedModuleCache(t)
	n := Notebook{Version: 1, Imports: []string{"fmt", "example.com/model"}, Dependencies: map[string]string{"example.com/model": "v1.2.3"}, Cells: []Cell{{Code: `fmt.Println(model.Value)`}}}
	b, err := json.Marshal(n)
	if err != nil {
		t.Fatal(err)
	}
	var restored Notebook
	if err = json.Unmarshal(b, &restored); err != nil || restored.Dependencies["example.com/model"] != "v1.2.3" {
		t.Fatalf("notebook roundtrip: %+v %v", restored, err)
	}
	r := (Kernel{}).Run(context.Background(), restored)
	if r.Error != "" || r.Stdout != "42\n" || !strings.Contains(r.Provenance.GoSum, "example.com/model v1.2.3 h1:") || r.Provenance.GoVersionSource != "notebook binary" {
		t.Fatalf("explicit dependency: %+v", r)
	}
}

func writeProxyModule(t *testing.T, proxy, path, version, mod, code string) {
	t.Helper()
	dir := filepath.Join(proxy, filepath.FromSlash(path), "@v")
	if err := os.MkdirAll(dir, 0700); err != nil {
		t.Fatal(err)
	}
	for name, data := range map[string]string{version + ".mod": mod, version + ".info": fmt.Sprintf(`{"Version":%q,"Time":"2026-01-01T00:00:00Z"}`, version), "list": version + "\n"} {
		if err := os.WriteFile(filepath.Join(dir, name), []byte(data), 0600); err != nil {
			t.Fatal(err)
		}
	}
	f, err := os.Create(filepath.Join(dir, version+".zip"))
	if err != nil {
		t.Fatal(err)
	}
	z := zip.NewWriter(f)
	for name, data := range map[string]string{"go.mod": mod, "model.go": code} {
		w, err := z.Create(path + "@" + version + "/" + name)
		if err != nil {
			t.Fatal(err)
		}
		if _, err = w.Write([]byte(data)); err != nil {
			t.Fatal(err)
		}
	}
	if err = z.Close(); err != nil {
		t.Fatal(err)
	}
	if err = f.Close(); err != nil {
		t.Fatal(err)
	}
}

func TestExactPinConflict(t *testing.T) {
	proxy := t.TempDir()
	writeProxyModule(t, proxy, "example.com/model", "v1.2.3", "module example.com/model\n\ngo 1.25.0\n", "package model\nconst Value = 42\n")
	writeProxyModule(t, proxy, "example.com/model", "v1.3.0", "module example.com/model\n\ngo 1.25.0\n", "package model\nconst Value = 43\n")
	writeProxyModule(t, proxy, "example.com/consumer", "v1.0.0", "module example.com/consumer\n\ngo 1.25.0\n\nrequire example.com/model v1.3.0\n", "package consumer\nimport \"example.com/model\"\nconst Value = model.Value\n")
	t.Setenv("GOPROXY", fileProxyURL(proxy))
	t.Setenv("GOSUMDB", "off")
	isolatedModuleCache(t)
	n := Notebook{Version: 1, Imports: []string{"fmt", "example.com/consumer"}, Dependencies: map[string]string{"example.com/model": "v1.2.3", "example.com/consumer": "v1.0.0"}, Cells: []Cell{{Code: `fmt.Println(consumer.Value)`}}}
	r := (Kernel{}).Run(context.Background(), n)
	if !strings.Contains(r.Error, "exact pin") || r.Stdout != "" {
		t.Fatalf("silently ran upgraded module: %+v", r)
	}
}

func TestModuleProvenanceBound(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "go.mod"), []byte("module example.com/test\n"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "go.sum"), make([]byte, 256*1024+1), 0600); err != nil {
		t.Fatal(err)
	}
	var p Provenance
	if err := captureModules(dir, &p); err == nil || p.GoSum != "" || p.GoMod != "module example.com/test\n" {
		t.Fatalf("unbounded provenance: %d bytes, %v", len(p.GoSum), err)
	}
}

func TestNotebookFilesRoundTripAndBoundary(t *testing.T) {
	n := Notebook{Version: 1, Dependencies: map[string]string{"example.com/model/v2": "v2.0.1"}}
	for i := 0; i < 8; i++ {
		n.Files = append(n.Files, InputFile{Name: fmt.Sprintf("data%d.csv", i), Data: base64.StdEncoding.EncodeToString(make([]byte, 16*1024))})
	}
	data, err := json.Marshal(n)
	if err != nil {
		t.Fatal(err)
	}
	var restored Notebook
	if err = json.Unmarshal(data, &restored); err != nil {
		t.Fatal(err)
	}
	if _, err = source(restored); err != nil {
		t.Fatal(err)
	}
	if len(restored.Files) != 8 || restored.Files[7].Data != n.Files[7].Data || restored.Dependencies["example.com/model/v2"] != "v2.0.1" {
		t.Fatal("lost reproducibility fields")
	}
}

func TestInheritedCompilerFlagsCannotChangePins(t *testing.T) {
	for _, from := range []string{"environment", "saved-config"} {
		t.Run(from, func(t *testing.T) {
			proxy := t.TempDir()
			writeProxyModule(t, proxy, "example.com/model", "v1.2.3", "module example.com/model\n\ngo 1.25.0\n", "package model\nconst Value = 42\n")
			writeProxyModule(t, proxy, "example.com/model", "v1.3.0", "module example.com/model\n\ngo 1.25.0\n", "package model\nconst Value = 43\n")
			t.Setenv("GOPROXY", fileProxyURL(proxy))
			t.Setenv("GOSUMDB", "off")
			isolatedModuleCache(t)
			alternate := filepath.Join(t.TempDir(), "other.mod")
			if err := os.WriteFile(alternate, []byte("module gobook.local/notebook\n\ngo 1.25.0\n\nrequire example.com/model v1.3.0\n"), 0600); err != nil {
				t.Fatal(err)
			}
			flags := "-modfile=" + alternate
			if from == "environment" {
				t.Setenv("GOFLAGS", flags)
			} else {
				config := filepath.Join(t.TempDir(), "goenv")
				if err := os.WriteFile(config, []byte("GOFLAGS="+flags+"\n"), 0600); err != nil {
					t.Fatal(err)
				}
				t.Setenv("GOENV", config)
				t.Setenv("GOFLAGS", "")
			}
			n := Notebook{Version: 1, Imports: []string{"fmt", "example.com/model"}, Dependencies: map[string]string{"example.com/model": "v1.2.3"}, Cells: []Cell{{Code: `fmt.Println(model.Value)`}}}
			r := (Kernel{}).Run(context.Background(), n)
			if r.Error != "" || r.Stdout != "42\n" || !strings.Contains(r.Provenance.GoSum, "example.com/model v1.2.3 h1:") || strings.Contains(r.Provenance.GoSum, "example.com/model v1.3.0") {
				t.Fatalf("inherited compiler flags changed execution: %+v", r)
			}
		})
	}
}

func isolatedModuleCache(t *testing.T) {
	t.Helper()
	dir := t.TempDir()
	t.Setenv("GOMODCACHE", dir)
	// Module directories are read-only by default; let TempDir clean the fixture.
	t.Cleanup(func() {
		if err := filepath.WalkDir(dir, func(path string, d os.DirEntry, err error) error {
			if err != nil {
				return err
			}
			if d.IsDir() {
				return os.Chmod(path, 0700)
			}
			return nil
		}); err != nil {
			t.Error(err)
		}
	})
}

func TestRunThroughExecutesPrefix(t *testing.T) {
	n := notebookJSON(t, `"runThrough":1,"imports":["fmt"],"cells":[{"code":"fmt.Println(\"one\") // trailing comment"},{"code":"fmt.Println(\"two\")"}]`)
	r := (Kernel{}).Run(context.Background(), n)
	if r.Error != "" || r.Stdout != "one\n" || r.Provenance.RunThrough == nil || *r.Provenance.RunThrough != 1 {
		t.Fatalf("prefix execution: %+v", r)
	}
}

func TestRunThroughCompilesLaterUses(t *testing.T) {
	n := notebookJSON(t, `"runThrough":1,"imports":["fmt"],"cells":[{"code":"value := 42"},{"code":"fmt.Println(value)"}]`)
	r := (Kernel{}).Run(context.Background(), n)
	if r.Error != "" || r.Stdout != "" {
		t.Fatalf("later uses must compile without executing: %+v", r)
	}
}

func TestRunThroughValidation(t *testing.T) {
	for _, value := range []int{-1, 0, 3} {
		n := notebookJSON(t, fmt.Sprintf(`"runThrough":%d,"cells":[{"code":""},{"code":""}]`, value))
		if _, err := source(n); err == nil {
			t.Errorf("accepted runThrough %d", value)
		}
	}
	if _, err := source(notebookJSON(t, `"runThrough":1,"cells":[]`)); err == nil {
		t.Error("accepted runThrough for empty notebook")
	}
}

func TestRunThroughLastCellMatchesFullRun(t *testing.T) {
	full := notebookJSON(t, `"imports":["fmt"],"cells":[{"code":"value := 42"},{"code":"fmt.Println(value)"}]`)
	throughLast := notebookJSON(t, `"runThrough":2,"imports":["fmt"],"cells":[{"code":"value := 42"},{"code":"fmt.Println(value)"}]`)
	a, err := source(full)
	if err != nil {
		t.Fatal(err)
	}
	b, err := source(throughLast)
	if err != nil {
		t.Fatal(err)
	}
	if a != b {
		t.Fatal("last-cell selection changed full-run source")
	}
	r := (Kernel{}).Run(context.Background(), throughLast)
	if r.Error != "" || r.Stdout != "42\n" {
		t.Fatalf("full run changed: %+v", r)
	}
}

func TestRunThroughStillCompilesLaterCells(t *testing.T) {
	n := notebookJSON(t, `"runThrough":1,"imports":["fmt"],"cells":[{"code":"fmt.Println(\"one\")"},{"code":"this is invalid Go"}]`)
	r := (Kernel{}).Run(context.Background(), n)
	if r.Error == "" || r.Stdout != "" || !strings.Contains(r.Error+r.Stderr, "cell2.go:1") {
		t.Fatalf("later compilation error missing: %+v", r)
	}
}

func TestRunThroughRejectsGoto(t *testing.T) {
	n := notebookJSON(t, `"runThrough":1,"imports":["fmt"],"cells":[{"code":"goto later"},{"code":"later: fmt.Println(\"two\")"}]`)
	r := (Kernel{}).Run(context.Background(), n)
	if !strings.Contains(r.Error, "Run to here does not support goto; use Run all") || r.Stdout != "" {
		t.Fatalf("goto escaped selected cell: %+v", r)
	}
}

func TestRunThroughRejectsCrossCellBlocks(t *testing.T) {
	n := notebookJSON(t, `"runThrough":1,"imports":["fmt"],"cells":[{"code":"if false {"},{"code":"} // closes previous cell\nfmt.Println(\"two\")"}]`)
	r := (Kernel{}).Run(context.Background(), n)
	if !strings.Contains(r.Error, "complete statements in each cell") || r.Stdout != "" {
		t.Fatalf("cross-cell block escaped selection: %+v", r)
	}
}

func TestFullRunPreservesCrossCellControlFlow(t *testing.T) {
	for _, selection := range []string{"", `"runThrough":2,`} {
		n := notebookJSON(t, selection+`"imports":["fmt"],"cells":[{"code":"goto later\nif false {"},{"code":"}\nlater: fmt.Println(\"two\")"}]`)
		r := (Kernel{}).Run(context.Background(), n)
		if r.Error != "" || r.Stdout != "two\n" {
			t.Fatalf("full run changed: %+v", r)
		}
	}
}

// A drive-letter path needs a leading slash for file:///C:/... on Windows.
func fileProxyURL(path string) string {
	path = filepath.ToSlash(path)
	if !strings.HasPrefix(path, "/") {
		path = "/" + path
	}
	return (&url.URL{Scheme: "file", Path: path}).String()
}

func TestFileProxyURL(t *testing.T) {
	for _, tt := range []struct{ path, want string }{{"/tmp/module proxy", "file:///tmp/module%20proxy"}, {"C:/Users/example/module proxy", "file:///C:/Users/example/module%20proxy"}} {
		if got := fileProxyURL(tt.path); got != tt.want {
			t.Errorf("file proxy URL for %q: %q, want %q", tt.path, got, tt.want)
		}
	}
}
