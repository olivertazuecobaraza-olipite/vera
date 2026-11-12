//go:build !windows

package setup

import (
	"os/exec"
	"syscall"
)

// detach hace que el proceso siga vivo al cerrar la terminal.
func detach(c *exec.Cmd) { c.SysProcAttr = &syscall.SysProcAttr{Setsid: true} }
