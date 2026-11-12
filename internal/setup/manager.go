// Package setup instala y arranca Ollama, y gestiona sus modelos (tamaño, descarga, listado, borrado).
//
// Nunca pregunta nada al usuario ni escribe en la terminal: informa del progreso con callbacks
// y devuelve errores con tipo. Las preguntas («¿Instalar Ollama?») son cosa de la CLI.
package setup

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"time"
)

// Progress informa de cómo va una instalación o una descarga.
type Progress struct {
	Status string // StatusDownloading, StatusExtracting, StatusInstalling, StatusVerifying, StatusSuccess
	Done   int64
	Total  int64
}

const (
	StatusDownloading = "downloading"
	StatusExtracting  = "extracting"
	StatusInstalling  = "installing"
	StatusVerifying   = "verifying"
	StatusSuccess     = "success"
)

var (
	ErrNotInstalled   = errors.New("Ollama no está instalado")
	ErrRemote         = errors.New("OLLAMA_HOST apunta a otro equipo; ahí no puedo instalar ni arrancar Ollama")
	ErrSystemService  = errors.New("Ollama está instalado como servicio pero está parado. Arráncalo con:\n  sudo systemctl start ollama")
	ErrModelNotFound  = errors.New("el modelo no existe")
	ErrInvalidName    = errors.New("no es un nombre de modelo válido")
	ErrNoSpace        = errors.New("no queda espacio en el disco para el modelo")
	ErrUnknownSize    = errors.New("no se pudo averiguar el tamaño")
	ErrPullIncomplete = errors.New("la descarga terminó sin confirmar. Repite el comando para continuar")
)

// Manager instala, arranca y gestiona Ollama. Todo lo que toca el sistema está en campos
// inyectables, para poder probarlo sin instalar nada.
type Manager struct {
	host     string // URL del servidor de Ollama
	baseURL  string // de dónde se descarga Ollama: https://ollama.com
	registry string // registro de modelos de Ollama: https://registry.ollama.ai
	home     string // carpeta del usuario
	appsDir  string // macOS: /Applications
	appData  string // Windows: %LOCALAPPDATA%
	unitDir  string // Linux: carpeta de servicios de usuario de systemd

	http     *http.Client
	lookPath func(string) (string, error)
	stat     func(string) (os.FileInfo, error)
	run      func(ctx context.Context, name string, args ...string) error           // espera a que termine
	output   func(ctx context.Context, name string, args ...string) ([]byte, error) // espera y devuelve la salida
	spawn    func(name string, args, env []string) error                            // arranca y no espera

	waitTimeout time.Duration // cuánto esperar a que Ollama responda tras arrancarlo
	pollEvery   time.Duration
}

// NewManager crea el gestor real para el servidor host.
func NewManager(host string) *Manager {
	home, _ := os.UserHomeDir()
	cfgDir, _ := os.UserConfigDir()
	return &Manager{
		host:        strings.TrimRight(host, "/"),
		baseURL:     "https://ollama.com",
		registry:    "https://registry.ollama.ai",
		home:        home,
		appsDir:     "/Applications",
		appData:     os.Getenv("LOCALAPPDATA"),
		unitDir:     filepath.Join(cfgDir, "systemd", "user"),
		http:        &http.Client{},
		lookPath:    exec.LookPath,
		stat:        os.Stat,
		run:         runCommand,
		output:      outputCommand,
		spawn:       spawnDetached,
		waitTimeout: 60 * time.Second,
		pollEvery:   500 * time.Millisecond,
	}
}

func runCommand(ctx context.Context, name string, args ...string) error {
	// Sin capturar la salida: si el proceso deja hijos abiertos (p. ej. el instalador abre Ollama),
	// capturarla dejaría a vera esperando para siempre.
	return exec.CommandContext(ctx, name, args...).Run()
}

func outputCommand(ctx context.Context, name string, args ...string) ([]byte, error) {
	return exec.CommandContext(ctx, name, args...).CombinedOutput()
}

func spawnDetached(name string, args, env []string) error {
	cmd := exec.Command(name, args...)
	cmd.Env = append(os.Environ(), env...)
	detach(cmd)
	if err := cmd.Start(); err != nil {
		return err
	}
	return cmd.Process.Release()
}

// Host es la URL del servidor de Ollama.
func (m *Manager) Host() string { return m.host }

// IsLocal dice si Ollama está (o estaría) en este mismo equipo.
func (m *Manager) IsLocal() bool {
	u, err := url.Parse(m.host)
	if err != nil {
		return false
	}
	switch u.Hostname() {
	case "127.0.0.1", "localhost", "::1":
		return true
	}
	return false
}

// Running dice si el servidor de Ollama responde.
func (m *Manager) Running(ctx context.Context) bool {
	ctx, cancel := context.WithTimeout(ctx, 2*time.Second)
	defer cancel()
	req, _ := http.NewRequestWithContext(ctx, http.MethodGet, m.host+"/api/version", nil)
	resp, err := m.http.Do(req)
	if err != nil {
		return false
	}
	resp.Body.Close()
	return resp.StatusCode == http.StatusOK
}

// Binary devuelve la ruta del ejecutable de Ollama, o "" si no está instalado.
func (m *Manager) Binary() string {
	if p, err := m.lookPath("ollama"); err == nil {
		return p
	}
	for _, c := range m.candidates() {
		if fi, err := m.stat(c); err == nil && !fi.IsDir() {
			return c
		}
	}
	return ""
}

// IsInstalled dice si Ollama está instalado en este equipo.
func (m *Manager) IsInstalled() bool { return m.Binary() != "" }

// Install descarga e instala Ollama. Devuelve dónde ha quedado.
func (m *Manager) Install(ctx context.Context, p func(Progress)) (string, error) {
	if p == nil {
		p = func(Progress) {}
	}
	where, err := m.install(ctx, p)
	if err != nil {
		if ctx.Err() != nil {
			return "", ctx.Err()
		}
		return "", fmt.Errorf("no se pudo instalar Ollama: %w", err)
	}
	return where, nil
}

// Start arranca el servidor de Ollama y espera a que responda.
func (m *Manager) Start(ctx context.Context) error {
	bin := m.Binary()
	if bin == "" {
		return ErrNotInstalled
	}
	if err := m.start(ctx, bin); err != nil {
		return err
	}
	if !m.waitUp(ctx, m.waitTimeout) {
		if ctx.Err() != nil {
			return ctx.Err()
		}
		return fmt.Errorf("Ollama no arranca (no responde en %s). Ábrelo a mano para ver qué pasa", m.host)
	}
	return nil
}

// EnsureRunning deja Ollama funcionando: si responde, nada; si está cerrado, lo arranca.
// Devuelve ErrNotInstalled si no está instalado y ErrRemote si está en otro equipo y no responde.
func (m *Manager) EnsureRunning(ctx context.Context) error {
	if m.Running(ctx) {
		return nil
	}
	if !m.IsLocal() {
		return fmt.Errorf("no puedo conectar con Ollama en %s (%w)", m.host, ErrRemote)
	}
	if !m.IsInstalled() {
		return ErrNotInstalled
	}
	return m.Start(ctx)
}

// spawnServe arranca «ollama serve» en segundo plano, escuchando en el host configurado.
func (m *Manager) spawnServe(bin string) error {
	u, _ := url.Parse(m.host)
	if err := m.spawn(bin, []string{"serve"}, []string{"OLLAMA_HOST=" + u.Host}); err != nil {
		return fmt.Errorf("no se pudo arrancar Ollama: %w", err)
	}
	return nil
}

func (m *Manager) waitUp(ctx context.Context, d time.Duration) bool {
	end := time.Now().Add(d)
	for {
		if m.Running(ctx) {
			return true
		}
		if time.Now().After(end) {
			return false
		}
		select {
		case <-ctx.Done():
			return false
		case <-time.After(m.pollEvery):
		}
	}
}

func (m *Manager) port() string {
	u, _ := url.Parse(m.host)
	return u.Port()
}

func goarch() string { return runtime.GOARCH }
