package ui

import (
	"fmt"
	"strings"
	"time"
)

// HumanBytes escribe un tamaño en B, KB, MB o GB.
func HumanBytes(n int64) string {
	const u = 1024
	switch {
	case n >= u*u*u:
		return fmt.Sprintf("%.2f GB", float64(n)/(u*u*u))
	case n >= u*u:
		return fmt.Sprintf("%.1f MB", float64(n)/(u*u))
	case n >= u:
		return fmt.Sprintf("%.0f KB", float64(n)/u)
	}
	return fmt.Sprintf("%d B", n)
}

// Bar es una barra de progreso de una línea:
//
//	llama3.2 [█████████░░░░░░░░░░░░░░░░░░░░░]  31%  620.4 MB / 1.97 GB  25.3 MB/s
type Bar struct {
	t       *Terminal
	label   string
	now     func() time.Time
	start   time.Time
	base    int64 // bytes que ya había al empezar (para calcular la velocidad)
	last    time.Time
	lastLen int
	started bool
	done    bool
}

// NewBar crea una barra. No se pinta nada hasta el primer Update.
func (t *Terminal) NewBar(label string) *Bar {
	if n := []rune(label); len(n) > 24 {
		label = string(n[:23]) + "…"
	}
	return &Bar{t: t, label: label, now: time.Now}
}

// Started dice si la barra ya se ha pintado alguna vez.
func (b *Bar) Started() bool { return b.started }

// Update redibuja la barra (como mucho cada 100 ms; sin terminal, nada).
func (b *Bar) Update(done, total int64) { b.draw(done, total, false) }

// Finish pinta el estado final y salta de línea.
func (b *Bar) Finish(done, total int64) {
	if b.done {
		return
	}
	b.draw(done, total, true)
	b.done = true
	fmt.Fprintln(b.t.Out)
}

// Abort salta de línea sin redibujar (p. ej. tras un error a mitad).
func (b *Bar) Abort() {
	if b.started && !b.done {
		b.done = true
		fmt.Fprintln(b.t.Out)
	}
}

func (b *Bar) draw(done, total int64, force bool) {
	now := b.now()
	if !b.started {
		b.started, b.start, b.base = true, now, done
	}
	// Sin terminal (salida redirigida) solo se dibuja la línea final.
	if !force && (!b.t.TTY || now.Sub(b.last) < 100*time.Millisecond) {
		return
	}
	b.last = now
	line := b.line(done, total, now)
	// Se rellena con espacios para borrar restos de la línea anterior (funciona también sin ANSI).
	n := visibleLen(line)
	pad := ""
	if n < b.lastLen {
		pad = strings.Repeat(" ", b.lastLen-n)
	}
	b.lastLen = n
	fmt.Fprint(b.t.Out, "\r"+line+pad)
}

func (b *Bar) line(done, total int64, now time.Time) string {
	const width = 30
	var line string
	if total > 0 {
		if done > total {
			done = total
		}
		frac := float64(done) / float64(total)
		fill := int(frac * width)
		pct := int(frac * 100) // hacia abajo: 100% solo cuando de verdad ha terminado
		bar := b.t.Paint(Green, strings.Repeat("█", fill)) + b.t.Paint(Gray, strings.Repeat("░", width-fill))
		line = fmt.Sprintf("  %s [%s] %3d%%  %s / %s", b.label, bar, pct, HumanBytes(done), HumanBytes(total))
	} else {
		line = fmt.Sprintf("  %s  %s", b.label, HumanBytes(done))
	}
	if secs := now.Sub(b.start).Seconds(); secs > 0.5 && done > b.base {
		line += fmt.Sprintf("  %s/s", HumanBytes(int64(float64(done-b.base)/secs)))
	}
	return line
}

// visibleLen cuenta los caracteres que se ven, sin los códigos de color.
func visibleLen(s string) int {
	n, esc := 0, false
	for _, r := range s {
		switch {
		case r == '\x1b':
			esc = true
		case esc:
			if r == 'm' {
				esc = false
			}
		default:
			n++
		}
	}
	return n
}
