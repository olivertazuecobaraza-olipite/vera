//go:build darwin

package setup

import (
	"context"
	"net/url"
	"path/filepath"
	"strings"
)

// start abre la app de Ollama en segundo plano (que arranca el servidor en el puerto estándar).
// Si Ollama no viene de la app (Homebrew) o se usa otro puerto, arranca «ollama serve».
func (m *Manager) start(ctx context.Context, bin string) error {
	if real, err := filepath.EvalSymlinks(bin); err == nil {
		bin = real // /usr/local/bin/ollama suele ser un enlace a Ollama.app
	}
	if i := strings.Index(bin, ".app/"); i >= 0 && m.port() == "11434" {
		if m.run(ctx, "open", "-g", bin[:i+len(".app")]) == nil {
			return nil
		}
	}
	return m.spawnServe(bin)
}

func (m *Manager) port() string {
	u, _ := url.Parse(m.host)
	return u.Port()
}
