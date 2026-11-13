package cli

import (
	"context"
	"errors"
	"fmt"

	"github.com/olipite/vera/internal/setup"
	"github.com/olipite/vera/internal/suggest"
	"github.com/olipite/vera/internal/ui"
)

var errCancelled = errors.New("cancelado")

func isYesAnswer(s string) bool { return ui.IsYes(s) }

// suggestWithSpinner pide la sugerencia mostrando un spinner mientras el modelo piensa.
func suggestWithSpinner(ctx context.Context, d Deps, query string) (suggest.Suggestion, error) {
	stop := d.UI.Spinner("Pensando…")
	defer stop()
	return d.Suggester.Suggest(ctx, query)
}

// installOllama descarga e instala Ollama con barra de progreso.
func installOllama(ctx context.Context, d Deps) error {
	d.UI.Step("Ollama no está instalado. Descargando…")
	bar := d.UI.NewBar("Ollama")
	var done, total int64
	where, err := d.Manager.Install(ctx, func(p setup.Progress) {
		switch p.Status {
		case setup.StatusDownloading:
			done, total = p.Done, p.Total
			bar.Update(done, total)
		case setup.StatusExtracting:
			bar.Finish(done, total)
			d.UI.Info("Descomprimiendo…")
		case setup.StatusInstalling:
			bar.Finish(done, total)
			d.UI.Info("Instalando (puede tardar un poco)…")
		}
	})
	if err != nil {
		bar.Abort()
		if ctx.Err() != nil {
			d.UI.Warn("Descarga cancelada.")
			return ctx.Err()
		}
		return err
	}
	d.UI.Success("Ollama instalado en " + where)
	return nil
}

// startOllama arranca Ollama y espera a que responda.
func startOllama(ctx context.Context, d Deps) error {
	d.UI.Info("Arrancando Ollama…")
	if err := d.Manager.Start(ctx); err != nil {
		return err
	}
	d.UI.Success("Ollama funcionando")
	return nil
}

// sizeSuffix devuelve « (4.7 GB)», o "" si no se sabe; error si el modelo no existe.
func sizeSuffix(ctx context.Context, d Deps, model string) (string, error) {
	size, err := d.Manager.RemoteSize(ctx, model)
	switch {
	case errors.Is(err, setup.ErrModelNotFound):
		return "", fmt.Errorf("el modelo %q no existe. Búscalo en https://ollama.com/search", model)
	case err != nil:
		return "", nil
	}
	return " (" + ui.HumanBytes(size) + ")", nil
}

// confirmDownload enseña el tamaño y pide permiso (salvo que se haya desactivado en «vera config»).
// question lleva un %s para «modelo (tamaño)».
func confirmDownload(ctx context.Context, d Deps, model, question string) (bool, error) {
	st, err := sizeSuffix(ctx, d, model)
	if err != nil {
		return false, err
	}
	if !d.Config.Confirm() {
		return true, nil
	}
	return d.confirm(ctx, fmt.Sprintf(question, model+st), true)
}

// pullModel descarga un modelo con barra de progreso.
func pullModel(ctx context.Context, d Deps, model string) error {
	d.UI.Step(fmt.Sprintf("Descargando el modelo %s…", model))
	bar := d.UI.NewBar(model)
	err := d.Manager.Pull(ctx, model, func(p setup.Progress) {
		switch p.Status {
		case setup.StatusDownloading:
			bar.Update(p.Done, p.Total)
		case setup.StatusVerifying:
			bar.Finish(p.Total, p.Total) // descarga terminada: 100%
			d.UI.Info("Verificando…")
		case setup.StatusSuccess:
			if bar.Started() {
				bar.Finish(p.Total, p.Total)
			}
		}
	})
	if err != nil {
		bar.Abort()
		if ctx.Err() != nil {
			d.UI.Warn("Descarga en pausa. Repite el comando y seguirá donde se quedó.")
			return ctx.Err()
		}
		return fmt.Errorf("no se pudo descargar el modelo: %w", err)
	}
	d.UI.Success(fmt.Sprintf("Modelo %s descargado", model))
	return nil
}

// ensureModel descarga el modelo si falta (ask: pedir confirmación con el tamaño antes).
// Devuelve si sirve para chatear y si ya estaba descargado.
func ensureModel(ctx context.Context, d Deps, model string, ask bool) (chat, existed bool, err error) {
	info, err := d.Manager.ModelInfo(ctx, model)
	if err != nil {
		return false, false, err
	}
	if info.Exists {
		d.UI.Success(fmt.Sprintf("El modelo %s ya está descargado", model))
		return info.CanChat(), true, nil
	}
	if ask {
		ok, err := confirmDownload(ctx, d, model, "¿Descargar el modelo %s? [S/n] ")
		if err != nil {
			return false, false, err
		}
		if !ok {
			return false, false, errCancelled
		}
	}
	if err := pullModel(ctx, d, model); err != nil {
		return false, false, err
	}
	info, _ = d.Manager.ModelInfo(ctx, model)
	return info.CanChat(), false, nil
}
