// Package cli es la capa de presentación: interpreta los argumentos (con Cobra), pregunta al usuario
// y orquesta los servicios (suggest, setup). Es la única capa que habla con el usuario.
package cli

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/olivertazuecobaraza-olipite/vera/internal/config"
	"github.com/olivertazuecobaraza-olipite/vera/internal/setup"
	"github.com/olivertazuecobaraza-olipite/vera/internal/suggest"
	"github.com/olivertazuecobaraza-olipite/vera/internal/sysinfo"
	"github.com/olivertazuecobaraza-olipite/vera/internal/ui"
)

// Suggester es lo que la CLI necesita para proponer comandos (*suggest.Suggester lo cumple).
type Suggester interface {
	Suggest(ctx context.Context, query string) (suggest.Suggestion, error)
}

// Manager es lo que la CLI necesita de Ollama (*setup.Manager y setuptest.FakeManager lo cumplen).
type Manager interface {
	IsLocal() bool
	IsInstalled() bool
	Running(ctx context.Context) bool
	Install(ctx context.Context, p func(setup.Progress)) (string, error)
	Start(ctx context.Context) error
	ModelInfo(ctx context.Context, name string) (setup.ModelInfo, error)
	RemoteSize(ctx context.Context, name string) (int64, error)
	Pull(ctx context.Context, name string, p func(setup.Progress)) error
	List(ctx context.Context) ([]setup.Model, error)
	Delete(ctx context.Context, name string) error
	AutostartState(ctx context.Context) setup.Autostart
	SetAutostart(ctx context.Context, on bool) error
}

// Deps son todas las dependencias de la CLI. Se montan en cmd/vera/main.go; en los tests, con fakes.
type Deps struct {
	Suggester Suggester
	Manager   Manager
	Config    *config.Config
	UI        *ui.Terminal
	Info      func() sysinfo.Info                                                       // sistema actual (para ejecutar el comando)
	Exec      func(ctx context.Context, info sysinfo.Info, command string) (int, error) // ejecuta el comando aceptado
	Clipboard *ui.Clipboard                                                             // nil si el sistema no tiene cómo copiar
	Version   string
}

// exitCode termina vera con ese código sin escribir nada más.
type exitCode int

func (e exitCode) Error() string { return fmt.Sprintf("código %d", int(e)) }

// Run ejecuta vera con los argumentos dados y devuelve el código de salida.
func Run(ctx context.Context, args []string, d Deps) int {
	d.UI.WithContext(ctx)
	root := NewRootCmd(d)
	root.SetArgs(preRoute(args))
	root.SetOut(d.UI.Out)
	root.SetErr(d.UI.Err)
	err := root.ExecuteContext(ctx)

	var code exitCode
	switch {
	case err == nil:
		return 0
	case errors.As(err, &code):
		return int(code)
	case errors.Is(err, context.Canceled):
		return 130 // Ctrl+C: los mensajes ya se han escrito donde tocaba
	case strings.Contains(err.Error(), "unknown flag") || strings.Contains(err.Error(), "unknown shorthand flag"):
		d.UI.Error(fmt.Sprintf("Opción desconocida (%s).\nSi forma parte de lo que quieres hacer, escríbelo después de «--»:  vera -- %s", err, strings.Join(args, " ")))
		return 2
	}
	d.UI.Error(err.Error())
	return 1
}

// confirm pregunta sí/no; si se pulsó Ctrl+C devuelve el error del contexto.
func (d Deps) confirm(ctx context.Context, question string, def bool) (bool, error) {
	ok := d.UI.Confirm(question, def)
	if ctx.Err() != nil {
		return false, ctx.Err()
	}
	return ok, nil
}
