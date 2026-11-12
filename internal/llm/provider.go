// Package llm abstrae el modelo de lenguaje: quien sugiere comandos no sabe que detrás está Ollama.
package llm

import (
	"context"
	"errors"
)

// Request es una petición al modelo.
type Request struct {
	Model       string
	System      string
	Prompt      string
	Schema      any // JSON schema de la respuesta (salida estructurada); nil = texto libre
	Temperature float64
	MaxTokens   int // límite de la respuesta; sin límite, algunos modelos pequeños no paran nunca
}

// Provider genera una respuesta para una petición.
type Provider interface {
	Generate(ctx context.Context, req Request) (string, error)
}

var (
	ErrModelNotFound = errors.New("modelo no encontrado")
	ErrUnreachable   = errors.New("no puedo conectar con Ollama")
	ErrNotChat       = errors.New("el modelo no sirve para conversar")
	ErrTimeout       = errors.New("Ollama tardó demasiado en responder (más de 3 minutos). Prueba otra vez o usa un modelo más ligero")
)
