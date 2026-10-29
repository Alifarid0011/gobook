$ErrorActionPreference = 'Stop'
$root = Split-Path $PSScriptRoot -Parent
$temp = Join-Path ([IO.Path]::GetTempPath()) ('gobook launcher ' + [guid]::NewGuid())
New-Item -ItemType Directory $temp | Out-Null
$oldPath = $env:PATH
$oldArgs = $env:GOBOOK_TEST_ARGS
try {
    Copy-Item "$PSScriptRoot/windows/gobook.ps1" $temp
    @'
package main
import ("encoding/json"; "os")
func main() { b, _ := json.Marshal(os.Args[1:]); if err := os.WriteFile(os.Getenv("GOBOOK_TEST_ARGS"), b, 0600); err != nil { panic(err) } }
'@ | Set-Content "$temp/stub.go"
    & go build -o "$temp/gobook.exe" "$temp/stub.go"
    if ($LASTEXITCODE -ne 0) { throw 'Could not build launcher test fixture' }
    $env:GOBOOK_TEST_ARGS = "$temp/args.json"
    $shell = (Get-Command powershell.exe).Source
    & $shell -NoProfile -ExecutionPolicy Bypass -File "$temp/gobook.ps1" -chartframe 'a path with spaces'
    if ($LASTEXITCODE -ne 0) { throw 'Windows launcher failed' }
    $actual = Get-Content "$temp/args.json" -Raw | ConvertFrom-Json
    $expected = @('-open', '-addr', '127.0.0.1:0', '-chartframe', 'a path with spaces')
    if (($actual | ConvertTo-Json -Compress) -cne ($expected | ConvertTo-Json -Compress)) { throw 'Launcher did not preserve arguments' }
    $env:PATH = $temp
    $message = & $shell -NoProfile -ExecutionPolicy Bypass -File "$temp/gobook.ps1" 2>&1 | Out-String
    if ($LASTEXITCODE -eq 0 -or $message -notmatch 'Go compiler.*https://go.dev/dl/') { throw 'Missing Go compiler did not produce a useful failure' }
    Write-Host 'Windows launcher checks passed'
} finally {
    $env:PATH = $oldPath
    $env:GOBOOK_TEST_ARGS = $oldArgs
    Remove-Item -Recurse -Force $temp
}
