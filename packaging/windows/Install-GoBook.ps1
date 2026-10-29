param(
    [string]$InstallDir = (Join-Path $env:LOCALAPPDATA 'Programs\GoBook'),
    [string]$ShortcutDir = (Join-Path ([Environment]::GetFolderPath('Programs')) 'GoBook')
)
$ErrorActionPreference = 'Stop'
$compiler = Get-Command go -CommandType Application -ErrorAction SilentlyContinue
if (-not $compiler) { throw 'Install Go 1.25+ from https://go.dev/dl/ and add it to PATH first.' }
New-Item -ItemType Directory -Force $InstallDir, $ShortcutDir | Out-Null
$InstallDir = (Get-Item $InstallDir).FullName
$ShortcutDir = (Get-Item $ShortcutDir).FullName
if ($InstallDir -ne $PSScriptRoot) {
    Copy-Item "$PSScriptRoot/gobook.exe" "$InstallDir/gobook.exe" -Force
    if (Test-Path "$PSScriptRoot/gopls.exe") { Copy-Item "$PSScriptRoot/gopls.exe" "$InstallDir/gopls.exe" -Force }
    if (Test-Path "$PSScriptRoot/THIRD_PARTY_NOTICES.txt") { Copy-Item "$PSScriptRoot/THIRD_PARTY_NOTICES.txt" "$InstallDir/THIRD_PARTY_NOTICES.txt" -Force }
    Copy-Item "$PSScriptRoot/GoBook-Local.ps1" "$InstallDir/GoBook-Local.ps1" -Force
}
@{ goDirectory = (Split-Path $compiler.Source -Parent) } | ConvertTo-Json | Set-Content "$InstallDir/launcher.json" -Encoding UTF8
$shell = New-Object -ComObject WScript.Shell
$shortcut = $shell.CreateShortcut((Join-Path $ShortcutDir 'GoBook Local.lnk'))
$shortcut.TargetPath = (Get-Command powershell.exe).Source
$shortcut.Arguments = '-NoProfile -WindowStyle Hidden -ExecutionPolicy Bypass -File "' + (Join-Path $InstallDir 'GoBook-Local.ps1') + '"'
$shortcut.WorkingDirectory = $InstallDir
$shortcut.WindowStyle = 7
$shortcut.Description = 'Go notebooks with an automatically managed local host'
$shortcut.Save()
Write-Host 'Installed GoBook Local. Open it from the Start menu. Quit GoBook before upgrading.'
