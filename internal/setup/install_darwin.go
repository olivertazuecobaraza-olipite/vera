//go:build darwin

package setup

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
)

func (m *Manager) appDirs() []string {
	return []string{filepath.Join(m.appsDir, "Ollama.app"), filepath.Join(m.home, "Applications", "Ollama.app")}
}

func (m *Manager) candidates() []string {
	var c []string
	for _, app := range m.appDirs() {
		c = append(c, filepath.Join(app, "Contents", "Resources", "ollama"))
	}
	return append(c, "/opt/homebrew/bin/ollama", "/usr/local/bin/ollama")
}

// install descarga la app oficial y la deja en /Applications (o en ~/Applications si no hay permisos).
func (m *Manager) install(ctx context.Context, p func(Progress)) (string, error) {
	zip, err := m.download(ctx, m.baseURL+"/download/Ollama-darwin.zip", p)
	if err != nil {
		return "", err
	}
	defer os.Remove(zip)
	p(Progress{Status: StatusExtracting})
	dest := m.appsDir
	if err := m.run(ctx, "ditto", "-x", "-k", zip, dest); err != nil {
		if ctx.Err() != nil {
			return "", ctx.Err()
		}
		dest = filepath.Join(m.home, "Applications")
		if err := os.MkdirAll(dest, 0o755); err != nil {
			return "", err
		}
		if err := m.run(ctx, "ditto", "-x", "-k", zip, dest); err != nil {
			return "", fmt.Errorf("no se pudo descomprimir: %w", err)
		}
	}
	return filepath.Join(dest, "Ollama.app"), nil
}
