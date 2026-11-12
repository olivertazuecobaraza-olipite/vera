//go:build windows

package ui

import (
	"os"
	"syscall"
	"unsafe"
)

// enableColor activa los códigos de color ANSI en la consola de Windows 10+.
func enableColor() bool {
	k32 := syscall.NewLazyDLL("kernel32.dll")
	getMode, setMode := k32.NewProc("GetConsoleMode"), k32.NewProc("SetConsoleMode")
	h := syscall.Handle(os.Stdout.Fd())
	var mode uint32
	if r, _, _ := getMode.Call(uintptr(h), uintptr(unsafe.Pointer(&mode))); r == 0 {
		return false
	}
	const enableVirtualTerminalProcessing = 0x0004
	r, _, _ := setMode.Call(uintptr(h), uintptr(mode|enableVirtualTerminalProcessing))
	return r != 0
}
