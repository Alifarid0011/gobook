# GoBook desktop packages

GoBook provides a portable native host executable for Linux and Windows on
amd64 (x64) and arm64. Its IDE-like workspace runs in your default browser.
The package includes the workspace assets and per-user shortcut installers. It
does not embed a browser, install a login service, or require administrator access.

## Requirements

Install [Go 1.25 or newer](https://go.dev/dl/) and make `go` available on PATH.
A browser is required. [Delve](https://github.com/go-delve/delve/tree/master/Documentation/installation)
is optional and needed only for interactive debugging. Packages do not download
or install these tools. GoBook's native compiled workers use your local Go
compiler. Notebook dependencies that use CGO or native libraries still need the
appropriate C toolchain, headers and libraries on your host; these are not bundled.

## Install the local application

Download and verify the archive for your OS and processor from
[GitHub Releases](https://github.com/Alifarid0011/gobook/releases/latest), then extract it.

Linux:

```sh
sha256sum -c gobook-v0.8.0-linux-amd64.tar.gz.sha256
tar -xzf gobook-v0.8.0-linux-amd64.tar.gz
cd gobook-v0.8.0-linux-amd64
./install.sh
```

The installer copies the executable and icon under `$XDG_DATA_HOME/gobook`
(default `~/.local/share/gobook`) and registers **GoBook Local** in your application
menu. It records the installed Go directory in its launcher, so the graphical
session does not need the same PATH as your terminal. No root privileges are used.

Windows PowerShell, from the extracted directory:

```powershell
(Get-FileHash .\gobook-v0.8.0-windows-amd64.zip -Algorithm SHA256).Hash
Get-Content .\gobook-v0.8.0-windows-amd64.zip.sha256
# Compare the hashes, then extract and run the installer in its directory:
.\Install-GoBook.ps1
```

The Windows installer copies the executable and launcher to
`%LOCALAPPDATA%\Programs\GoBook` and creates **GoBook Local** in the Start menu.
It captures the Go path and starts the host without a terminal window. If the
organization's execution policy blocks the script, use the portable command-line
mode or ask the administrator; do not change machine policy for this application.
The shortcut uses a process-scoped execution-policy setting and changes no saved
policy. Quit GoBook before replacing a running Windows executable during updates.

## Launch, reopen and quit

Open **GoBook Local** from the application menu. Its host starts automatically on
`127.0.0.1:43856`, and the launcher opens the current private capability URL in
your default browser. There is no external hosting account or manual token entry.
A second launch authenticates the running local instance and reopens that host;
it does not replace its worker. It can open another browser page. Unsaved browser
edits are not automatically copied between pages; save the notebook before closing
its page. Recovering an active kernel's notebook in another page remains explicit.

The workspace shows **Connected automatically · runs on this computer**. Use
**Quit GoBook** to terminate the host and its kernels. Save the notebook and export
models first: quitting discards live RAM. Closing a browser window leaves the host
running; use the same menu entry to reopen it. No automatic start at OS login is
installed. A browser-installed PWA shortcut cannot boot a stopped HTTP host; use
**GoBook Local**, rather than the older browser shortcut, for automatic startup.

Compiler and notebook temporary directories use the user cache under `gobook/tmp`,
rather than a possibly memory-limited system `/tmp`. Standard Go build and module
caches remain controlled by Go and its cache environment settings.

The desktop capability file lives under the user's config directory in
`gobook/desktop.url`, with a private directory and file mode on Unix and inherited
user-profile permissions on Windows. It retains the last URL after quit; this stale record cannot authenticate to a
stopped or replaced host. A new launch atomically replaces it with a fresh token. Never share it or `host.log`; both can
contain a private URL that authorizes execution as your user. No authentication is
disabled for desktop mode. Remote proxies cannot access desktop lifecycle controls.

If the default port is occupied by something other than the authenticated GoBook
host, the launcher refuses to connect. Stop the conflicting application or use
`gobook -desktop -addr 127.0.0.1:PORT`. Logs are in the Linux installation's
`host.log` or `%LOCALAPPDATA%\Programs\GoBook\host.log` on Windows.

## Portable command-line mode

`./gobook.sh` on Linux or `GoBook.cmd` on Windows retains the foreground portable
mode. Keep that terminal open and use its printed private URL. This mode may use
a random port and does not provide desktop quit controls.

For direct use, `gobook -desktop` starts or reopens desktop mode;
`gobook -desktop -open=false` suppresses browser launch. `-desktop-dir DIR` selects
an isolated private state directory, useful for tests. Set a different explicit
loopback port when using more than one isolated desktop profile on a machine.

To uninstall, first quit GoBook, then remove its installation folder and the
GoBook Local shortcut (`$XDG_DATA_HOME/applications/io.github.Alifarid0011.GoBook.desktop`
on Linux, or the Start menu shortcut on Windows). Saved notebook files remain
where you saved them. Remove a separately installed browser/PWA shortcut in the
browser's application settings if desired.

## Building and release checks

From a checkout with Go installed, run:

```sh
bash packaging/build-linux.sh vX.Y.Z amd64
```

On Windows, run `./packaging/build-windows.ps1 -Version vX.Y.Z -Arch amd64`.
Use `arm64` for that processor. Output is under `dist/`, with a SHA-256 file
beside each archive. The host itself is built with CGO disabled; this does not
change the native-library requirements of notebooks compiled at runtime.

The tag release workflow tests natively on Linux and Windows for both
architectures, including opt-in native worker integration tests. It extracts
each archive and verifies that its executable serves the workspace, rejects
unauthenticated execution, and compiles/runs a small notebook using local Go.
It also starts desktop mode, verifies authenticated status, reopens the same host,
quits through the API, and verifies cancellation of active notebook execution.
Launcher checks cover paths with spaces, argument forwarding and missing Go.
Only after all targets pass does the publishing job upload release assets.
These are correctness checks, not performance benchmarks. A local cross-build
alone does not establish native Windows or arm64 runtime support.
