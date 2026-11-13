package suggest_test

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/olipite/vera/internal/llm"
	"github.com/olipite/vera/internal/llm/llmtest"
	"github.com/olipite/vera/internal/suggest"
	"github.com/olipite/vera/internal/sysinfo"
)

type settings struct {
	model string
	temp  float64
}

func (s settings) ModelName() string { return s.model }
func (s settings) Temp() float64     { return s.temp }

func archZsh() sysinfo.Info {
	return sysinfo.Info{OS: "linux", OSName: "Arch Linux", Distro: "arch", Arch: "amd64", Shell: "zsh", ShellKind: "posix", PkgManager: "pacman", Cwd: "/home/ana"}
}

func TestSuggest(t *testing.T) {
	tests := []struct {
		name          string
		llmResp       string
		llmErr        error
		wantCmd       string
		wantExpl      string
		wantDangerous bool
		wantErr       error // nil = sin error; errAny = cualquiera
	}{
		{
			name:     "respuesta válida",
			llmResp:  `{"command":"ss -tulpn","explanation":"Lista los puertos abiertos.","dangerous":false}`,
			wantCmd:  "ss -tulpn",
			wantExpl: "Lista los puertos abiertos.",
		},
		{
			name:     "el modelo responde texto en vez de JSON: plan B",
			llmResp:  "```bash\n$ ls -a\n```",
			wantCmd:  "ls -a",
			wantExpl: "",
		},
		{
			name:    "bloque <think> antes del JSON",
			llmResp: "<think>pienso...</think>\n" + `{"command":"df -h","explanation":"Espacio en disco.","dangerous":false}`,
			wantCmd: "df -h", wantExpl: "Espacio en disco.",
		},
		{name: "comando vacío", llmResp: `{"command":"","explanation":"","dangerous":false}`, wantErr: suggest.ErrEmpty},
		{name: "respuesta vacía", llmResp: "  \n ", wantErr: suggest.ErrEmpty},
		{name: "modelo no descargado", llmErr: llm.ErrModelNotFound, wantErr: llm.ErrModelNotFound},
		{name: "Ollama no responde", llmErr: llm.ErrUnreachable, wantErr: llm.ErrUnreachable},
		{
			name:          "vera detecta el peligro aunque el modelo diga que no",
			llmResp:       `{"command":"rm -rf ~/proyecto","explanation":"Borra la carpeta.","dangerous":false}`,
			wantCmd:       "rm -rf ~/proyecto",
			wantExpl:      "Borra la carpeta.",
			wantDangerous: true,
		},
		{
			name:          "el modelo lo marca como peligroso",
			llmResp:       `{"command":"sudo systemctl stop sshd","explanation":"Para SSH.","dangerous":true}`,
			wantCmd:       "sudo systemctl stop sshd",
			wantExpl:      "Para SSH.",
			wantDangerous: true,
		},
		{
			name:     "comando de varias líneas: se queda con la primera",
			llmResp:  `{"command":"ls -la\nrm x","explanation":"x","dangerous":false}`,
			wantCmd:  "ls -la",
			wantExpl: "x",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			p := &llmtest.FakeProvider{Response: tt.llmResp, Err: tt.llmErr}
			s := suggest.New(p, archZsh, settings{"llama3.2", 0.1})
			got, err := s.Suggest(context.Background(), "ver puertos abiertos")
			if tt.wantErr != nil {
				if !errors.Is(err, tt.wantErr) {
					t.Fatalf("err = %v, quería %v", err, tt.wantErr)
				}
				return
			}
			if err != nil {
				t.Fatalf("error inesperado: %v", err)
			}
			if got.Command != tt.wantCmd || got.Explanation != tt.wantExpl || got.Dangerous != tt.wantDangerous {
				t.Errorf("got %+v", got)
			}
			if got.Dangerous && got.Reason == "" {
				t.Error("un comando peligroso debería llevar motivo")
			}
		})
	}
}

func TestSuggestEnviaContextoYModelo(t *testing.T) {
	p := &llmtest.FakeProvider{Response: `{"command":"ls -a","explanation":"x","dangerous":false}`}
	s := suggest.New(p, archZsh, settings{"qwen2.5", 0.2})
	if _, err := s.Suggest(context.Background(), "archivos ocultos"); err != nil {
		t.Fatal(err)
	}
	req := p.LastReq()
	if req.Model != "qwen2.5" || req.Temperature != 0.2 || req.MaxTokens != suggest.MaxTokens {
		t.Errorf("req = %+v", req)
	}
	if req.Prompt != "archivos ocultos" {
		t.Errorf("Prompt = %q", req.Prompt)
	}
	for _, want := range []string{"zsh", "pacman", "Arch Linux", "/home/ana"} {
		if !strings.Contains(req.System, want) {
			t.Errorf("el prompt de sistema no contiene %q", want)
		}
	}
	if req.Schema == nil {
		t.Error("no se está mandando el JSON schema")
	}
}

func TestSuggestLeeElModeloEnCadaPeticion(t *testing.T) {
	p := &llmtest.FakeProvider{Response: `{"command":"ls","explanation":"","dangerous":false}`}
	cfg := &mutableSettings{model: "a"}
	s := suggest.New(p, archZsh, cfg)
	s.Suggest(context.Background(), "x")
	cfg.model = "b" // p. ej. tras descargar otro modelo
	s.Suggest(context.Background(), "x")
	if p.Requests[0].Model != "a" || p.Requests[1].Model != "b" {
		t.Errorf("modelos usados: %q, %q", p.Requests[0].Model, p.Requests[1].Model)
	}
}

type mutableSettings struct{ model string }

func (m *mutableSettings) ModelName() string { return m.model }
func (m *mutableSettings) Temp() float64     { return 0 }

func TestSuggestSinPeticion(t *testing.T) {
	s := suggest.New(&llmtest.FakeProvider{}, archZsh, settings{})
	if _, err := s.Suggest(context.Background(), "   "); err == nil {
		t.Error("una petición vacía debería dar error")
	}
}
