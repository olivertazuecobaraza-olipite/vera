// Package ollama implementa llm.Provider hablando con la API HTTP de Ollama (/api/chat).
package ollama

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"strings"
	"time"

	"github.com/olivertazuecobaraza-olipite/vera/internal/llm"
)

// Client habla con un servidor de Ollama.
type Client struct {
	host string
	http *http.Client
}

// NewClient crea un cliente para host (p. ej. "http://127.0.0.1:11434").
func NewClient(host string) *Client {
	return &Client{host: strings.TrimRight(host, "/"), http: &http.Client{Timeout: 3 * time.Minute}}
}

type message struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

type chatRequest struct {
	Model    string         `json:"model"`
	Messages []message      `json:"messages"`
	Stream   bool           `json:"stream"`
	Think    *bool          `json:"think,omitempty"`
	Format   any            `json:"format,omitempty"`
	Options  map[string]any `json:"options"`
}

type chatResponse struct {
	Message message `json:"message"`
	Error   string  `json:"error"`
}

// Generate pide una respuesta. Se pide sin «modo pensar» (más rápido); si el modelo no lo admite, se repite sin ese parámetro.
func (c *Client) Generate(ctx context.Context, req llm.Request) (string, error) {
	no := false
	out, err := c.chat(ctx, req, &no)
	if err != nil && strings.Contains(err.Error(), "think") {
		out, err = c.chat(ctx, req, nil)
	}
	return out, err
}

func (c *Client) chat(ctx context.Context, req llm.Request, think *bool) (string, error) {
	opts := map[string]any{"temperature": req.Temperature}
	if req.MaxTokens > 0 {
		opts["num_predict"] = req.MaxTokens
	}
	var msgs []message
	if req.System != "" {
		msgs = append(msgs, message{"system", req.System})
	}
	msgs = append(msgs, message{"user", req.Prompt})
	body, _ := json.Marshal(chatRequest{Model: req.Model, Messages: msgs, Think: think, Format: req.Schema, Options: opts})

	hreq, err := http.NewRequestWithContext(ctx, http.MethodPost, c.host+"/api/chat", bytes.NewReader(body))
	if err != nil {
		return "", err
	}
	hreq.Header.Set("Content-Type", "application/json")
	resp, err := c.http.Do(hreq)
	if err != nil {
		var ne net.Error
		switch {
		case ctx.Err() != nil:
			return "", ctx.Err()
		case errors.As(err, &ne) && ne.Timeout():
			return "", llm.ErrTimeout
		}
		return "", fmt.Errorf("%w en %s", llm.ErrUnreachable, c.host)
	}
	defer resp.Body.Close()
	raw, _ := io.ReadAll(resp.Body)

	var r chatResponse
	if err := json.Unmarshal(raw, &r); err != nil {
		return "", fmt.Errorf("respuesta inesperada de Ollama: %s", strings.TrimSpace(string(raw)))
	}
	if r.Error != "" {
		switch {
		case strings.Contains(r.Error, "not found"):
			return "", fmt.Errorf("%w: %s", llm.ErrModelNotFound, req.Model)
		case strings.Contains(r.Error, "does not support chat"), strings.Contains(r.Error, "does not support generate"):
			return "", fmt.Errorf("%w: %s", llm.ErrNotChat, req.Model)
		}
		return "", fmt.Errorf("error de Ollama: %s", r.Error)
	}
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("error de Ollama: %s", resp.Status)
	}
	return r.Message.Content, nil
}
