package setup

import (
	"archive/tar"
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/klauspost/compress/zstd"
)

// fakeOllama es un servidor de Ollama de mentira. up decide si /api/version responde.
type fakeOllama struct {
	up      atomic.Bool
	handler func(w http.ResponseWriter, r *http.Request) // para el resto de rutas
}

func (f *fakeOllama) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path == "/api/version" {
		if !f.up.Load() {
			w.WriteHeader(http.StatusServiceUnavailable)
			return
		}
		fmt.Fprint(w, `{"version":"0.0.0"}`)
		return
	}
	if f.handler != nil {
		f.handler(w, r)
		return
	}
	http.NotFound(w, r)
}

// testManager crea un Manager contra srv, sin tocar el sistema: nada está instalado
// y ningún comando se ejecuta de verdad.
func testManager(t *testing.T, srv *httptest.Server) *Manager {
	t.Helper()
	home := t.TempDir()
	return &Manager{
		host:        srv.URL,
		baseURL:     srv.URL,
		registry:    srv.URL,
		home:        home,
		appsDir:     filepath.Join(home, "Applications-sistema"),
		appData:     filepath.Join(home, "AppData"),
		unitDir:     filepath.Join(home, "systemd"),
		http:        srv.Client(),
		lookPath:    func(string) (string, error) { return "", errors.New("no encontrado") },
		stat:        func(string) (fs.FileInfo, error) { return nil, fs.ErrNotExist },
		run:         func(context.Context, string, ...string) error { return nil },
		output:      func(context.Context, string, ...string) ([]byte, error) { return nil, nil },
		spawn:       func(string, []string, []string) error { return nil },
		waitTimeout: 2 * time.Second,
		pollEvery:   10 * time.Millisecond,
	}
}

func TestIsLocal(t *testing.T) {
	tests := map[string]bool{
		"http://127.0.0.1:11434":   true,
		"http://localhost:11434":   true,
		"http://[::1]:11434":       true,
		"http://192.168.1.5:11434": false,
		"https://ollama.example":   false,
	}
	for host, want := range tests {
		if got := (&Manager{host: host}).IsLocal(); got != want {
			t.Errorf("IsLocal(%q) = %v", host, got)
		}
	}
}

func TestBinary(t *testing.T) {
	srv := httptest.NewServer(&fakeOllama{})
	defer srv.Close()
	m := testManager(t, srv)

	if m.IsInstalled() {
		t.Fatal("sin nada en el PATH ni en las rutas conocidas no debería estar instalado")
	}

	m.lookPath = func(string) (string, error) { return "/en/el/path/ollama", nil }
	if m.Binary() != "/en/el/path/ollama" {
		t.Errorf("Binary = %q, quería el del PATH", m.Binary())
	}

	m.lookPath = func(string) (string, error) { return "", errors.New("no") }
	want := m.candidates()[0]
	m.stat = func(p string) (fs.FileInfo, error) {
		if p == want {
			return os.Stat(os.Args[0]) // un archivo cualquiera que existe
		}
		return nil, fs.ErrNotExist
	}
	if m.Binary() != want {
		t.Errorf("Binary = %q, quería la ruta conocida %q", m.Binary(), want)
	}
}

func TestEnsureRunning(t *testing.T) {
	t.Run("ya funciona", func(t *testing.T) {
		f := &fakeOllama{}
		f.up.Store(true)
		srv := httptest.NewServer(f)
		defer srv.Close()
		if err := testManager(t, srv).EnsureRunning(context.Background()); err != nil {
			t.Error(err)
		}
	})

	t.Run("no instalado", func(t *testing.T) {
		srv := httptest.NewServer(&fakeOllama{})
		defer srv.Close()
		if err := testManager(t, srv).EnsureRunning(context.Background()); !errors.Is(err, ErrNotInstalled) {
			t.Errorf("err = %v, quería ErrNotInstalled", err)
		}
	})

	t.Run("en otro equipo", func(t *testing.T) {
		m := &Manager{host: "http://192.0.2.1:11434", http: &http.Client{Timeout: 100 * time.Millisecond}}
		if err := m.EnsureRunning(context.Background()); !errors.Is(err, ErrRemote) {
			t.Errorf("err = %v, quería ErrRemote", err)
		}
	})

	t.Run("instalado y cerrado: lo arranca", func(t *testing.T) {
		f := &fakeOllama{}
		srv := httptest.NewServer(f)
		defer srv.Close()
		m := testManager(t, srv)
		m.lookPath = func(string) (string, error) { return "/bin/ollama-falso", nil }
		var gotName string
		var gotArgs, gotEnv []string
		m.spawn = func(name string, args, env []string) error {
			gotName, gotArgs, gotEnv = name, args, env
			f.up.Store(true) // el «servidor» arranca
			return nil
		}
		m.run = func(context.Context, string, ...string) error { f.up.Store(true); return nil } // open -g en macOS
		if err := m.EnsureRunning(context.Background()); err != nil {
			t.Fatal(err)
		}
		if gotName != "" && (gotArgs[0] != "serve" || !strings.HasPrefix(gotEnv[0], "OLLAMA_HOST=127.0.0.1:")) {
			t.Errorf("arrancó %s %v %v", gotName, gotArgs, gotEnv)
		}
	})

	t.Run("no llega a responder", func(t *testing.T) {
		srv := httptest.NewServer(&fakeOllama{})
		defer srv.Close()
		m := testManager(t, srv)
		m.lookPath = func(string) (string, error) { return "/bin/ollama-falso", nil }
		m.waitTimeout = 50 * time.Millisecond
		if err := m.EnsureRunning(context.Background()); err == nil || !strings.Contains(err.Error(), "no arranca") {
			t.Errorf("err = %v", err)
		}
	})
}

func TestModelInfo(t *testing.T) {
	f := &fakeOllama{handler: func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		switch {
		case r.URL.Path != "/api/show":
			http.NotFound(w, r)
		case bytes.Contains(body, []byte(`"llama3.2"`)):
			fmt.Fprint(w, `{"capabilities":["completion","tools"]}`)
		case bytes.Contains(body, []byte(`"all-minilm"`)):
			fmt.Fprint(w, `{"capabilities":["embedding"]}`)
		case bytes.Contains(body, []byte(`"viejo"`)):
			fmt.Fprint(w, `{}`) // Ollama antiguo: sin capacidades
		default:
			w.WriteHeader(404)
			fmt.Fprint(w, `{"error":"model not found"}`)
		}
	}}
	srv := httptest.NewServer(f)
	defer srv.Close()
	m := testManager(t, srv)

	tests := []struct {
		name         string
		exists, chat bool
	}{
		{"llama3.2", true, true},
		{"all-minilm", true, false},
		{"viejo", true, true},
		{"no-esta", false, true},
	}
	for _, tt := range tests {
		info, err := m.ModelInfo(context.Background(), tt.name)
		if err != nil {
			t.Fatal(err)
		}
		if info.Exists != tt.exists || (tt.exists && info.CanChat() != tt.chat) {
			t.Errorf("%s: %+v (chat=%v)", tt.name, info, info.CanChat())
		}
	}
}

func pullServer(lines ...string) *httptest.Server {
	return httptest.NewServer(&fakeOllama{handler: func(w http.ResponseWriter, r *http.Request) {
		for _, l := range lines {
			fmt.Fprintln(w, l)
		}
	}})
}

func TestPullReportaProgreso(t *testing.T) {
	srv := pullServer(
		`{"status":"pulling manifest"}`,
		`{"status":"pulling a","digest":"sha256:a","total":100,"completed":50}`,
		`{"status":"pulling b","digest":"sha256:b","total":300,"completed":0}`,
		`{"status":"pulling a","digest":"sha256:a","total":100,"completed":100}`,
		`{"status":"pulling b","digest":"sha256:b","total":300,"completed":300}`,
		`{"status":"verifying sha256 digest"}`,
		`{"status":"writing manifest"}`,
		`{"status":"success"}`,
	)
	defer srv.Close()

	var got []Progress
	err := testManager(t, srv).Pull(context.Background(), "llama3.2", func(p Progress) { got = append(got, p) })
	if err != nil {
		t.Fatal(err)
	}
	want := []Progress{
		{StatusDownloading, 50, 100},
		{StatusDownloading, 50, 400}, // las capas se suman
		{StatusDownloading, 100, 400},
		{StatusDownloading, 400, 400},
		{StatusVerifying, 400, 400}, // una sola vez, aunque lleguen dos estados
		{StatusSuccess, 400, 400},
	}
	if fmt.Sprint(got) != fmt.Sprint(want) {
		t.Errorf("progreso:\n got %v\nwant %v", got, want)
	}
}

func TestPullSinDigest(t *testing.T) {
	// Formato simplificado (sin "digest"), como en la propuesta de arquitectura.
	srv := pullServer(
		`{"status":"downloading","total":100,"completed":50}`,
		`{"status":"downloading","total":100,"completed":100}`,
		`{"status":"success"}`,
	)
	defer srv.Close()
	var last Progress
	if err := testManager(t, srv).Pull(context.Background(), "x", func(p Progress) { last = p }); err != nil {
		t.Fatal(err)
	}
	if last.Status != StatusSuccess || last.Total != 100 {
		t.Errorf("último = %+v", last)
	}
}

func TestPullErrores(t *testing.T) {
	tests := []struct {
		name  string
		lines []string
		want  error
	}{
		{"no existe", []string{`{"status":"pulling manifest"}`, `{"error":"pull model manifest: file does not exist"}`}, ErrModelNotFound},
		{"nombre inválido", []string{`{"error":"invalid model name"}`}, ErrInvalidName},
		{"sin espacio", []string{`{"status":"pulling a","digest":"a","total":9,"completed":1}`, `{"error":"write: no space left on device"}`}, ErrNoSpace},
		{"se corta sin success", []string{`{"status":"pulling a","digest":"a","total":9,"completed":1}`}, ErrPullIncomplete},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			srv := pullServer(tt.lines...)
			defer srv.Close()
			if err := testManager(t, srv).Pull(context.Background(), "x", nil); !errors.Is(err, tt.want) {
				t.Errorf("err = %v, quería %v", err, tt.want)
			}
		})
	}
}

func TestPullCancelado(t *testing.T) {
	srv := httptest.NewServer(&fakeOllama{handler: func(w http.ResponseWriter, r *http.Request) {
		io.Copy(io.Discard, r.Body)
		fmt.Fprintln(w, `{"status":"pulling a","digest":"a","total":100,"completed":1}`)
		w.(http.Flusher).Flush()
		select {
		case <-r.Context().Done():
		case <-time.After(5 * time.Second):
		}
	}})
	defer srv.Close()
	ctx, cancel := context.WithCancel(context.Background())
	err := testManager(t, srv).Pull(ctx, "x", func(Progress) { cancel() })
	if !errors.Is(err, context.Canceled) {
		t.Errorf("err = %v, quería context.Canceled", err)
	}
}

func TestModelRef(t *testing.T) {
	tests := []struct{ in, host, path, tag string }{
		{"llama3.2", "registry.ollama.ai", "library/llama3.2", "latest"},
		{"qwen3.5:9b", "registry.ollama.ai", "library/qwen3.5", "9b"},
		{"usuario/modelo:v1", "registry.ollama.ai", "usuario/modelo", "v1"},
		{"hf.co/org/repo:Q4_K_M", "hf.co", "org/repo", "Q4_K_M"},
		{"localhost:5000/m", "localhost:5000", "library/m", "latest"},
	}
	for _, tt := range tests {
		h, p, tag := modelRef(tt.in)
		if h != tt.host || p != tt.path || tag != tt.tag {
			t.Errorf("modelRef(%q) = %s %s %s", tt.in, h, p, tag)
		}
	}
}

func TestRemoteSize(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/v2/library/llama3.2/manifests/latest":
			if !strings.Contains(r.Header.Get("Accept"), "manifest.v2+json") {
				t.Errorf("Accept = %q", r.Header.Get("Accept"))
			}
			fmt.Fprint(w, `{"config":{"size":100},"layers":[{"size":1000},{"size":24}]}`)
		case "/v2/library/roto/manifests/latest":
			w.WriteHeader(500)
		default:
			w.WriteHeader(404)
		}
	}))
	defer srv.Close()
	m := testManager(t, srv)

	if size, err := m.RemoteSize(context.Background(), "llama3.2"); err != nil || size != 1124 {
		t.Errorf("size = %d, err = %v", size, err)
	}
	if _, err := m.RemoteSize(context.Background(), "no-existe"); !errors.Is(err, ErrModelNotFound) {
		t.Errorf("err = %v, quería ErrModelNotFound", err)
	}
	if _, err := m.RemoteSize(context.Background(), "roto"); !errors.Is(err, ErrUnknownSize) {
		t.Errorf("err = %v, quería ErrUnknownSize", err)
	}
}

func TestListYDelete(t *testing.T) {
	var deleted string
	srv := httptest.NewServer(&fakeOllama{handler: func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.URL.Path == "/api/tags":
			fmt.Fprint(w, `{"models":[{"name":"a:latest","size":10},{"name":"b:1b","size":20}]}`)
		case r.URL.Path == "/api/delete" && r.Method == http.MethodDelete:
			b, _ := io.ReadAll(r.Body)
			deleted = string(b)
		case r.URL.Path == "/api/delete":
			w.WriteHeader(405)
		}
	}})
	defer srv.Close()
	m := testManager(t, srv)

	models, err := m.List(context.Background())
	if err != nil || len(models) != 2 || models[1].Name != "b:1b" || models[1].Size != 20 {
		t.Errorf("models = %+v, err = %v", models, err)
	}
	if err := m.Delete(context.Background(), "a"); err != nil || !strings.Contains(deleted, `"model":"a"`) {
		t.Errorf("err = %v, cuerpo = %s", err, deleted)
	}
}

func TestDownload(t *testing.T) {
	payload := bytes.Repeat([]byte("x"), 10000)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/ok.bin":
			w.Header().Set("Content-Length", "10000")
			w.Write(payload)
		case "/corto.bin":
			w.Header().Set("Content-Length", "20000") // promete más de lo que manda
			w.Write(payload)
		default:
			w.WriteHeader(404)
		}
	}))
	defer srv.Close()
	m := testManager(t, srv)

	var last Progress
	path, err := m.download(context.Background(), srv.URL+"/ok.bin", func(p Progress) { last = p })
	if err != nil {
		t.Fatal(err)
	}
	defer os.Remove(path)
	if b, _ := os.ReadFile(path); !bytes.Equal(b, payload) || last.Done != 10000 || last.Total != 10000 {
		t.Errorf("descarga incorrecta: %d bytes, progreso %+v", len(b), last)
	}

	before := tempFiles(t, "corto.bin")
	if _, err := m.download(context.Background(), srv.URL+"/corto.bin", func(Progress) {}); err == nil {
		t.Error("una descarga cortada debería fallar")
	}
	if after := tempFiles(t, "corto.bin"); after != before {
		t.Error("la descarga cortada dejó el archivo temporal")
	}

	if _, err := m.download(context.Background(), srv.URL+"/no-existe", func(Progress) {}); err == nil || !strings.Contains(err.Error(), "404") {
		t.Errorf("err = %v", err)
	}
}

func tempFiles(t *testing.T, suffix string) int {
	t.Helper()
	m, _ := filepath.Glob(filepath.Join(os.TempDir(), "vera-*-"+suffix))
	return len(m)
}

// tarZst crea un .tar.zst en memoria con los archivos dados (nombre → contenido; "->x" = enlace a x).
func tarZst(t *testing.T, files map[string]string) []byte {
	t.Helper()
	var buf bytes.Buffer
	zw, _ := zstd.NewWriter(&buf)
	tw := tar.NewWriter(zw)
	for name, content := range files {
		if target, ok := strings.CutPrefix(content, "->"); ok {
			tw.WriteHeader(&tar.Header{Name: name, Typeflag: tar.TypeSymlink, Linkname: target})
			continue
		}
		tw.WriteHeader(&tar.Header{Name: name, Typeflag: tar.TypeReg, Mode: 0o755, Size: int64(len(content))})
		tw.Write([]byte(content))
	}
	tw.Close()
	zw.Close()
	return buf.Bytes()
}

func TestExtractTarZst(t *testing.T) {
	file := filepath.Join(t.TempDir(), "o.tar.zst")
	os.WriteFile(file, tarZst(t, map[string]string{
		"bin/ollama":              "#!/bin/sh\necho ollama\n",
		"lib/ollama/libggml.so.1": "lib",
		"lib/ollama/libggml.so":   "->libggml.so.1",
	}), 0o644)
	dest := t.TempDir()
	if err := extractTarZst(file, dest); err != nil {
		t.Fatal(err)
	}
	fi, err := os.Stat(filepath.Join(dest, "bin", "ollama"))
	if err != nil || fi.Mode()&0o100 == 0 {
		t.Errorf("bin/ollama debería existir y ser ejecutable: %v %v", fi, err)
	}
	if target, err := os.Readlink(filepath.Join(dest, "lib", "ollama", "libggml.so")); err != nil || target != "libggml.so.1" {
		t.Errorf("enlace = %q, %v", target, err)
	}
}

func TestExtractTarZstRechazaRutasFuera(t *testing.T) {
	file := filepath.Join(t.TempDir(), "malo.tar.zst")
	os.WriteFile(file, tarZst(t, map[string]string{"../../fuera": "x"}), 0o644)
	if err := extractTarZst(file, t.TempDir()); err == nil {
		t.Error("debería rechazar rutas que salen de la carpeta de destino")
	}
}

func TestSameModel(t *testing.T) {
	if !SameModel("llama3.2", "llama3.2:latest") || !SameModel("Qwen3.5:9B", "qwen3.5:9b") || SameModel("a:1b", "a:7b") {
		t.Error("SameModel incorrecto")
	}
	if !SameModel("hf.co/org/repo", "hf.co/org/repo:latest") {
		t.Error("SameModel con registro propio")
	}
}
