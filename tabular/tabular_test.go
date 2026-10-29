package tabular

import (
	"bytes"
	"fmt"
	"math"
	"strings"
	"sync"
	"testing"
)

func near(t *testing.T, got, want float64) {
	t.Helper()
	if math.Abs(got-want) > 1e-12*math.Max(1, math.Abs(want)) {
		t.Fatalf("got %.17g, want %.17g", got, want)
	}
}

func TestOLSReferenceAndSerialization(t *testing.T) {
	// Hand-computed slope 1.9, intercept 1.0; training values are deliberately noisy.
	m, err := Fit(Dataset{X: []float64{1, 2, 3, 4}, Y: []float64{3, 5, 6, 9}})
	if err != nil {
		t.Fatal(err)
	}
	defer m.Close()
	p, err := m.Predict([]float64{0, 5})
	if err != nil {
		t.Fatal(err)
	}
	near(t, p[0], 1)
	near(t, p[1], 10.5)
	var b bytes.Buffer
	if err = m.Save(&b); err != nil {
		t.Fatal(err)
	}
	loaded, err := Load(&b)
	if err != nil {
		t.Fatal(err)
	}
	defer loaded.Close()
	q, err := loaded.Predict([]float64{0, 5})
	if err != nil {
		t.Fatal(err)
	}
	for i := range p {
		if p[i] != q[i] {
			t.Fatal("serialization changed prediction")
		}
	}
	metrics, err := Evaluate([]float64{1, 2, 3}, []float64{1, 2, 4})
	if err != nil {
		t.Fatal(err)
	}
	near(t, metrics.MAE, 1.0/3)
	near(t, metrics.RMSE, math.Sqrt(1.0/3))
	near(t, metrics.R2, 0.5)
}

func TestStrictCSVAndSplit(t *testing.T) {
	d, err := ReadCSV(strings.NewReader("id,x,y\na,1,3\nb,2,5\nc,3,7\nd,4,1000\n"), "x", "y")
	if err != nil {
		t.Fatal(err)
	}
	train, test, err := Split(d, 3)
	if err != nil {
		t.Fatal(err)
	}
	m, err := Fit(train)
	if err != nil {
		t.Fatal(err)
	}
	defer m.Close()
	p, err := m.Predict(test.X)
	if err != nil {
		t.Fatal(err)
	}
	near(t, p[0], 9)
	train.X[0] = 99
	test.Y[0] = 99
	if d.X[0] != 1 || d.Y[3] != 1000 {
		t.Fatal("split aliases input")
	}
	for _, csv := range []string{"", "x,x,y\n1,2,3\n", "x,z\n1,2\n", "x,y\n1\n", "x,y\n1,no\n", "x,y\n1,NaN\n", "x,y\n1,+Inf\n", "x,y\n", "x,y\n1,2,3\n"} {
		if _, err := ReadCSV(strings.NewReader(csv), "x", "y"); err == nil {
			t.Fatalf("accepted %q", csv)
		}
	}
	if _, err := ReadCSV(strings.NewReader("x,y\n1,no\n"), "x", "y"); err == nil || !strings.Contains(err.Error(), "row 2") || !strings.Contains(err.Error(), "y") {
		t.Fatalf("missing context: %v", err)
	}
	if _, _, err := Split(d, 0); err == nil {
		t.Fatal("empty training accepted")
	}
	if _, _, err := Split(d, 4); err == nil {
		t.Fatal("empty holdout accepted")
	}
}

func TestRejectInvalidNumericInput(t *testing.T) {
	for _, d := range []Dataset{{}, {X: []float64{1}, Y: []float64{}}, {X: []float64{1}, Y: []float64{2}}, {X: []float64{1, 1}, Y: []float64{1, 2}}, {X: []float64{1, math.NaN()}, Y: []float64{1, 2}}, {X: []float64{1, 2}, Y: []float64{1, math.Inf(1)}}, {X: []float64{0, math.SmallestNonzeroFloat64}, Y: []float64{0, math.MaxFloat64}}} {
		if m, err := Fit(d); err == nil {
			m.Close()
			t.Fatalf("accepted %#v", d)
		}
	}
	m, err := Fit(Dataset{X: []float64{0, 1}, Y: []float64{0, 2}})
	if err != nil {
		t.Fatal(err)
	}
	defer m.Close()
	for _, x := range [][]float64{nil, {math.NaN()}, {math.Inf(1)}, {math.MaxFloat64}} {
		if _, err := m.Predict(x); err == nil {
			t.Fatalf("accepted %v", x)
		}
	}
	for _, v := range []struct{ a, p []float64 }{{nil, nil}, {[]float64{1}, nil}, {[]float64{math.NaN()}, []float64{1}}, {[]float64{1}, []float64{math.Inf(1)}}} {
		if _, err := Evaluate(v.a, v.p); err == nil {
			t.Fatal("invalid metrics accepted")
		}
	}
	for _, p := range [][]float64{{1, 1}, {0, 2}} {
		s, err := Evaluate([]float64{1, 1}, p)
		if err != nil {
			t.Fatal(err)
		}
		want := 0.0
		if p[0] == 1 {
			want = 1
		}
		near(t, s.R2, want)
	}
}

func TestExtremeFiniteArithmetic(t *testing.T) {
	// Squaring these values in float64 overflows, but the model y=x is representable.
	x := []float64{-math.MaxFloat64, 0, math.MaxFloat64}
	m, err := Fit(Dataset{X: x, Y: x})
	if err != nil {
		t.Fatal(err)
	}
	defer m.Close()
	p, err := m.Predict(x)
	if err != nil {
		t.Fatal(err)
	}
	for i := range x {
		if p[i] != x[i] {
			t.Fatal(p)
		}
	}
	s, err := Evaluate(x, p)
	if err != nil {
		t.Fatal(err)
	}
	near(t, s.R2, 1)
	near(t, s.RMSE, 0)
	s, err = Evaluate([]float64{math.MaxFloat64, -math.MaxFloat64}, []float64{0, 0})
	if err != nil {
		t.Fatal(err)
	}
	if s.RMSE != math.MaxFloat64 {
		t.Fatal(s)
	}
	// Exact intermediate cancellation must not become infinity.
	m2, err := Load(strings.NewReader(`{"schema":"gobook.ols","version":1,"x_origin":0,"y_origin":-1.7976931348623157e308,"x_scale":1,"y_scale":1.7976931348623157e308,"x_mean":0,"y_mean":0,"slope":1.1125369292536007e-308}`))
	if err != nil {
		t.Fatal(err)
	}
	defer m2.Close()
	p, err = m2.Predict([]float64{math.MaxFloat64})
	if err != nil || math.Abs(p[0]/math.MaxFloat64-1) > 1e-15 {
		t.Fatalf("%v %v", p, err)
	}
}

func TestStrictModelAndLifecycle(t *testing.T) {
	valid := `{"schema":"gobook.ols","version":1,"x_origin":0,"y_origin":1,"x_scale":1,"y_scale":1,"x_mean":0,"y_mean":0,"slope":2}`
	for _, s := range []string{"", `{}`, strings.Replace(valid, `"version":1`, `"version":2`, 1), strings.Replace(valid, `"schema":"gobook.ols"`, `"schema":"other"`, 1), strings.Replace(valid, `"slope":2`, `"slope":null`, 1), strings.Replace(valid, `"slope":2`, `"slope":1e999`, 1), strings.Replace(valid, `"slope":2`, `"slope":"NaN"`, 1), strings.Replace(valid, `"slope":2`, `"slope":2,"extra":1`, 1), strings.Replace(valid, `"slope":2`, `"slope":2,"slope":3`, 1), valid + `{}`, valid + strings.Repeat(" ", 65536)} {
		if m, err := Load(strings.NewReader(s)); err == nil {
			m.Close()
			t.Fatalf("accepted model %.100q", s)
		}
	}
	m, err := Load(strings.NewReader(valid))
	if err != nil {
		t.Fatal(err)
	}
	var wg sync.WaitGroup
	for range 10 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for range 10 {
				m.Predict([]float64{1})
			}
		}()
	}
	m.Close()
	wg.Wait()
	if _, err := m.Predict([]float64{1}); err == nil {
		t.Fatal("closed model predicts")
	}
	if err := m.Save(&bytes.Buffer{}); err == nil {
		t.Fatal("closed model saves")
	}
	if err := m.Close(); err != nil {
		t.Fatal(err)
	}
	var zero Model
	if _, err := zero.Predict([]float64{1}); err == nil {
		t.Fatal("zero model predicts")
	}
}

func Example() {
	d, _ := ReadCSV(strings.NewReader("x,y\n1,3\n2,5\n3,7\n4,9\n5,11\n"), "x", "y")
	train, test, _ := Split(d, 3)
	model, _ := Fit(train)
	defer model.Close()
	predicted, _ := model.Predict(test.X)
	score, _ := Evaluate(test.Y, predicted)
	var saved bytes.Buffer
	_ = model.Save(&saved)
	restored, _ := Load(&saved)
	defer restored.Close()
	again, _ := restored.Predict(test.X)
	fmt.Printf("held-out predictions: %v; restored: %v\n", predicted, again)
	fmt.Printf("MAE %.1f RMSE %.1f R2 %.1f\n", score.MAE, score.RMSE, score.R2)
	// Output:
	// held-out predictions: [9 11]; restored: [9 11]
	// MAE 0.0 RMSE 0.0 R2 1.0
}

func BenchmarkFit(b *testing.B) {
	for _, n := range []int{1000, 100000} {
		b.Run(fmt.Sprint(n), func(b *testing.B) {
			d := benchmarkData(n)
			b.ReportAllocs()
			b.ResetTimer()
			for b.Loop() {
				m, err := Fit(d)
				if err != nil {
					b.Fatal(err)
				}
				m.Close()
			}
		})
	}
}
func BenchmarkPredict(b *testing.B) {
	for _, n := range []int{1000, 100000} {
		b.Run(fmt.Sprint(n), func(b *testing.B) {
			d := benchmarkData(n)
			m, err := Fit(d)
			if err != nil {
				b.Fatal(err)
			}
			defer m.Close()
			b.ReportAllocs()
			b.ResetTimer()
			for b.Loop() {
				if _, err := m.Predict(d.X); err != nil {
					b.Fatal(err)
				}
			}
		})
	}
}
func benchmarkData(n int) Dataset {
	d := Dataset{X: make([]float64, n), Y: make([]float64, n)}
	for i := range d.X {
		d.X[i] = float64(i) / 100
		d.Y[i] = 2*d.X[i] + 3
	}
	return d
}

func TestMetricsScaleResidualsIndependently(t *testing.T) {
	// A large exactly predicted observation must not erase the other row's error.
	got, err := Evaluate([]float64{1e308, 1e-100}, []float64{1e308, 0})
	if err != nil {
		t.Fatal(err)
	}
	if math.Abs(got.MAE/5e-101-1) > 1e-15 || math.Abs(got.RMSE/(1e-100/math.Sqrt2)-1) > 1e-15 {
		t.Fatalf("lost small residual: %+v", got)
	}
}

func TestLargeNearbyPredictors(t *testing.T) {
	a := 1e200
	b := math.Nextafter(a, math.Inf(1))
	c := math.Nextafter(b, math.Inf(1))
	m, err := Fit(Dataset{X: []float64{a, b, c}, Y: []float64{1, 2, 3}})
	if err != nil {
		t.Fatal(err)
	}
	defer m.Close()
	p, err := m.Predict([]float64{a, b, c})
	if err != nil {
		t.Fatal(err)
	}
	for i, want := range []float64{1, 2, 3} {
		near(t, p[i], want)
	}
}

func TestCSVRejectsBlankRecordsWithoutBreakingQuotedNewlines(t *testing.T) {
	for _, text := range []string{"\nx,y\n1,2\n", "x,y\n\n1,2\n", "x,y\n1,2\n\n", "x,y\n1,2\n\n3,4\n"} {
		if _, err := ReadCSV(strings.NewReader(text), "x", "y"); err == nil {
			t.Fatalf("silently skipped blank record %q", text)
		}
	}
	d, err := ReadCSV(strings.NewReader("note,x,y\n\"two\nlines\",1,2\nplain,2,4\n"), "x", "y")
	if err != nil {
		t.Fatal(err)
	}
	if len(d.X) != 2 {
		t.Fatal(d)
	}
}
