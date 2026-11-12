// Package ui es todo lo que vera escribe en la terminal o lee de ella:
// colores, mensajes, preguntas [s/N], menús, barra de progreso y spinner.
package ui

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"os"
	"strings"
	"unicode"
	"unicode/utf8"
)

// Terminal agrupa la entrada y salida. En los tests se crea con buffers.
type Terminal struct {
	in    *bufio.Reader
	Out   io.Writer
	Err   io.Writer
	TTY   bool // la salida es una terminal (si no, no se animan barras ni spinners)
	Color bool

	ctx   context.Context // si se cancela (Ctrl+C), las preguntas dejan de esperar
	lines   chan readResult // resultado de la lectura de stdin en curso
	pending bool            // hay una lectura en curso
}

type readResult struct {
	line string
	err  error
}

// New crea una terminal con la entrada y salidas dadas.
func New(in io.Reader, out, errw io.Writer, tty, color bool) *Terminal {
	return &Terminal{in: bufio.NewReader(in), Out: out, Err: errw, TTY: tty, Color: color, ctx: context.Background()}
}

// NewStd crea la terminal real: detecta si stdout es una terminal y si se pueden usar colores.
func NewStd() *Terminal {
	tty := false
	if st, err := os.Stdout.Stat(); err == nil {
		tty = st.Mode()&os.ModeCharDevice != 0
	}
	color := os.Getenv("NO_COLOR") == "" && tty && enableColor()
	return New(os.Stdin, os.Stdout, os.Stderr, tty, color)
}

// WithContext hace que las preguntas dejen de esperar si ctx se cancela (Ctrl+C).
func (t *Terminal) WithContext(ctx context.Context) *Terminal {
	t.ctx = ctx
	return t
}

// ---------- Colores y mensajes ----------

const (
	Red    = "31"
	Green  = "32"
	Yellow = "33"
	Cyan   = "36"
	Gray   = "90"
	Bold   = "1"
)

// Paint colorea s si los colores están activos.
func (t *Terminal) Paint(code, s string) string {
	if !t.Color {
		return s
	}
	return "\x1b[" + code + "m" + s + "\x1b[0m"
}

func (t *Terminal) Println(a ...any)               { fmt.Fprintln(t.Out, a...) }
func (t *Terminal) Printf(format string, a ...any) { fmt.Fprintf(t.Out, format, a...) }

// Step anuncia algo que va a pasar: «❯ Descargando…».
func (t *Terminal) Step(msg string) { t.Println(t.Paint(Cyan, "❯"), msg) }

// Info es un mensaje secundario, en gris.
func (t *Terminal) Info(msg string) { t.Println(t.Paint(Gray, "  "+msg)) }

// Success es algo que ha salido bien: «✓ …».
func (t *Terminal) Success(msg string) { t.Println(t.Paint(Green, "✓ "+msg)) }

// Warn es un aviso: «! …».
func (t *Terminal) Warn(msg string) { t.Println(t.Paint(Yellow, "! "+msg)) }

// Error escribe un error en rojo por la salida de errores, con la primera letra en mayúscula.
func (t *Terminal) Error(msg string) {
	if r, n := utf8.DecodeRuneInString(msg); n > 0 {
		msg = string(unicode.ToUpper(r)) + msg[n:]
	}
	fmt.Fprintln(t.Err, t.Paint(Red, msg))
}

// ---------- Preguntas ----------

// ReadLine pregunta y devuelve la respuesta sin espacios.
// ok es false si se cerró la entrada (Ctrl+D, stdin redirigido vacío) o se pulsó Ctrl+C.
func (t *Terminal) ReadLine(prompt string) (string, bool) {
	fmt.Fprint(t.Out, prompt)
	if t.lines == nil {
		t.lines = make(chan readResult, 1)
	}
	// La lectura va en una goroutine para poder dejar de esperar si llega Ctrl+C.
	// Si una pregunta anterior se canceló, su lectura sigue pendiente y se reutiliza.
	if !t.pending {
		t.pending = true
		go func() {
			line, err := t.in.ReadString('\n')
			t.lines <- readResult{line, err}
		}()
	}
	select {
	case r := <-t.lines:
		t.pending = false
		if r.err != nil && r.line == "" {
			fmt.Fprintln(t.Out)
			return "", false
		}
		return strings.TrimSpace(r.line), true
	case <-t.ctx.Done():
		fmt.Fprintln(t.Out)
		return "", false
	}
}

// Confirm hace una pregunta de sí/no. Con Enter vale def; sin respuesta posible es siempre «no».
func (t *Terminal) Confirm(question string, def bool) bool {
	ans, ok := t.ReadLine(question)
	if !ok {
		return false
	}
	if ans == "" {
		return def
	}
	return IsYes(ans)
}

// IsYes reconoce «s», «sí», «si», «y», «yes» (sin distinguir mayúsculas).
func IsYes(s string) bool {
	switch strings.ToLower(strings.TrimSpace(s)) {
	case "s", "si", "sí", "y", "yes":
		return true
	}
	return false
}

// SiNo convierte un booleano en «sí»/«no».
func SiNo(b bool) string {
	if b {
		return "sí"
	}
	return "no"
}
