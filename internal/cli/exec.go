package cli

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"

	"github.com/olipite/vera/internal/sysinfo"
)

// runCommand ejecuta el comando aceptado y termina vera con su mismo código de salida.
func runCommand(ctx context.Context, d Deps, command string) error {
	code, err := d.Exec(ctx, d.Info(), command)
	if err != nil {
		return err
	}
	if code == 0 {
		d.UI.Println(d.UI.Paint("32", "✓ terminado"), d.UI.Paint("90", "(si no ves nada arriba, el comando no produjo salida)"))
		return nil
	}
	d.UI.Println(d.UI.Paint("31", fmt.Sprintf("✗ el comando falló (código %d)", code)))
	return exitCode(code)
}

// ExecShell ejecuta command con la shell del sistema, conectado a la terminal.
// No usa ctx a propósito: Ctrl+C le llega al comando (mismo grupo de procesos), y vera solo espera a que termine.
func ExecShell(_ context.Context, info sysinfo.Info, command string) (int, error) {
	run := command
	if info.ShellKind == "powershell" {
		// Para que los acentos se vean bien en la consola de Windows.
		run = "[Console]::OutputEncoding=[Text.Encoding]::UTF8; " + command
	}
	c := exec.Command(info.ShellPath, append(append([]string{}, info.ShellArgs...), run)...)
	c.Stdin, c.Stdout, c.Stderr = os.Stdin, os.Stdout, os.Stderr
	if err := c.Run(); err != nil {
		var ee *exec.ExitError
		if errors.As(err, &ee) {
			return ee.ExitCode(), nil
		}
		return 0, fmt.Errorf("no se pudo ejecutar con %s: %v", info.Shell, err)
	}
	return 0, nil
}
