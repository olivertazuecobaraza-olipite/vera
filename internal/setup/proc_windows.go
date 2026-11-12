//go:build windows

package setup

import (
	"os/exec"
	"syscall"
)

// detach arranca el proceso sin ventana y separado de la consola.
func detach(c *exec.Cmd) {
	const detachedProcess, newProcessGroup = 0x00000008, 0x00000200
	c.SysProcAttr = &syscall.SysProcAttr{CreationFlags: detachedProcess | newProcessGroup, HideWindow: true}
}
