package config

import (
	"os"
	"path/filepath"
	"testing"
)

func noEnv(string) string { return "" }

func envOf(m map[string]string) func(string) string {
	return func(k string) string { return m[k] }
}

func TestGuardarYCargar(t *testing.T) {
	path := filepath.Join(t.TempDir(), "vera", "config.json")
	temp, no := 0.3, false
	c := &Config{Path: path, File: File{Model: "llama3.2", Temperature: &temp, ConfirmDownloads: &no}}
	if err := c.Save(); err != nil {
		t.Fatal(err)
	}
	got, err := Load(path)
	if err != nil {
		t.Fatal(err)
	}
	got.Getenv = noEnv
	if got.ModelName() != "llama3.2" || got.Temp() != 0.3 || got.Confirm() {
		t.Errorf("got model=%q temp=%v confirm=%v", got.ModelName(), got.Temp(), got.Confirm())
	}
}

func TestValoresPorDefectoSiNoExiste(t *testing.T) {
	c, err := Load(filepath.Join(t.TempDir(), "no-existe.json"))
	if err != nil {
		t.Fatalf("no debería fallar si no hay archivo: %v", err)
	}
	c.Getenv = noEnv
	if c.OllamaHost() != DefaultHost {
		t.Errorf("host = %q", c.OllamaHost())
	}
	if c.ModelName() != DefaultModel() {
		t.Errorf("model = %q", c.ModelName())
	}
	if c.Temp() != DefaultTemperature || !c.Confirm() {
		t.Errorf("temp=%v confirm=%v", c.Temp(), c.Confirm())
	}
}

func TestArchivoCorrupto(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.json")
	os.WriteFile(path, []byte(`{"model": [esto no cierra`), 0o644)
	c, err := Load(path)
	if err == nil {
		t.Fatal("debería avisar de que el archivo está dañado")
	}
	c.Getenv = noEnv
	if c == nil || c.ModelName() != DefaultModel() {
		t.Error("con el archivo dañado debería seguir funcionando con los valores por defecto")
	}
}

func TestConfigAntiguaSigueValiendo(t *testing.T) {
	// El config.json que ya tienen los usuarios (antes de esta arquitectura).
	path := filepath.Join(t.TempDir(), "config.json")
	os.WriteFile(path, []byte("{\n  \"model\": \"gemma4:12b\",\n  \"confirm_downloads\": false\n}"), 0o644)
	c, err := Load(path)
	if err != nil {
		t.Fatal(err)
	}
	c.Getenv = noEnv
	if c.ModelName() != "gemma4:12b" || c.Confirm() {
		t.Errorf("model=%q confirm=%v", c.ModelName(), c.Confirm())
	}
}

func TestPrioridadEntorno(t *testing.T) {
	c := &Config{File: File{Model: "del-archivo", Host: "otro:1234"}}

	c.Getenv = envOf(map[string]string{"VERA_MODEL": " de-env ", "OLLAMA_HOST": "0.0.0.0"})
	if c.ModelName() != "de-env" || !c.ModelFromEnv() {
		t.Errorf("VERA_MODEL debería mandar: %q", c.ModelName())
	}
	if c.OllamaHost() != "http://127.0.0.1:11434" {
		t.Errorf("OLLAMA_HOST debería mandar: %q", c.OllamaHost())
	}

	c.Getenv = noEnv
	if c.ModelName() != "del-archivo" || c.ModelFromEnv() {
		t.Errorf("sin entorno debería usar el archivo: %q", c.ModelName())
	}
	if c.OllamaHost() != "http://otro:1234" {
		t.Errorf("host del archivo: %q", c.OllamaHost())
	}
}

func TestSaveNoGuardaElEntorno(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.json")
	c := &Config{Path: path, Getenv: envOf(map[string]string{"VERA_MODEL": "de-env"})}
	if err := c.Save(); err != nil {
		t.Fatal(err)
	}
	b, _ := os.ReadFile(path)
	if string(b) != "{}\n" {
		t.Errorf("no debería guardar valores del entorno, guardó: %s", b)
	}
}

func TestNormalizeHost(t *testing.T) {
	tests := map[string]string{
		"":                          DefaultHost,
		"localhost":                 "http://localhost:11434",
		"0.0.0.0":                   "http://127.0.0.1:11434",
		"0.0.0.0:9999":              "http://127.0.0.1:9999",
		"192.168.1.5:8080":          "http://192.168.1.5:8080",
		"https://ollama.example/":   "https://ollama.example:11434",
		"http://127.0.0.1:11434///": "http://127.0.0.1:11434",
	}
	for in, want := range tests {
		if got := NormalizeHost(in); got != want {
			t.Errorf("NormalizeHost(%q) = %q, quería %q", in, got, want)
		}
	}
}

func TestDefaultModel(t *testing.T) {
	if defaultModel("darwin", "arm64") != "qwen3.5:9b-mlx" || defaultModel("linux", "amd64") != "qwen3.5:9b" {
		t.Error("modelo por defecto incorrecto")
	}
}
