// Package tabular implements a small, deterministic, single-predictor OLS workflow.
// All observations must be finite. No rows are silently dropped or imputed.
package tabular

import (
	"bytes"
	"encoding/csv"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math"
	"math/big"
	"strconv"
	"strings"
	"sync"
)

// Dataset pairs one numeric predictor X with a numeric response Y.
// Callers own the slices and must not mutate them during a function call.
type Dataset struct{ X, Y []float64 }

func finite(v float64) bool { return !math.IsNaN(v) && !math.IsInf(v, 0) }
func validate(d Dataset) error {
	if len(d.X) == 0 || len(d.X) != len(d.Y) {
		return errors.New("tabular: nonempty X and Y must have equal lengths")
	}
	for i := range d.X {
		if !finite(d.X[i]) || !finite(d.Y[i]) {
			return fmt.Errorf("tabular: observation %d is not finite", i)
		}
	}
	return nil
}

// ReadCSV extracts two distinct named numeric columns. Other columns are ignored.
// Headers must be nonempty and unique; every row must match the header width.
// Blank records are errors; quoted embedded newlines are supported.
// Values follow strconv.ParseFloat syntax; missing and nonfinite values are errors.
// The caller is responsible for bounding the reader for untrusted CSV input.
func ReadCSV(r io.Reader, predictor, target string) (Dataset, error) {
	var d Dataset
	if predictor == "" || target == "" || predictor == target {
		return d, errors.New("tabular: choose two distinct nonempty column names")
	}
	reader := csv.NewReader(r)
	header, err := reader.Read()
	if err != nil {
		return d, fmt.Errorf("tabular: CSV header: %w", err)
	}
	headerLine, _ := reader.FieldPos(0)
	if headerLine != 1 {
		return d, errors.New("tabular: blank CSV record before header")
	}
	lastLine, _ := reader.FieldPos(len(header) - 1)
	nextLine := lastLine + strings.Count(header[len(header)-1], "\n") + 1
	columns := map[string]int{}
	for i, name := range header {
		if name == "" {
			return d, errors.New("tabular: empty CSV header")
		}
		if _, ok := columns[name]; ok {
			return d, fmt.Errorf("tabular: duplicate CSV header %q", name)
		}
		columns[name] = i
	}
	xi, xok := columns[predictor]
	yi, yok := columns[target]
	if !xok || !yok {
		return d, errors.New("tabular: requested CSV columns are missing")
	}
	for row := 2; ; row++ {
		previousOffset := reader.InputOffset()
		record, err := reader.Read()
		if err == io.EOF {
			if reader.InputOffset() > previousOffset {
				return Dataset{}, fmt.Errorf("tabular: blank CSV record at line %d", nextLine)
			}
			break
		}
		if err != nil {
			return Dataset{}, fmt.Errorf("tabular: CSV row %d: %w", row, err)
		}
		firstLine, _ := reader.FieldPos(0)
		if firstLine != nextLine {
			return Dataset{}, fmt.Errorf("tabular: blank CSV record at line %d", nextLine)
		}
		lastLine, _ := reader.FieldPos(len(record) - 1)
		nextLine = lastLine + strings.Count(record[len(record)-1], "\n") + 1
		values := [2]float64{}
		for j, column := range []int{xi, yi} {
			v, err := strconv.ParseFloat(record[column], 64)
			if err != nil || !finite(v) {
				return Dataset{}, fmt.Errorf("tabular: CSV row %d column %q: expected finite number, got %q", row, header[column], record[column])
			}
			values[j] = v
		}
		d.X = append(d.X, values[0])
		d.Y = append(d.Y, values[1])
	}
	if err := validate(d); err != nil {
		return Dataset{}, err
	}
	return d, nil
}

// Split copies the first trainCount rows into training data and the remaining
// rows into held-out data. It preserves order and never shuffles. Both are nonempty.
// Choose or shuffle row order externally when sequential holdout is inappropriate.
func Split(d Dataset, trainCount int) (Dataset, Dataset, error) {
	if err := validate(d); err != nil {
		return Dataset{}, Dataset{}, err
	}
	if trainCount < 1 || trainCount >= len(d.X) {
		return Dataset{}, Dataset{}, errors.New("tabular: split must leave nonempty training and test sets")
	}
	return Dataset{append([]float64(nil), d.X[:trainCount]...), append([]float64(nil), d.Y[:trainCount]...)}, Dataset{append([]float64(nil), d.X[trainCount:]...), append([]float64(nil), d.Y[trainCount:]...)}, nil
}

// Model is a fitted single-predictor ordinary least squares model. It must not
// be copied. Predict, Save and Close may be called concurrently. The zero value
// is unusable. Close is idempotent and invalidates further Predict and Save calls.
type Model struct {
	mu    sync.RWMutex
	ready bool
	state modelJSON
}

type modelJSON struct {
	Schema  string  `json:"schema"`
	Version int     `json:"version"`
	XOrigin float64 `json:"x_origin"`
	YOrigin float64 `json:"y_origin"`
	XScale  float64 `json:"x_scale"`
	YScale  float64 `json:"y_scale"`
	XMean   float64 `json:"x_mean"`
	YMean   float64 `json:"y_mean"`
	Slope   float64 `json:"slope"`
}

type sum struct{ value, correction float64 }

func (s *sum) add(v float64) {
	y := v - s.correction
	t := s.value + y
	s.correction = (t - s.value) - y
	s.value = t
}

// delta avoids overflowing the subtraction for opposite extreme values, while
// subtracting first preserves close differences between large same-sign values.
func delta(v, origin, scale float64) float64 {
	d := v - origin
	if finite(d) {
		return d / scale
	}
	return v/scale - origin/scale
}

// Fit learns from d only. It requires at least two observations and a varying
// predictor. Scaled centered sums with compensated accumulation limit float64
// overflow and rounding error. Numerically degenerate or unrepresentable slopes
// are rejected. No training slices are retained.
func Fit(d Dataset) (*Model, error) {
	if err := validate(d); err != nil {
		return nil, err
	}
	if len(d.X) < 2 {
		return nil, errors.New("tabular: fit requires at least two observations")
	}
	s := modelJSON{Schema: "gobook.ols", Version: 1, XOrigin: d.X[0], YOrigin: d.Y[0]}
	for i := range d.X {
		s.XScale = math.Max(s.XScale, math.Abs(d.X[i]))
		s.YScale = math.Max(s.YScale, math.Abs(d.Y[i]))
	}
	if s.XScale == 0 {
		return nil, errors.New("tabular: constant predictor")
	}
	if s.YScale == 0 {
		s.YScale = 1
	}
	var sx, sy sum
	n := float64(len(d.X))
	for i := range d.X {
		sx.add(delta(d.X[i], s.XOrigin, s.XScale) / n)
		sy.add(delta(d.Y[i], s.YOrigin, s.YScale) / n)
	}
	s.XMean = sx.value
	s.YMean = sy.value
	var xx, xy sum
	for i := range d.X {
		x := delta(d.X[i], s.XOrigin, s.XScale) - s.XMean
		y := delta(d.Y[i], s.YOrigin, s.YScale) - s.YMean
		xx.add(x * x / n)
		xy.add(x * y / n)
	}
	if xx.value <= 0 || !finite(xx.value) {
		return nil, errors.New("tabular: constant or numerically degenerate predictor")
	}
	s.Slope = xy.value / xx.value
	// Multiplication order must not overflow solely because scales differ greatly.
	physicalSlope := scaledProduct(s.Slope, s.YScale, s.XScale)
	if !finite(s.Slope) || !finite(physicalSlope) {
		return nil, errors.New("tabular: slope is not representable as float64")
	}
	return &Model{ready: true, state: s}, nil
}

func scaledProduct(a, b, c float64) float64 {
	am, ae := math.Frexp(a)
	bm, be := math.Frexp(b)
	cm, ce := math.Frexp(c)
	return math.Ldexp(am*bm/cm, ae+be-ce)
}

// Predict returns fresh predictions. Empty, nonfinite, or unrepresentable inputs
// produce an error and no partial output. Extreme overflowing intermediates use
// a bounded-precision fallback; ordinary observations use float64 arithmetic.
func (m *Model) Predict(x []float64) ([]float64, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	if !m.ready {
		return nil, errors.New("tabular: model is closed or uninitialized")
	}
	if len(x) == 0 {
		return nil, errors.New("tabular: prediction input is empty")
	}
	s := m.state
	p := make([]float64, len(x))
	for i, v := range x {
		if !finite(v) {
			return nil, fmt.Errorf("tabular: predictor %d is not finite", i)
		}
		z := delta(v, s.XOrigin, s.XScale) - s.XMean
		normalized := math.FMA(s.Slope, z, s.YMean)
		p[i] = math.FMA(normalized, s.YScale, s.YOrigin)
		if !finite(z) || !finite(normalized) || !finite(p[i]) {
			p[i] = extremePrediction(s, v)
		}
		if !finite(p[i]) {
			return nil, fmt.Errorf("tabular: prediction %d overflows float64", i)
		}
	}
	return p, nil
}

func extremePrediction(s modelJSON, x float64) float64 {
	// 256 bits exceeds float64 precision and permits cancellation before rounding.
	f := func(v float64) *big.Float { return new(big.Float).SetPrec(256).SetFloat64(v) }
	z := f(x)
	z.Sub(z, f(s.XOrigin))
	z.Quo(z, f(s.XScale))
	z.Sub(z, f(s.XMean))
	z.Mul(z, f(s.Slope))
	z.Add(z, f(s.YMean))
	z.Mul(z, f(s.YScale))
	z.Add(z, f(s.YOrigin))
	v, _ := z.Float64()
	return v
}

// Close releases the fitted state. It is safe to call repeatedly.
func (m *Model) Close() error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.ready = false
	m.state = modelJSON{}
	return nil
}

// Save writes a schema-versioned JSON model. The caller owns and closes w.
func (m *Model) Save(w io.Writer) error {
	m.mu.RLock()
	defer m.mu.RUnlock()
	if !m.ready {
		return errors.New("tabular: model is closed or uninitialized")
	}
	return json.NewEncoder(w).Encode(m.state)
}

// Load reads one model of at most 64 KiB, including whitespace. Missing, unknown,
// duplicate, nonfinite or invalid fields and trailing JSON are rejected.
// The caller owns and closes r. Version 1 describes scaled centered float64 OLS.
func Load(r io.Reader) (*Model, error) {
	data, err := io.ReadAll(io.LimitReader(r, 65537))
	if err != nil {
		return nil, err
	}
	if len(data) > 65536 {
		return nil, errors.New("tabular: model exceeds 64 KiB")
	}
	dec := json.NewDecoder(bytes.NewReader(data))
	token, err := dec.Token()
	if err != nil || token != json.Delim('{') {
		return nil, errors.New("tabular: expected model JSON object")
	}
	fields := map[string]json.RawMessage{}
	for dec.More() {
		token, err := dec.Token()
		if err != nil {
			return nil, err
		}
		key, ok := token.(string)
		if !ok {
			return nil, errors.New("tabular: invalid model key")
		}
		if _, ok := fields[key]; ok {
			return nil, fmt.Errorf("tabular: duplicate model field %q", key)
		}
		var raw json.RawMessage
		if err := dec.Decode(&raw); err != nil {
			return nil, err
		}
		if bytes.Equal(bytes.TrimSpace(raw), []byte("null")) {
			return nil, fmt.Errorf("tabular: null model field %q", key)
		}
		fields[key] = raw
	}
	if _, err := dec.Token(); err != nil {
		return nil, err
	}
	if _, err := dec.Token(); err != io.EOF {
		return nil, errors.New("tabular: trailing model data")
	}
	required := []string{"schema", "version", "x_origin", "y_origin", "x_scale", "y_scale", "x_mean", "y_mean", "slope"}
	if len(fields) != len(required) {
		return nil, errors.New("tabular: missing or unknown model fields")
	}
	for _, key := range required {
		if _, ok := fields[key]; !ok {
			return nil, fmt.Errorf("tabular: missing model field %q", key)
		}
	}
	var s modelJSON
	if err := json.Unmarshal(data, &s); err != nil {
		return nil, fmt.Errorf("tabular: invalid model: %w", err)
	}
	if s.Schema != "gobook.ols" || s.Version != 1 {
		return nil, errors.New("tabular: unsupported model schema or version")
	}
	for _, v := range []float64{s.XOrigin, s.YOrigin, s.XScale, s.YScale, s.XMean, s.YMean, s.Slope} {
		if !finite(v) {
			return nil, errors.New("tabular: nonfinite model parameter")
		}
	}
	if s.XScale <= 0 || s.YScale <= 0 || math.Abs(s.XOrigin) > s.XScale || math.Abs(s.YOrigin) > s.YScale || math.Abs(s.XMean) > 2 || math.Abs(s.YMean) > 2 || !finite(scaledProduct(s.Slope, s.YScale, s.XScale)) {
		return nil, errors.New("tabular: invalid model scaling or slope")
	}
	return &Model{ready: true, state: s}, nil
}

// Metrics reports mean absolute error, root mean squared error and coefficient
// of determination. For constant actual values, R2 is 1 for exact predictions,
// otherwise 0. Other R2 values may be negative.
type Metrics struct {
	MAE  float64 `json:"mae"`
	RMSE float64 `json:"rmse"`
	R2   float64 `json:"r2"`
}

// Evaluate scores held-out actual values against predictions, without fitting.
// All values must be finite and slices must be nonempty with equal lengths.
func Evaluate(actual, predicted []float64) (Metrics, error) {
	if err := validate(Dataset{actual, predicted}); err != nil {
		return Metrics{}, err
	}
	responseScale, residualScale, allScale := 0.0, 0.0, 0.0
	constant, exact, overflowingDifference := true, true, false
	for i, a := range actual {
		responseScale = math.Max(responseScale, math.Abs(a))
		allScale = math.Max(allScale, math.Max(math.Abs(a), math.Abs(predicted[i])))
		residual := a - predicted[i]
		if finite(residual) {
			residualScale = math.Max(residualScale, math.Abs(residual))
		} else {
			overflowingDifference = true
		}
		constant = constant && a == actual[0]
		exact = exact && a == predicted[i]
	}
	if overflowingDifference {
		residualScale = allScale
	}
	if residualScale == 0 {
		return Metrics{R2: 1}, nil
	}
	if responseScale == 0 {
		responseScale = 1
	}
	n := float64(len(actual))
	var absolute, squared, mean sum
	for i, a := range actual {
		r := delta(a, predicted[i], residualScale)
		absolute.add(math.Abs(r) / n)
		squared.add(r * r / n)
		mean.add(delta(a, actual[0], responseScale) / n)
	}
	var total sum
	for _, a := range actual {
		d := delta(a, actual[0], responseScale) - mean.value
		total.add(d * d / n)
	}
	result := Metrics{MAE: absolute.value * residualScale, RMSE: math.Sqrt(squared.value) * residualScale}
	if constant {
		if exact {
			result.R2 = 1
		}
	} else {
		if total.value <= 0 {
			return Metrics{}, errors.New("tabular: response variation is numerically degenerate")
		}
		ratio := scaledProduct(math.Sqrt(squared.value)/math.Sqrt(total.value), residualScale, responseScale)
		result.R2 = math.FMA(-ratio, ratio, 1)
	}
	if !finite(result.MAE) || !finite(result.RMSE) || !finite(result.R2) {
		return Metrics{}, errors.New("tabular: evaluation overflows float64")
	}
	return result, nil
}
