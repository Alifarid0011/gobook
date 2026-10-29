# GoBook desktop packages

GoBook provides a portable native host executable for Linux and Windows on
amd64 (x64) and arm64. Its IDE-like workspace runs in your default browser.
The package includes the workspace assets; it does not embed a browser or ship
an operating-system installer.

## Requirements

Install [Go 1.25 or newer](https://go.dev/dl/) and make `go` available on PATH.
A browser is required. [Delve](https://github.com/go-delve/delve/tree/master/Documentation/installation)
is optional and needed only for interactive debugging. Packages do not download
or install these tools. GoBook's native compiled workers use your local Go
compiler. Notebook dependencies that use CGO or native libraries still need the
appropriate C toolchain, headers and libraries on your host; these are not bundled.

## Download and launch

Choose the archive matching your OS and processor from the GitHub release.
Keep all extracted files together in a writable folder you control.

Linux:

```sh
sha256sum -c gobook-vX.Y.Z-linux-amd64.tar.gz.sha256
tar -xzf gobook-vX.Y.Z-linux-amd64.tar.gz
cd gobook-vX.Y.Z-linux-amd64
./gobook.sh
```

Windows PowerShell:

```powershell
(Get-FileHash .\gobook-vX.Y.Z-windows-amd64.zip -Algorithm SHA256).Hash
Get-Content .\gobook-vX.Y.Z-windows-amd64.zip.sha256
Expand-Archive .\gobook-vX.Y.Z-windows-amd64.zip -DestinationPath .
.\gobook-vX.Y.Z-windows-amd64\GoBook.cmd
```

On Windows, compare the hash with the first field of the checksum file
(case does not matter). You can also double-click `GoBook.cmd` after extraction.
The wrapper invokes the included PowerShell script with a process-scoped
execution-policy setting; it does not change your saved execution policy.
Your organization's policy may still block scripts. In that case run
`gobook.exe -open -addr 127.0.0.1:0` in a terminal with Go on PATH.

The launcher selects a free localhost port and opens the private capability URL
in your default browser. If no browser opens, copy the URL printed in the
terminal. Keep that terminal open while working; press Ctrl+C to stop the host.
Each launch creates a new token and may use a different port. Do not share the
URL: notebooks execute local code with your full user permissions.

Arguments pass through to the host, for example `./gobook.sh -open=false` or
`GoBook.cmd -addr 127.0.0.1:8080`. A fixed port can be useful for a browser's
"Install app" or "Create shortcut" feature where supported. Such a browser/PWA
window still needs the GoBook host running; it is not an embedded native IDE or
an offline replacement for the host. Open the new private URL after restarting
so the browser receives the current token.

To remove GoBook, stop it and delete its extracted folder. Notebook files you
saved and any browser shortcut/PWA are managed separately.

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
Launcher checks cover paths with spaces, argument forwarding and missing Go.
Only after all targets pass does the publishing job upload release assets.
These are correctness checks, not performance benchmarks. A local cross-build
alone does not establish native Windows or arm64 runtime support.
