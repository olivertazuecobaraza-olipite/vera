# Instala "vera" (y el alias "ia") en Windows (en %LOCALAPPDATA%\Programs\vera) y lo anade al PATH del usuario.
# Uso (en PowerShell, desde esta carpeta):
#   powershell -ExecutionPolicy Bypass -File .\install.ps1

$ErrorActionPreference = "Stop"
$arch = if ($env:PROCESSOR_ARCHITECTURE -eq "ARM64") { "arm64" } else { "amd64" }
$src  = Join-Path $PSScriptRoot "dist\vera-windows-$arch.exe"
if (-not (Test-Path $src)) { $src = Join-Path $PSScriptRoot "vera-windows-$arch.exe" }
if (-not (Test-Path $src)) { throw "No encuentro vera-windows-$arch.exe junto a este script ni en dist\" }

$dest = Join-Path $env:LOCALAPPDATA "Programs\vera"
New-Item -ItemType Directory -Force -Path $dest | Out-Null
Copy-Item $src (Join-Path $dest "vera.exe") -Force
Unblock-File (Join-Path $dest "vera.exe")
# "ia" es un alias: una copia del mismo binario (los enlaces en Windows requieren permisos de administrador).
Copy-Item (Join-Path $dest "vera.exe") (Join-Path $dest "ia.exe") -Force

$userPath = [Environment]::GetEnvironmentVariable("Path", "User")
if (($userPath -split ";") -notcontains $dest) {
    [Environment]::SetEnvironmentVariable("Path", ($userPath.TrimEnd(";") + ";" + $dest), "User")
    Write-Host "Anadido $dest al PATH. Abre una terminal nueva para usar 'vera' o 'ia'."
}
Write-Host "OK: instalado en $dest\vera.exe (y ia.exe)"
