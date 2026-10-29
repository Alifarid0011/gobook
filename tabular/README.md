# tabular

A standard-library-only, single-predictor ordinary least squares workflow for small tabular experiments. Import `github.com/Alifarid0011/gobook/tabular` from GoBook v0.2.0 or later. This package does not implement scikit-learn parity, multiple regression, preprocessing, cross-validation, or GPU training.

```go
data, err := tabular.ReadCSV(reader, "hours", "score")
// Check err after each call; the runnable notebook does so.
train, heldout, err := tabular.Split(data, 6)
model, err := tabular.Fit(train)
defer model.Close()
predicted, err := model.Predict(heldout.X)
metrics, err := tabular.Evaluate(heldout.Y, predicted)
```

Run the complete synthetic CSV example (including fit, honest held-out metrics, JSON round-trip, and close):

```sh
go test ./tabular -run Example -v
```

Open [`../examples/tabular.gobook.json`](../examples/tabular.gobook.json) for the full notebook, including `os.Create("model.json")`, `tabular.Load` from `os.Open`, and `predictions.csv`. The dependency is pinned to GoBook v0.2.0. Its metrics are computed from its synthetic fixture when run; they are not evidence about real-world predictive performance.

`Split(data, trainCount)` makes independent copies of the first `trainCount` rows and the remaining rows. It is deterministic and preserves order; it does **not** claim to produce a random or representative sample. For randomly sampled experiments, shuffle paired rows externally using a recorded procedure before splitting. For time series, consider whether this simple regression is appropriate at all. Fit only the training data; evaluate only held-out predictions. Dataset slices remain caller-owned and must not be mutated during calls. Fit retains no training data.

`ReadCSV` requires distinct named predictor/response columns, unique nonempty headers, matching record widths, finite numeric values, and at least one data row. It ignores unselected columns. Standard CSV quoting and embedded newlines apply; blank records are errors, including trailing blank records. Missing and invalid observations are errors, with row and column context; there is no imputation. Bound untrusted CSV readers externally (for example through a limited upload). Fit additionally requires at least two rows and a varying predictor.

Fit uses centered, scaled float64 arithmetic with compensated sums. The persisted model keeps the training origins, scales, centered means and normalized slope to avoid rebuilding a potentially unstable intercept. Ordinary fit and prediction use float64; prediction falls back to 256-bit standard-library arithmetic only when intermediate operations overflow. This is floating-point numerical software: rounding and conditioning still matter, and numerically degenerate predictors or unrepresentable slopes/predictions are errors. No accuracy promise extends beyond float64 precision. Benchmark the supplied `BenchmarkFit` and `BenchmarkPredict` on your own machine with `go test ./tabular -bench . -benchmem`.

`Evaluate(actual, predicted)` returns MAE, RMSE, and R². MAE and RMSE retain the response's units. R² can be negative; with constant actual values it is 1 for exactly correct predictions and 0 otherwise. Nonfinite values, empty or mismatched slices, overflow, and numerically unresolved response variation are errors. The metric accumulations are scaled to avoid squaring large raw values.

`model.Save(io.Writer)` and `tabular.Load(io.Reader)` leave stream ownership with the caller; check write, flush and close errors. The JSON schema is `gobook.ols`, version `1`, with numeric `x_origin`, `y_origin`, `x_scale`, `y_scale`, `x_mean`, `y_mean`, and `slope` parameters. Its prediction formula is:

```
y_origin + y_scale * (y_mean + slope * ((x - x_origin)/x_scale - x_mean))
```

Load accepts at most 64 KiB including whitespace, rejects unsupported schemas/versions, unknown/missing/duplicate/null fields, invalid scales, nonfinite parameters, and trailing content. The schema version is independent of the GoBook release version. Round-tripping preserves the fitted float64 representation and predictions.

Model fields are private. Do not copy a Model. `Predict`, `Save` and `Close` are safe to call concurrently; `Close` waits for an active call, clears state, and makes later prediction or save fail. Repeated close succeeds. An uninitialized zero Model cannot predict or save. This lifecycle is explicit even though the current model owns only memory.
