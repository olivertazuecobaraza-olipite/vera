//go:build darwin

package setup

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"
)

func zipServer() *httptest.Server {
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/download/Ollama-darwin.zip" {
			w.WriteHeader(404)
			return
		}
		fmt.Fprint(w, "zip falso")
	}))
}

func TestInstallMac(t *testing.T) {
	srv := zipServer()
	defer srv.Close()
	m := testManager(t, srv)
	var calls [][]string
	m.run = func(_ context.Context, name string, args ...string) error {
		calls = append(calls, append([]string{name}, args...))
		return nil
	}
	where, err := m.Install(context.Background(), nil)
	if err != nil {
		t.Fatal(err)
	}
	if where != filepath.Join(m.appsDir, "Ollama.app") {
		t.Errorf("instalado en %q", where)
	}
	if len(calls) != 1 || calls[0][0] != "ditto" || calls[0][4] != m.appsDir {
		t.Errorf("comandos = %v", calls)
	}
}

func TestInstallMacSinPermisosEnApplications(t *testing.T) {
	srv := zipServer()
	defer srv.Close()
	m := testManager(t, srv)
	var dests []string
	m.run = func(_ context.Context, name string, args ...string) error {
		dests = append(dests, args[len(args)-1])
		if len(dests) == 1 {
			return errors.New("permiso denegado")
		}
		return nil
	}
	where, err := m.Install(context.Background(), nil)
	if err != nil {
		t.Fatal(err)
	}
	if want := filepath.Join(m.home, "Applications", "Ollama.app"); where != want {
		t.Errorf("instalado en %q, quería %q", where, want)
	}
}

func TestStartMacAbreLaApp(t *testing.T) {
	srv := zipServer()
	defer srv.Close()
	m := testManager(t, srv)
	m.host = "http://127.0.0.1:11434"
	var opened string
	m.run = func(_ context.Context, name string, args ...string) error {
		opened = name + " " + args[0] + " " + args[1]
		return nil
	}
	if err := m.start(context.Background(), "/Applications/Ollama.app/Contents/Resources/ollama"); err != nil {
		t.Fatal(err)
	}
	if opened != "open -g /Applications/Ollama.app" {
		t.Errorf("abrió %q", opened)
	}
}

func TestStartMacOtroPuertoUsaServe(t *testing.T) {
	srv := zipServer()
	defer srv.Close()
	m := testManager(t, srv) // srv usa un puerto aleatorio, no el 11434
	served := false
	m.spawn = func(name string, args, env []string) error { served = args[0] == "serve"; return nil }
	if err := m.start(context.Background(), "/Applications/Ollama.app/Contents/Resources/ollama"); err != nil || !served {
		t.Errorf("con otro puerto debería usar «ollama serve» (err %v)", err)
	}
}
