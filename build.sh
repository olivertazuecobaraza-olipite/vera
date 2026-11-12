#!/bin/sh
# Compila "vera" para todos los sistemas en la carpeta dist/
set -e
cd "$(dirname "$0")"
VERSION="${VERSION:-1.0.0}"
rm -rf dist && mkdir -p dist

for target in darwin/arm64 darwin/amd64 linux/amd64 linux/arm64 windows/amd64 windows/arm64; do
  os="${target%/*}"; arch="${target#*/}"
  out="dist/vera-$os-$arch"; [ "$os" = windows ] && out="$out.exe"
  CGO_ENABLED=0 GOOS=$os GOARCH=$arch go build -trimpath \
    -ldflags "-s -w -X main.version=$VERSION" -o "$out" .
  echo "✓ $out"
done
