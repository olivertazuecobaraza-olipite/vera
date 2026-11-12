//go:build !darwin && !linux && !windows

package setup

import (
	"context"
	"errors"
)

func (m *Manager) candidates() []string { return nil }

func (m *Manager) install(context.Context, func(Progress)) (string, error) {
	return "", errors.New("en este sistema hay que instalar Ollama a mano: https://ollama.com/download")
}

func (m *Manager) start(_ context.Context, bin string) error { return m.spawnServe(bin) }
