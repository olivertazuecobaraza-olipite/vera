package cli

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"strconv"
	"strings"

	"github.com/spf13/cobra"

	"github.com/olipite/vera/internal/setup"
	"github.com/olipite/vera/internal/ui"
)

const configHelp = `vera config

Abre un menú para ajustar vera: modelo por defecto, confirmar antes de descargar,
abrir Ollama al iniciar sesión y borrar modelos.
`

func newConfigCmd(d Deps) *cobra.Command {
	return &cobra.Command{
		Use:  "config",
		Long: configHelp,
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			runConfigMenu(cmd.Context(), d)
			return cmd.Context().Err()
		},
	}
}

func runConfigMenu(ctx context.Context, d Deps) {
	t := d.UI
	for {
		auto := d.Manager.AutostartState(ctx)
		current := d.Config.ModelName()
		if d.Config.ModelFromEnv() {
			current += t.Paint(ui.Gray, " (de VERA_MODEL)")
		}
		t.Println()
		t.Println(t.Paint(ui.Bold, "Configuración de vera"))
		t.Println()
		t.Printf("  1) Modelo por defecto                %s\n", current)
		t.Printf("  2) Confirmar antes de descargar      %s\n", ui.SiNo(d.Config.Confirm()))
		t.Printf("  3) Abrir Ollama al iniciar sesión    %s\n", auto.State)
		t.Printf("  4) Borrar modelos\n")
		t.Printf("  0) Salir\n")
		t.Println()
		opt, ok := t.ReadLine("Elige una opción: ")
		switch {
		case !ok || ctx.Err() != nil || opt == "" || opt == "0" || strings.EqualFold(opt, "q"):
			return
		case opt == "1":
			configModel(ctx, d)
		case opt == "2":
			v := !d.Config.Confirm()
			d.Config.ConfirmDownloads = &v
			report(t, d.Config.Save(), "Confirmar antes de descargar: "+ui.SiNo(v))
		case opt == "3":
			if !auto.CanChange {
				t.Info(auto.Detail)
			} else {
				report(t, d.Manager.SetAutostart(ctx, !auto.Enabled), "Abrir Ollama al iniciar sesión: "+ui.SiNo(!auto.Enabled))
			}
		case opt == "4":
			configDelete(ctx, d)
		default:
			t.Println(t.Paint(ui.Yellow, "  Opción no válida"))
		}
	}
}

func report(t *ui.Terminal, err error, okMsg string) {
	if err != nil {
		t.Println(t.Paint(ui.Red, "  "+err.Error()))
	} else {
		t.Println(t.Paint(ui.Green, "  ✓ "+okMsg))
	}
}

// needOllama deja Ollama funcionando (arrancándolo si hace falta) para las opciones que lo usan.
func needOllama(ctx context.Context, d Deps) bool {
	m := d.Manager
	switch {
	case m.Running(ctx):
		return true
	case !m.IsLocal():
		report(d.UI, remoteError(d), "")
		return false
	case !m.IsInstalled():
		d.UI.Println(d.UI.Paint(ui.Yellow, "  Ollama no está instalado. Instálalo con: vera ollama"))
		return false
	}
	if err := startOllama(ctx, d); err != nil {
		report(d.UI, err, "")
		return false
	}
	return true
}

func sortedModels(ctx context.Context, d Deps) ([]setup.Model, bool) {
	all, err := d.Manager.List(ctx)
	if err != nil {
		report(d.UI, err, "")
		return nil, false
	}
	sort.Slice(all, func(i, j int) bool { return all[i].Name < all[j].Name })
	return all, true
}

func configModel(ctx context.Context, d Deps) {
	t := d.UI
	if !needOllama(ctx, d) {
		return
	}
	all, ok := sortedModels(ctx, d)
	if !ok {
		return
	}
	var chat []setup.Model
	for _, m := range all {
		if info, err := d.Manager.ModelInfo(ctx, m.Name); err == nil && info.CanChat() {
			chat = append(chat, m)
		}
	}
	cur := d.Config.ModelName()
	t.Println()
	for i, m := range chat {
		mark := ""
		if setup.SameModel(m.Name, cur) {
			mark = t.Paint(ui.Green, "  ← actual")
		}
		t.Printf("  %d) %-28s %9s%s\n", i+1, m.Name, ui.HumanBytes(m.Size), mark)
	}
	t.Println("  n) Otro (escribir el nombre; se descarga si hace falta)")
	opt, ok := t.ReadLine("Elige un modelo (Enter para volver): ")
	if !ok || opt == "" {
		return
	}
	var model string
	if strings.EqualFold(opt, "n") {
		if model, ok = t.ReadLine("Nombre del modelo (p. ej. llama3.2): "); !ok || model == "" {
			return
		}
		chatOK, _, err := ensureModel(ctx, d, model, true)
		switch {
		case errors.Is(err, errCancelled):
			return
		case err != nil:
			report(t, err, "")
			return
		case !chatOK:
			t.Println(t.Paint(ui.Red, "  Ese modelo no sirve para conversar. Puedes borrarlo en la opción 4."))
			return
		}
	} else if n, err := strconv.Atoi(opt); err == nil && n >= 1 && n <= len(chat) {
		model = chat[n-1].Name
	} else {
		t.Println(t.Paint(ui.Yellow, "  Opción no válida"))
		return
	}
	d.Config.Model = model
	report(t, d.Config.Save(), "Modelo por defecto: "+model)
	warnEnvModel(d, model)
}

func configDelete(ctx context.Context, d Deps) {
	t := d.UI
	if !needOllama(ctx, d) {
		return
	}
	all, ok := sortedModels(ctx, d)
	if !ok {
		return
	}
	if len(all) == 0 {
		t.Info("No hay modelos descargados.")
		return
	}
	cur := d.Config.ModelName()
	t.Println()
	for i, m := range all {
		mark := ""
		if setup.SameModel(m.Name, cur) {
			mark = t.Paint(ui.Yellow, "  ← modelo por defecto")
		} else if info, err := d.Manager.ModelInfo(ctx, m.Name); err == nil && !info.CanChat() {
			mark = t.Paint(ui.Gray, "  (no sirve para vera)")
		}
		t.Printf("  %d) %-28s %9s%s\n", i+1, m.Name, ui.HumanBytes(m.Size), mark)
	}
	opt, ok := t.ReadLine("Números a borrar, separados por comas (Enter para volver): ")
	if !ok || opt == "" {
		return
	}
	var pick []setup.Model
	var total int64
	seen := map[int]bool{}
	for _, f := range strings.FieldsFunc(opt, func(r rune) bool { return r == ',' || r == ' ' }) {
		n, err := strconv.Atoi(f)
		if err != nil || n < 1 || n > len(all) {
			t.Println(t.Paint(ui.Yellow, "  Opción no válida: "+f))
			return
		}
		if !seen[n] {
			seen[n] = true
			pick = append(pick, all[n-1])
			total += all[n-1].Size
		}
	}
	names := make([]string, len(pick))
	for i, m := range pick {
		names[i] = m.Name
	}
	if ok, _ := d.confirm(ctx, fmt.Sprintf("¿Borrar %s? Se liberarán %s. [s/N] ", strings.Join(names, ", "), ui.HumanBytes(total)), false); !ok {
		return
	}
	for _, m := range pick {
		report(t, d.Manager.Delete(ctx, m.Name), "Borrado "+m.Name)
		if setup.SameModel(m.Name, cur) {
			t.Println(t.Paint(ui.Yellow, "  ! Era tu modelo por defecto. Elige otro en la opción 1."))
		}
	}
}
