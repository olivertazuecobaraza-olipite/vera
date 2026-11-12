#!/bin/sh
# Instala "vera" (y el alias "ia") en ~/.local/bin (macOS y Linux).
# Uso: ./install.sh            (elige el binario de dist/ según tu sistema)
set -e
cd "$(dirname "$0")"

os=$(uname -s | tr '[:upper:]' '[:lower:]')
case "$(uname -m)" in
  arm64|aarch64) arch=arm64 ;;
  x86_64|amd64)  arch=amd64 ;;
  *) echo "Arquitectura no soportada: $(uname -m)"; exit 1 ;;
esac

bin="dist/vera-$os-$arch"
[ -f "$bin" ] || { echo "No encuentro $bin. Ejecuta antes ./build.sh"; exit 1; }

mkdir -p "$HOME/.local/bin"
cp "$bin" "$HOME/.local/bin/vera"
chmod +x "$HOME/.local/bin/vera"
# macOS marca como "en cuarentena" los archivos descargados; se lo quitamos.
[ "$os" = darwin ] && xattr -d com.apple.quarantine "$HOME/.local/bin/vera" 2>/dev/null || true
# "ia" es un alias: un enlace al mismo binario.
ln -sf vera "$HOME/.local/bin/ia"

echo "✓ Instalado en $HOME/.local/bin/vera (también disponible como 'ia')"
case ":$PATH:" in
  *":$HOME/.local/bin:"*) ;;
  *) echo "Añade esto a tu ~/.zshrc o ~/.bashrc:  export PATH=\"\$HOME/.local/bin:\$PATH\"" ;;
esac
