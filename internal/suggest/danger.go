package suggest

import "regexp"

// rule es un patrón de comando peligroso con el motivo que se le enseña al usuario.
type rule struct {
	re     *regexp.Regexp
	reason string
}

func r(pattern, reason string) rule { return rule{regexp.MustCompile(pattern), reason} }

// Reglas para shells tipo Unix (bash, zsh, sh…).
var posixRules = []rule{
	r(`(^|[;&|]\s*|\bsudo\s+)rm\s+(\S+\s+)*(-[a-zA-Z]*[rR][a-zA-Z]*|--recursive)\b`, "borra carpetas enteras sin posibilidad de recuperarlas."),
	r(`\bfind\b.*(\s-delete\b|-exec\s+rm\b)`, "borra todos los archivos que encuentre."),
	r(`\bdd\b.*\bof=/dev/`, "escribe directamente sobre un disco y puede destruir sus datos."),
	r(`\bmkfs(\.\w+)?\b`, "formatea un disco: se pierde todo lo que tiene."),
	r(`\b(fdisk|parted|sfdisk|wipefs|shred)\b`, "modifica o borra discos y particiones."),
	r(`>\s*/dev/(sd|nvme|hd|disk|vd)`, "sobrescribe un disco entero."),
	r(`:\(\)\s*\{.*:\s*\|\s*:`, "bloquea el sistema (bomba de procesos)."),
	r(`\bch(mod|own)\s+(\S+\s+)*-[a-zA-Z]*R[a-zA-Z]*\s+(\S+\s+)*/(\*|\s|$)`, "cambia los permisos de todo el sistema."),
	r(`\b(curl|wget)\b.*\|\s*(sudo\s+)?(ba|z|da)?sh\b`, "ejecuta un script descargado de internet sin revisarlo."),
	r(`\bgit\s+(reset\s+--hard|clean\s+-[a-zA-Z]*f)`, "descarta cambios sin guardar de forma irreversible."),
}

// Reglas para Windows (PowerShell y cmd).
var windowsRules = []rule{
	r(`(?i)\b(remove-item|rm|ri|del|erase|rd|rmdir)\b.*\s(-r(ecurse)?\b|/s\b)`, "borra carpetas enteras sin posibilidad de recuperarlas."),
	r(`(?i)\bformat(-volume)?\s+[a-z]:`, "formatea una unidad: se pierde todo lo que tiene."),
	r(`(?i)\b(format-volume|clear-disk|initialize-disk|diskpart)\b`, "modifica o borra discos y particiones."),
	r(`(?i)\b(iwr|irm|invoke-webrequest|invoke-restmethod|curl|wget)\b.*\|\s*(iex|invoke-expression)\b`, "ejecuta un script descargado de internet sin revisarlo."),
	r(`(?i)\b(cipher\s+/w|vssadmin\s+delete)`, "borra datos de forma irreversible."),
}

// IsDangerous es la segunda opinión de vera (el modelo puede equivocarse): dice si el comando
// es peligroso para la shell indicada ("posix", "powershell" o "cmd") y por qué.
func IsDangerous(cmd, shellKind string) (bool, string) {
	rules := posixRules
	if shellKind == "powershell" || shellKind == "cmd" {
		rules = windowsRules
	}
	for _, rl := range rules {
		if rl.re.MatchString(cmd) {
			return true, rl.reason
		}
	}
	return false, ""
}
