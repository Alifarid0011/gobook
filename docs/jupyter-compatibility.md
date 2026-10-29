# Notebook capabilities and remaining work

GoBook is a Go computational notebook under active development. It does not yet
provide all Jupyter functionality. This checklist is a product scope statement,
not a claim of compatibility with a particular Jupyter release.

| Capability | Current behavior |
| --- | --- |
| Code cells and shared variables | Editable retained interpreter, or compiled Go main function in Native mode |
| Output under each cell | Text, stderr, supported figures/tables/files; one run, no repeated side effects |
| Run cell | Auto-starts resident kernel, sends exactly one command; missing names/prerequisites are reported without replay |
| Run all | Explicit sequential execution in resident memory; legacy fresh process available in Kernel controls |
| Interactive debugger | Real Delve, gutter breakpoint selection, source line, local values, stack, stepping and pause |
| DataFrames and plots | Versioned Go modules, sheetframe example, chartframe figures |
| Dependency provenance | Exact direct module versions, recorded go.mod/go.sum and compiler metadata |
| Save and reopen experiments | GoBook JSON bundles; imported results are explicitly unverified |
| Persistent kernel | Interactive mode retains values through code edits; Native mode requires restart after source edits |
| Cell graph | Conservative lexical dependencies; no automatic parallel cell scheduler |
| Desktop packages | Linux/Windows portable hosts and browser workspace; local Go required |
| Markdown/math cells | Not implemented |
| Go language server completion | Bundled gopls with typed source-based suggestions; local lexical/stdlib fallback |
| Jupyter kernel protocol and ipynb | Not implemented |
| Interactive widget protocol | Not implemented |
| Notebook extensions | Ordinary Go modules are supported; no Jupyter or browser plugin API |
| Collaboration/version history | Not implemented |

## Output attribution

For complete-statement cells, GoBook inserts random per-run boundaries into a
single generated program. Text belongs to the cell active when it is written;
background goroutines can therefore emit under a later cell. Deferred output
belongs to the final active cell. Cross-cell blocks or goto keep their original
aggregate behavior, because inserting boundaries could change control flow.

Files are assigned to the cell in which their content last changed at an observed
boundary, using bounded SHA-256 snapshots. Only final versions are retained.
Abnormal termination such as os.Exit can skip the final snapshot; unmatched files
remain in Other files instead of receiving a guessed owner. Large output is
truncated, and a timeout can leave a final partial cell output. These are execution
observations, not a sandbox or a proof of code provenance.

Each bundle contains aggregate outputs for backward compatibility plus optional
cell results. Editing the source labels inline results as belonging to the prior
run. Editing while a run is in progress keeps the run's output in Results view,
without attaching it to modified cells. Opening a bundle never executes its code.

## Next milestones

1. Markdown/math cells and a documented notebook schema migration.
2. Richer kernel inspection and cancellation without discarding the whole worker.
3. Extend gopls integration with diagnostics and navigation, and improve interpreter source modeling.
4. Jupyter messaging/ipynb interoperability and rich-display contracts.
5. Widget support and package examples validated with real research users.

Each milestone needs its own executable examples, compatibility tests and measured
resource behavior before it is advertised as supported. General AI training and
inference capability depends on the imported libraries and host runtimes; a
notebook interface alone does not make Go a replacement for an entire ecosystem.

Memory sessions have distinct [execution and retention rules](native-memory.md).
