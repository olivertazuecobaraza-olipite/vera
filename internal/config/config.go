// Package config guarda y carga la configuración de vera (config.json).
//
// Prioridad de cada valor: variable de entorno > archivo > valor por defecto.
// El archivo solo guarda lo que el usuario ha elegido; las variables de entorno nunca se escriben en él.
package config

import (
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"net/url"
	"os"
	"path/filepath"
	"runtime"
	"strings"
)

// File es lo que se guarda en disco. Los campos vacíos/nil significan «usar el valor por defecto».
type File struct {
	Model            string   `json:"model,omitempty"`
	Host             string   `json:"host,omitempty"`
	Temperature      *float64 `json:"temperature,omitempty"`
	ConfirmDownloads *bool    `json:"confirm_downloads,omitempty"`
}

// Config es la configuración en uso: lo guardado + las variables de entorno.
type Config struct {
	File
	Path   string              // dónde se guarda
	Getenv func(string) string // os.Getenv; inyectable en tests
}

const (
	DefaultHost        = "http://127.0.0.1:11434"
	DefaultTemperature = 0.1
)

// DefaultModel es el modelo que se usa si no se ha elegido otro.
func DefaultModel() string { return defaultModel(runtime.GOOS, runtime.GOARCH) }

func defaultModel(goos, goarch string) string {
	// MLX es la variante optimizada para Apple Silicon.
	if goos == "darwin" && goarch == "arm64" {
		return "qwen3.5:9b-mlx"
	}
	return "qwen3.5:9b"
}

// DefaultPath es config.json dentro de la carpeta de configuración del usuario.
func DefaultPath() string {
	d, err := os.UserConfigDir()
	if err != nil {
		return ""
	}
	return filepath.Join(d, "vera", "config.json")
}

// Load lee el archivo. Si no existe, devuelve los valores por defecto sin error.
// Si está corrupto, devuelve igualmente una configuración usable (por defecto) y el error.
func Load(path string) (*Config, error) {
	c := &Config{Path: path, Getenv: os.Getenv}
	b, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) || path == "" {
		return c, nil
	}
	if err != nil {
		return c, fmt.Errorf("no puedo leer %s: %w", path, err)
	}
	if err := json.Unmarshal(b, &c.File); err != nil {
		c.File = File{}
		return c, fmt.Errorf("el archivo de configuración %s está dañado (%w); se usan los valores por defecto", path, err)
	}
	return c, nil
}

// Save guarda solo lo elegido por el usuario (no las variables de entorno).
func (c *Config) Save() error {
	if c.Path == "" {
		return errors.New("no encuentro la carpeta de configuración")
	}
	if err := os.MkdirAll(filepath.Dir(c.Path), 0o755); err != nil {
		return err
	}
	b, _ := json.MarshalIndent(c.File, "", "  ")
	return os.WriteFile(c.Path, append(b, '\n'), 0o644)
}

func (c *Config) env(k string) string {
	if c.Getenv == nil {
		return strings.TrimSpace(os.Getenv(k))
	}
	return strings.TrimSpace(c.Getenv(k))
}

// ModelName es el modelo en uso: VERA_MODEL > config.json > por defecto.
func (c *Config) ModelName() string {
	if m := c.env("VERA_MODEL"); m != "" {
		return m
	}
	if m := strings.TrimSpace(c.Model); m != "" {
		return m
	}
	return DefaultModel()
}

// ModelFromEnv dice si el modelo viene de VERA_MODEL (y por tanto manda sobre lo guardado).
func (c *Config) ModelFromEnv() bool { return c.env("VERA_MODEL") != "" }

// OllamaHost es la URL de Ollama: OLLAMA_HOST > config.json > por defecto.
// Se interpreta igual que Ollama: "host", "host:puerto" o una URL completa.
func (c *Config) OllamaHost() string {
	h := c.env("OLLAMA_HOST")
	if h == "" {
		h = strings.TrimSpace(c.Host)
	}
	return NormalizeHost(h)
}

// NormalizeHost convierte "host", "host:puerto", "0.0.0.0" o una URL en una URL a la que conectarse.
func NormalizeHost(h string) string {
	if h == "" {
		return DefaultHost
	}
	if !strings.Contains(h, "://") {
		h = "http://" + h
	}
	u, err := url.Parse(h)
	if err != nil {
		return DefaultHost
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

// Temp es la temperatura del modelo (baja = respuestas más deterministas).
func (c *Config) Temp() float64 {
	if c.Temperature != nil {
		return *c.Temperature
	}
	return DefaultTemperature
}

// Confirm dice si hay que pedir confirmación antes de descargar modelos.
func (c *Config) Confirm() bool { return c.ConfirmDownloads == nil || *c.ConfirmDownloads }
