package ui

import (
	"bytes"
	"errors"
	"strings"
	"sync"
	"testing"
	"time"
)

func lookPathOf(bins ...string) func(string) (string, error) {
	return func(name string) (string, error) {
		for _, b := range bins {
			if b == name {
				return "/usr/bin/" + name, nil
			}
		}
		return "", errors.New("no")
	}
}

func envOf(m map[string]string) func(string) string { return func(k string) string { return m[k] } }

func TestDetectClipboard(t *testing.T) {
	tests := []struct {
		name string
		goos string
		bins []string
		env  map[string]string
		want string // "" = no hay portapapeles
	}{
		{"macOS", "darwin", []string{"pbcopy"}, nil, "pbcopy"},
		{"Windows", "windows", []string{"clip.exe"}, nil, "clip.exe"},
		{"Wayland", "linux", []string{"wl-copy", "xclip"}, map[string]string{"WAYLAND_DISPLAY": "wayland-0", "DISPLAY": ":0"}, "wl-copy"},
		{"X11 con xclip", "linux", []string{"xclip"}, map[string]string{"DISPLAY": ":0"}, "xclip"},
		{"X11 con xsel", "linux", []string{"xsel"}, map[string]string{"DISPLAY": ":0"}, "xsel"},
		{"SSH sin entorno gráfico", "linux", []string{"xclip"}, nil, ""},
		{"Linux sin herramientas", "linux", nil, map[string]string{"DISPLAY": ":0"}, ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c := detectClipboard(tt.goos, lookPathOf(tt.bins...), envOf(tt.env))
			got := ""
			if c != nil {
				got = c.Name
			}
			if got != tt.want {
				t.Errorf("portapapeles = %q, quería %q", got, tt.want)
			}
		})
	}
}

func TestClipboardWindowsUTF16(t *testing.T) {
	c := detectClipboard("windows", lookPathOf("clip.exe"), envOf(nil))
	var got []byte
	c.run = func(_ string, _ []string, in []byte) error { got = in; return nil }
	c.Copy("dir ñ")
	want := []byte{0xFF, 0xFE, 'd', 0, 'i', 0, 'r', 0, ' ', 0, 0xF1, 0}
	if !bytes.Equal(got, want) {
		t.Errorf("clip.exe necesita UTF-16LE con BOM:\n got %x\nwant %x", got, want)
	}
}

func TestClipboardUnixUTF8(t *testing.T) {
	c := detectClipboard("linux", lookPathOf("xclip"), envOf(map[string]string{"DISPLAY": ":0"}))
	var gotArgs []string
	var got []byte
	c.run = func(_ string, args []string, in []byte) error { gotArgs, got = args, in; return nil }
	c.Copy("ls ñ")
	if string(got) != "ls ñ" || strings.Join(gotArgs, " ") != "-selection clipboard" {
		t.Errorf("args %v, datos %q", gotArgs, got)
	}
}

// syncBuffer es un buffer seguro para escribir desde la goroutine del spinner.
type syncBuffer struct {
	mu sync.Mutex
	b  bytes.Buffer
}

func (s *syncBuffer) Write(p []byte) (int, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.b.Write(p)
}
func (s *syncBuffer) String() string { s.mu.Lock(); defer s.mu.Unlock(); return s.b.String() }

func TestSpinnerSinTerminalNoPinta(t *testing.T) {
	var out syncBuffer
	term := New(strings.NewReader(""), &out, &out, false, false)
	stop := term.Spinner("Pensando…")
	time.Sleep(300 * time.Millisecond)
	stop()
	if out.String() != "" {
		t.Errorf("sin terminal no debería pintar nada: %q", out.String())
	}
}

func TestSpinnerRapidoNoParpadea(t *testing.T) {
	var out syncBuffer
	term := New(strings.NewReader(""), &out, &out, true, false)
	term.Spinner("Pensando…")() // respuesta inmediata
	if out.String() != "" {
		t.Errorf("si la respuesta es inmediata no debería aparecer: %q", out.String())
	}
}

func TestSpinnerSeBorraAlParar(t *testing.T) {
	var out syncBuffer
	term := New(strings.NewReader(""), &out, &out, true, false)
	stop := term.Spinner("Pensando…")
	time.Sleep(400 * time.Millisecond)
	stop()
	stop() // llamarlo dos veces no debe colgarse
	s := out.String()
	if !strings.Contains(s, "Pensando…") {
		t.Errorf("debería haberse visto el spinner: %q", s)
	}
	if !strings.HasSuffix(s, "\r") || !strings.Contains(s, strings.Repeat(" ", visibleLen("⠋ Pensando…"))) {
		t.Errorf("al parar debería borrar la línea: %q", s)
	}
}
