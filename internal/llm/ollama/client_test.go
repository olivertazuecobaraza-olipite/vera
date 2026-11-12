package ollama_test

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/olipite/vera/internal/llm"
	"github.com/olipite/vera/internal/llm/ollama"
)

func TestGenerate(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || r.URL.Path != "/api/chat" {
			t.Errorf("petición = %s %s, quería POST /api/chat", r.Method, r.URL.Path)
		}
		var body map[string]any
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Errorf("el cuerpo no es JSON: %v", err)
		}
		if body["model"] != "llama3.2" {
			t.Errorf("model = %v", body["model"])
		}
		if body["stream"] != false {
			t.Error("stream debería ser false")
		}
		if body["think"] != false {
			t.Error("debería pedirse sin modo pensar")
		}
		if body["format"] == nil {
			t.Error("falta el schema en format")
		}
		opts, _ := body["options"].(map[string]any)
		if opts["num_predict"] != float64(1024) || opts["temperature"] != 0.2 {
			t.Errorf("options = %v", opts)
		}
		msgs, _ := body["messages"].([]any)
		if len(msgs) != 2 {
			t.Errorf("quería mensaje de sistema + usuario, hay %d", len(msgs))
		}
		fmt.Fprint(w, `{"message":{"role":"assistant","content":"{\"command\":\"ls\"}"},"done":true}`)
	}))
	defer srv.Close()

	got, err := ollama.NewClient(srv.URL).Generate(context.Background(), llm.Request{
		Model: "llama3.2", System: "sistema", Prompt: "listar",
		Schema: map[string]any{"type": "object"}, Temperature: 0.2, MaxTokens: 1024,
	})
	if err != nil {
		t.Fatal(err)
	}
	if got != `{"command":"ls"}` {
		t.Errorf("got %q", got)
	}
}

func TestReintentaSinThink(t *testing.T) {
	calls := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		var body map[string]any
		json.NewDecoder(r.Body).Decode(&body)
		if _, ok := body["think"]; ok {
			w.WriteHeader(http.StatusBadRequest)
			fmt.Fprint(w, `{"error":"\"x\" does not support thinking"}`)
			return
		}
		fmt.Fprint(w, `{"message":{"content":"ok"}}`)
	}))
	defer srv.Close()

	got, err := ollama.NewClient(srv.URL).Generate(context.Background(), llm.Request{Model: "x"})
	if err != nil || got != "ok" || calls != 2 {
		t.Errorf("got %q, err %v, llamadas %d (quería reintento sin think)", got, err, calls)
	}
}

func TestErrores(t *testing.T) {
	tests := []struct {
		name   string
		status int
		body   string
		want   error
	}{
		{"modelo no existe", 404, `{"error":"model 'xyz' not found"}`, llm.ErrModelNotFound},
		{"modelo de embeddings", 400, `{"error":"\"all-minilm\" does not support chat"}`, llm.ErrNotChat},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(tt.status)
				fmt.Fprint(w, tt.body)
			}))
			defer srv.Close()
			_, err := ollama.NewClient(srv.URL).Generate(context.Background(), llm.Request{Model: "xyz"})
			if !errors.Is(err, tt.want) {
				t.Errorf("err = %v, quería %v", err, tt.want)
			}
		})
	}
}

func TestOtroErrorSeMuestraTalCual(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(500)
		fmt.Fprint(w, `{"error":"out of memory"}`)
	}))
	defer srv.Close()
	_, err := ollama.NewClient(srv.URL).Generate(context.Background(), llm.Request{})
	if err == nil || err.Error() != "error de Ollama: out of memory" {
		t.Errorf("err = %v", err)
	}
}

func TestNoConecta(t *testing.T) {
	srv := httptest.NewServer(http.NotFoundHandler())
	url := srv.URL
	srv.Close() // nadie escucha ya en esa dirección
	_, err := ollama.NewClient(url).Generate(context.Background(), llm.Request{})
	if !errors.Is(err, llm.ErrUnreachable) {
		t.Errorf("err = %v, quería ErrUnreachable", err)
	}
}

func TestCtrlCNoSeConfundeConNoConecta(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		io.Copy(io.Discard, r.Body) // sin leer el cuerpo, el servidor no detecta que el cliente se ha ido
		select {                    // el modelo «piensa» hasta que se cancela
		case <-r.Context().Done():
		case <-time.After(5 * time.Second):
		}
	}))
	defer srv.Close()
	ctx, cancel := context.WithCancel(context.Background())
	go func() { time.Sleep(50 * time.Millisecond); cancel() }()
	_, err := ollama.NewClient(srv.URL).Generate(ctx, llm.Request{})
	if !errors.Is(err, context.Canceled) {
		t.Errorf("err = %v, quería context.Canceled", err)
	}
}

func TestRespuestaNoJSON(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, "<html>proxy</html>")
	}))
	defer srv.Close()
	if _, err := ollama.NewClient(srv.URL).Generate(context.Background(), llm.Request{}); err == nil {
		t.Error("debería fallar con una respuesta que no es JSON")
	}
}
