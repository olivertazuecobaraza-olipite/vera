package cli

import (
	"errors"
	"fmt"

	"github.com/spf13/cobra"

	"github.com/olipite/vera/internal/config"
)

const ollamaHelp = `vera ollama [modelo]

Instala Ollama si no está, lo arranca, descarga el modelo si falta
y lo deja como modelo por defecto de vera.

  vera ollama               usa el modelo actual (%s)
  vera ollama llama3.2      cualquier modelo de https://ollama.com/search
`

func newOllamaCmd(d Deps) *cobra.Command {
	cmd := &cobra.Command{
		Use:  "ollama [modelo]",
		Args: cobra.MaximumNArgs(1), // segunda barrera: preRoute ya manda las frases a la raíz
		RunE: func(cmd *cobra.Command, args []string) error {
			ctx := cmd.Context()
			model := d.Config.ModelName()
			if len(args) == 1 {
				model = args[0]
			}
			m := d.Manager

			// 1. Ollama
			switch {
			case m.Running(ctx):
				d.UI.Success("Ollama está instalado y funcionando")
			case !m.IsLocal():
				return remoteError(d)
			default:
				if !m.IsInstalled() {
					if err := installOllama(ctx, d); err != nil {
						return err
					}
				}
				if err := startOllama(ctx, d); err != nil {
					return err
				}
			}

			// 2. Modelo
			chat, existed, err := ensureModel(ctx, d, model, true)
			switch {
			case errors.Is(err, errCancelled):
				d.UI.Println("No se ha descargado nada.")
				return nil
			case err != nil:
				return err
			}
			if !chat {
				d.UI.Println(d.UI.Paint("31", fmt.Sprintf("El modelo %s no sirve para conversar (es de otro tipo, p. ej. embeddings), así que vera no puede usarlo.", model)))
				// Si lo acabamos de descargar, lo normal es borrarlo; si ya lo tenías, por defecto se queda.
				q := "¿Borrarlo para liberar espacio? [S/n] "
				if existed {
					q = "¿Borrarlo para liberar espacio? [s/N] "
				}
				ok, err := d.confirm(ctx, q, !existed)
				if err != nil {
					return err
				}
				if ok {
					if err := m.Delete(ctx, model); err != nil {
						d.UI.Warn("No se pudo borrar: " + err.Error())
					} else {
						d.UI.Success("Borrado")
					}
				}
				return fmt.Errorf("elige un modelo de chat, por ejemplo: vera ollama %s", config.DefaultModel())
			}

			// 3. Modelo por defecto
			d.Config.Model = model
			if err := d.Config.Save(); err != nil {
				d.UI.Warn("No se pudo guardar el modelo por defecto: " + err.Error())
			} else {
				d.UI.Success(fmt.Sprintf("vera usará %s a partir de ahora", model))
			}
			warnEnvModel(d, model)
			return nil
		},
	}
	cmd.Long = fmt.Sprintf(ollamaHelp, d.Config.ModelName())
	return cmd
}
