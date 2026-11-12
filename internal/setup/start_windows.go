//go:build windows

package setup

import (
	"context"
	"time"
)

// start espera un poco por si el instalador ya ha abierto Ollama; si no, arranca «ollama serve».
func (m *Manager) start(ctx context.Context, bin string) error {
	if m.waitUp(ctx, 10*time.Second) {
		return nil
	}
	return m.spawnServe(bin)
}
