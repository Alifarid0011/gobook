package main

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"strconv"
	"strings"
)

// Instrument only complete statement cells. Legacy cross-cell blocks and goto
// keep their original program and aggregate output; ownership must not be guessed.
func tracedSource(n Notebook) (string, string, string, error) {
	plain, err := source(n)
	if err != nil || validatePartialCells(n.Cells) != nil || len(n.Cells) == 0 || len(n.Cells) > 1000 {
		return plain, "", "", err
	}
	nonce := make([]byte, 12)
	if _, err = rand.Read(nonce); err != nil {
		return "", "", "", err
	}
	id := "gobookTrace" + hex.EncodeToString(nonce)
	marker := "\x1e" + id + ":"
	traceName := "." + id + ".trace"
	// Insert by reconstructing cells, never by replacing text in user code.
	instrumented := n
	instrumented.Cells = append([]Cell(nil), n.Cells...)
	for i, c := range n.Cells {
		prefix := fmt.Sprintf("//line gobook_trace.go:1\n%sBegin(%d)\n", id, i+1)
		if i == 0 {
			prefix = "//line gobook_trace.go:1\ndefer " + id + "Finish()\n" + prefix
		}
		instrumented.Cells[i].Code = prefix + fmt.Sprintf("//line cell%d.go:1\n", i+1) + c.Code
	}
	traced, err := source(instrumented)
	if err != nil {
		return "", "", "", err
	}
	helper := strings.NewReplacer("TRACEID", id, "MARKER", strconv.Quote(marker), "TRACEFILE", strconv.Quote(traceName)).Replace(traceRuntime)
	return traced, helper, marker, nil
}

const traceRuntime = `package main
import (
 "crypto/sha256"
 "encoding/hex"
 "encoding/json"
 "fmt"
 "io"
 "os"
 "path/filepath"
 "strings"
)
var TRACEIDCell int
var TRACEIDFiles = map[string]struct{Cell int; Hash string}{}
func TRACEIDSnapshot() {
 entries,err:=os.ReadDir(".");if err!=nil{return}
 budget:=int64(8<<20); count:=0
 for _,entry:=range entries {
  ext:=strings.ToLower(filepath.Ext(entry.Name()))
  if ext!=".svg" && ext!=".png" && ext!=".csv" && ext!=".json" {continue}
  info,err:=entry.Info();if err!=nil || !info.Mode().IsRegular() || info.Size()>budget {continue}
  if count>=64 {break};count++;budget-=info.Size()
  f,err:=os.Open(entry.Name());if err!=nil{continue}
  opened,err:=f.Stat();if err!=nil || !opened.Mode().IsRegular(){f.Close();continue}
  h:=sha256.New();_,err=io.Copy(h,io.LimitReader(f,info.Size()+1));f.Close();if err!=nil{continue}
  hash:=hex.EncodeToString(h.Sum(nil))
  if previous,ok:=TRACEIDFiles[entry.Name()];!ok || previous.Hash!=hash {
   if !ok && len(TRACEIDFiles)>=64 {continue}
   TRACEIDFiles[entry.Name()]=struct{Cell int;Hash string}{TRACEIDCell,hash}
  }
 }
 data,err:=json.Marshal(TRACEIDFiles);if err==nil { _=os.WriteFile(TRACEFILE,data,0600) }
}
func TRACEIDBegin(cell int) {
 TRACEIDSnapshot(); TRACEIDCell=cell
 fmt.Fprintf(os.Stdout,MARKER+"%d\x1f",cell)
 fmt.Fprintf(os.Stderr,MARKER+"%d\x1f",cell)
}
func TRACEIDFinish(){TRACEIDSnapshot()}
`

func splitCellStream(text, marker string, count int) (string, map[int]string) {
	var all strings.Builder
	cells := map[int]string{}
	cell := 0
	for {
		before, after, found := strings.Cut(text, marker)
		all.WriteString(before)
		if cell > 0 {
			cells[cell] += before
		}
		if !found {
			break
		}
		number, rest, closed := strings.Cut(after, "\x1f")
		next, err := strconv.Atoi(number)
		if !closed || err != nil || next < 1 || next > count {
			all.WriteString(marker)
			if cell > 0 {
				cells[cell] += marker
			}
			text = after
			continue
		}
		cell = next
		if _, ok := cells[cell]; !ok {
			cells[cell] = ""
		}
		text = rest
	}
	return all.String(), cells
}

func attributeCells(r *Result, dir, marker string, count int) {
	if marker == "" {
		return
	}
	var out, stderr map[int]string
	r.Stdout, out = splitCellStream(r.Stdout, marker, count)
	r.Stderr, stderr = splitCellStream(r.Stderr, marker, count)
	// Protocol bytes do not count toward the public output budget.
	remaining := outputLimit
	trim := func(s string) string {
		if len(s) > remaining {
			s = s[:remaining]
			r.Truncated = true
		}
		remaining -= len(s)
		return s
	}
	unassignedOut, unassignedErr := len(r.Stdout), len(r.Stderr)
	for _, value := range out {
		unassignedOut -= len(value)
	}
	for _, value := range stderr {
		unassignedErr -= len(value)
	}
	r.Stdout = trim(r.Stdout)
	r.Stderr = trim(r.Stderr)
	remaining = max(0, len(r.Stdout)-unassignedOut)
	for i := 1; i <= count; i++ {
		if value, ok := out[i]; ok {
			out[i] = trim(value)
		}
	}
	remaining = max(0, len(r.Stderr)-unassignedErr)
	for i := 1; i <= count; i++ {
		if value, ok := stderr[i]; ok {
			stderr[i] = trim(value)
		}
	}
	for i := 1; i <= count; i++ {
		a, okA := out[i]
		b, okB := stderr[i]
		if !okA && !okB {
			continue
		}
		r.Cells = append(r.Cells, CellResult{Cell: i, Stdout: a, Stderr: b})
	}
	ownership := map[string]struct {
		Cell int
		Hash string
	}{}
	name := "." + strings.TrimSuffix(strings.TrimPrefix(marker, "\x1e"), ":") + ".trace"
	root, err := os.OpenRoot(dir)
	if err != nil {
		return
	}
	defer root.Close()
	f, err := openArtifact(root, name)
	if err != nil {
		return
	}
	defer f.Close()
	if info, err := f.Stat(); err != nil || !info.Mode().IsRegular() || info.Size() > 65536 {
		return
	}
	if json.NewDecoder(f).Decode(&ownership) != nil {
		return
	}
	for _, artifact := range r.Artifacts {
		owner := ownership[artifact.Name]
		if owner.Cell < 1 || owner.Cell > len(r.Cells) {
			continue
		}
		data, err := base64.StdEncoding.DecodeString(artifact.Data)
		if err != nil {
			continue
		}
		hash := sha256.Sum256(data)
		if hex.EncodeToString(hash[:]) != owner.Hash {
			continue
		}
		for i := range r.Cells {
			if r.Cells[i].Cell == owner.Cell {
				r.Cells[i].Artifacts = append(r.Cells[i].Artifacts, artifact)
				break
			}
		}
	}
}
