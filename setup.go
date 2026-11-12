// setup.go: instalar/arrancar Ollama y descargar modelos.
// Lo usa el comando "vera ollama [modelo]" y también "vera <petición>" cuando falta algo.
package main

import (
	"archive/tar"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"runtime"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/klauspost/compress/zstd"
)

// ---------- Ctrl+C durante descargas ----------

// onInterrupt ejecuta cleanup y sale si el usuario pulsa Ctrl+C. Devuelve la función para desactivarlo.
func onInterrupt(cleanup func()) (stop func()) {
	sig := make(chan os.Signal, 1)
	signal.Notify(sig, os.Interrupt)
	done := make(chan struct{})
	go func() {
		select {
		case <-sig:
			fmt.Println()
			cleanup()
			os.Exit(130)
		case <-done:
		}
	}()
	return func() { signal.Stop(sig); close(done) }
}

// ---------- Barra de progreso ----------

type progressBar struct {
	label   string
	start   time.Time
	base    int64 // bytes que ya había al empezar (para calcular la velocidad)
	last    time.Time
	lastLen int
	started bool
	done    bool
}

func newBar(label string) *progressBar { return &progressBar{label: label} }

func humanBytes(n int64) string {
	const u = 1024
	switch {
	case n >= u*u*u:
		return fmt.Sprintf("%.2f GB", float64(n)/(u*u*u))
	case n >= u*u:
		return fmt.Sprintf("%.1f MB", float64(n)/(u*u))
	case n >= u:
		return fmt.Sprintf("%.0f KB", float64(n)/u)
	}
	return fmt.Sprintf("%d B", n)
}

func (b *progressBar) update(done, total int64) { b.draw(done, total, false) }

func (b *progressBar) finish(done, total int64) {
	if b.done {
		return
	}
	b.draw(done, total, true)
	b.done = true
	fmt.Println()
}

func (b *progressBar) draw(done, total int64, force bool) {
	now := time.Now()
	if !b.started {
		b.started, b.start, b.base = true, now, done
	}
	// Sin terminal (salida redirigida) solo se dibuja la línea final.
	if !force && (!isTTY || now.Sub(b.last) < 100*time.Millisecond) {
		return
	}
	b.last = now

	const width = 30
	var line string
	if total > 0 {
		if done > total {
			done = total
		}
		frac := float64(done) / float64(total)
		pct := int(frac * 100) // hacia abajo: 100% solo cuando de verdad ha terminado
		fill := int(frac * width)
		bar := color("32", strings.Repeat("█", fill)) + color("90", strings.Repeat("░", width-fill))
		line = fmt.Sprintf("  %s [%s] %3d%%  %s / %s", b.label, bar, pct, humanBytes(done), humanBytes(total))
	} else {
		line = fmt.Sprintf("  %s  %s", b.label, humanBytes(done))
	}
	if secs := now.Sub(b.start).Seconds(); secs > 0.5 && done > b.base {
		line += fmt.Sprintf("  %s/s", humanBytes(int64(float64(done-b.base)/secs)))
	}
	// Se rellena con espacios para borrar restos de la línea anterior (funciona también sin ANSI).
	n := visibleLen(line)
	pad := ""
	if n < b.lastLen {
		pad = strings.Repeat(" ", b.lastLen-n)
	}
	b.lastLen = n
	fmt.Print("\r" + line + pad)
}

// visibleLen cuenta los caracteres que se ven, sin los códigos de color.
func visibleLen(s string) int {
	n, esc := 0, false
	for _, r := range s {
		switch {
		case r == '\x1b':
			esc = true
		case esc:
			if r == 'm' {
				esc = false
			}
		default:
			n++
		}
	}
	return n
}

// progressWriter cuenta lo escrito y actualiza la barra.
type progressWriter struct {
	bar   *progressBar
	done  int64
	total int64
}

func (w *progressWriter) Write(p []byte) (int, error) {
	w.done += int64(len(p))
	w.bar.update(w.done, w.total)
	return len(p), nil
}

// download guarda url en un archivo temporal mostrando la barra de progreso.
// Si se pulsa Ctrl+C, borra el archivo a medias.
func download(rawURL, label string) (string, error) {
	resp, err := http.Get(rawURL)
	if err != nil {
		return "", fmt.Errorf("no hay conexión con %s (%v)", rawURL, err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("%s respondió %s", rawURL, resp.Status)
	}
	u, _ := url.Parse(rawURL)
	f, err := os.CreateTemp("", "vera-*-"+filepath.Base(u.Path))
	if err != nil {
		return "", err
	}
	stop := onInterrupt(func() {
		f.Close()
		os.Remove(f.Name())
		fmt.Println(color("33", "Descarga cancelada."))
	})
	defer stop()

	pw := &progressWriter{bar: newBar(label), total: resp.ContentLength}
	_, err = io.Copy(io.MultiWriter(f, pw), resp.Body)
	f.Close()
	if err == nil && resp.ContentLength > 0 && pw.done != resp.ContentLength {
		err = io.ErrUnexpectedEOF
	}
	if err != nil {
		fmt.Println()
		os.Remove(f.Name())
		return "", fmt.Errorf("la descarga se cortó (%v). Revisa la conexión y repite", err)
	}
	pw.bar.finish(pw.done, pw.total)
	return f.Name(), nil
}

// ---------- Ollama: detectar, instalar y arrancar ----------

func ollamaUp(base string) bool {
	c := &http.Client{Timeout: 2 * time.Second}
	resp, err := c.Get(base + "/api/version")
	if err != nil {
		return false
	}
	resp.Body.Close()
	return resp.StatusCode == http.StatusOK
}

func isLocal(base string) bool {
	u, err := url.Parse(base)
	if err != nil {
		return false
	}
	switch u.Hostname() {
	case "127.0.0.1", "localhost", "::1":
		return true
	}
	return false
}

func macApps() []string {
	home, _ := os.UserHomeDir()
	return []string{"/Applications/Ollama.app", filepath.Join(home, "Applications", "Ollama.app")}
}

// findOllama devuelve la ruta del ejecutable de Ollama, o "" si no está instalado.
func findOllama() string {
	if p, err := exec.LookPath("ollama"); err == nil {
		return p
	}
	home, _ := os.UserHomeDir()
	var candidates []string
	switch runtime.GOOS {
	case "darwin":
		for _, app := range macApps() {
			candidates = append(candidates, filepath.Join(app, "Contents", "Resources", "ollama"))
		}
		candidates = append(candidates, "/opt/homebrew/bin/ollama", "/usr/local/bin/ollama")
	case "linux":
		candidates = []string{filepath.Join(home, ".local", "bin", "ollama"), "/usr/local/bin/ollama", "/usr/bin/ollama"}
	case "windows":
		candidates = []string{filepath.Join(os.Getenv("LOCALAPPDATA"), "Programs", "Ollama", "ollama.exe")}
	}
	for _, c := range candidates {
		if fi, err := os.Stat(c); err == nil && !fi.IsDir() {
			return c
		}
	}
	return ""
}

func installOllama() error {
	switch runtime.GOOS {
	case "darwin":
		return installMac()
	case "linux":
		return installLinux()
	case "windows":
		return installWindows()
	}
	return fmt.Errorf("sistema no soportado: %s", runtime.GOOS)
}

func installMac() error {
	zip, err := download("https://ollama.com/download/Ollama-darwin.zip", "Ollama")
	if err != nil {
		return err
	}
	defer os.Remove(zip)
	defer onInterrupt(func() { os.Remove(zip) })()
	fmt.Println(color("90", "  Descomprimiendo…"))
	dest := "/Applications"
	if err := exec.Command("ditto", "-x", "-k", zip, dest).Run(); err != nil {
		// Sin permisos en /Applications: se instala en ~/Applications.
		home, _ := os.UserHomeDir()
		dest = filepath.Join(home, "Applications")
		if err := os.MkdirAll(dest, 0o755); err != nil {
			return err
		}
		if out, err := exec.Command("ditto", "-x", "-k", zip, dest).CombinedOutput(); err != nil {
			return fmt.Errorf("no se pudo descomprimir: %v %s", err, out)
		}
	}
	fmt.Println(color("32", "✓ Ollama instalado en "+filepath.Join(dest, "Ollama.app")))
	return nil
}

func installLinux() error {
	arch := runtime.GOARCH
	if arch != "amd64" && arch != "arm64" {
		return fmt.Errorf("arquitectura no soportada: %s", arch)
	}
	home, _ := os.UserHomeDir()
	dest := filepath.Join(home, ".local") // quedará en ~/.local/bin/ollama y ~/.local/lib/ollama, sin sudo
	file, err := download("https://ollama.com/download/ollama-linux-"+arch+".tar.zst", "Ollama")
	if err != nil {
		return err
	}
	defer os.Remove(file)
	defer onInterrupt(func() { os.Remove(file) })()

	fmt.Println(color("90", "  Descomprimiendo…"))
	os.RemoveAll(filepath.Join(dest, "lib", "ollama")) // librerías de una versión anterior
	if err := extractTarZst(file, dest); err != nil {
		return fmt.Errorf("no se pudo descomprimir: %v", err)
	}
	fmt.Println(color("32", "✓ Ollama instalado en "+filepath.Join(dest, "bin", "ollama")))
	return nil
}

// extractTarZst descomprime un .tar.zst en dest (sin depender del programa zstd).
func extractTarZst(file, dest string) error {
	f, err := os.Open(file)
	if err != nil {
		return err
	}
	defer f.Close()
	zr, err := zstd.NewReader(f)
	if err != nil {
		return err
	}
	defer zr.Close()
	tr := tar.NewReader(zr)
	root := filepath.Clean(dest) + string(os.PathSeparator)
	for {
		h, err := tr.Next()
		if err == io.EOF {
			return nil
		}
		if err != nil {
			return err
		}
		target := filepath.Join(dest, h.Name)
		if !strings.HasPrefix(target, root) {
			return fmt.Errorf("ruta no válida en el archivo: %s", h.Name)
		}
		switch h.Typeflag {
		case tar.TypeDir:
			if err := os.MkdirAll(target, 0o755); err != nil {
				return err
			}
		case tar.TypeSymlink:
			os.MkdirAll(filepath.Dir(target), 0o755)
			os.Remove(target)
			if err := os.Symlink(h.Linkname, target); err != nil {
				return err
			}
		case tar.TypeReg:
			os.MkdirAll(filepath.Dir(target), 0o755)
			os.Remove(target) // por si es un ejecutable en uso
			out, err := os.OpenFile(target, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, os.FileMode(h.Mode)&0o777|0o600)
			if err != nil {
				return err
			}
			_, err = io.Copy(out, tr)
			out.Close()
			if err != nil {
				return err
			}
		}
	}
}

func installWindows() error {
	exe, err := download("https://ollama.com/download/OllamaSetup.exe", "Ollama")
	if err != nil {
		return err
	}
	defer os.Remove(exe)
	fmt.Println(color("90", "  Instalando (puede tardar un poco)…"))
	// Instalador sin ventanas; se instala en la cuenta del usuario, sin permisos de administrador.
	// Sin capturar la salida: si el instalador abre Ollama, capturarla dejaría a vera esperando para siempre.
	if err := exec.Command(exe, "/VERYSILENT", "/SUPPRESSMSGBOXES", "/NORESTART", "/SP-").Run(); err != nil {
		return fmt.Errorf("el instalador falló: %v", err)
	}
	fmt.Println(color("32", "✓ Ollama instalado"))
	return nil
}

// startOllama arranca el servidor y espera a que responda.
func startOllama(base, bin string) error {
	if runtime.GOOS == "linux" {
		if systemService() {
			return errors.New("Ollama está instalado como servicio pero está parado. Arráncalo con:\n  sudo systemctl start ollama")
		}
		// Si se activó "abrir Ollama al iniciar sesión" en vera config, se arranca con ese servicio.
		if _, err := os.Stat(userUnitPath()); err == nil && exec.Command("systemctl", "--user", "start", userUnit).Run() == nil {
			if waitUp(base, 60*time.Second) {
				return nil
			}
		}
	}
	if runtime.GOOS == "windows" && waitUp(base, 10*time.Second) {
		return nil // el instalador ya abrió Ollama
	}

	started := false
	if real, err := filepath.EvalSymlinks(bin); err == nil {
		bin = real // /usr/local/bin/ollama suele ser un enlace a Ollama.app
	}
	u, _ := url.Parse(base)
	if i := strings.Index(bin, ".app/"); runtime.GOOS == "darwin" && i >= 0 && u.Port() == "11434" {
		// En macOS se abre la app (en segundo plano), que arranca el servidor en el puerto estándar.
		started = exec.Command("open", "-g", bin[:i+len(".app")]).Run() == nil
	}
	if !started {
		cmd := exec.Command(bin, "serve")
		cmd.Env = append(os.Environ(), "OLLAMA_HOST="+u.Host)
		detach(cmd)
		if err := cmd.Start(); err != nil {
			return fmt.Errorf("no se pudo arrancar Ollama: %v", err)
		}
		_ = cmd.Process.Release()
	}
	if !waitUp(base, 60*time.Second) {
		return fmt.Errorf("Ollama no arranca (no responde en %s). Ábrelo a mano para ver qué pasa", base)
	}
	return nil
}

func waitUp(base string, d time.Duration) bool {
	for end := time.Now().Add(d); time.Now().Before(end); time.Sleep(500 * time.Millisecond) {
		if ollamaUp(base) {
			return true
		}
	}
	return false
}

// ensureOllama deja Ollama instalado y funcionando. Si install es false y no está instalado, devuelve errNotInstalled.
var errNotInstalled = errors.New("Ollama no está instalado")

func ensureOllama(base string, install bool) error {
	if ollamaUp(base) {
		return nil
	}
	if !isLocal(base) {
		return fmt.Errorf("no puedo conectar con Ollama en %s (OLLAMA_HOST apunta a otro equipo; ahí no lo puedo instalar ni arrancar)", base)
	}
	bin := findOllama()
	if bin == "" {
		if !install {
			return errNotInstalled
		}
		fmt.Println(color("36", "❯"), "Ollama no está instalado. Descargando…")
		if err := installOllama(); err != nil {
			return fmt.Errorf("no se pudo instalar Ollama: %v", err)
		}
		if bin = findOllama(); bin == "" {
			return errors.New("Ollama se instaló pero no encuentro el ejecutable. Ábrelo a mano y repite")
		}
	}
	fmt.Println(color("90", "  Arrancando Ollama…"))
	if err := startOllama(base, bin); err != nil {
		return err
	}
	fmt.Println(color("32", "✓ Ollama funcionando"))
	return nil
}

// ---------- Modelos ----------

// modelInfo dice si el modelo está descargado y qué sabe hacer ("completion", "embedding"...).
func modelInfo(base, mdl string) (exists bool, caps []string, err error) {
	body, _ := json.Marshal(map[string]string{"model": mdl, "name": mdl}) // "name" para versiones antiguas de Ollama
	c := &http.Client{Timeout: 30 * time.Second}
	resp, err := c.Post(base+"/api/show", "application/json", strings.NewReader(string(body)))
	if err != nil {
		return false, nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return false, nil, nil
	}
	var r struct {
		Capabilities []string `json:"capabilities"`
	}
	_ = json.NewDecoder(resp.Body).Decode(&r)
	return true, r.Capabilities, nil
}

// canChat: si Ollama no informa de capacidades (versiones antiguas), se da por bueno.
func canChat(caps []string) bool {
	if len(caps) == 0 {
		return true
	}
	for _, c := range caps {
		if c == "completion" {
			return true
		}
	}
	return false
}

type pullMsg struct {
	Status    string `json:"status"`
	Digest    string `json:"digest"`
	Total     int64  `json:"total"`
	Completed int64  `json:"completed"`
	Error     string `json:"error"`
}

func pullModel(base, mdl string) error {
	body, _ := json.Marshal(map[string]any{"model": mdl, "name": mdl, "stream": true})
	resp, err := http.Post(base+"/api/pull", "application/json", strings.NewReader(string(body)))
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	label := mdl
	if utf8.RuneCountInString(label) > 24 {
		label = string([]rune(label)[:23]) + "…"
	}
	bar := newBar(label)
	totals, done := map[string]int64{}, map[string]int64{}
	sum := func() (d, t int64) {
		for k, v := range totals {
			t += v
			d += done[k]
		}
		return
	}
	complete := func() { _, t := sum(); bar.finish(t, t) } // descarga terminada: 100%

	stop := onInterrupt(func() {
		fmt.Println(color("33", "Descarga en pausa. Repite el comando y seguirá donde se quedó."))
	})
	defer stop()

	dec := json.NewDecoder(resp.Body)
	for {
		var m pullMsg
		if err := dec.Decode(&m); err == io.EOF {
			break
		} else if err != nil {
			if bar.started && !bar.done {
				fmt.Println()
			}
			return fmt.Errorf("se perdió la conexión con Ollama (%v). Repite el comando para continuar", err)
		}
		if m.Error != "" {
			if bar.started && !bar.done {
				fmt.Println()
			}
			switch {
			case strings.Contains(m.Error, "file does not exist"), strings.Contains(m.Error, "not found"):
				return fmt.Errorf("el modelo %q no existe. Búscalo en https://ollama.com/search", mdl)
			case strings.Contains(m.Error, "invalid model name"):
				return fmt.Errorf("%q no es un nombre de modelo válido (ejemplos: llama3.2, qwen3.5:9b)", mdl)
			case strings.Contains(m.Error, "no space left"):
				return errors.New("no queda espacio en el disco para el modelo")
			}
			return errors.New(m.Error)
		}
		switch {
		case m.Digest != "" && m.Total > 0:
			totals[m.Digest], done[m.Digest] = m.Total, m.Completed
			bar.update(sum())
		case m.Status == "success":
			if bar.started {
				complete()
			}
			return nil
		case bar.started && !bar.done:
			// Se acabó la descarga; ahora Ollama verifica y guarda.
			complete()
			fmt.Println(color("90", "  Verificando…"))
		}
	}
	if bar.started && !bar.done {
		fmt.Println()
	}
	return errors.New("la descarga terminó sin confirmar. Repite el comando para continuar")
}
