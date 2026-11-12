//go:build linux

package setup

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
)

func (m *Manager) candidates() []string {
	return []string{filepath.Join(m.home, ".local", "bin", "ollama"), "/usr/local/bin/ollama", "/usr/bin/ollama"}
}

// install descarga el paquete oficial y lo descomprime en ~/.local (sin sudo):
// queda en ~/.local/bin/ollama y ~/.local/lib/ollama.
func (m *Manager) install(ctx context.Context, p func(Progress)) (string, error) {
	arch := goarch()
	if arch != "amd64" && arch != "arm64" {
		return "", fmt.Errorf("arquitectura no soportada: %s", arch)
	}
	dest := filepath.Join(m.home, ".local")
	file, err := m.download(ctx, m.baseURL+"/download/ollama-linux-"+arch+".tar.zst", p)
	if err != nil {
		return "", err
	}
	defer os.Remove(file)
	p(Progress{Status: StatusExtracting})
	os.RemoveAll(filepath.Join(dest, "lib", "ollama")) // librerías de una versión anterior
	if err := extractTarZst(file, dest); err != nil {
		return "", fmt.Errorf("no se pudo descomprimir: %v", err)
	}
	return filepath.Join(dest, "bin", "ollama"), nil
}
