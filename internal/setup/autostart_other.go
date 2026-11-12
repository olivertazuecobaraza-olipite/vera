//go:build !linux

package setup

import (
	"context"
	"errors"
	"runtime"
)

// Autostart describe si Ollama se abre al iniciar sesión.
type Autostart struct {
	State     string // corto, para el menú
	Detail    string // explicación cuando no se puede cambiar
	CanChange bool
	Enabled   bool
}

// AutostartState: en macOS y Windows lo controla la propia app de Ollama.
func (m *Manager) AutostartState(context.Context) Autostart {
	switch runtime.GOOS {
	case "darwin":
		return Autostart{State: "automático (lo hace la app de Ollama)", Detail: "En macOS lo controla la app de Ollama: Ajustes del Sistema → General → Ítems de inicio.", Enabled: true}
	case "windows":
		return Autostart{State: "automático (lo hace la app de Ollama)", Detail: "En Windows lo controla la app de Ollama: Configuración → Aplicaciones → Inicio.", Enabled: true}
	}
	return Autostart{State: "no disponible", Detail: "vera abrirá Ollama cuando lo necesite."}
}

// SetAutostart no está disponible fuera de Linux.
func (m *Manager) SetAutostart(context.Context, bool) error {
	return errors.New("en este sistema lo gestiona la app de Ollama")
}
