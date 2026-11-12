package ui

import (
	"bytes"
	"context"
	"io"
	"strings"
	"testing"
	"time"
)

func newTest(input string, tty bool) (*Terminal, *bytes.Buffer) {
	var out bytes.Buffer
	return New(strings.NewReader(input), &out, &out, tty, false), &out
}

func TestConfirm(t *testing.T) {
	tests := []struct {
		name  string
		input string
		def   bool
		want  bool
	}{
		{"enter usa el defecto sí", "\n", true, true},
		{"enter usa el defecto no", "\n", false, false},
		{"s", "s\n", false, true},
		{"sí con tilde", "Sí\n", false, true},
		{"yes", "yes\n", false, true},
		{"n", "n\n", true, false},
		{"cualquier otra cosa es no", "quizás\n", true, false},
		{"entrada cerrada es siempre no", "", true, false},
		{"sin salto de línea final", "s", false, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			term, _ := newTest(tt.input, true)
			if got := term.Confirm("¿Seguro? ", tt.def); got != tt.want {
				t.Errorf("Confirm = %v, quería %v", got, tt.want)
			}
		})
	}
}

func TestReadLineSeCancelaConCtrlC(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	pr, pw := io.Pipe() // nadie escribe: la lectura se quedaría esperando para siempre
	defer pw.Close()
	term := New(pr, io.Discard, io.Discard, true, false).WithContext(ctx)

	done := make(chan bool)
	go func() {
		_, ok := term.ReadLine("? ")
		done <- ok
	}()
	cancel()
	select {
	case ok := <-done:
		if ok {
			t.Error("tras Ctrl+C ReadLine debería devolver ok=false")
		}
	case <-time.After(2 * time.Second):
		t.Fatal("ReadLine no deja de esperar al cancelar el contexto")
	}
}

func TestVariasPreguntasSeguidas(t *testing.T) {
	term, _ := newTest("uno\ndos\n", true)
	a, _ := term.ReadLine("")
	b, _ := term.ReadLine("")
	if a != "uno" || b != "dos" {
		t.Errorf("got %q, %q", a, b)
	}
}

func TestBarra100SoloAlTerminar(t *testing.T) {
	term, out := newTest("", true)
	b := term.NewBar("modelo")
	clock := time.Unix(0, 0)
	b.now = func() time.Time { clock = clock.Add(time.Second); return clock }

	b.Update(996, 1000) // 99.6 %
	if s := out.String(); !strings.Contains(s, " 99%") || strings.Contains(s, "100%") {
		t.Errorf("99.6%% debería mostrarse como 99%%: %q", s)
	}
	b.Finish(1000, 1000)
	if !strings.Contains(out.String(), "100%") || !strings.HasSuffix(out.String(), "\n") {
		t.Errorf("al terminar debería verse 100%% y saltar de línea: %q", out.String())
	}
}

func TestBarraSinTerminalSoloLineaFinal(t *testing.T) {
	term, out := newTest("", false)
	b := term.NewBar("x")
	for i := int64(0); i <= 100; i += 10 {
		b.Update(i, 100)
	}
	if out.Len() != 0 {
		t.Errorf("sin terminal no debería pintar nada intermedio: %q", out.String())
	}
	b.Finish(100, 100)
	if strings.Count(out.String(), "\n") != 1 || !strings.Contains(out.String(), "100%") {
		t.Errorf("debería haber una sola línea final: %q", out.String())
	}
}

func TestBarraBorraRestos(t *testing.T) {
	term, out := newTest("", true)
	b := term.NewBar("x")
	clock := time.Unix(0, 0)
	b.now = func() time.Time { clock = clock.Add(time.Second); return clock }
	b.Update(500*1024*1024, 0) // "500.0 MB" + velocidad
	first := b.lastLen
	b.Update(1, 0) // línea más corta
	if !strings.HasSuffix(out.String(), strings.Repeat(" ", first-b.lastLen)) {
		t.Error("una línea más corta debería rellenarse con espacios para borrar la anterior")
	}
}

func TestEtiquetaLarga(t *testing.T) {
	term, _ := newTest("", true)
	b := term.NewBar("hf.co/usuario/un-modelo-con-nombre-larguisimo")
	if n := len([]rune(b.label)); n != 24 {
		t.Errorf("la etiqueta debería recortarse a 24 caracteres, tiene %d", n)
	}
}

func TestHumanBytes(t *testing.T) {
	tests := map[int64]string{0: "0 B", 1023: "1023 B", 2048: "2 KB", 5 << 20: "5.0 MB", 3 << 30: "3.00 GB"}
	for in, want := range tests {
		if got := HumanBytes(in); got != want {
			t.Errorf("HumanBytes(%d) = %q, quería %q", in, got, want)
		}
	}
}

func TestVisibleLenIgnoraColores(t *testing.T) {
	term := New(strings.NewReader(""), io.Discard, io.Discard, true, true)
	if n := visibleLen(term.Paint(Green, "hola")); n != 4 {
		t.Errorf("visibleLen = %d", n)
	}
}

func TestErrorEnMayuscula(t *testing.T) {
	term, out := newTest("", false)
	term.Error("ño pasa nada")
	if out.String() != "Ño pasa nada\n" {
		t.Errorf("got %q", out.String())
	}
}

func TestSuggestionPeligrosa(t *testing.T) {
	term, out := newTest("", false)
	term.Suggestion("rm -rf ~/x", "Borra la carpeta x.", true, "borra carpetas enteras.")
	s := out.String()
	for _, want := range []string{"❯ rm -rf ~/x", "Borra la carpeta x.", "⚠ Comando peligroso: borra carpetas enteras."} {
		if !strings.Contains(s, want) {
			t.Errorf("falta %q en %q", want, s)
		}
	}
	out.Reset()
	term.Suggestion("ls", "", false, "")
	if strings.Contains(out.String(), "⚠") {
		t.Error("un comando normal no debería llevar aviso")
	}
}
