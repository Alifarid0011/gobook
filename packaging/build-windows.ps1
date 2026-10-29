param(
    [Parameter(Mandatory)][ValidatePattern('^v[0-9]+\.[0-9]+\.[0-9]+([-.][A-Za-z0-9.-]+)?$')][string]$Version,
    [Parameter(Mandatory)][ValidateSet('amd64', 'arm64')][string]$Arch
)
$ErrorActionPreference = 'Stop'
Set-Location (Split-Path $PSScriptRoot -Parent)
$name = "gobook-$Version-windows-$Arch"
$stage = Join-Path 'dist' $name
New-Item -ItemType Directory -Force $stage | Out-Null
$env:CGO_ENABLED = '0'
$env:GOOS = 'windows'
$env:GOARCH = $Arch
& go build -trimpath -o "$stage/gobook.exe" .
if ($LASTEXITCODE -ne 0) { throw 'GoBook build failed' }
Copy-Item packaging/windows/* $stage
Copy-Item docs/desktop.md "$stage/README.md"
Copy-Item LICENSE $stage
$archive = "dist/$name.zip"
Compress-Archive -Path $stage -DestinationPath $archive -Force
$hash = (Get-FileHash $archive -Algorithm SHA256).Hash.ToLowerInvariant()
[IO.File]::WriteAllText("$archive.sha256", "$hash  $name.zip`n", [Text.Encoding]::ASCII)
