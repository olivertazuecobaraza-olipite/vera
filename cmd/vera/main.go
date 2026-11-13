// vera: describe lo que quieres hacer y un modelo local (Ollama) te propone el comando.
// Funciona en macOS, Linux y Windows. Se instala también como «ia».
//
// Este es el único sitio que conoce todas las piezas: las crea y se las pasa a la CLI.
package main

import (
	"context"
	"os"
	"os/signal"

	"github.com/olipite/vera/internal/cli"
	"github.com/olipite/vera/internal/config"
	"github.com/olipite/vera/internal/llm/ollama"
	"github.com/olipite/vera/internal/setup"
	"github.com/olipite/vera/internal/setup/setuptest"
	"github.com/olipite/vera/internal/suggest"
	"github.com/olipite/vera/internal/sysinfo"
	"github.com/olipite/vera/internal/ui"
)

// version se fija al compilar: go build -ldflags "-X main.version=1.2.3"
var version = "dev"

// Comprobaciones en tiempo de compilación: las piezas reales (y el fake) cumplen lo que pide la CLI.
var (
	_ cli.Manager   = (*setup.Manager)(nil)
	_ cli.Manager   = (*setuptest.FakeManager)(nil)
	_ cli.Suggester = (*suggest.Suggester)(nil)
)

func main() {
	// Ctrl+C cancela el contexto: las descargas se detienen y borran sus temporales.
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
	defer stop()

	term := ui.NewStd()
	cfg, err := config.Load(config.DefaultPath())
	if err != nil {
		term.Warn(err.Error()) // archivo dañado: se sigue con los valores por defecto
	}

	host := cfg.OllamaHost()
	os.Exit(cli.Run(ctx, os.Args[1:], cli.Deps{
		Suggester: suggest.New(ollama.NewClient(host), sysinfo.Detect, cfg),
		Manager:   setup.NewManager(host),
		Config:    cfg,
		UI:        term,
		Info:      sysinfo.Detect,
		Exec:      cli.ExecShell,
		Clipboard: ui.NewClipboard(),
		Version:   version,
	}))
}
