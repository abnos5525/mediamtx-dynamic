# Download MediaMTX standalone for Windows amd64.
# See: https://mediamtx.org/docs/kickoff/install#standalone-binary
$ErrorActionPreference = "Stop"
$ver = if ($env:MTX_VERSION) { $env:MTX_VERSION } else { "v1.21.1" }
$zip = "mediamtx_${ver}_windows_amd64.zip"
$url = "https://github.com/bluenviron/mediamtx/releases/download/$ver/$zip"
$root = Split-Path -Parent $PSScriptRoot
$out = Join-Path $root $zip
Write-Host "Downloading $url"
Invoke-WebRequest -Uri $url -OutFile $out
Expand-Archive -Path $out -DestinationPath $root -Force
Remove-Item $out -Force
Write-Host "Done: $(Join-Path $root 'mediamtx.exe')"
