# Debugging notebook cells with Delve

GoBook uses the real [Delve debugger](https://github.com/go-delve/delve) through
its [streaming JSON-RPC API v2](https://github.com/go-delve/delve/blob/master/Documentation/api/json-rpc/README.md).
Install a Delve release compatible with your Go toolchain, then put `dlv` on
`PATH`, or pass its executable path with `gobook -delve /path/to/dlv`.
Delve v1.27.2 works with Go 1.26. Debugging needs the operating system to permit
process tracing (`ptrace` on Linux); restrictive containers may refuse it.
Failures are reported in the debugger panel.

1. Set breakpoints using a cell's 1-based line numbers.
2. Start Debug. GoBook compiles the entire notebook with
   `-gcflags=all=-N -l` and continues to the first requested breakpoint. With no
   breakpoints it stops at the first executable location in `main.main`.
3. Use Continue, Next, Step, Step out, or Halt. Next steps over function calls;
   Step enters them; Step out returns to the caller. Halt interrupts a running
   command. Locals, function arguments, and up to 32 stack frames are inspected
   after the program pauses.
4. Stop ends Delve and its notebook process. Starting a new session recompiles
   the current notebook. Edit code and restart to debug the changed program.

`Run to here` does not limit debug builds. All cells share the normal generated
`main` function and Go's lexical scope; compiler line directives map paused
locations back to `cellN.go` and the corresponding cell/line. Some blank lines,
comments, declarations, or braces have no executable instruction and cannot be
breakpoints. Go may associate multiple statements with one machine location.
Library/runtime stack frames show their source filename and function without a
cell number. Variable displays are bounded to one level of children, 256 string
characters, 32 array elements, and 32 struct fields. Values that Delve cannot read
carry an `unreadable` message. Composite values include child fields.

Only one run or debugger session can own the kernel at a time. Commands return
immediately and the browser polls state while the target runs. A session has a
15-minute lifetime, including its build; normal build timeout still applies.
Stop, expiration, and server shutdown clean up the debugger process group and
temporary source directory. Captured stdout/stderr share the usual 1 MiB cap.
Dependency pins, supplied input file validation, and the generated module rules
are the same as Run. Compiler settings clear `GOFLAGS`, disable `GOENV`, and
set `GOWORK=off`.

Debugging runs notebook code with the host account's permissions, just like
Run. Delve listens on an ephemeral `127.0.0.1` port and is never published as an
HTTP endpoint. GoBook's debugger HTTP routes require the same bearer capability,
Host/Origin, and Fetch Metadata checks as Run. Delve's local RPC socket itself
has no authentication, so other processes running on this host must be trusted.

The browser uses these authenticated routes:

| Route | Request | Response |
| --- | --- | --- |
| `POST /api/debug/start` | `{"notebook":{...},"breakpoints":[{"cell":1,"line":2}]}` | Current snapshot |
| `POST /api/debug/command` | `{"command":"next"}` | Current snapshot |
| `GET /api/debug/state` | None | Current snapshot; an exited session releases the kernel |
| `POST /api/debug/stop` | Empty body or `{}` | Exited snapshot |

Commands are `continue`, `next`, `step`, `stepOut`, and `halt`. Snapshots include
`running`, `exited`, `cell`, `line`, `function`, `locals`, `arguments`, `stack`,
`breakpoints`, `stdout`, `stderr`, `truncated`, and optional `error`. Poll until
`running` becomes false before issuing another step. Stop is idempotent.
JSON bodies are limited to 256 KiB and reject unknown fields and trailing JSON.

Run the real debugger tests on a machine that permits tracing:

```sh
GOBOOK_DLV=/absolute/path/to/dlv go test -run 'TestDebug|TestDelve' -v
```

The integration tests also run automatically when `dlv` is on `PATH`; otherwise
only the portable protocol, validation, and HTTP checks run. Finding an installed
Delve and then failing to launch or trace the target fails the test, rather than
silently skipping it.

## Workspace controls

Select a breakpoint from the editor's line-number gutter. The debug dock stays
beside source on desktop and stacks above it on narrow screens. While paused,
the current source line is highlighted and the dock shows local values, arguments,
stack frames and output. Changed values are highlighted briefly. Start options
collapse during an active session to make room for inspection.

Shortcuts during a session: F8 continues/pauses, F10 steps over, F11 steps in,
and Shift+F11 steps out. Normal notebook runs and debugging use separate fresh
processes; prior cell outputs are cleared when starting a debug session.
