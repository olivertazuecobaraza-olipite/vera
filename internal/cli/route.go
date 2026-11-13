package cli

// maxArgs dice cuántas palabras admite cada subcomando. Si llegan más, no es el subcomando:
// es una petición en lenguaje natural que empieza por esa palabra
// (p. ej. «vera ollama no me arranca, cómo lo reinicio»).
var maxArgs = map[string]int{"ollama": 1, "config": 0}

// preRoute antepone «--» a esas peticiones para que Cobra las mande a la raíz como texto libre.
func preRoute(args []string) []string {
	if len(args) > 1 {
		if n, ok := maxArgs[args[0]]; ok && len(args)-1 > n && !isHelpFlag(args[1]) {
			return append([]string{"--"}, args...)
		}
	}
	return args
}

func isHelpFlag(s string) bool { return s == "-h" || s == "--help" }
