# GoBook

A small computational notebook for Go. Write editable cells in your browser, run them with the real Go compiler, and keep notes, output, and chart images together. The server and UI use the Go standard library; there is no Python runtime, CDN, telemetry, or AI service.

Built by [Ali Farid](https://github.com/Alifarid0011).

**Status: early research tool.** GoBook currently computes Go code. NotebookLM-style document sources, retrieval, and AI Q&A are future work and are not implemented.

![GoBook workspace with a chart computed from Go cells](docs/workspace.jpg)

## Local app

Download a [release archive](https://github.com/Alifarid0011/gobook/releases/latest),
then run `./install.sh` on Linux or `./Install-GoBook.ps1` in Windows PowerShell.
Open **GoBook Local** from your application menu. It starts its local host, opens
the authenticated workspace and reuses the live host on a second launch. No URL
or token entry is required. Use **Quit GoBook** in the workspace to stop it.
Go 1.25+ and a browser must be installed. [Installation and lifecycle](docs/desktop.md).

The browser's **Install app** shortcut is a web shortcut and cannot start an
absent host. Use the installed **GoBook Local** menu entry for automatic startup.

## Start

Install Go 1.25 or newer:

```sh
go install github.com/Alifarid0011/gobook@v0.6.0
gobook
```

If your Go binary directory is not on PATH, use `go run github.com/Alifarid0011/gobook@v0.6.0` instead.

Or run from a source checkout:

```sh
go run .
```

Open the local capability URL printed in the terminal. Keep it private: it authorizes code execution. GoBook listens only on `127.0.0.1`; it refuses other bind addresses. A free port is available with `go run . -addr 127.0.0.1:0`.

For a reusable executable:

```sh
go build -o gobook .
./gobook
```

The browser layout adapts to small screens. The Go compiler and server run on a Go-enabled host; this is not a native phone application.

| Capability | Current scope |
| --- | --- |
| Native computation | Go 1.25+ compiler on the host |
| Browser UI | Responsive HTML/CSS/JavaScript; no external assets |
| Linux | Build and kernel/API tests verified locally |
| macOS / Windows | Cross-compilation verified for Windows amd64 and macOS arm64; runtime behavior not verified locally |
| Mobile / tablets | Browser client through a user-managed HTTPS reverse proxy; Go runs on the host |
| Install | Local executable plus web app manifest; browser installation support varies |
| AI / source Q&A | Not implemented; no backend connected |

## Use

The editor highlights Go keywords, strings, comments, numbers and function calls.
**Appearance settings** changes paper/dark theme, code size, indentation and line
wrapping. These settings stay in browser storage; source and host tokens are not
saved with them. Scrollbars remain visible with quieter colors.

The **DataFrame to trained model** example uses the real CC0 Palmer Penguins
dataset through **sheetframe v0.2.0**. It shows `Shape`, `Schema` and `Head(8)`,
explicitly counts exclusions for missing measurements, shuffles paired rows with
a fixed seed, fits only training rows and compares held-out errors with a
training-mean baseline. [Source, split, results and limitations](docs/examples/penguins.md).
The tables, model, predictions and diagnostic charts can all be inspected or saved.


1. Choose **Describe a sample**, **Explore a relationship**, **Inspect a distribution**, or **Train & save a model**. Loading an example never runs it.
2. Replace the synthetic sample data with your own, and give the notebook a title and note.
3. List package imports, separated by commas. Use package paths without aliases.
4. Write Go statements in cells, then choose **Run all**.
5. Save JSON in your browser; **Open JSON** loads a notebook without running it.

Use Ctrl/Cmd+Enter to run all cells, or Shift+Enter inside a cell to run through it. Tab indents code; Shift+Tab moves keyboard
focus out of the editor. Cell arrows reorder execution. Editing during a run
keeps its output tied to the earlier source snapshot.

All code cells are concatenated inside one `func main()`. Variables declared in an earlier cell are available below it. Imports belong in the explicit imports field; top-level function/type declarations can be written as local function values/types where Go permits. Standard Go compile rules apply, including unused imports. Build errors identify `cell1.go`, `cell2.go`, and so on.

Every run compiles and launches a fresh process in a fresh temporary directory. **Run all repeats every cell and every side effect.** Each cell also has **Run to here**: it executes cells 1 through that cell in a fresh process. The whole notebook is compiled, so syntax/type errors in later cells can still block a run. Partial runs require complete statements in each cell and do not support `goto`; use Run all for code with cross-cell blocks or jumps. This keeps Go imports and references valid; it is not a persistent REPL or isolated cell execution. There is no interrupt button or live output streaming.

Use **Notebook / Code / Results** to show inline outputs, code only, or aggregate results. On small screens the notebook keeps output beside its source. Figure expansion supports keyboard dismissal. Results and figures can be downloaded individually.

### Debug a notebook

Install the optional [Delve](https://github.com/go-delve/delve) CLI, or choose its
path with `gobook -delve /absolute/path/to/dlv`. Choose **Debug**, select a cell and
executable line, then **Start debugging**. Continue, Next, Step in, Step out, Pause
and Stop operate on a real Delve session. Local variables, arguments, call stack,
source location and captured output update while you work. Normal notebook runs
do not require Delve. Debug builds disable compiler optimizations and are not a
performance measurement.

Code and attached inputs stay locked to the debugging snapshot. After a page
reload, an existing session can be stopped; its source is not restored into the
new page, and stepping is disabled there to avoid showing misleading locations.
Sessions expire after 15 minutes. Stop or normal server shutdown cleans up the
inferior process and temporary files. Platform debugging permissions can prevent
launch; GoBook reports that error rather than simulating a debugger. Real runtime
verification was performed on Linux; macOS/Windows debug runtime is not claimed.
See [debugger details](docs/debugger.md).

### Save and replay an experiment

**Save notebook** downloads the current source, dependency pins and input files. **Save experiment** downloads a versioned bundle containing the source snapshot used by the last run, its inputs, captured output, generated files and compiler/module provenance. If you edited during execution, the experiment still contains the earlier source. Later edits belong to Save notebook. Downloaded files are your durable copy; no server history or automatic backup is provided.

**Open JSON** accepts either format. Imported results are labeled historical and unverified by the current host; opening them never executes code. Run details include UTC timestamps, compiler/platform, generated `go.mod`/`go.sum`, timeouts and build/run durations. Durations are diagnostics, not controlled benchmark results. A recorded module lock is provenance, not an enforced offline replay environment.

### Dependencies and input data

Open **Experiment settings** and specify direct module versions:

```json
{"github.com/Alifarid0011/gobook": "v0.2.0"}
```

Versions must be exact Go semantic versions. Branches, `latest`, local paths and conflicting transitive upgrades are rejected. Pins take priority over built-in chartframe/Gonum defaults. Builds run with `GOWORK=off`, empty `GOFLAGS`, and `GOENV=off` so hidden workspace/modfile/overlay configuration cannot replace the recorded graph. Export cache, proxy and private-module settings as environment variables if needed; saved `go env -w` settings are not read. Downloading/compiling dependencies uses the host's network and permissions.

Attach up to eight `.csv` or `.json` inputs, at most 128 KiB decoded in total. Names must use letters, digits, underscores or dashes before the extension (maximum 59 characters); reserved device names and case-insensitive duplicates are rejected. Code reads them from its temporary directory. Notebook JSON and its complete execution request must fit 256 KiB.

### A complete small-model workflow

**Train & save a model** reads synthetic CSV, makes a documented ordered holdout, fits single-predictor OLS using only training rows, evaluates MAE/RMSE/R², saves `model.json`, reloads it, and exports `predictions.csv`. **Use a saved model** loads an attached `model.json` for new predictions. Generated data/model files have a download link and **Use as input** action. The current model owns only a small coefficient state; `Close` invalidates it. No resident transformer/GPU runtime is implemented.

See [tabular](tabular/README.md) for numerical behavior, lifecycle, schema, ownership and limitations. This is one transparent workflow, not a scikit-learn replacement.

### Charts and numerical packages

Clone [chartframe](https://github.com/Alifarid0011/chartframe) beside GoBook, or select its local module directory:

```sh
go run . -chartframe /absolute/path/to/chartframe
```

The default lookup is `../chartframe` relative to the launch directory. Open `examples/sine.gobook.json` to sample a sine function and render SVG. GoBook displays regular `.svg` and `.png` files from the run directory as images. Files are collected only from the directory's top level. Use ordinary relative filenames, for example `sine.svg`.

Imports under `gonum.org/v1/gonum/` add Gonum v0.17.0 to the temporary module. chartframe uses v0.1.0 unless a local module replacement is configured. Module downloads use the host's Go module cache/network configuration; a first run can take longer. Other third-party modules can be declared through exact dependency pins.

`examples/simulation.gobook.json` runs a reproducible Monte Carlo experiment with a fixed random seed. Its timing excludes compilation and makes no cross-language performance claim.

## Trust and limits

**Execute only code you trust. Notebook code has your full user permissions**, including access to files, environment credentials, subprocesses, and the network. A temporary working directory is not a security sandbox. The HTTP guards protect the local control endpoint; they do not restrict code that you choose to run.

The server checks the exact listening Host, rejects cross-origin requests, and requires a random bearer capability for execution. The private capability URL enables execution; the page keeps the token in tab session storage and removes it from the address bar. The static page shell itself is public on loopback and cannot execute code without the capability. The capability grants full host code execution. Share it only with people you trust to run code on that host. Keep the default loopback setup unless you intentionally configure the HTTPS proxy described below.

Execution is serial (another run receives HTTP 409). Requests are capped at 256 KiB. Captured stdout and stderr share a 1 MiB budget; further output is discarded and marked truncated. Build timeout is 60 seconds, execution timeout is 30 seconds, with `GOMAXPROCS=2` and compiler `-p 2`. These reduce normal resource use; they are not CPU/memory/disk security quotas. POSIX process groups are killed on timeout and after execution; code can deliberately escape this mechanism. Windows tree termination uses `taskkill` and remains unverified locally.

Up to 16 generated SVG, PNG, JSON or CSV files and 8 MiB total are returned per run. Unmodified input files are excluded. Non-regular files and symlinks are skipped. Images use data URLs in `<img>` elements; SVG and notebook output are never inserted as active HTML. There is no arbitrary filesystem browsing API. Temporary run files are removed after collection.

## Other devices and browser installation

For a phone or another computer, keep the kernel on a Go-enabled host and configure your own HTTPS reverse proxy (with a valid trusted certificate) forwarding to the loopback listener:

```sh
go run . -addr 127.0.0.1:8080 -public-origin https://book.example
```

Replace `book.example` with the exact host you control. The proxy must preserve the public `Host` and browser `Origin` headers and forward to `127.0.0.1:8080` on the same host. Configure proxy timeouts to allow a 90-second build/run request. `-public-origin` accepts one HTTPS origin, without a path, credentials, query, or fragment; it does not set up DNS, certificates, authentication at the proxy, or networking for you. Forwarded headers are not trusted. Direct listening remains loopback-only, even in this mode. Keep the private capability URL secret and avoid access logs that record its query string.

Open the printed proxy capability URL on the other device. The browser executes only the interface; all Go code still runs with full permissions on the host. The header's **Connect** action accepts that private URL or token when starting a new installed-app session. Capabilities live for the server process lifetime and must be entered again after a restart.

A manifest and SVG icon let supporting browsers offer **Install app** or their own add-to-home-screen flow. The install button appears when the browser supplies an installation prompt. HTTPS (or localhost) is required. Installation support varies across browsers and operating systems; native mobile execution is not provided. No service worker or offline cache is installed. Execution requires the host and proxy to be running and reachable; notebook source and API requests are never cached by a service worker.

## Development

```sh
go test -p 2 ./...
go vet ./...
go build ./...
```

Tests exercise real compilation, syntax errors, bounded output, timeouts, artifact handling, imports, JSON round trips, and API request guards. CI is configured for Go 1.25 on Linux, macOS, and Windows; configured jobs are not evidence that those jobs have run.

## Extend GoBook

Use ordinary versioned Go modules. The [library guide](docs/extensions.md) explains
dependency pins, table/image/data outputs, model schemas and cleanup. This release
does not implement Jupyter kernel protocol compatibility, arbitrary live-value migration,
Markdown cells, interactive widget packages or Jupyter's extension ecosystem.

## Research and citation

The [research protocol](docs/research/manuscript.md) defines reproducibility and lifecycle experiments. It is a working draft by **Ali Farid**, not a peer-reviewed results paper. It reports no language ranking or new performance result. [Raw tabular microbenchmarks](tabular/benchmarks-go1.26.8-linux-amd64.txt) describe one development machine; they are not cross-language evidence.

A [citation file](CITATION.cff) identifies this software and its author. Source code is MIT-licensed; do not assume that imported datasets or model weights share that license.

MIT license · Ali Farid

### Inline outputs and debugging workspace

Normal runs show text, stderr, supported tables and figures below each originating
cell. Cells compile and execute together once, preserving shared variables without
repeating side effects. File ownership follows the last observed content change;
abrupt exits and cross-cell control structures can leave aggregate-only output.
Editing source labels previous inline outputs as stale.

Click a line number to choose a breakpoint. The source-adjacent debug dock follows
the current line and displays local values; changed values are highlighted briefly.
Use F8 to continue/pause, F10 to step over, F11 to step in and Shift+F11 to step out.
The dock adapts to a stacked layout on narrow displays.

See [capabilities and remaining Jupyter work](docs/jupyter-compatibility.md) for
exact behavior and the next milestones. Full Jupyter parity is not implemented.


### Native memory sessions

Open **Kernel → Start memory kernel** to compile once and retain native values
between cell commands. Try **Memory & goroutines**: repeat cell 2 to increment a
counter without reallocating the shared slice; run cell 3, then cell 2 to observe
its mutation. The example uses a synchronized worker pool with ordered results.

Editing execution inputs requires an explicit restart. Undoing those edits allows
the original worker to continue. Stop, a crash, or a command timeout discards RAM;
saved experiments contain source, results and command history, not a memory image.
See [native execution semantics](docs/native-memory.md) before relying on repeated
side effects or background work. **Cell graph** shows conservative lexical
relationships; it does not schedule cells concurrently.

[Portable Linux and Windows packages](docs/desktop.md) launch the same workspace
in your browser. They require Go on the host; native dependencies remain the
responsibility of the imported modules.
