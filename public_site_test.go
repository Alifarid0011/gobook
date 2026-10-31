package main

import (
	"bytes"
	"encoding/json"
	"os"
	"testing"
)

// Check the actual download with the default Interactive evaluator, not only
// the native compiler. No host, installed application, or live kernel is used.
func TestPublicStarterNotebook(t *testing.T) {
	data, err := os.ReadFile("site/examples/first-steps.gobook.json")
	if err != nil {
		t.Fatal(err)
	}
	var n Notebook
	if err := json.Unmarshal(data, &n); err != nil {
		t.Fatal(err)
	}
	var output bytes.Buffer
	evaluator := newInteractiveEvaluator(&output, &output)
	for i, cell := range n.Cells {
		if err := evaluator.eval(n.Imports, cell.Code); err != nil {
			t.Fatalf("starter cell %d: %v", i+1, err)
		}
	}
	if got, want := output.String(), "Loaded 3 amounts\nTotal: 60\nAverage: 20\n"; got != want {
		t.Fatalf("starter output %q, want %q", got, want)
	}
}
