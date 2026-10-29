# Build a library for GoBook

GoBook runs ordinary Go programs. A library does not need a special plugin loader:
create a Go module, publish an immutable semantic-version tag, and put that version
in the notebook's Experiment settings. Import the package in Imports and call its
API in a code cell. Keep reusable algorithms in the library and experiment-specific
data, seeds and parameters in the notebook.

For example, a notebook using the existing tabular package declares:

```json
{"github.com/Alifarid0011/gobook": "v0.2.0"}
```

Private module access uses the host's exported Go environment. Dependency code runs
with the user's permissions. GoBook does not sandbox imports or install browser
extensions. There is no arbitrary JavaScript plugin API.

## Add your own package

1. Create a regular module: `go mod init github.com/YOURNAME/researchkit`.
2. Put exported functions in an importable package, add tests with `go test ./...`,
   then commit and publish a semantic version such as `v0.1.0`.
3. Open **Experiment settings** in GoBook and enter the exact module version:

   ```json
   {"github.com/YOURNAME/researchkit": "v0.1.0"}
   ```

4. Add `github.com/YOURNAME/researchkit` in **Imports**, then call its exported
   functions from a code cell. Use **Run to here** while developing an analysis.
5. Save the experiment to preserve the source, dependency declarations and run
   provenance alongside the results.

GoBook resolves modules through the host's Go toolchain. A package must support
that host's operating system and architecture; C/CUDA dependencies still require
their own compilers and runtimes. Local unpublished module replacements are not
currently configurable through the notebook UI. Publish a version for portable
notebooks, or test the module with ordinary Go tooling before publishing.

## Customize the workspace

Open the gear button in the toolbar to choose Paper or Dark, a code font size,
indent width and line wrapping. These preferences survive reloads in this
browser; they are not part of a notebook or experiment bundle. Another browser
or device keeps its own preferences. No source code or host token is saved in
the preferences record.

## Output contracts

Write files in the run directory with ordinary relative filenames. SVG and PNG
are displayed as images. CSV and JSON are downloadable data/model artifacts.
Unmodified input files are not returned as generated outputs. Current bounds are
16 generated artifacts and 8 MiB total; input files are limited to 8 and 128 KiB.
Check all write and close errors. Avoid partial model files on failure.

A JSON file with the following shape is rendered as a DataFrame preview:

```json
{
  "format": "gobook.table.v1",
  "title": "Sample · Head(2)",
  "columns": ["region", "value"],
  "rows": [["North", 12.5], ["South", null]]
}
```

Preview cells are strings, numbers, booleans or null. Columns must be nonempty;
each row must have the same width. At most 50 columns and 200 rows are rendered;
the encoded artifact must fit the preview limit (512 KiB of base64). Larger or
unsupported JSON remains a downloadable file. Null is displayed as an em dash.
All titles, headers and cells are inserted as text, not executable HTML.

With sheetframe, use `frame.Columns()` and `frame.Head(8).Records()` as the columns
and rows. Encode that object with `encoding/json` into `preview.json`.
[The Penguins notebook](../examples/penguins.gobook.json) demonstrates this using
a real DataFrame, missing-value handling, held-out prediction and diagnostic plots.

Model serialization is the library's responsibility. Version the schema separately
from the module, validate input, document numerical limits and test round-trip
predictions. If a model owns native memory, file descriptors or device buffers,
provide an explicit close method and define concurrent-use behavior.

## Development checklist

- Explain the input types, missing-value behavior and numerical assumptions.
- Keep training-only transformations separate from evaluation data.
- Include an executable notebook and a small deterministic test fixture.
- Test validation, serialization and cleanup, including failure paths.
- Record benchmark timing boundaries and raw measurements before claiming speed.
- Document dataset/model licenses separately from the source-code license.
