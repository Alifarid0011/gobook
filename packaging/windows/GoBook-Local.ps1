$ErrorActionPreference = 'Stop'
try {
    $settings = Get-Content (Join-Path $PSScriptRoot 'launcher.json') -Raw | ConvertFrom-Json
    $env:PATH = $settings.goDirectory + ';' + $env:PATH
    & (Join-Path $PSScriptRoot 'gobook.exe') -desktop @args >> (Join-Path $PSScriptRoot 'host.log') 2>&1
    if ($LASTEXITCODE -ne 0) { throw 'The local host could not start. See host.log in the GoBook installation folder.' }
} catch {
    Add-Type -AssemblyName System.Windows.Forms
    [System.Windows.Forms.MessageBox]::Show($_.Exception.Message, 'GoBook Local') | Out-Null
    exit 1
}
