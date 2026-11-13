package ui

import (
	"fmt"
	"strings"
	"sync"
	"time"
)

var spinnerFrames = []string{"⠋", "⠙", "⠹", "⠸", "⠼", "⠴", "⠦", "⠧", "⠇", "⠏"}

// Spinner muestra «⠋ msg» animado hasta que se llama a la función devuelta, que borra la línea.
// Sin terminal no hace nada. Tarda 150 ms en aparecer para no parpadear si la respuesta es inmediata.
func (t *Terminal) Spinner(msg string) (stop func()) {
	if !t.TTY {
		return func() {}
	}
	done, finished := make(chan struct{}), make(chan struct{})
	go func() {
		defer close(finished)
		select {
		case <-done:
			return
		case <-time.After(150 * time.Millisecond):
		}
		width := 0
		tick := time.NewTicker(80 * time.Millisecond)
		defer tick.Stop()
		for i := 0; ; i++ {
			line := t.Paint(Cyan, spinnerFrames[i%len(spinnerFrames)]) + " " + t.Paint(Gray, msg)
			width = visibleLen(line)
			fmt.Fprint(t.Out, "\r"+line)
			select {
			case <-done:
				fmt.Fprint(t.Out, "\r"+strings.Repeat(" ", width)+"\r")
				return
			case <-tick.C:
			}
		}
	}()
	var once sync.Once
	return func() { once.Do(func() { close(done); <-finished }) }
}
