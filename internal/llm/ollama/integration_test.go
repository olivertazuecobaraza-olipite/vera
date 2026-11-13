//go:build integration

// Tests con un Ollama de verdad. No se compilan con «go test ./...»; se lanzan con:
//
//	go test -tags=integration ./...            (o ./build.sh integration)
//	VERA_TEST_MODEL=qwen3.5:9b go test -tags=integration ./internal/llm/ollama
package ollama_test

import (
	"context"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/olivertazuecobaraza-olipite/vera/internal/config"
	"github.com/olivertazuecobaraza-olipite/vera/internal/llm/ollama"
	"github.com/olivertazuecobaraza-olipite/vera/internal/setup"
	"github.com/olivertazuecobaraza-olipite/vera/internal/suggest"
	"github.com/olivertazuecobaraza-olipite/vera/internal/sysinfo"
)

type fixed struct{ model string }

func (f fixed) ModelName() string { return f.model }
func (f fixed) Temp() float64     { return 0 } // lo más determinista posible

func TestIntegracionSugerenciaReal(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Minute)
	defer cancel()

	host := config.NormalizeHost(os.Getenv("OLLAMA_HOST"))
	mgr := setup.NewManager(host)
	if !mgr.Running(ctx) {
		t.Skip("Ollama no está funcionando; me salto el test")
	}
	model := os.Getenv("VERA_TEST_MODEL")
	if model == "" {
		model = "qwen3:0.6b"
	}
	if info, err := mgr.ModelInfo(ctx, model); err != nil || !info.Exists {
		t.Skipf("no está descargado %s (elige otro con VERA_TEST_MODEL)", model)
	}

	s := suggest.New(ollama.NewClient(host), sysinfo.Detect, fixed{model})

	// NO se compara el texto exacto: un modelo no es determinista. Se comprueban propiedades.
	got, err := s.Suggest(ctx, "listar archivos ocultos del directorio actual")
	if err != nil {
		t.Fatal(err)
	}
	t.Logf("sugerencia: %+v", got)
	if !strings.Contains(got.Command, "ls") && !strings.Contains(got.Command, "Get-ChildItem") && !strings.Contains(got.Command, "dir") {
		t.Errorf("esperaba algo con ls/Get-ChildItem/dir, got %q", got.Command)
	}
	if got.Dangerous {
		t.Error("listar archivos no debería ser peligroso")
	}
	if got.Explanation == "" {
		t.Error("debería venir una explicación")
	}
}
