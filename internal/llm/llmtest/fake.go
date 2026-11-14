// Package llmtest tiene un Provider falso para tests.
package llmtest

import (
	"context"

	"github.com/olivertazuecobaraza-olipite/vera/internal/llm"
)

// FakeProvider devuelve lo que se le diga y apunta las peticiones recibidas.
// Si Responses tiene elementos, cada llamada consume uno (para simular reintentos).
type FakeProvider struct {
	Response  string
	Err       error
	Responses []Reply
	Requests  []llm.Request
}

// Reply es una respuesta preparada.
type Reply struct {
	Text string
	Err  error
}

// LastReq es la última petición recibida.
func (f *FakeProvider) LastReq() llm.Request {
	if len(f.Requests) == 0 {
		return llm.Request{}
	}
	return f.Requests[len(f.Requests)-1]
}

func (f *FakeProvider) Generate(_ context.Context, req llm.Request) (string, error) {
	f.Requests = append(f.Requests, req)
	if len(f.Responses) > 0 {
		r := f.Responses[0]
		f.Responses = f.Responses[1:]
		return r.Text, r.Err
	}
	return f.Response, f.Err
}
