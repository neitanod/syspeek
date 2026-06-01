#requires -Version 5.1
# Runs syspeek.exe (Windows equivalent of the bash `run` script).
# Builds the binary first if it does not exist yet.
$ErrorActionPreference = 'Stop'
Set-Location $PSScriptRoot

if (-not (Test-Path .\syspeek.exe)) {
    Write-Host "Binary not found, building first..."
    & "$PSScriptRoot\build.ps1"
}

& .\syspeek.exe @args
exit $LASTEXITCODE
