# Package the Windows Flutter release output into a zip.
# The Flutter Windows build emits the exe + runtime DLLs directly into
# app\build\windows\x64\runner\Release; we zip that whole directory.
#
# Run on a Windows host (pwsh). Env: GITHUB_WORKSPACE (repo root, set by GitHub
# Actions; falls back to the current directory).
#
# Produces: dist\omniproxy-${Version}-windows-x64.zip
param(
    [Parameter(Mandatory = $true)][string]$Version
)

$ErrorActionPreference = 'Stop'

$Workspace = if ($env:GITHUB_WORKSPACE) { $env:GITHUB_WORKSPACE } else { (Get-Location).Path }
$ReleaseDir = Join-Path $Workspace 'app\build\windows\x64\runner\Release'

if (-not (Test-Path $ReleaseDir)) {
    throw "Windows release dir not found: $ReleaseDir"
}

$DistDir = Join-Path $Workspace 'dist'
New-Item -ItemType Directory -Force -Path $DistDir | Out-Null

$ZipPath = Join-Path $DistDir "omniproxy-${Version}-windows-x64.zip"
Compress-Archive -Path (Join-Path $ReleaseDir '*') -DestinationPath $ZipPath

if (-not (Test-Path $ZipPath)) {
    throw "Packaging failed: $ZipPath was not created"
}

Write-Output "==> packaged $ZipPath"
Get-ChildItem -Path $DistDir | Select-Object Name, Length | Format-Table -AutoSize
