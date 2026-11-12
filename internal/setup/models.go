package setup

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

// ModelInfo dice si un modelo está descargado y qué sabe hacer.
type ModelInfo struct {
	Exists       bool
	Capabilities []string // "completion", "embedding", "vision", "tools"…
}

// CanChat dice si el modelo sirve para conversar. Si Ollama no informa (versiones antiguas), se da por bueno.
func (i ModelInfo) CanChat() bool {
	if len(i.Capabilities) == 0 {
		return true
	}
	for _, c := range i.Capabilities {
		if c == "completion" {
			return true
		}
	}
	return false
}

// Model es un modelo descargado.
type Model struct {
	Name string `json:"name"`
	Size int64  `json:"size"`
}

func (m *Manager) postJSON(ctx context.Context, path string, body any) (*http.Response, error) {
	b, _ := json.Marshal(body)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, m.host+path, strings.NewReader(string(b)))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	return m.http.Do(req)
}

// nameBody incluye "name" además de "model" para versiones antiguas de Ollama.
func nameBody(name string) map[string]any { return map[string]any{"model": name, "name": name} }

// ModelInfo consulta si el modelo está descargado y sus capacidades.
func (m *Manager) ModelInfo(ctx context.Context, name string) (ModelInfo, error) {
	ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	resp, err := m.postJSON(ctx, "/api/show", nameBody(name))
	if err != nil {
		return ModelInfo{}, fmt.Errorf("no puedo consultar los modelos de Ollama: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return ModelInfo{}, nil
	}
	var r struct {
		Capabilities []string `json:"capabilities"`
	}
	_ = json.NewDecoder(resp.Body).Decode(&r)
	return ModelInfo{Exists: true, Capabilities: r.Capabilities}, nil
}

type pullMsg struct {
	Status    string `json:"status"`
	Digest    string `json:"digest"`
	Total     int64  `json:"total"`
	Completed int64  `json:"completed"`
	Error     string `json:"error"`
}

// Pull descarga un modelo informando del progreso total (todas las capas juntas).
// Si se cancela (Ctrl+C), Ollama guarda lo descargado y la próxima vez sigue donde se quedó.
func (m *Manager) Pull(ctx context.Context, name string, p func(Progress)) error {
	if p == nil {
		p = func(Progress) {}
	}
	b := nameBody(name)
	b["stream"] = true
	resp, err := m.postJSON(ctx, "/api/pull", b)
	if err != nil {
		if ctx.Err() != nil {
			return ctx.Err()
		}
		return fmt.Errorf("no puedo conectar con Ollama: %w", err)
	}
	defer resp.Body.Close()

	totals, done := map[string]int64{}, map[string]int64{}
	var order []string
	sum := func() (d, t int64) {
		for _, k := range order {
			t += totals[k]
			d += done[k]
		}
		return
	}
	downloading := false

	dec := json.NewDecoder(resp.Body)
	for {
		var msg pullMsg
		if err := dec.Decode(&msg); err == io.EOF {
			return ErrPullIncomplete
		} else if err != nil {
			if ctx.Err() != nil {
				return ctx.Err()
			}
			return fmt.Errorf("se perdió la conexión con Ollama (%v). Repite el comando para continuar", err)
		}
		if msg.Error != "" {
			return pullError(name, msg.Error)
		}
		switch {
		case msg.Total > 0:
			key := msg.Digest
			if key == "" {
				key = msg.Status
			}
			if _, seen := totals[key]; !seen {
				order = append(order, key)
			}
			totals[key], done[key] = msg.Total, msg.Completed
			downloading = true
			d, t := sum()
			p(Progress{Status: StatusDownloading, Done: d, Total: t})
		case msg.Status == "success":
			_, t := sum()
			p(Progress{Status: StatusSuccess, Done: t, Total: t})
			return nil
		case downloading:
			// Se acabó la descarga; ahora Ollama verifica y guarda.
			downloading = false
			_, t := sum()
			p(Progress{Status: StatusVerifying, Done: t, Total: t})
		}
	}
}

func pullError(name, msg string) error {
	switch {
	case strings.Contains(msg, "file does not exist"), strings.Contains(msg, "not found"):
		return fmt.Errorf("%w: %q (búscalo en https://ollama.com/search)", ErrModelNotFound, name)
	case strings.Contains(msg, "invalid model name"):
		return fmt.Errorf("%q %w (ejemplos: llama3.2, qwen3.5:9b)", name, ErrInvalidName)
	case strings.Contains(msg, "no space left"):
		return ErrNoSpace
	}
	return errors.New(msg)
}

// List devuelve los modelos descargados.
func (m *Manager) List(ctx context.Context) ([]Model, error) {
	ctx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	req, _ := http.NewRequestWithContext(ctx, http.MethodGet, m.host+"/api/tags", nil)
	resp, err := m.http.Do(req)
	if err != nil {
		return nil, fmt.Errorf("no puedo leer los modelos: %w", err)
	}
	defer resp.Body.Close()
	var r struct {
		Models []Model `json:"models"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&r); err != nil {
		return nil, fmt.Errorf("no puedo leer los modelos: %w", err)
	}
	return r.Models, nil
}

// Delete borra un modelo descargado.
func (m *Manager) Delete(ctx context.Context, name string) error {
	ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	b, _ := json.Marshal(nameBody(name))
	req, _ := http.NewRequestWithContext(ctx, http.MethodDelete, m.host+"/api/delete", strings.NewReader(string(b)))
	req.Header.Set("Content-Type", "application/json")
	resp, err := m.http.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		var r struct{ Error string }
		_ = json.NewDecoder(resp.Body).Decode(&r)
		if r.Error == "" {
			r.Error = resp.Status
		}
		return errors.New(r.Error)
	}
	return nil
}

// SameModel compara nombres teniendo en cuenta que «llama3.2» es «llama3.2:latest».
func SameModel(a, b string) bool {
	norm := func(s string) string {
		if i := strings.LastIndex(s, ":"); i <= strings.LastIndex(s, "/") {
			s += ":latest"
		}
		return strings.ToLower(s)
	}
	return norm(a) == norm(b)
}
