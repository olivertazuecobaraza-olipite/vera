#!/bin/sh
# Compilar y probar vera.
#
#   ./build.sh               compila para todos los sistemas en dist/
#   ./build.sh test          tests rápidos (sin red ni Ollama), con detector de carreras
#   ./build.sh cover         cobertura por paquete e informe HTML en cover.html
#   ./build.sh integration   incluye los tests con Ollama de verdad (VERA_TEST_MODEL elige el modelo)
#   ./build.sh golden        regenera los golden files del prompt (revisa luego con git diff)
set -e
cd "$(dirname "$0")"

case "${1:-build}" in
  test)
    go vet ./...
    go test -race ./...
    ;;
  cover)
    go test -coverprofile=cover.out ./...
    go tool cover -html=cover.out -o cover.html
    echo "Informe: cover.html"
    ;;
  integration)
    go test -tags=integration -v ./...
    ;;
  golden)
    go test ./internal/suggest -update
    git diff --stat -- internal/suggest/testdata
    ;;
  build)
    VERSION="${VERSION:-1.0.0}"
    rm -rf dist && mkdir -p dist
    for target in darwin/arm64 darwin/amd64 linux/amd64 linux/arm64 windows/amd64 windows/arm64; do
      os="${target%/*}"; arch="${target#*/}"
      out="dist/vera-$os-$arch"; [ "$os" = windows ] && out="$out.exe"
      CGO_ENABLED=0 GOOS=$os GOARCH=$arch go build -trimpath \
        -ldflags "-s -w -X main.version=$VERSION" -o "$out" ./cmd/vera
      echo "✓ $out"
    done
    ;;
  *)
    echo "Uso: ./build.sh [build|test|cover|integration|golden]"; exit 1
    ;;
esac
