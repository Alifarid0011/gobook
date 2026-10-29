# Palmer Penguins: a DataFrame-to-model notebook

Open [`examples/penguins.gobook.json`](../../examples/penguins.gobook.json) in GoBook and run all five editable cells. Its input `penguins.csv` is embedded as base64, so running the notebook needs no dataset download. The notebook is 32754 bytes, below the 128 KiB notebook request limit; the decoded CSV is 15,241 bytes. The first execution may download the pinned Go modules.

The example uses published `sheetframe v0.2.0`, `gobook v0.2.0`, and `chartframe v0.1.0`, with Go 1.25 or later. These versions appear explicitly in the notebook dependency map. Go's module graph can select newer transitive versions where another direct dependency requires them; GoBook's run metadata records the resolved `go.mod` and `go.sum`.

## Source and attribution

The CSV is an unchanged snapshot from the primary [allisonhorst/palmerpenguins repository](https://github.com/allisonhorst/palmerpenguins), pinned at commit [`8957207b78d6ccd1b4654a9dd9c9041b657478ab`](https://github.com/allisonhorst/palmerpenguins/tree/8957207b78d6ccd1b4654a9dd9c9041b657478ab). Download the exact [source CSV](https://raw.githubusercontent.com/allisonhorst/palmerpenguins/8957207b78d6ccd1b4654a9dd9c9041b657478ab/inst/extdata/penguins.csv). Its SHA-256 is:

```
f204db2c753b0937caac3cb35258562c14f073e4bbc76be24b4c51ce22767a93
```

The repository documents the data as [CC0](https://github.com/allisonhorst/palmerpenguins#license). Observations were collected by Kristen Gorman and the Palmer Station Long Term Ecological Research program. The source's [citation and additional data-use information](https://github.com/allisonhorst/palmerpenguins#citation) should accompany research reuse. Package reference: Horst AM, Hill AP, Gorman KB (2020), *palmerpenguins: Palmer Archipelago (Antarctica) penguin data*, [DOI 10.5281/zenodo.3960218](https://doi.org/10.5281/zenodo.3960218). The original study is [Gorman, Williams and Fraser (2014)](https://doi.org/10.1371/journal.pone.0090081).

## What the cells do

1. Verify the embedded checksum, read the CSV into a sheetframe DataFrame, print `Shape` and `Schema`, and preview `Head(5)` using `WriteCSV(os.Stdout)`. It also writes `Head(8)` as `data-preview.json` in the library-neutral `gobook.table.v1` format so GoBook displays a visible table.
2. Explicitly convert only `flipper_length_mm` and `body_mass_g` to nullable numeric columns with `WithColumn`. The source uses the literal `NA`, which sheetframe does not silently treat as null: affected raw columns infer as strings. This cell maps `NA` to null and parses other selected values, failing on unexpected input. It counts each selected column's missing observations and the union excluded by `NotNull` filtering. Other missing attributes do not exclude a row. The cleaned numeric DataFrame is serialized to CSV in memory and passed to strict `tabular.ReadCSV`. A second visible `Head(8)` table is saved as `clean-preview.json`.
3. Use `math/rand.New(rand.NewSource(42)).Perm(n)` to shuffle paired rows. The first `floor(0.8*n)` rows train the one-predictor OLS model; the remaining rows are held out. Fit sees training rows only. Compare test MAE, RMSE and R² with a baseline that predicts the **training** response mean for every held-out row.
4. Export the train/test relationship with the training-only fit, an actual-versus-predicted plot colored by species with a perfect-prediction line, and residuals versus predictions with a zero line. Residual means actual minus predicted mass.
5. Save metrics, model, and predictions. Load the saved model and verify bitwise prediction parity before closing both model instances. File errors and close errors are checked. Output source-row identifiers preserve the link to the original CSV, including its header as row 1.

## Reproduced result

Measured on 2026-10-29 with Go 1.26.8, published sheetframe/chartframe versions, and the local GoBook checkout. These are results of this fixed dataset/split, not a performance benchmark or an external validation study.

The source has **344 rows and 8 columns**. Both selected measurements are missing on the same **2 rows** (source CSV rows 5 and 273): each missing count is 2, and the excluded union is 2. The remaining **342 paired observations** produce **273 training** and **69 held-out** rows. The baseline's training mean is **4238.461538 g**.

| Held-out predictor | MAE (g) | RMSE (g) | R² |
| --- | ---: | ---: | ---: |
| Training-only OLS from flipper length | 287.253723 | 383.076879 | 0.768277 |
| Constant training-mean baseline | 713.210702 | 816.328911 | -0.052270 |

A separate Python standard-library calculation using `statistics.linear_regression` confirmed the fitted predictions, baseline, and all metrics to floating-point tolerance. Every exported held-out row was checked against its original source row; its complement contained the 273 complete training rows. These checks validate this example's wiring, not the scientific adequacy of its model.

There are eight output artifacts: two visible DataFrame tables (`data-preview.json` and `clean-preview.json`, each eight rows), plus `relationship.svg`, `actual-vs-predicted.svg`, `residuals.svg`, `metrics.json`, `model.json`, and `predictions.csv`. The metrics JSON includes source commit/checksum, missing counts, split method/seed/counts, training mean, both evaluations, units/column names and limitations. The prediction CSV includes original source CSV row, species, predictor, actual response, OLS prediction, baseline prediction and residual. Download them from the notebook's result area or export the full experiment bundle to retain notebook, inputs, execution metadata and outputs together.

## Interpretation and useful edits

The pooled relationship can reflect differences among species, sex, island and year. The model omits these attributes. A random row holdout may share population and collection context with training data; it does not test a new species, site, season or independent population. One split does not quantify split uncertainty. The plots expose errors and group structure; they do not establish causation or all OLS assumptions.

Change the split seed to study sensitivity, inspect the species-colored diagnostic, or change the numeric predictor and update its units/labels and missing-value handling together. Keep the same held-out rows when comparing models, avoid selecting a seed for an attractive score, and do not tune using the test responses. Replacing the embedded CSV also requires reviewing its provenance, schema and checksum guard. More predictors or a different validation design require a different model/workflow; this example intentionally demonstrates one transparent regression.

The two preview artifacts contain `format`, `title`, `columns`, and `rows` fields; their data comes directly from sheetframe `Columns()` and `Head(8).Records()`. The raw preview preserves the eight original columns and literal missing markers; the cleaned preview has nine columns including the original CSV-row identifier, and selected measurements are numeric. Both fit comfortably within the UI table limits of 50 columns and 200 rows. All eight generated artifacts together are below 8 MiB.
