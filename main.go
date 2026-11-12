// vera: describe lo que quieres hacer y un modelo local (Ollama) te propone el comando.
// Funciona en macOS, Linux y Windows.
package main

import (
	"bufio"
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"runtime"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"
)

var version = "1.0.0"

// ---------- Configuración ----------

func defaultModel() string {
	// MLX es la variante optimizada para Apple Silicon.
	if runtime.GOOS == "darwin" && runtime.GOARCH == "arm64" {
		return "qwen3.5:9b-mlx"
	}
	return "qwen3.5:9b"
}

func model() string {
	if m := strings.TrimSpace(os.Getenv("VERA_MODEL")); m != "" {
		return m
	}
	if m := strings.TrimSpace(loadConfig().Model); m != "" {
		return m
	}
	return defaultModel()
}

// ollamaURL interpreta OLLAMA_HOST igual que Ollama: "host", "host:puerto" o una URL completa.
func ollamaURL() string {
	h := strings.TrimSpace(os.Getenv("OLLAMA_HOST"))
	if h == "" {
		return "http://127.0.0.1:11434"
	}
	if !strings.Contains(h, "://") {
		h = "http://" + h
	}
	u, err := url.Parse(h)
	if err != nil {
		return "http://127.0.0.1:11434"
	}
	host, port := u.Hostname(), u.Port()
	if host == "" || host == "0.0.0.0" || host == "::" {
		host = "127.0.0.1" // 0.0.0.0 sirve para escuchar, no para conectarse
	}
	if port == "" {
		port = "11434"
	}
	u.Host = net.JoinHostPort(host, port)
	return strings.TrimRight(u.String(), "/")
}

// ---------- Sistema y shell ----------

type shell struct {
	name string   // nombre legible para el prompt
	path string   // ejecutable
	args []string // argumentos antes del comando
	kind string   // "posix", "powershell" o "cmd"
}

func detectShell() shell {
	if runtime.GOOS == "windows" {
		if strings.EqualFold(os.Getenv("VERA_SHELL"), "cmd") {
			return shell{"cmd.exe", "cmd.exe", []string{"/C"}, "cmd"}
		}
		if p, err := exec.LookPath("pwsh"); err == nil {
			return shell{"PowerShell 7", p, []string{"-NoProfile", "-Command"}, "powershell"}
		}
		return shell{"Windows PowerShell", "powershell.exe", []string{"-NoProfile", "-Command"}, "powershell"}
	}
	sh := os.Getenv("VERA_SHELL")
	if sh == "" {
		sh = os.Getenv("SHELL")
	}
	if sh == "" {
		sh = "/bin/sh"
	}
	return shell{filepath.Base(sh), sh, []string{"-c"}, "posix"}
}

func osName() string {
	switch runtime.GOOS {
	case "darwin":
		return "macOS"
	case "windows":
		return "Windows"
	case "linux":
		if b, err := os.ReadFile("/etc/os-release"); err == nil {
			for _, l := range strings.Split(string(b), "\n") {
				if v, ok := strings.CutPrefix(l, "PRETTY_NAME="); ok {
					return strings.Trim(v, `"`)
				}
			}
		}
		return "Linux"
	}
	return runtime.GOOS
}

func systemPrompt(sh shell, cwd string) string {
	var hints string
	switch {
	case runtime.GOOS == "darwin":
		hints = `Usa las herramientas que ya trae macOS (traceroute, ping, lsof, du, find, networksetup, open, pbcopy...) y sintaxis BSD, no de Linux.
Ejemplos: 'ver por dónde pasan los paquetes hasta google' -> traceroute google.com ; 'qué proceso usa el puerto 3000' -> lsof -i :3000`
	case runtime.GOOS == "linux":
		hints = `Usa herramientas estándar de Linux (ip, ss, ping, traceroute o tracepath, du, find, systemctl, journalctl...) con sintaxis GNU.
Ejemplos: 'ver mi IP' -> ip -brief address ; 'qué proceso usa el puerto 3000' -> ss -ltnp | grep :3000`
	case sh.kind == "cmd":
		hints = `Usa comandos de cmd.exe y herramientas de Windows (tracert, ping, netstat, ipconfig, dir, findstr, tasklist...). NO uses comandos de Linux ni de PowerShell.
Ejemplos: 'ver por dónde pasan los paquetes hasta google' -> tracert google.com ; 'qué proceso usa el puerto 3000' -> netstat -ano | findstr :3000`
	default: // PowerShell
		hints = `Usa cmdlets de PowerShell y herramientas de Windows (Get-ChildItem, Get-Process, Get-NetTCPConnection, Test-NetConnection, tracert, ipconfig...). NO uses comandos de Linux (grep, awk, sed, ls -la...).
Ejemplos: 'ver por dónde pasan los paquetes hasta google' -> tracert google.com ; 'qué proceso usa el puerto 3000' -> Get-NetTCPConnection -LocalPort 3000 | Select-Object OwningProcess`
	}
	return fmt.Sprintf(`Eres un experto en terminal. Sistema: %s (%s), shell: %s, directorio actual: %s.
Responde SOLO con el comando que resuelve la petición, en una sola línea, sin explicaciones, sin markdown y sin comillas invertidas.
Usa siempre el comando MÁS SIMPLE posible. Si existe una herramienta que hace exactamente lo pedido, úsala directamente en vez de construir una cadena de pipes.
%s`, osName(), runtime.GOARCH, sh.name, cwd, hints)
}

// ---------- Ollama ----------

type message struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

type chatRequest struct {
	Model    string         `json:"model"`
	Messages []message      `json:"messages"`
	Stream   bool           `json:"stream"`
	Think    *bool          `json:"think,omitempty"`
	Options  map[string]any `json:"options"`
}

type chatResponse struct {
	Message message `json:"message"`
	Error   string  `json:"error"`
}

var errConnect = errors.New("connect")

func ask(base, mdl, system, user string, think *bool) (string, error) {
	body, _ := json.Marshal(chatRequest{
		Model:    mdl,
		Messages: []message{{"system", system}, {"user", user}},
		Stream:   false,
		Think:    think,
		// num_predict: solo queremos una línea; sin límite, algunos modelos pequeños no paran nunca.
		Options: map[string]any{"temperature": 0.1, "num_predict": 1024},
	})
	client := &http.Client{Timeout: 3 * time.Minute}
	resp, err := client.Post(base+"/api/chat", "application/json", bytes.NewReader(body))
	var ne net.Error
	if errors.As(err, &ne) && ne.Timeout() {
		return "", errors.New("Ollama tardó demasiado en responder (más de 3 minutos). Prueba otra vez o usa un modelo más ligero")
	}
	if err != nil {
		return "", errConnect
	}
	defer resp.Body.Close()
	raw, _ := io.ReadAll(resp.Body)
	var r chatResponse
	if err := json.Unmarshal(raw, &r); err != nil {
		return "", fmt.Errorf("respuesta inesperada de Ollama: %s", strings.TrimSpace(string(raw)))
	}
	if r.Error != "" {
		return "", errors.New(r.Error)
	}
	return r.Message.Content, nil
}

var (
	thinkRe = regexp.MustCompile(`(?s)<think>.*?</think>`)
	fenceRe = regexp.MustCompile("^```[a-zA-Z]*$")
)

// cleanCommand se queda con la primera línea útil de la respuesta del modelo.
func cleanCommand(s string) string {
	s = thinkRe.ReplaceAllString(s, "")
	for _, l := range strings.Split(s, "\n") {
		l = strings.TrimSpace(l)
		if l == "" || fenceRe.MatchString(l) {
			continue
		}
		l = strings.Trim(l, "`")
		for _, p := range []string{"$ ", "PS> ", "> "} {
			l = strings.TrimPrefix(l, p)
		}
		return strings.TrimSpace(l)
	}
	return ""
}

// ---------- Salida ----------

var (
	useColor = true
	isTTY    = true
)

func color(code, s string) string {
	if !useColor {
		return s
	}
	return "\x1b[" + code + "m" + s + "\x1b[0m"
}

var stdin = bufio.NewReader(os.Stdin)

// confirm hace una pregunta de sí/no. Con Enter vale def; sin respuesta (stdin cerrado) es siempre "no".
func confirm(question string, def bool) bool {
	fmt.Print(question)
	line, err := stdin.ReadString('\n')
	ans := strings.ToLower(strings.TrimSpace(line))
	if ans == "" {
		if err != nil {
			fmt.Println()
			return false
		}
		return def
	}
	switch ans {
	case "s", "si", "sí", "y", "yes":
		return true
	}
	return false
}

func fail(format string, a ...any) {
	msg := fmt.Sprintf(format, a...)
	if r, n := utf8.DecodeRuneInString(msg); n > 0 {
		msg = string(unicode.ToUpper(r)) + msg[n:]
	}
	fmt.Fprintln(os.Stderr, color("31", msg))
	os.Exit(1)
}

const usage = `vera %s — comandos de terminal con IA local (Ollama)

Uso:
  vera <qué quieres hacer>
  ia <qué quieres hacer>      (alias de vera)
  vera ollama [modelo]        instala Ollama y descarga el modelo si faltan,
                              y lo deja como modelo por defecto
  vera config                 menú para ajustar vera (modelo, descargas, borrar modelos…)

Ejemplos:
  vera listar los 5 archivos más grandes de esta carpeta
  vera qué proceso usa el puerto 3000
  vera ollama llama3.2

Variables de entorno:
  VERA_MODEL   modelo de Ollama (ahora: %s)
  OLLAMA_HOST  dirección de Ollama (por defecto: 127.0.0.1:11434)
  VERA_SHELL   shell con la que ejecutar (en Windows: "cmd" para usar cmd.exe)
  NO_COLOR     desactiva los colores
`

func main() {
	stat, _ := os.Stdout.Stat()
	isTTY = stat.Mode()&os.ModeCharDevice != 0
	useColor = os.Getenv("NO_COLOR") == "" && isTTY && enableColor()

	args := os.Args[1:]
	if len(args) == 0 || args[0] == "-h" || args[0] == "--help" {
		fmt.Printf(usage, version, model())
		if len(args) == 0 {
			os.Exit(1)
		}
		return
	}
	if args[0] == "-v" || args[0] == "--version" {
		fmt.Println("vera", version)
		return
	}
	if args[0] == "config" && len(args) == 1 {
		runConfig()
		return
	}
	if args[0] == "ollama" && len(args) <= 2 {
		mdl := ""
		if len(args) == 2 {
			mdl = args[1]
		}
		runSetup(mdl)
		return
	}

	sh := detectShell()
	cwd, _ := os.Getwd()
	base, mdl := ollamaURL(), model()
	no := false

	query := func() (string, error) {
		out, err := ask(base, mdl, systemPrompt(sh, cwd), strings.Join(args, " "), &no)
		if err != nil && strings.Contains(err.Error(), "think") {
			// Modelos sin modo "pensar": repetir sin ese parámetro.
			out, err = ask(base, mdl, systemPrompt(sh, cwd), strings.Join(args, " "), nil)
		}
		return out, err
	}
	modelMissing := func(err error) bool { return err != nil && strings.Contains(err.Error(), "not found") }

	out, err := query()
	if errors.Is(err, errConnect) {
		// Ollama instalado pero cerrado: se arranca solo. No instalado: se ofrece instalarlo.
		switch e := ensureOllama(base, false); {
		case errors.Is(e, errNotInstalled):
			st, _ := sizeText(mdl)
			if !confirm(fmt.Sprintf("Ollama no está instalado. ¿Instalarlo ahora junto con el modelo %s%s? [S/n] ", mdl, st), true) {
				fail("Sin Ollama vera no puede funcionar. Puedes instalarlo cuando quieras con: vera ollama")
			}
			if e = ensureOllama(base, true); e != nil {
				fail("%v", e)
			}
			if _, _, e = ensureModel(base, mdl, false); e != nil {
				fail("No se pudo descargar el modelo: %v", e)
			}
		case e != nil:
			fail("%v", e)
		}
		out, err = query()
	}
	if modelMissing(err) {
		ok, e := confirmDownload(mdl, "No tienes el modelo %s. ¿Descargarlo ahora? [S/n] ")
		if e != nil {
			fail("%v", e)
		}
		if !ok {
			fail("Descárgalo cuando quieras con: vera ollama %s", mdl)
		}
		if _, _, e := ensureModel(base, mdl, false); e != nil {
			fail("No se pudo descargar el modelo: %v", e)
		}
		out, err = query()
	}
	switch {
	case errors.Is(err, errConnect):
		fail("No puedo conectar con Ollama en %s.", base)
	case modelMissing(err):
		fail("No tienes el modelo %q. Descárgalo con:\n  vera ollama %s", mdl, mdl)
	case err != nil && strings.Contains(err.Error(), "does not support"):
		fail("El modelo %s no sirve para conversar. Elige otro con: vera ollama <modelo>", mdl)
	case err != nil:
		fail("Error de Ollama: %v", err)
	}

	cmd := cleanCommand(out)
	if cmd == "" {
		fail("El modelo no devolvió ningún comando.")
	}

	fmt.Println(color("36", "❯"), cmd)
	if !confirm("¿Ejecutar? [s/N] ", false) {
		return
	}

	run := cmd
	if sh.kind == "powershell" {
		// Para que los acentos se vean bien en la consola de Windows.
		run = "[Console]::OutputEncoding=[Text.Encoding]::UTF8; " + cmd
	}
	c := exec.Command(sh.path, append(sh.args, run)...)
	c.Stdin, c.Stdout, c.Stderr = os.Stdin, os.Stdout, os.Stderr
	code := 0
	if err := c.Run(); err != nil {
		var ee *exec.ExitError
		if errors.As(err, &ee) {
			code = ee.ExitCode()
		} else {
			fail("No se pudo ejecutar con %s: %v", sh.name, err)
		}
	}
	if code == 0 {
		fmt.Println(color("32", "✓ terminado"), color("90", "(si no ves nada arriba, el comando no produjo salida)"))
	} else {
		fmt.Println(color("31", fmt.Sprintf("✗ el comando falló (código %d)", code)))
	}
	os.Exit(code)
}
