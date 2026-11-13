package cli

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/spf13/cobra"

	"github.com/olivertazuecobaraza-olipite/vera/internal/llm"
)

const rootHelp = `vera %s — comandos de terminal con IA local (Ollama)

Uso:
  vera <qué quieres hacer>
  ia <qué quieres hacer>      (alias de vera)
  vera ollama [modelo]        instala Ollama y descarga el modelo si faltan,
                              y lo deja como modelo por defecto
  vera config                 menú para ajustar vera (modelo, descargas, borrar modelos…)
  vera -- <petición>          todo lo que va detrás es la petición (p. ej. «vera -- config»)

Ejemplos:
  vera listar los 5 archivos más grandes de esta carpeta
  vera qué proceso usa el puerto 3000
  vera ollama llama3.2

Variables de entorno:
  VERA_MODEL   modelo de Ollama (ahora: %s)
  OLLAMA_HOST  dirección de Ollama (ahora: %s)
  VERA_SHELL   shell con la que ejecutar (en Windows: "cmd" para usar cmd.exe)
  NO_COLOR     desactiva los colores
`

// NewRootCmd monta la CLI. Ojo con las tres trampas de Cobra con texto libre (ver ARQUITECTURA.md §4).
func NewRootCmd(d Deps) *cobra.Command {
	root := &cobra.Command{
		Use:           "vera <qué quieres hacer>",
		Short:         "Comandos de terminal con IA local (Ollama)",
		Args:          cobra.ArbitraryArgs, // trampa 1: sin esto, el texto libre da «unknown command»
		Version:       d.Version,
		SilenceUsage:  true,
		SilenceErrors: true, // los errores los escribe Run, en español
		RunE: func(cmd *cobra.Command, args []string) error {
			if len(args) == 0 {
				_ = cmd.Help()
				return exitCode(1)
			}
			return runSuggest(cmd.Context(), d, strings.Join(args, " "))
		},
	}
	root.Flags().SetInterspersed(false)             // trampa 2: «ls -la» dentro de la petición no es una opción de vera
	root.CompletionOptions.DisableDefaultCmd = true // trampa 3: «completion» y «help» no son palabras reservadas
	root.SetHelpCommand(&cobra.Command{Hidden: true})
	root.SetVersionTemplate("vera {{.Version}}\n")
	root.SetHelpFunc(func(cmd *cobra.Command, _ []string) {
		if cmd.Long != "" {
			fmt.Fprint(cmd.OutOrStdout(), cmd.Long)
			return
		}
		fmt.Fprintf(cmd.OutOrStdout(), rootHelp, d.Version, d.Config.ModelName(), d.Config.OllamaHost())
	})
	root.AddCommand(newOllamaCmd(d), newConfigCmd(d))
	return root
}

// runSuggest es el flujo principal: Ollama listo → sugerencia → mostrar → preguntar → ejecutar.
func runSuggest(ctx context.Context, d Deps, query string) error {
	model := d.Config.ModelName()
	if err := ensureOllamaForQuery(ctx, d, model); err != nil {
		return err
	}

	sug, err := suggestWithSpinner(ctx, d, query)
	if errors.Is(err, llm.ErrModelNotFound) {
		// Ojo: sin «:=» aquí dentro, para no tapar el err de fuera (el del reintento).
		ok, cerr := confirmDownload(ctx, d, model, "No tienes el modelo %s. ¿Descargarlo ahora? [S/n] ")
		if cerr != nil {
			return cerr
		}
		if !ok {
			return fmt.Errorf("descárgalo cuando quieras con: vera ollama %s", model)
		}
		if perr := pullModel(ctx, d, model); perr != nil {
			return perr
		}
		sug, err = suggestWithSpinner(ctx, d, query)
	}
	switch {
	case errors.Is(err, llm.ErrModelNotFound):
		return fmt.Errorf("no tienes el modelo %q. Descárgalo con:\n  vera ollama %s", model, model)
	case errors.Is(err, llm.ErrNotChat):
		return fmt.Errorf("el modelo %s no sirve para conversar. Elige otro con: vera ollama <modelo>", model)
	case err != nil:
		return err
	}

	d.UI.Suggestion(sug.Command, sug.Explanation, sug.Dangerous, sug.Reason)
	switch askRun(ctx, d) {
	case "run":
		return runCommand(ctx, d, sug.Command)
	case "copy":
		if err := d.Clipboard.Copy(sug.Command); err != nil {
			return fmt.Errorf("no se pudo copiar: %w", err)
		}
		d.UI.Success("Copiado al portapapeles")
	}
	if ctx.Err() != nil {
		return ctx.Err()
	}
	return nil
}

// ensureOllamaForQuery deja Ollama funcionando antes de preguntar al modelo:
// cerrado → se arranca solo; no instalado → se ofrece instalarlo junto con el modelo.
func ensureOllamaForQuery(ctx context.Context, d Deps, model string) error {
	m := d.Manager
	if m.Running(ctx) {
		return nil
	}
	if !m.IsLocal() {
		return remoteError(d)
	}
	if m.IsInstalled() {
		return startOllama(ctx, d)
	}
	st, err := sizeSuffix(ctx, d, model)
	if err != nil {
		st = ""
	}
	ok, err := d.confirm(ctx, fmt.Sprintf("Ollama no está instalado. ¿Instalarlo ahora junto con el modelo %s%s? [S/n] ", model, st), true)
	if err != nil {
		return err
	}
	if !ok {
		return errors.New("sin Ollama vera no puede funcionar. Puedes instalarlo cuando quieras con: vera ollama")
	}
	if err := installOllama(ctx, d); err != nil {
		return err
	}
	if err := startOllama(ctx, d); err != nil {
		return err
	}
	_, _, err = ensureModel(ctx, d, model, false)
	return err
}

func remoteError(d Deps) error {
	return fmt.Errorf("no puedo conectar con Ollama en %s (OLLAMA_HOST apunta a otro equipo; ahí no lo puedo instalar ni arrancar)", d.Config.OllamaHost())
}

// askRun pregunta qué hacer con el comando: "run", "copy" o "" (nada).
func askRun(ctx context.Context, d Deps) string {
	q := "¿Ejecutar? [s/N] "
	if d.Clipboard != nil {
		q = "¿Ejecutar? [s/N/c=copiar] "
	}
	ans, ok := d.UI.ReadLine(q)
	switch {
	case !ok || ctx.Err() != nil:
		return ""
	case d.Clipboard != nil && strings.EqualFold(ans, "c"):
		return "copy"
	case isYesAnswer(ans):
		return "run"
	}
	return ""
}

func warnEnvModel(d Deps, model string) {
	if d.Config.ModelFromEnv() && d.Config.ModelName() != model {
		d.UI.Warn(fmt.Sprintf("Ojo: tienes VERA_MODEL=%s, que tiene prioridad sobre esto.", d.Config.ModelName()))
	}
}
