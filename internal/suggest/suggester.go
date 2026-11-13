// Package suggest es el caso de uso principal: convierte una descripción en lenguaje natural
// en un comando, con el contexto del sistema, y lo valida.
package suggest

import (
	"context"
	"errors"
	"strings"

	"github.com/olipite/vera/internal/llm"
	"github.com/olipite/vera/internal/sysinfo"
)

// Settings es lo que el Suggester necesita de la configuración (*config.Config lo cumple).
// Se lee en cada petición: si el modelo cambia (p. ej. tras descargar otro), se usa el nuevo.
type Settings interface {
	ModelName() string
	Temp() float64
}

// MaxTokens limita la respuesta: basta para un comando y una frase, y evita que un modelo pequeño no pare nunca.
const MaxTokens = 1024

// Suggester propone comandos.
type Suggester struct {
	provider llm.Provider
	info     func() sysinfo.Info
	cfg      Settings
}

// New crea un Suggester. info se llama en cada petición (detecta el sistema actual).
func New(p llm.Provider, info func() sysinfo.Info, cfg Settings) *Suggester {
	return &Suggester{provider: p, info: info, cfg: cfg}
}

// Suggest propone un comando para la descripción query.
func (s *Suggester) Suggest(ctx context.Context, query string) (Suggestion, error) {
	if strings.TrimSpace(query) == "" {
		return Suggestion{}, errors.New("no has dicho qué quieres hacer")
	}
	info := s.info()
	raw, err := s.provider.Generate(ctx, llm.Request{
		Model:       s.cfg.ModelName(),
		System:      BuildPrompt(info),
		Prompt:      query,
		Schema:      schema,
		Temperature: s.cfg.Temp(),
		MaxTokens:   MaxTokens,
	})
	if err != nil {
		return Suggestion{}, err
	}
	sug, err := parse(raw)
	if err != nil {
		return Suggestion{}, err
	}
	// No basta con lo que diga el modelo: vera aplica su propia comprobación.
	if bad, reason := IsDangerous(sug.Command, info.ShellKind); bad {
		sug.Dangerous, sug.Reason = true, reason
	} else if sug.Dangerous {
		sug.Reason = "el modelo lo considera peligroso; revisa bien lo que hace antes de ejecutarlo."
	}
	return sug, nil
}
