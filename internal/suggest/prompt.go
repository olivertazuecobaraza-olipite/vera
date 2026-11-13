package suggest

import (
	"embed"
	"strings"
	"text/template"

	"github.com/olipite/vera/internal/sysinfo"
)

//go:embed templates/system.tmpl
var templates embed.FS

var systemTmpl = template.Must(template.ParseFS(templates, "templates/system.tmpl"))

// hints son las pistas por sistema/shell: qué herramientas usar y qué sintaxis.
func hints(info sysinfo.Info) (hint, examples string) {
	switch {
	case info.OS == "darwin":
		return "Usa las herramientas que ya trae macOS (traceroute, ping, lsof, du, find, networksetup, open, pbcopy...) y sintaxis BSD, no de Linux.",
			"'ver por dónde pasan los paquetes hasta google' -> traceroute google.com ; 'qué proceso usa el puerto 3000' -> lsof -i :3000"
	case info.OS == "linux":
		return "Usa herramientas estándar de Linux (ip, ss, ping, traceroute o tracepath, du, find, systemctl, journalctl...) con sintaxis GNU.",
			"'ver mi IP' -> ip -brief address ; 'qué proceso usa el puerto 3000' -> ss -ltnp | grep :3000"
	case info.ShellKind == "cmd":
		return "Usa comandos de cmd.exe y herramientas de Windows (tracert, ping, netstat, ipconfig, dir, findstr, tasklist...). NO uses comandos de Linux ni de PowerShell.",
			"'ver por dónde pasan los paquetes hasta google' -> tracert google.com ; 'qué proceso usa el puerto 3000' -> netstat -ano | findstr :3000"
	case info.ShellKind == "powershell":
		return "Usa cmdlets de PowerShell y herramientas de Windows (Get-ChildItem, Get-Process, Get-NetTCPConnection, Test-NetConnection, tracert, ipconfig...). NO uses comandos de Linux (grep, awk, sed, ls -la...).",
			"'ver por dónde pasan los paquetes hasta google' -> tracert google.com ; 'qué proceso usa el puerto 3000' -> Get-NetTCPConnection -LocalPort 3000 | Select-Object OwningProcess"
	}
	return "Usa herramientas estándar del sistema.", "'listar archivos' -> ls"
}

// BuildPrompt construye el prompt de sistema con el contexto del equipo (SO, shell, gestor de paquetes…).
func BuildPrompt(info sysinfo.Info) string {
	h, ex := hints(info)
	var b strings.Builder
	_ = systemTmpl.Execute(&b, struct {
		Info            sysinfo.Info
		Hints, Examples string
	}{info, h, ex})
	return b.String()
}

// schema es el JSON schema que se le pasa a Ollama para que la respuesta tenga siempre esta forma.
var schema = map[string]any{
	"type": "object",
	"properties": map[string]any{
		"command":     map[string]any{"type": "string"},
		"explanation": map[string]any{"type": "string"},
		"dangerous":   map[string]any{"type": "boolean"},
	},
	"required": []string{"command", "explanation", "dangerous"},
}
