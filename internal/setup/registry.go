package setup

import (
	"context"
	"encoding/json"
	"net/http"
	"net/url"
	"strings"
	"time"
)

const defaultRegistry = "registry.ollama.ai"

// modelRef separa «usuario/modelo:etiqueta» en registro, ruta y etiqueta, como hace Ollama.
func modelRef(name string) (host, path, tag string) {
	tag = "latest"
	if i := strings.LastIndex(name, ":"); i > strings.LastIndex(name, "/") {
		name, tag = name[:i], name[i+1:]
	}
	parts := strings.Split(name, "/")
	host = defaultRegistry
	if len(parts) > 1 && strings.ContainsAny(parts[0], ".:") { // p. ej. hf.co/usuario/modelo
		host, parts = parts[0], parts[1:]
	}
	if len(parts) == 1 {
		parts = append([]string{"library"}, parts...)
	}
	return host, strings.Join(parts, "/"), tag
}

// RemoteSize consulta en el registro cuánto ocupa un modelo, sin descargarlo.
// Devuelve ErrModelNotFound si no existe y ErrUnknownSize si no se pudo averiguar.
func (m *Manager) RemoteSize(ctx context.Context, name string) (int64, error) {
	host, path, tag := modelRef(name)
	base := "https://" + host
	if host == defaultRegistry {
		base = m.registry
	}
	ctx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, base+"/v2/"+path+"/manifests/"+url.PathEscape(tag), nil)
	if err != nil {
		return 0, ErrUnknownSize
	}
	req.Header.Set("Accept", "application/vnd.docker.distribution.manifest.v2+json")
	resp, err := m.http.Do(req)
	if err != nil {
		return 0, ErrUnknownSize
	}
	defer resp.Body.Close()
	switch resp.StatusCode {
	case http.StatusOK:
	case http.StatusNotFound:
		return 0, ErrModelNotFound
	default:
		return 0, ErrUnknownSize
	}
	var mf struct {
		Config struct{ Size int64 }   `json:"config"`
		Layers []struct{ Size int64 } `json:"layers"`
	}
	if json.NewDecoder(resp.Body).Decode(&mf) != nil {
		return 0, ErrUnknownSize
	}
	size := mf.Config.Size
	for _, l := range mf.Layers {
		size += l.Size
	}
	if size <= 0 {
		return 0, ErrUnknownSize
	}
	return size, nil
}
