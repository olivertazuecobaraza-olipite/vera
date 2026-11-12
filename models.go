// models.go: tamaño, descarga, listado y borrado de modelos, y el comando "vera ollama".
package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"os"
	"strings"
	"time"
)

var errCancelled = errors.New("cancelado")

// modelRef separa "usuario/modelo:etiqueta" en registro, ruta y etiqueta, como hace Ollama.
func modelRef(mdl string) (host, path, tag string) {
	name, tag := mdl, "latest"
	if i := strings.LastIndex(name, ":"); i > strings.LastIndex(name, "/") {
		name, tag = name[:i], name[i+1:]
	}
	parts := strings.Split(name, "/")
	host = "registry.ollama.ai"
	if len(parts) > 1 && strings.ContainsAny(parts[0], ".:") { // p. ej. hf.co/usuario/modelo
		host, parts = parts[0], parts[1:]
	}
	if len(parts) == 1 {
		parts = append([]string{"library"}, parts...)
	}
	return host, strings.Join(parts, "/"), tag
}

// remoteSize consulta en el registro cuánto ocupa un modelo.
// status: 200 si se sabe, 404 si el modelo no existe, 0 si no se pudo averiguar.
func remoteSize(mdl string) (size int64, status int) {
	host, path, tag := modelRef(mdl)
	u := "https://" + host + "/v2/" + path + "/manifests/" + url.PathEscape(tag)
	req, err := http.NewRequest("GET", u, nil)
	if err != nil {
		return 0, 0
	}
	req.Header.Set("Accept", "application/vnd.docker.distribution.manifest.v2+json")
	c := &http.Client{Timeout: 10 * time.Second}
	resp, err := c.Do(req)
	if err != nil {
		return 0, 0
	}
	defer resp.Body.Close()
	if resp.StatusCode == http.StatusNotFound {
		return 0, 404
	}
	if resp.StatusCode != http.StatusOK {
		return 0, 0
	}
	var m struct {
		Config struct{ Size int64 }   `json:"config"`
		Layers []struct{ Size int64 } `json:"layers"`
	}
	if json.NewDecoder(resp.Body).Decode(&m) != nil {
		return 0, 0
	}
	size = m.Config.Size
	for _, l := range m.Layers {
		size += l.Size
	}
	return size, 200
}

// sizeText devuelve " (4.7 GB)" o "" si no se sabe; y un error si el modelo no existe.
func sizeText(mdl string) (string, error) {
	size, st := remoteSize(mdl)
	switch {
	case st == 404:
		return "", fmt.Errorf("el modelo %q no existe. Búscalo en https://ollama.com/search", mdl)
	case st == 200 && size > 0:
		return " (" + humanBytes(size) + ")", nil
	}
	return "", nil
}

// confirmDownload enseña el tamaño y pide permiso (salvo que se haya desactivado en vera config).
func confirmDownload(mdl, question string) (bool, error) {
	st, err := sizeText(mdl)
	if err != nil {
		return false, err
	}
	if !loadConfig().confirmDownloads() {
		return true, nil
	}
	return confirm(fmt.Sprintf(question, mdl+st), true), nil
}

// ensureModel descarga el modelo si falta. ask: pedir confirmación con el tamaño antes.
// Devuelve si sirve para chatear y si ya estaba descargado.
func ensureModel(base, mdl string, ask bool) (chat, existed bool, err error) {
	exists, caps, err := modelInfo(base, mdl)
	if err != nil {
		return false, false, fmt.Errorf("no puedo consultar los modelos de Ollama: %v", err)
	}
	if exists {
		fmt.Println(color("32", fmt.Sprintf("✓ El modelo %s ya está descargado", mdl)))
		return canChat(caps), true, nil
	}
	if ask {
		ok, err := confirmDownload(mdl, "¿Descargar el modelo %s? [S/n] ")
		if err != nil {
			return false, false, err
		}
		if !ok {
			return false, false, errCancelled
		}
	}
	fmt.Println(color("36", "❯"), fmt.Sprintf("Descargando el modelo %s…", mdl))
	if err := pullModel(base, mdl); err != nil {
		return false, false, err
	}
	fmt.Println(color("32", fmt.Sprintf("✓ Modelo %s descargado", mdl)))
	_, caps, _ = modelInfo(base, mdl)
	return canChat(caps), false, nil
}

type localModel struct {
	Name string `json:"name"`
	Size int64  `json:"size"`
}

func listModels(base string) ([]localModel, error) {
	c := &http.Client{Timeout: 10 * time.Second}
	resp, err := c.Get(base + "/api/tags")
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	var r struct {
		Models []localModel `json:"models"`
	}
	err = json.NewDecoder(resp.Body).Decode(&r)
	return r.Models, err
}

func deleteModel(base, mdl string) error {
	body, _ := json.Marshal(map[string]string{"model": mdl, "name": mdl})
	req, _ := http.NewRequest("DELETE", base+"/api/delete", strings.NewReader(string(body)))
	req.Header.Set("Content-Type", "application/json")
	c := &http.Client{Timeout: 30 * time.Second}
	resp, err := c.Do(req)
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

// sameModel compara nombres teniendo en cuenta que "llama3.2" es "llama3.2:latest".
func sameModel(a, b string) bool {
	norm := func(s string) string {
		if i := strings.LastIndex(s, ":"); i <= strings.LastIndex(s, "/") {
			s += ":latest"
		}
		return strings.ToLower(s)
	}
	return norm(a) == norm(b)
}

// ---------- Comando "vera ollama" ----------

const setupUsage = `vera ollama [modelo]

Instala Ollama si no está, lo arranca, descarga el modelo si falta
y lo deja como modelo por defecto de vera.

  vera ollama               usa el modelo por defecto (%s)
  vera ollama llama3.2      cualquier modelo de https://ollama.com/search
`

func runSetup(mdl string) {
	if mdl == "-h" || mdl == "--help" {
		fmt.Printf(setupUsage, model())
		return
	}
	if mdl == "" {
		mdl = model()
	}
	base := ollamaURL()

	if ollamaUp(base) {
		fmt.Println(color("32", "✓ Ollama está instalado y funcionando"))
	} else if err := ensureOllama(base, true); err != nil {
		fail("%v", err)
	}

	chat, existed, err := ensureModel(base, mdl, true)
	switch {
	case errors.Is(err, errCancelled):
		fmt.Println("No se ha descargado nada.")
		return
	case err != nil:
		fail("No se pudo descargar el modelo: %v", err)
	}
	if !chat {
		fmt.Println(color("31", fmt.Sprintf("El modelo %s no sirve para conversar (es de otro tipo, p. ej. embeddings), así que vera no puede usarlo.", mdl)))
		// Si lo acabamos de descargar, lo normal es borrarlo; si ya lo tenías, por defecto se queda.
		def, hint := "[S/n]", !existed
		if existed {
			def = "[s/N]"
		}
		if confirm(fmt.Sprintf("¿Borrarlo para liberar espacio? %s ", def), hint) {
			if err := deleteModel(base, mdl); err != nil {
				fmt.Println(color("31", "No se pudo borrar: "+err.Error()))
			} else {
				fmt.Println(color("32", "✓ Borrado"))
			}
		}
		fail("Elige un modelo de chat, por ejemplo: vera ollama %s", defaultModel())
	}

	c := loadConfig()
	c.Model = mdl
	if err := saveConfig(c); err != nil {
		fmt.Println(color("33", "! No se pudo guardar el modelo por defecto: "+err.Error()))
	} else {
		fmt.Println(color("32", fmt.Sprintf("✓ vera usará %s a partir de ahora", mdl)))
	}
	warnEnvModel(mdl)
}

func warnEnvModel(mdl string) {
	if env := strings.TrimSpace(os.Getenv("VERA_MODEL")); env != "" && env != mdl {
		fmt.Println(color("33", fmt.Sprintf("! Ojo: tienes VERA_MODEL=%s, que tiene prioridad sobre esto.", env)))
	}
}
