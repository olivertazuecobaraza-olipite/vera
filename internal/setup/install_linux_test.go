//go:build linux

package setup

import (
	"context"
	"errors"
	"io/fs"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
)

func TestInstallLinux(t *testing.T) {
	pkg := tarZst(t, map[string]string{"bin/ollama": "binario", "lib/ollama/x.so": "lib"})
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/download/ollama-linux-"+goarch()+".tar.zst" {
			w.WriteHeader(404)
			return
		}
		w.Write(pkg)
	}))
	defer srv.Close()
	m := testManager(t, srv)
	var statuses []string
	where, err := m.Install(context.Background(), func(p Progress) {
		if len(statuses) == 0 || statuses[len(statuses)-1] != p.Status {
			statuses = append(statuses, p.Status)
		}
	})
	if err != nil {
		t.Fatal(err)
	}
	if want := filepath.Join(m.home, ".local", "bin", "ollama"); where != want {
		t.Errorf("instalado en %q", where)
	}
	if _, err := os.Stat(filepath.Join(m.home, ".local", "lib", "ollama", "x.so")); err != nil {
		t.Error("faltan las librerías")
	}
	if len(statuses) != 2 || statuses[0] != StatusDownloading || statuses[1] != StatusExtracting {
		t.Errorf("estados = %v", statuses)
	}
}

func TestStartLinuxConServicioDelSistema(t *testing.T) {
	srv := httptest.NewServer(http.NotFoundHandler())
	defer srv.Close()
	m := testManager(t, srv)
	m.stat = func(p string) (fs.FileInfo, error) {
		if p == systemUnits[0] {
			return os.Stat(os.Args[0])
		}
		return nil, fs.ErrNotExist
	}
	if err := m.start(context.Background(), "/usr/bin/ollama"); !errors.Is(err, ErrSystemService) {
		t.Errorf("err = %v, quería ErrSystemService", err)
	}
}

func TestStartLinuxConServicioDeUsuario(t *testing.T) {
	srv := httptest.NewServer(http.NotFoundHandler())
	defer srv.Close()
	m := testManager(t, srv)
	m.stat = func(p string) (fs.FileInfo, error) {
		if p == m.userUnitPath() {
			return os.Stat(os.Args[0])
		}
		return nil, fs.ErrNotExist
	}
	var ran []string
	m.run = func(_ context.Context, name string, args ...string) error {
		ran = append([]string{name}, args...)
		return nil
	}
	m.spawn = func(string, []string, []string) error { t.Error("no debería usar ollama serve"); return nil }
	if err := m.start(context.Background(), "/x/ollama"); err != nil || len(ran) != 4 || ran[3] != userUnit {
		t.Errorf("err = %v, ejecutó %v", err, ran)
	}
}

func TestAutostartLinux(t *testing.T) {
	srv := httptest.NewServer(http.NotFoundHandler())
	defer srv.Close()
	m := testManager(t, srv)
	m.lookPath = func(name string) (string, error) {
		if name == "systemctl" || name == "ollama" {
			return "/usr/bin/" + name, nil
		}
		return "", errors.New("no")
	}
	enabled := false
	var calls [][]string
	m.output = func(_ context.Context, name string, args ...string) ([]byte, error) {
		calls = append(calls, args)
		switch args[1] {
		case "is-system-running":
			return []byte("running\n"), nil
		case "is-enabled":
			if enabled {
				return []byte("enabled\n"), nil
			}
			return []byte("disabled\n"), nil
		case "enable":
			enabled = true
		}
		return nil, nil
	}
	if a := m.AutostartState(context.Background()); !a.CanChange || a.Enabled {
		t.Fatalf("estado inicial = %+v", a)
	}
	if err := m.SetAutostart(context.Background(), true); err != nil {
		t.Fatal(err)
	}
	unit, err := os.ReadFile(m.userUnitPath())
	if err != nil || !contains(string(unit), "ExecStart=/usr/bin/ollama serve") {
		t.Errorf("servicio = %q, %v", unit, err)
	}
	if a := m.AutostartState(context.Background()); !a.Enabled || a.State != "sí" {
		t.Errorf("tras activarlo = %+v", a)
	}
}

func TestAutostartSinSystemd(t *testing.T) {
	srv := httptest.NewServer(http.NotFoundHandler())
	defer srv.Close()
	m := testManager(t, srv) // lookPath no encuentra systemctl
	if a := m.AutostartState(context.Background()); a.CanChange || a.State != "no disponible" {
		t.Errorf("sin systemd = %+v", a)
	}
}

func contains(s, sub string) bool {
	return len(s) >= len(sub) && (s == sub || len(sub) == 0 || indexOf(s, sub) >= 0)
}

func indexOf(s, sub string) int {
	for i := 0; i+len(sub) <= len(s); i++ {
		if s[i:i+len(sub)] == sub {
			return i
		}
	}
	return -1
}
