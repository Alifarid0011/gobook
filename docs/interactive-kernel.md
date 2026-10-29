# Editable Go cells

GoBook has two execution engines. Choose **Kernel → Engine** before starting.
The browser defaults to **Interactive**, and existing native API clients keep the
compiled engine unless they request `engine: "interactive"`.

| | Interactive | Native |
| --- | --- | --- |
| Execution | Pinned Yaegi v0.16.1 in a separate worker | Host Go compiler in a separate worker |
| Edit code without losing other values | Yes; run only the edited cell | Restart required |
| Add a cell without restarting | Yes | Restart required |
| Cell order | Any selected cell; required names must already exist | Previously reached cells or next cell |
| Functions | Named functions and function values, within interpreter support | Function values inside statement cells; imported Go functions |
| Libraries | Bundled standard-library symbols only | Normal versioned Go modules, subject to toolchain/platform |
| Generics / language compatibility | Interpreter limitations apply | Host compiler support |
| Recovered syntax/evaluation errors | Worker remains active; earlier values remain | Build errors block startup; runtime failures may terminate worker |

There is no Python process and no substitution of saved JSON for live memory.
Both workers run with your user permissions. They are not security sandboxes.
An `os.Exit`, fatal error, timeout, Stop or Restart ends the worker and loses its
RAM. A normal interpreter error does not automatically restart it.

## Try live editing

1. Keep imports `fmt` and choose Interactive.
2. In cell 1, run:

   ```go
   data := []int{4, 6}
   calls := 0
   sum := func() int { calls++; return data[0] + data[1] }
   fmt.Println("setup once")
   ```

3. In cell 2, run `value := sum(); fmt.Println(value, calls)`: `10 1`.
4. Change only cell 2 to `value := sum()+20; fmt.Println(value, calls)` and run it:
   `30 2`. Setup did not run again; the slice and captured counter survived.
5. Add a cell with `fmt.Println(value)` to use the previous return value.

Running a declaration again may replace that binding according to the
interpreter's semantics. A retained older closure can still refer to an older
binding. Changes of types, function definitions and concurrent code must not be
assumed to behave like recompiling an ordinary Go application. Interpreter
execution is not a native-speed performance claim.

Imports listed in the notebook are evaluated incrementally; removing an import
from the editor does not unload a package already used in the live interpreter.
Module pins and unsupported packages return an explicit Native-mode instruction.
Replacing input files or dependency configuration requires an explicit restart.
Stop before switching engines. Loading other notebook source does not roll back
memory or external effects of previously executed code.

## Results and reproducibility

Other cells' output stays visible when an interactive cell runs again. An edited
cell's older inline output is removed; a result arriving after its source was
edited remains in aggregate Results, without being attached to the changed cell.
A failed command can have completed some statements and effects: there is no
transactional rollback. Synchronize your goroutines when ordering matters.

Snapshots atomically pair session ID, accepted source, fingerprint, history and
results. Another page cannot execute using an outdated history revision. Saved
interactive bundles contain final submitted source, results and cell numbers,
not every intermediate edited revision or live RAM. Interactive history replay
is therefore disabled; execute the saved cells explicitly to reproduce them.

The standard text, artifact, timeout and history limits apply to both workers.
The native debugger remains a separate compiled session; stop the interactive
worker before launching it. It does not inspect the interpreter's live stack.
