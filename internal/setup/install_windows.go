//go:build windows

package setup

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
)

func (m *Manager) candidates() []string {
	return []string{filepath.Join(m.appData, "Programs", "Ollama", "ollama.exe")}
}

// install descarga el instalador oficial y lo ejecuta sin ventanas.
// Se instala en la cuenta del usuario, sin permisos de administrador.
func (m *Manager) install(ctx context.Context, p func(Progress)) (string, error) {
	exe, err := m.download(ctx, m.baseURL+"/download/OllamaSetup.exe", p)
	if err != nil {
		return "", err
	}
	defer os.Remove(exe)
	p(Progress{Status: StatusInstalling})
	if err := m.run(ctx, exe, "/VERYSILENT", "/SUPPRESSMSGBOXES", "/NORESTART", "/SP-"); err != nil {
		return "", fmt.Errorf("el instalador falló: %v", err)
	}
	return filepath.Join(m.appData, "Programs", "Ollama"), nil
}
