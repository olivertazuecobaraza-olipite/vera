#!/bin/sh
# Instala vera (y el alias «ia») en ~/.local/bin. macOS y Linux.
#
#   curl -fsSL https://raw.githubusercontent.com/olivertazuecobaraza-olipite/vera/main/install.sh | sh
#
# Opciones (variables de entorno):
#   VERA_VERSION=1.2.0        instala esa versión en vez de la última
#   VERA_INSTALL_DIR=/ruta    instala en otra carpeta (por defecto ~/.local/bin)
#
# Desde una copia del repositorio, si existe dist/vera-<so>-<arq> (./build.sh), se instala ese binario.
set -eu

REPO="olivertazuecobaraza-olipite/vera"
BIN_DIR="${VERA_INSTALL_DIR:-$HOME/.local/bin}"

fail() { echo "✗ $*" >&2; exit 1; }

os=$(uname -s | tr '[:upper:]' '[:lower:]')
case "$os" in
  darwin|linux) ;;
  *) fail "Sistema no soportado: $os (en Windows usa install.ps1)" ;;
esac
case "$(uname -m)" in
  arm64|aarch64) arch=arm64 ;;
  x86_64|amd64)  arch=amd64 ;;
  *) fail "Arquitectura no soportada: $(uname -m)" ;;
esac

tmp=$(mktemp -d)
trap 'rm -rf "$tmp"' EXIT

get() { # get URL DESTINO
  if command -v curl >/dev/null 2>&1; then
    curl -fsSL --proto '=https' --tlsv1.2 -o "$2" "$1"
  elif command -v wget >/dev/null 2>&1; then
    wget -q --https-only -O "$2" "$1"
  else
    fail "Hace falta curl o wget"
  fi
}

sha256() {
  if command -v sha256sum >/dev/null 2>&1; then
    sha256sum "$1" | cut -d' ' -f1
  else
    shasum -a 256 "$1" | cut -d' ' -f1
  fi
}

# ¿Se ejecuta desde una copia del repositorio con binarios compilados?
src=""
if [ -f "$0" ]; then
  local_bin="$(cd "$(dirname "$0")" && pwd)/dist/vera-$os-$arch"
  [ -f "$local_bin" ] && src="$local_bin"
fi

if [ -n "$src" ]; then
  echo "Instalando el binario local $src"
else
  version="${VERA_VERSION:-}"
  if [ -z "$version" ]; then
    get "https://api.github.com/repos/$REPO/releases/latest" "$tmp/latest.json"
    version=$(sed -n 's/.*"tag_name": *"v\{0,1\}\([^"]*\)".*/\1/p' "$tmp/latest.json" | head -n 1)
    [ -n "$version" ] || fail "No encuentro la última versión de vera"
  fi
  version="${version#v}"
  archive="vera_${version}_${os}_${arch}.tar.gz"
  base="https://github.com/$REPO/releases/download/v$version"

  echo "Descargando vera $version ($os/$arch)…"
  get "$base/$archive" "$tmp/$archive" || fail "No se pudo descargar $base/$archive"
  get "$base/checksums.txt" "$tmp/checksums.txt" || fail "No se pudo descargar checksums.txt"

  expected=$(grep "  $archive\$" "$tmp/checksums.txt" | cut -d' ' -f1)
  [ -n "$expected" ] || fail "$archive no aparece en checksums.txt"
  actual=$(sha256 "$tmp/$archive")
  [ "$expected" = "$actual" ] || fail "La suma SHA-256 no coincide: el archivo está dañado o ha sido manipulado"
  echo "✓ Suma SHA-256 verificada"

  tar -xzf "$tmp/$archive" -C "$tmp" vera
  src="$tmp/vera"
fi

mkdir -p "$BIN_DIR"
cp "$src" "$BIN_DIR/vera.new"
chmod 755 "$BIN_DIR/vera.new"
mv -f "$BIN_DIR/vera.new" "$BIN_DIR/vera" # reemplazo atómico, aunque vera esté en uso
# macOS marca como «en cuarentena» los archivos descargados con el navegador.
[ "$os" = darwin ] && xattr -d com.apple.quarantine "$BIN_DIR/vera" 2>/dev/null || true
ln -sf vera "$BIN_DIR/ia" # «ia» es un alias: un enlace al mismo binario

echo "✓ Instalado en $BIN_DIR/vera (también disponible como «ia»)"
"$BIN_DIR/vera" --version
case ":$PATH:" in
  *":$BIN_DIR:"*) ;;
  *) echo "Añade esto a tu ~/.zshrc o ~/.bashrc:  export PATH=\"$BIN_DIR:\$PATH\"" ;;
esac
