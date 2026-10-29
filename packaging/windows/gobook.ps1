$ErrorActionPreference = 'Stop'
if (-not (Get-Command go -ErrorAction SilentlyContinue)) {
    Write-Error 'Go compiler not found. Install Go 1.25 or newer from https://go.dev/dl/ and add go to PATH.'
    exit 1
}
& (Join-Path $PSScriptRoot 'gobook.exe') -open -addr '127.0.0.1:0' @args
exit $LASTEXITCODE
