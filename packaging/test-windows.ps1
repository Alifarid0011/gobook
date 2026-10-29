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
    if ($LASTEXITCODE -eq 0 -or $message -notmatch 'Go compiler' -or $message -notmatch 'https://go\.dev/dl/') { throw "Missing Go compiler did not produce a useful failure (exit $LASTEXITCODE): $message" }
    $env:PATH = $oldPath
    Copy-Item "$PSScriptRoot/windows/Install-GoBook.ps1", "$PSScriptRoot/windows/GoBook-Local.ps1" $temp
    $app = Join-Path $temp 'installed app'
    $shortcuts = Join-Path $temp 'start menu'
    & "$temp/Install-GoBook.ps1" -InstallDir $app -ShortcutDir $shortcuts
    & "$temp/Install-GoBook.ps1" -InstallDir $app -ShortcutDir $shortcuts
    $link = (New-Object -ComObject WScript.Shell).CreateShortcut((Join-Path $shortcuts 'GoBook Local.lnk'))
    if ($link.Arguments -notlike '*-WindowStyle Hidden*' -or $link.Arguments -notlike '*GoBook-Local.ps1*') { throw 'Installed shortcut does not launch local mode' }
    & $shell -NoProfile -ExecutionPolicy Bypass -File "$app/GoBook-Local.ps1" -open=false
    if ($LASTEXITCODE -ne 0) { throw 'Installed desktop launcher failed' }
    $actual = Get-Content "$temp/args.json" -Raw | ConvertFrom-Json
    $expected = @('-desktop', '-open=false')
    if (($actual | ConvertTo-Json -Compress) -cne ($expected | ConvertTo-Json -Compress)) { throw 'Desktop launch arguments changed' }
    Write-Host 'Windows launcher and installation checks passed'
} finally {
    $env:PATH = $oldPath
    $env:GOBOOK_TEST_ARGS = $oldArgs
    Remove-Item -Recurse -Force $temp
}
