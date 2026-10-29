# Native memory sessions

This page describes **Native** mode. For code edits without restarting, choose
**Interactive** mode and read [editable cells](interactive-kernel.md).

GoBook compiles the whole notebook into one Go main function and pauses a
resident worker between complete-statement cells. This keeps ordinary slices,
maps, pointers, handles and goroutines alive without interpreting Go or replaying
previous cells. Select Native before starting; a cell Run or Run all then automatically starts it. Use the **Kernel** panel to stop, restart or inspect it.

1. Load **Memory & goroutines** and start the memory kernel.
2. Run cells 1 and 2. Repeat cell 2: its counter increases and the slice address
   stays the same. Prior allocation does not execute again.
3. Run cell 3, then cell 2. The synchronized worker pool sees the changed value.
4. Edit source: execution is disabled until you undo that edit or explicitly
   restart. Export useful datasets and models before restarting.

## Order and scope

Only previously reached cells or the next cell can execute. Requesting an unavailable later
cell is rejected without running its prerequisites. Run those cells explicitly first. Repeating an earlier cell invalidates later
outputs and the execution frontier; it does not roll back mutations, files,
external side effects or background goroutines. **Run all** explicitly repeats
every cell in the same worker. It does not implicitly reset memory.

All cells must compile, including later cells. Cells contain complete Go statements
inside one function; cross-cell blocks and goto are unsupported in memory mode.
Use function literals or imported packages for reusable functions and types with
methods. Normal Go modules, generics and CGO remain subject to the local toolchain
and system libraries. This is not a general source-redefinition REPL.

`defer` belongs to the enclosing function, not a cell. Repeating a cell can
accumulate defers and goroutines. Synchronize work before relying on a result;
background output is attributed to whichever cell is active when it arrives.
Stopping forcibly terminates the worker and does not promise defer execution.
A command timeout or process failure also ends the worker. No idle expiry is
currently imposed; stop a session when it is no longer needed.

## Results and saved work

Outputs appear below cells and are bounded to 1 MiB combined displayed text.
Canonical session files remain available when an unchanged file is not emitted
again; an observed deletion removes the file. Up to 16 supported artifacts and
8 MiB of decoded file data are collected per scan. A file can outlive the cell
output that created it. The Kernel panel lists retained files, including those
whose originating cell is no longer known on this page.

A saved experiment contains source, module provenance, bounded outputs and command
history. It contains no live RAM, pointers or goroutines. Opening it executes
nothing. **Replay saved session** explicitly executes its commands again, including
side effects. Replay is not proof of deterministic reproduction.

One host has one memory worker. A session identifier and expected history length
reject stale commands from another page. Refreshing status after another page
executes or replaces the worker clears cached cell outputs; restoring the kernel
notebook is explicit. Earlier cell stdout is not reconstructed after page reload.
At most 1,000 commands are retained; restart before executing further commands.

The displayed heap value is Go heap allocation sampled at the last cell boundary,
not total resident process RAM. Goroutine count is sampled at that same boundary.
The cell graph follows lexical reads and writes. Calls, pointers, aliases,
closures, external effects and goroutine ordering require your judgment; it is
not a proof that cells can execute in parallel.

Code executes with the host user's permissions. The private loopback control
channel coordinates execution; it is not an isolation boundary for notebook code.

## Single-cell defaults and functions

Run cell and Shift+Enter execute exactly one command in the resident worker.
Run all explicitly executes every cell in order. The older fresh-process path
is available as **Kernel → Run all in a fresh process** after stopping a kernel.
The legacy `/api/run` endpoint remains a fresh-process API for existing clients.

Function values and closures share native scope with earlier values. Load
**Shared functions & data** for a tested allocation, function-call counter,
return value and later mutation. Define a closure with `name := func(...) ...`;
package-level named function declarations are outside the current cell syntax.

The initial build still compiles the whole notebook once. Source edits and added
cells require a restart. This is a native compiled kernel with an execution
frontier, not arbitrary out-of-order Jupyter evaluation or live code redefinition.
