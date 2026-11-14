//go:build linux

package setup

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// En Linux, si Ollama lo instaló vera (sin servicio del sistema), el arranque al iniciar sesión
// se hace con un servicio de usuario de systemd.
const userUnit = "ollama-vera.service"

var systemUnits = []string{"/etc/systemd/system/ollama.service", "/usr/lib/systemd/system/ollama.service", "/lib/systemd/system/ollama.service"}

func (m *Manager) userUnitPath() string { return filepath.Join(m.unitDir, userUnit) }

func (m *Manager) systemService() bool {
	for _, u := range systemUnits {
		if _, err := m.stat(u); err == nil {
			return true
		}
	}
	return false
}

func (m *Manager) userSystemd(ctx context.Context) bool {
	if _, err := m.lookPath("systemctl"); err != nil {
		return false
	}
	out, _ := m.output(ctx, "systemctl", "--user", "is-system-running")
	switch strings.TrimSpace(string(out)) {
	case "running", "degraded", "starting":
		return true
	}
	return false
}

// start: con servicio del sistema parado, hay que arrancarlo con sudo (no lo hace vera);
// con el servicio de usuario de vera, se usa ese; si no, «ollama serve» en segundo plano.
func (m *Manager) start(ctx context.Context, bin string) error {
	if m.systemService() {
		return ErrSystemService
	}
	if _, err := m.stat(m.userUnitPath()); err == nil && m.run(ctx, "systemctl", "--user", "start", userUnit) == nil {
		return nil
	}
	return m.spawnServe(bin)
}

// Autostart describe si Ollama se abre al iniciar sesión.
type Autostart struct {
	State     string // corto, para el menú: «sí», «no», «automático…»
	Detail    string // explicación cuando no se puede cambiar
	CanChange bool
	Enabled   bool
}

// AutostartState dice si Ollama se abre al iniciar sesión y si vera puede cambiarlo.
func (m *Manager) AutostartState(ctx context.Context) Autostart {
	if m.systemService() {
		return Autostart{State: "sí (servicio del sistema)", Detail: "Ollama está instalado como servicio del sistema; se gestiona con: sudo systemctl enable|disable ollama", Enabled: true}
	}
	if !m.userSystemd(ctx) {
		return Autostart{State: "no disponible", Detail: "Este sistema no usa systemd, así que vera no puede programar el arranque. vera abrirá Ollama cuando lo necesite."}
	}
	out, _ := m.output(ctx, "systemctl", "--user", "is-enabled", userUnit)
	on := strings.TrimSpace(string(out)) == "enabled"
	return Autostart{State: siNo(on), CanChange: true, Enabled: on}
}

// SetAutostart activa o desactiva el servicio de usuario. Desactivarlo no cierra Ollama si está abierto.
func (m *Manager) SetAutostart(ctx context.Context, on bool) error {
	if !on {
		return m.run(ctx, "systemctl", "--user", "disable", userUnit)
	}
	bin := m.Binary()
	if bin == "" {
		return errors.New("Ollama no está instalado. Instálalo primero con: vera ollama")
	}
	unit := fmt.Sprintf(`[Unit]
Description=Ollama (activado desde vera config)
After=network-online.target

[Service]
ExecStart=%s serve
Restart=on-failure
RestartSec=3

[Install]
WantedBy=default.target
`, bin)
	if err := os.MkdirAll(m.unitDir, 0o755); err != nil {
		return err
	}
	if err := os.WriteFile(m.userUnitPath(), []byte(unit), 0o644); err != nil {
		return err
	}
	for _, args := range [][]string{{"daemon-reload"}, {"enable", userUnit}} {
		if out, err := m.output(ctx, "systemctl", append([]string{"--user"}, args...)...); err != nil {
			return fmt.Errorf("systemctl %s: %w %s", strings.Join(args, " "), err, out)
		}
	}
	return nil
}

func siNo(b bool) string {
	if b {
		return "sí"
	}
	return "no"
}
