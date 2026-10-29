# Editable interactive kernel

The user selected a separate interpreted kernel for live editing on Linux and Windows, while retaining the native compiled kernel. This choice supersedes the earlier acceptance of restart after source edits for the interactive path.

## Approach

Use a pinned Yaegi interpreter in a dedicated child process, not in the HTTP host. Reuse the existing authenticated loopback worker transport, bounded console capture, artifact collection, cancellation and session/history compare-and-swap. A kernel engine selector makes the execution model visible. Existing API calls without an engine continue to select the native kernel.

The interactive worker evaluates exactly the selected cell in its persistent interpreter. It receives the current cell source at execution time. Cells can be edited or added without rebuilding the worker. Variables, closures and return values remain in interpreter memory. Re-running declarations follows the interpreter's tested behavior. Earlier cells are never replayed automatically. Imports are resolved only from bundled standard-library symbols in the first release; module pins and third-party imports must produce an explicit unsupported message. Native mode remains available for normal Go modules, generics and compilation compatibility.

## Failure and lifetime

Syntax/evaluation errors are returned as command results while the worker remains available. There is no rollback of statements or external effects already executed. Fatal worker exits and command timeouts end its RAM lifetime. Stop/Restart explicitly discard memory. Input files cannot change under an existing worker; restart is required for input replacement. Engine switching also requires Stop. Old pages are rejected using the same session/history guards.

## Validation

Test edit and re-run while a retained allocation and captured function counter survive; use a function's return value from another cell; add a cell; syntax error followed by valid access to retained state; named functions; unsupported dependency diagnostics; isolation of os.Exit; timeouts and stop. Verify native regressions and browser workflow, then platform CI.

## Alternatives

Native plugins could retain data directly but are unsupported on Windows, require identical compiler/dependency builds and cannot unload code. Serialize/rebuild cannot preserve arbitrary Go functions, pointer identity or goroutines. Neither meets the selected cross-platform interactive behavior.

## Limits

This is an opt-in/clearly identified interpreter, not arbitrary native hot reload. It cannot promise all current Go language or third-party library features. No Python dependency is introduced. Native mode remains separately identified. Interpreted computation has different performance characteristics and requires measurement, not native-speed claims.
