# Instala vera (y el alias «ia») en Windows, en %LOCALAPPDATA%\Programs\vera, y lo añade al PATH del usuario.
#
#   irm https://raw.githubusercontent.com/olivertazuecobaraza-olipite/vera/main/install.ps1 | iex
#
# Opciones: $env:VERA_VERSION = "1.2.0" para instalar una versión concreta.
# Desde una copia del repositorio, si existe dist\vera-windows-<arq>.exe (./build.sh), se instala ese binario:
#   powershell -ExecutionPolicy Bypass -File .\install.ps1

$ErrorActionPreference = "Stop"
$ProgressPreference = "SilentlyContinue" # Invoke-WebRequest es mucho más rápido sin barra
$repo = "olivertazuecobaraza-olipite/vera"
$arch = if ($env:PROCESSOR_ARCHITECTURE -eq "ARM64") { "arm64" } else { "amd64" }
$dest = Join-Path $env:LOCALAPPDATA "Programs\vera"
$tmp = Join-Path ([IO.Path]::GetTempPath()) ("vera-" + [Guid]::NewGuid())
New-Item -ItemType Directory -Force -Path $tmp | Out-Null

try {
    $src = $null
    if ($PSScriptRoot) {
        $local = Join-Path $PSScriptRoot "dist\vera-windows-$arch.exe"
        if (Test-Path $local) { $src = $local; Write-Host "Instalando el binario local $local" }
    }

    if (-not $src) {
        $version = $env:VERA_VERSION
        if (-not $version) {
            $version = (Invoke-RestMethod "https://api.github.com/repos/$repo/releases/latest").tag_name
        }
        $version = $version.TrimStart("v")
        $zip = "vera_${version}_windows_$arch.zip"
        $base = "https://github.com/$repo/releases/download/v$version"

        Write-Host "Descargando vera $version (windows/$arch)..."
        Invoke-WebRequest -UseBasicParsing "$base/$zip" -OutFile (Join-Path $tmp $zip)
        Invoke-WebRequest -UseBasicParsing "$base/checksums.txt" -OutFile (Join-Path $tmp "checksums.txt")

        $line = Get-Content (Join-Path $tmp "checksums.txt") | Where-Object { $_ -match "  $([regex]::Escape($zip))$" }
        if (-not $line) { throw "$zip no aparece en checksums.txt" }
        $expected = ($line -split "\s+")[0].ToLower()
        $actual = (Get-FileHash (Join-Path $tmp $zip) -Algorithm SHA256).Hash.ToLower()
        if ($expected -ne $actual) { throw "La suma SHA-256 no coincide: el archivo está dañado o ha sido manipulado" }
        Write-Host "OK: suma SHA-256 verificada"

        Expand-Archive (Join-Path $tmp $zip) -DestinationPath $tmp -Force
        $src = Join-Path $tmp "vera.exe"
    }

    New-Item -ItemType Directory -Force -Path $dest | Out-Null
    Copy-Item $src (Join-Path $dest "vera.exe") -Force
    Unblock-File (Join-Path $dest "vera.exe")
    # «ia» es un alias: una copia del mismo binario (los enlaces en Windows requieren permisos de administrador).
    Copy-Item (Join-Path $dest "vera.exe") (Join-Path $dest "ia.exe") -Force

    $userPath = [Environment]::GetEnvironmentVariable("Path", "User")
    if (($userPath -split ";") -notcontains $dest) {
        [Environment]::SetEnvironmentVariable("Path", ($userPath.TrimEnd(";") + ";" + $dest), "User")
        Write-Host "Añadido $dest al PATH. Abre una terminal nueva para usar 'vera' o 'ia'."
    }
    Write-Host "OK: instalado en $dest\vera.exe (y ia.exe)"
    & (Join-Path $dest "vera.exe") --version
}
finally {
    Remove-Item -Recurse -Force $tmp -ErrorAction SilentlyContinue
}
