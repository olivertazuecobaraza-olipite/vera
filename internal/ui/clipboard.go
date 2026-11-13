package ui

import (
	"bytes"
	"encoding/binary"
	"os"
	"os/exec"
	"runtime"
	"unicode/utf16"
)

// Clipboard copia texto al portapapeles con la herramienta que trae el sistema (sin dependencias).
type Clipboard struct {
	Name  string
	Args  []string
	utf16 bool // clip.exe solo entiende bien los acentos en UTF-16
	run   func(name string, args []string, input []byte) error
}

// NewClipboard detecta la herramienta del sistema. Devuelve nil si no hay ninguna
// (p. ej. Linux por SSH sin entorno gráfico): entonces vera no ofrece copiar.
func NewClipboard() *Clipboard {
	return detectClipboard(runtime.GOOS, exec.LookPath, os.Getenv)
}

func detectClipboard(goos string, lookPath func(string) (string, error), getenv func(string) string) *Clipboard {
	has := func(b string) bool { _, err := lookPath(b); return err == nil }
	c := &Clipboard{run: runWithInput}
	switch {
	case goos == "darwin" && has("pbcopy"):
		c.Name = "pbcopy"
	case goos == "windows" && has("clip.exe"):
		c.Name, c.utf16 = "clip.exe", true
	case goos == "windows" || goos == "darwin":
		return nil
	case getenv("WAYLAND_DISPLAY") != "" && has("wl-copy"):
		c.Name = "wl-copy"
	case getenv("DISPLAY") != "" && has("xclip"):
		c.Name, c.Args = "xclip", []string{"-selection", "clipboard"}
	case getenv("DISPLAY") != "" && has("xsel"):
		c.Name, c.Args = "xsel", []string{"--clipboard", "--input"}
	default:
		return nil
	}
	return c
}

// NewClipboardWith crea un portapapeles con una función de copia propia (para tests).
func NewClipboardWith(name string, run func(name string, args []string, input []byte) error) *Clipboard {
	return &Clipboard{Name: name, run: run}
}

// Copy copia text al portapapeles.
func (c *Clipboard) Copy(text string) error {
	data := []byte(text)
	if c.utf16 {
		data = toUTF16LE(text)
	}
	return c.run(c.Name, c.Args, data)
}

// toUTF16LE codifica en UTF-16 little endian con BOM (lo que espera clip.exe).
func toUTF16LE(s string) []byte {
	var b bytes.Buffer
	b.Write([]byte{0xFF, 0xFE})
	out := b.Bytes()
	for _, u := range utf16.Encode([]rune(s)) {
		out = binary.LittleEndian.AppendUint16(out, u)
	}
	return out
}

func runWithInput(name string, args []string, input []byte) error {
	cmd := exec.Command(name, args...)
	cmd.Stdin = bytes.NewReader(input)
	return cmd.Run()
}
