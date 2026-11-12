// config.go: configuración guardada y el menú "vera config".
package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"sort"
	"strconv"
	"strings"
)

// ---------- Configuración guardada ----------

type config struct {
	Model            string `json:"model,omitempty"`
	ConfirmDownloads *bool  `json:"confirm_downloads,omitempty"` // nil = sí
}

func (c config) confirmDownloads() bool { return c.ConfirmDownloads == nil || *c.ConfirmDownloads }

func configPath() string {
	d, err := os.UserConfigDir()
	if err != nil {
		return ""
	}
	return filepath.Join(d, "vera", "config.json")
}

func loadConfig() config {
	var c config
	if p := configPath(); p != "" {
		if b, err := os.ReadFile(p); err == nil {
			_ = json.Unmarshal(b, &c)
		}
	}
	return c
}

func saveConfig(c config) error {
	p := configPath()
	if p == "" {
		return errors.New("no encuentro la carpeta de configuración")
	}
	if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
		return err
	}
	b, _ := json.MarshalIndent(c, "", "  ")
	return os.WriteFile(p, b, 0o644)
}

// ---------- Abrir Ollama al iniciar sesión ----------

// En macOS y Windows lo hace la propia app de Ollama. En Linux (instalado por vera, sin servicio)
// se usa un servicio de usuario de systemd.
const userUnit = "ollama-vera.service"

func userUnitPath() string {
	d, err := os.UserConfigDir()
	if err != nil {
		return ""
	}
	return filepath.Join(d, "systemd", "user", userUnit)
}

func systemService() bool {
	for _, unit := range []string{"/etc/systemd/system/ollama.service", "/usr/lib/systemd/system/ollama.service", "/lib/systemd/system/ollama.service"} {
		if _, err := os.Stat(unit); err == nil {
			return true
		}
	}
	return false
}

func userSystemd() bool {
	if _, err := exec.LookPath("systemctl"); err != nil {
		return false
	}
	out, _ := exec.Command("systemctl", "--user", "is-system-running").Output()
	switch strings.TrimSpace(string(out)) {
	case "running", "degraded", "starting":
		return true
	}
	return false
}

// autostartState devuelve el estado (corto, para el menú), una explicación y si vera puede cambiarlo.
func autostartState() (state, detail string, can bool) {
	switch runtime.GOOS {
	case "darwin":
		return "automático (lo hace la app de Ollama)", "En macOS lo controla la app de Ollama: Ajustes del Sistema → General → Ítems de inicio.", false
	case "windows":
		return "automático (lo hace la app de Ollama)", "En Windows lo controla la app de Ollama: Configuración → Aplicaciones → Inicio.", false
	}
	if systemService() {
		return "sí (servicio del sistema)", "Ollama está instalado como servicio del sistema; se gestiona con: sudo systemctl enable|disable ollama", false
	}
	if !userSystemd() {
		return "no disponible", "Este sistema no usa systemd, así que vera no puede programar el arranque. vera abrirá Ollama cuando lo necesite.", false
	}
	out, _ := exec.Command("systemctl", "--user", "is-enabled", userUnit).Output()
	return siNo(strings.TrimSpace(string(out)) == "enabled"), "", true
}

func setAutostart(on bool) error {
	if !on {
		// Solo deja de arrancar al iniciar sesión; si está abierto, sigue abierto.
		return exec.Command("systemctl", "--user", "disable", userUnit).Run()
	}
	bin := findOllama()
	if bin == "" {
		return errors.New("Ollama no está instalado. Instálalo primero con: vera ollama")
	}
	unit := fmt.Sprintf(`[Unit]
Description=Ollama (activado desde vera config)
After=network-online.target

[Service]
ExecStart=%s serve
Restart=on-failure
RestartSec=3

[Install]
WantedBy=default.target
`, bin)
	p := userUnitPath()
	if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
		return err
	}
	if err := os.WriteFile(p, []byte(unit), 0o644); err != nil {
		return err
	}
	for _, args := range [][]string{{"daemon-reload"}, {"enable", userUnit}} {
		if out, err := exec.Command("systemctl", append([]string{"--user"}, args...)...).CombinedOutput(); err != nil {
			return fmt.Errorf("systemctl %s: %v %s", strings.Join(args, " "), err, out)
		}
	}
	return nil
}

// ---------- Menú ----------

func siNo(b bool) string {
	if b {
		return "sí"
	}
	return "no"
}

// readLine pregunta y devuelve la respuesta; ok es false si se cerró la entrada (Ctrl+D).
func readLine(prompt string) (string, bool) {
	fmt.Print(prompt)
	line, err := stdin.ReadString('\n')
	if err != nil && line == "" {
		fmt.Println()
		return "", false
	}
	return strings.TrimSpace(line), true
}

func runConfig() {
	base := ollamaURL()
	for {
		c := loadConfig()
		auto, _, _ := autostartState()
		current := model()
		if os.Getenv("VERA_MODEL") != "" {
			current += color("90", " (de VERA_MODEL)")
		}
		fmt.Println()
		fmt.Println(color("1", "Configuración de vera"))
		fmt.Println()
		fmt.Printf("  1) Modelo por defecto                %s\n", current)
		fmt.Printf("  2) Confirmar antes de descargar      %s\n", siNo(c.confirmDownloads()))
		fmt.Printf("  3) Abrir Ollama al iniciar sesión    %s\n", auto)
		fmt.Printf("  4) Borrar modelos\n")
		fmt.Printf("  0) Salir\n")
		fmt.Println()
		opt, ok := readLine("Elige una opción: ")
		switch {
		case !ok || opt == "" || opt == "0" || strings.EqualFold(opt, "q"):
			return
		case opt == "1":
			configModel(base)
		case opt == "2":
			v := !c.confirmDownloads()
			c.ConfirmDownloads = &v
			report(saveConfig(c), "Confirmar antes de descargar: "+siNo(v))
		case opt == "3":
			if state, detail, can := autostartState(); !can {
				fmt.Println(color("90", "  "+detail))
			} else {
				on := state != "sí"
				report(setAutostart(on), "Abrir Ollama al iniciar sesión: "+siNo(on))
			}
		case opt == "4":
			configDelete(base)
		default:
			fmt.Println(color("33", "  Opción no válida"))
		}
	}
}

func report(err error, okMsg string) {
	if err != nil {
		fmt.Println(color("31", "  "+err.Error()))
	} else {
		fmt.Println(color("32", "  ✓ "+okMsg))
	}
}

// needOllama comprueba que Ollama está funcionando (arrancándolo si hace falta) para las opciones que lo usan.
func needOllama(base string) bool {
	err := ensureOllama(base, false)
	if errors.Is(err, errNotInstalled) {
		fmt.Println(color("33", "  Ollama no está instalado. Instálalo con: vera ollama"))
		return false
	}
	if err != nil {
		fmt.Println(color("31", "  "+err.Error()))
		return false
	}
	return true
}

func configModel(base string) {
	if !needOllama(base) {
		return
	}
	all, err := listModels(base)
	if err != nil {
		fmt.Println(color("31", "  No puedo leer los modelos: "+err.Error()))
		return
	}
	var chat []localModel
	for _, m := range all {
		if _, caps, err := modelInfo(base, m.Name); err == nil && canChat(caps) {
			chat = append(chat, m)
		}
	}
	sort.Slice(chat, func(i, j int) bool { return chat[i].Name < chat[j].Name })

	cur := model()
	fmt.Println()
	for i, m := range chat {
		mark := ""
		if sameModel(m.Name, cur) {
			mark = color("32", "  ← actual")
		}
		fmt.Printf("  %d) %-28s %9s%s\n", i+1, m.Name, humanBytes(m.Size), mark)
	}
	fmt.Println("  n) Otro (escribir el nombre; se descarga si hace falta)")
	opt, ok := readLine("Elige un modelo (Enter para volver): ")
	if !ok || opt == "" {
		return
	}
	var mdl string
	if strings.EqualFold(opt, "n") {
		if mdl, ok = readLine("Nombre del modelo (p. ej. llama3.2): "); !ok || mdl == "" {
			return
		}
		chatOK, _, err := ensureModel(base, mdl, true)
		switch {
		case errors.Is(err, errCancelled):
			return
		case err != nil:
			fmt.Println(color("31", "  "+err.Error()))
			return
		case !chatOK:
			fmt.Println(color("31", "  Ese modelo no sirve para conversar. Puedes borrarlo en la opción 4."))
			return
		}
	} else if n, err := strconv.Atoi(opt); err == nil && n >= 1 && n <= len(chat) {
		mdl = chat[n-1].Name
	} else {
		fmt.Println(color("33", "  Opción no válida"))
		return
	}
	c := loadConfig()
	c.Model = mdl
	report(saveConfig(c), "Modelo por defecto: "+mdl)
	warnEnvModel(mdl)
}

func configDelete(base string) {
	if !needOllama(base) {
		return
	}
	all, err := listModels(base)
	if err != nil {
		fmt.Println(color("31", "  No puedo leer los modelos: "+err.Error()))
		return
	}
	if len(all) == 0 {
		fmt.Println(color("90", "  No hay modelos descargados."))
		return
	}
	sort.Slice(all, func(i, j int) bool { return all[i].Name < all[j].Name })
	cur := model()
	fmt.Println()
	for i, m := range all {
		mark := ""
		if sameModel(m.Name, cur) {
			mark = color("33", "  ← modelo por defecto")
		} else if _, caps, err := modelInfo(base, m.Name); err == nil && !canChat(caps) {
			mark = color("90", "  (no sirve para vera)")
		}
		fmt.Printf("  %d) %-28s %9s%s\n", i+1, m.Name, humanBytes(m.Size), mark)
	}
	opt, ok := readLine("Números a borrar, separados por comas (Enter para volver): ")
	if !ok || opt == "" {
		return
	}
	var pick []localModel
	var total int64
	seen := map[int]bool{}
	for _, f := range strings.FieldsFunc(opt, func(r rune) bool { return r == ',' || r == ' ' }) {
		n, err := strconv.Atoi(f)
		if err != nil || n < 1 || n > len(all) {
			fmt.Println(color("33", "  Opción no válida: "+f))
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
	if !confirm(fmt.Sprintf("¿Borrar %s? Se liberarán %s. [s/N] ", strings.Join(names, ", "), humanBytes(total)), false) {
		return
	}
	for _, m := range pick {
		report(deleteModel(base, m.Name), "Borrado "+m.Name)
		if sameModel(m.Name, cur) {
			fmt.Println(color("33", "  ! Era tu modelo por defecto. Elige otro en la opción 1."))
		}
	}
}
