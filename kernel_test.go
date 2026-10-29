package main

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"
)

func TestKernel(t *testing.T) {
	k := Kernel{BuildTimeout: 60 * time.Second, RunTimeout: time.Second}
	tests := []struct{ name, code, output, failure string }{
		{"arithmetic", `x := 6; fmt.Println(x*7)`, "42", ""},
		{"syntax", `this is not Go`, "", "build"},
		{"timeout", `for {}`, "", "deadline"},
		{"bounded", `for i:=0;i<200000;i++ { fmt.Println("0123456789") }`, "0123456789", ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			imports := []string{}
			if tt.name == "arithmetic" || tt.name == "bounded" {
				imports = []string{"fmt"}
			}
			r := k.Run(context.Background(), Notebook{Version: 1, Imports: imports, Cells: []Cell{{Code: tt.code}}})
			if !strings.Contains(r.Stdout, tt.output) || !strings.Contains(r.Error, tt.failure) {
				t.Fatalf("result: %+v", r)
			}
			if tt.failure == "" && r.Error != "" {
				t.Fatal(r.Error)
			}
			if len(r.Stdout)+len(r.Stderr) > outputLimit {
				t.Fatal("output unbounded")
			}
			if tt.name == "bounded" && !r.Truncated {
				t.Fatal("missing truncation flag")
			}
		})
	}
}
func TestImportValidation(t *testing.T) {
	for _, p := range []string{"fmt\"\n); func init(){", "../evil", "fmt\n"} {
		if _, err := source(Notebook{Version: 1, Imports: []string{p}}); err == nil {
			t.Errorf("accepted %q", p)
		}
	}
	n := Notebook{Version: 1, Title: "<script>", Cells: []Cell{{Code: `fmt.Println("x")`}}, Imports: []string{"fmt"}}
	b, _ := json.Marshal(n)
	var got Notebook
	if err := json.Unmarshal(b, &got); err != nil || got.Title != n.Title {
		t.Fatal("roundtrip", err)
	}
}
func TestArtifactSymlink(t *testing.T) {
	dir := t.TempDir()
	outside := filepath.Join(t.TempDir(), "secret.svg")
	os.WriteFile(outside, []byte("secret"), 0600)
	if err := os.Symlink(outside, filepath.Join(dir, "link.svg")); err != nil {
		if runtime.GOOS == "windows" {
			t.Skip("symlink creation requires Windows privilege")
		}
		t.Fatal(err)
	}
	os.WriteFile(filepath.Join(dir, "real.svg"), []byte(`<svg xmlns="http://www.w3.org/2000/svg"/>`), 0600)
	a, err := collectArtifacts(dir)
	if err != nil || len(a) != 1 || a[0].Name != "real.svg" {
		t.Fatalf("%+v %v", a, err)
	}
}

func TestExamples(t *testing.T) {
	for _, name := range []string{"simulation", "sine"} {
		t.Run(name, func(t *testing.T) {
			data, err := os.ReadFile(filepath.Join("examples", name+".gobook.json"))
			if err != nil {
				t.Fatal(err)
			}
			var n Notebook
			if err = json.Unmarshal(data, &n); err != nil {
				t.Fatal(err)
			}
			chart, err := filepath.Abs("../chartframe")
			if err != nil {
				t.Fatal(err)
			}
			if name == "sine" {
				if _, err = os.Stat(filepath.Join(chart, "go.mod")); err != nil {
					t.Skip("adjacent chartframe is optional")
				}
			}
			result := (Kernel{Chartframe: chart, BuildTimeout: 60 * time.Second}).Run(context.Background(), n)
			if result.Error != "" {
				t.Fatalf("%s: %s", result.Error, result.Stderr)
			}
			if name == "sine" && (len(result.Artifacts) != 1 || result.Artifacts[0].MIME != "image/svg+xml") {
				t.Fatalf("missing SVG: %+v", result)
			}
			if name == "simulation" && !strings.Contains(result.Stdout, "π ≈") {
				t.Fatalf("output: %s", result.Stdout)
			}
		})
	}
}
func TestArtifactLimits(t *testing.T) {
	dir := t.TempDir()
	for i := 0; i < 17; i++ {
		if err := os.WriteFile(filepath.Join(dir, fmt.Sprintf("image%02d.svg", i)), []byte("<svg/>"), 0600); err != nil {
			t.Fatal(err)
		}
	}
	a, err := collectArtifacts(dir)
	if err == nil || len(a) != 16 {
		t.Fatalf("got %d artifacts, err %v", len(a), err)
	}
}
