#requires -Version 5.1
# Compiles syspeek.exe (Windows equivalent of the bash `build` script).
$ErrorActionPreference = 'Stop'
Set-Location $PSScriptRoot

Write-Host "Building syspeek.exe..."
& go build -o syspeek.exe .
if ($LASTEXITCODE -ne 0) { throw "go build failed with code $LASTEXITCODE" }
Write-Host "Build succeeded: $PSScriptRoot\syspeek.exe"
