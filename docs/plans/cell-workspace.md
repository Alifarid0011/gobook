# Cell workspace implementation plan

Goal: show captured output below its originating code cell and make real Delve debugging usable beside that source.

Architecture: keep one compilation and one process, preserving shared Go scope. Add optional cell-boundary instrumentation with per-run random identifiers, split stdout/stderr at boundaries, and snapshot bounded artifact hashes. Keep aggregate results backward compatible. Cross-cell control structures keep aggregate output instead of guessing ownership. Debug builds continue using the original source.

- Add regression tests for shared scope, partial runs, stderr, failure, overwritten artifacts and legacy cross-cell programs.
- Implement bounded capture and attribution without evaluating cells multiple times.
- Render cell outputs safely, preserve export/import compatibility, and label stale results.
- Move the debugger into a source-adjacent dock; add accessible gutter breakpoint selection and current-line highlighting.
- Verify Go tests, browser workflows and responsive layout, document the remaining Jupyter compatibility work, then publish.

Limits: this is not a persistent kernel. Background goroutine output belongs to the active cell; outputs after the final cell belong to that final cell. Generated artifacts belong to their last changed cell. No Jupyter protocol or widget compatibility is claimed.
