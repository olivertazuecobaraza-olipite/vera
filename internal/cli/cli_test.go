package cli

import (
	"bytes"
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/olipite/vera/internal/config"
	"github.com/olipite/vera/internal/llm"
	"github.com/olipite/vera/internal/setup"
	"github.com/olipite/vera/internal/setup/setuptest"
	"github.com/olipite/vera/internal/suggest"
	"github.com/olipite/vera/internal/sysinfo"
	"github.com/olipite/vera/internal/ui"
)

// fakeSuggester apunta las peticiones y responde lo que se le diga.
type fakeSuggester struct {
	queries []string
	sug     suggest.Suggestion
	errs    []error // un error por llamada (se consumen en orden); vacío = sin error
}

func (f *fakeSuggester) Suggest(_ context.Context, q string) (suggest.Suggestion, error) {
	f.queries = append(f.queries, q)
	if len(f.errs) > 0 {
		err := f.errs[0]
		f.errs = f.errs[1:]
		if err != nil {
			return suggest.Suggestion{}, err
		}
	}
	return f.sug, nil
}

// harness monta la CLI con fakes. input es lo que «teclea» el usuario.
type harness struct {
	sug      *fakeSuggester
	mgr      *setuptest.FakeManager
	cfg      *config.Config
	out      *bytes.Buffer
	executed []string
	copied   []string
	execCode int
}

func newHarness(t *testing.T) *harness {
	t.Helper()
	cfg := &config.Config{Path: filepath.Join(t.TempDir(), "config.json"), Getenv: func(string) string { return "" }}
	cfg.Model = "modelo-test"
	return &harness{
		sug: &fakeSuggester{sug: suggest.Suggestion{Command: "ls -a", Explanation: "Lista archivos."}},
		mgr: setuptest.New(),
		cfg: cfg,
		out: &bytes.Buffer{},
	}
}

func (h *harness) run(input string, args ...string) int {
	d := Deps{
		Suggester: h.sug,
		Manager:   h.mgr,
		Config:    h.cfg,
		UI:        ui.New(strings.NewReader(input), h.out, h.out, false, false),
		Info:      func() sysinfo.Info { return sysinfo.Info{Shell: "zsh", ShellKind: "posix"} },
		Exec: func(_ context.Context, _ sysinfo.Info, cmd string) (int, error) {
			h.executed = append(h.executed, cmd)
			return h.execCode, nil
		},
		Clipboard: ui.NewClipboardWith("fake", func(_ string, _ []string, in []byte) error {
			h.copied = append(h.copied, string(in))
			return nil
		}),
		Version: "test",
	}
	return Run(context.Background(), args, d)
}

func (h *harness) wantOut(t *testing.T, subs ...string) {
	t.Helper()
	for _, s := range subs {
		if !strings.Contains(h.out.String(), s) {
			t.Errorf("falta %q en la salida:\n%s", s, h.out.String())
		}
	}
}

// ---------- Enrutado: el test más importante de la CLI ----------

func TestRouting(t *testing.T) {
	tests := []struct {
		name      string
		args      []string
		wantQuery string // lo que llega al Suggester ("" = no se llamó)
		wantSetup bool   // se ejecutó «vera ollama»
		wantMenu  bool   // se abrió «vera config»
	}{
		{"texto libre", []string{"ver", "puertos", "abiertos"}, "ver puertos abiertos", false, false},
		{"una sola palabra", []string{"uptime"}, "uptime", false, false},
		{"ollama sin modelo", []string{"ollama"}, "", true, false},
		{"ollama con modelo", []string{"ollama", "modelo-test"}, "", true, false},
		{"ollama + varias palabras", []string{"ollama", "no", "arranca"}, "ollama no arranca", false, false},
		{"config sola", []string{"config"}, "", false, true},
		{"config + palabras", []string{"config", "de", "nginx"}, "config de nginx", false, false},
		{"escape --", []string{"--", "config"}, "config", false, false},
		{"escape -- con ollama", []string{"--", "ollama"}, "ollama", false, false},
		{"trampa: guiones", []string{"listar", "con", "ls", "-la"}, "listar con ls -la", false, false},
		{"trampa: help", []string{"help", "me"}, "help me", false, false},
		{"trampa: help sola", []string{"help"}, "help", false, false},
		{"trampa: completion", []string{"completion", "de", "zsh"}, "completion de zsh", false, false},
		{"ayuda de ollama", []string{"ollama", "--help"}, "", false, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			h := newHarness(t)
			h.mgr.Models["modelo-test"] = 1
			h.run("0\n", tt.args...) // "n" a ejecutar / "0" para salir del menú

			got := strings.Join(h.sug.queries, "|")
			if got != tt.wantQuery {
				t.Errorf("query = %q, quería %q", got, tt.wantQuery)
			}
			ranSetup := strings.Contains(h.out.String(), "Ollama está instalado y funcionando")
			if ranSetup != tt.wantSetup {
				t.Errorf("vera ollama ejecutado = %v, quería %v\n%s", ranSetup, tt.wantSetup, h.out.String())
			}
			ranMenu := strings.Contains(h.out.String(), "Configuración de vera")
			if ranMenu != tt.wantMenu {
				t.Errorf("menú abierto = %v, quería %v", ranMenu, tt.wantMenu)
			}
		})
	}
}

func TestAyudaYVersion(t *testing.T) {
	h := newHarness(t)
	if code := h.run("", "--help"); code != 0 {
		t.Errorf("--help código %d", code)
	}
	h.wantOut(t, "vera ollama [modelo]", "vera -- <petición>", "modelo-test")

	h = newHarness(t)
	if code := h.run(""); code != 1 {
		t.Errorf("sin argumentos debería salir con 1, salió con %d", code)
	}
	h.wantOut(t, "Uso:")

	h = newHarness(t)
	h.run("", "--version")
	if strings.TrimSpace(h.out.String()) != "vera test" {
		t.Errorf("versión = %q", h.out.String())
	}

	h = newHarness(t)
	h.run("", "ollama", "-h")
	h.wantOut(t, "Instala Ollama si no está")
}

func TestOpcionDesconocida(t *testing.T) {
	h := newHarness(t)
	if code := h.run("", "-x", "algo"); code != 2 {
		t.Errorf("código %d", code)
	}
	h.wantOut(t, "Opción desconocida", "vera -- -x algo")
}

// ---------- vera <petición> ----------

func TestSugerirYEjecutar(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		wantExec bool
		wantCopy bool
		execCode int
		wantCode int
	}{
		{"s ejecuta", "s\n", true, false, 0, 0},
		{"Enter no ejecuta", "\n", false, false, 0, 0},
		{"n no ejecuta", "n\n", false, false, 0, 0},
		{"entrada cerrada no ejecuta", "", false, false, 0, 0},
		{"c copia", "c\n", false, true, 0, 0},
		{"devuelve el código del comando", "s\n", true, false, 3, 3},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			h := newHarness(t)
			h.execCode = tt.execCode
			code := h.run(tt.input, "listar", "archivos")
			if code != tt.wantCode {
				t.Errorf("código %d, quería %d", code, tt.wantCode)
			}
			if (len(h.executed) == 1) != tt.wantExec || (len(h.copied) == 1) != tt.wantCopy {
				t.Errorf("ejecutados %v, copiados %v", h.executed, h.copied)
			}
			h.wantOut(t, "❯ ls -a", "Lista archivos.", "[s/N/c=copiar]")
		})
	}
}

func TestComandoPeligroso(t *testing.T) {
	h := newHarness(t)
	h.sug.sug = suggest.Suggestion{Command: "rm -rf ~/x", Explanation: "Borra x.", Dangerous: true, Reason: "borra carpetas enteras."}
	h.run("\n", "borrar", "x")
	h.wantOut(t, "⚠ Comando peligroso: borra carpetas enteras.", "¿Ejecutar? [s/N")
	if len(h.executed) != 0 {
		t.Error("con Enter (por defecto no) nunca debe ejecutarse")
	}
}

func TestSinPortapapelesNoSeOfreceCopiar(t *testing.T) {
	h := newHarness(t)
	d := Deps{Suggester: h.sug, Manager: h.mgr, Config: h.cfg, UI: ui.New(strings.NewReader("c\n"), h.out, h.out, false, false),
		Info: func() sysinfo.Info { return sysinfo.Info{} }, Exec: func(context.Context, sysinfo.Info, string) (int, error) { return 0, nil }}
	Run(context.Background(), []string{"x"}, d)
	if strings.Contains(h.out.String(), "copiar") {
		t.Error("sin herramienta de copia no debería ofrecerse la opción c")
	}
}

func TestOllamaCerradoSeArrancaSolo(t *testing.T) {
	h := newHarness(t)
	h.mgr.Up = false
	h.run("n\n", "ver", "puertos")
	if !h.mgr.StartCalled || len(h.sug.queries) != 1 {
		t.Errorf("debería arrancar Ollama y seguir (start=%v, queries=%v)", h.mgr.StartCalled, h.sug.queries)
	}
	h.wantOut(t, "Arrancando Ollama…", "✓ Ollama funcionando")
}

func TestOllamaNoInstaladoAceptar(t *testing.T) {
	h := newHarness(t)
	h.mgr.Up, h.mgr.Installed = false, false
	h.mgr.Remote["modelo-test"] = 5 << 30
	code := h.run("s\nn\n", "ver", "puertos") // sí a instalar, no a ejecutar
	if code != 0 || !h.mgr.InstallCalled || !h.mgr.StartCalled || len(h.mgr.Pulled) != 1 || len(h.sug.queries) != 1 {
		t.Errorf("código %d, install=%v start=%v pulled=%v queries=%v\n%s", code, h.mgr.InstallCalled, h.mgr.StartCalled, h.mgr.Pulled, h.sug.queries, h.out)
	}
	h.wantOut(t, "junto con el modelo modelo-test (5.00 GB)", "Ollama instalado en", "Modelo modelo-test descargado")
}

func TestOllamaNoInstaladoRechazar(t *testing.T) {
	h := newHarness(t)
	h.mgr.Up, h.mgr.Installed = false, false
	code := h.run("n\n", "ver", "puertos")
	if code != 1 || h.mgr.InstallCalled || len(h.mgr.Pulled) != 0 || len(h.sug.queries) != 0 {
		t.Errorf("con «n» no debe tocar nada (código %d)", code)
	}
	h.wantOut(t, "Sin Ollama vera no puede funcionar")
}

func TestOllamaRemotoCaido(t *testing.T) {
	h := newHarness(t)
	h.mgr.Up, h.mgr.Local = false, false
	if code := h.run("", "x"); code != 1 || h.mgr.StartCalled || h.mgr.InstallCalled {
		t.Errorf("con Ollama en otro equipo no debe intentar instalar ni arrancar (código %d)", code)
	}
	h.wantOut(t, "OLLAMA_HOST apunta a otro equipo")
}

func TestModeloQueFaltaAceptar(t *testing.T) {
	h := newHarness(t)
	h.mgr.Remote["modelo-test"] = 2 << 30
	h.sug.errs = []error{llm.ErrModelNotFound, nil}
	h.run("s\nn\n", "ver", "puertos")
	if len(h.mgr.Pulled) != 1 || len(h.sug.queries) != 2 {
		t.Errorf("debería descargar y reintentar: pulled=%v queries=%v", h.mgr.Pulled, h.sug.queries)
	}
	h.wantOut(t, "No tienes el modelo modelo-test (2.00 GB). ¿Descargarlo ahora?", "❯ ls -a")
}

func TestModeloQueFaltaRechazar(t *testing.T) {
	h := newHarness(t)
	h.mgr.Remote["modelo-test"] = 1
	h.sug.errs = []error{llm.ErrModelNotFound}
	if code := h.run("n\n", "x"); code != 1 || len(h.mgr.Pulled) != 0 {
		t.Errorf("código %d, pulled %v", code, h.mgr.Pulled)
	}
	h.wantOut(t, "Descárgalo cuando quieras con: vera ollama modelo-test")
}

func TestModeloQueFaltaSinConfirmar(t *testing.T) {
	h := newHarness(t)
	no := false
	h.cfg.ConfirmDownloads = &no
	h.mgr.Remote["modelo-test"] = 1
	h.sug.errs = []error{llm.ErrModelNotFound, nil}
	h.run("n\n", "x") // la única respuesta es para «¿Ejecutar?»
	if len(h.mgr.Pulled) != 1 {
		t.Errorf("con la confirmación desactivada debería descargar sin preguntar: %v", h.mgr.Pulled)
	}
}

func TestModeloDeEmbeddings(t *testing.T) {
	h := newHarness(t)
	h.sug.errs = []error{llm.ErrNotChat}
	h.run("", "x")
	h.wantOut(t, "no sirve para conversar")
}

// ---------- vera ollama ----------

func TestOllamaCmdInstalaYDescargaSiFalta(t *testing.T) {
	h := newHarness(t)
	h.mgr.Up, h.mgr.Installed = false, false
	h.mgr.Remote["qwen2.5"] = 4 << 30
	if code := h.run("s\n", "ollama", "qwen2.5"); code != 0 {
		t.Fatalf("código %d\n%s", code, h.out)
	}
	if !h.mgr.InstallCalled || len(h.mgr.Pulled) != 1 || h.mgr.Pulled[0] != "qwen2.5" {
		t.Errorf("install=%v pulled=%v", h.mgr.InstallCalled, h.mgr.Pulled)
	}
	h.wantOut(t, "¿Descargar el modelo qwen2.5 (4.00 GB)?", "vera usará qwen2.5 a partir de ahora")
	saved, _ := config.Load(h.cfg.Path)
	if saved.Model != "qwen2.5" {
		t.Errorf("no se guardó el modelo: %q", saved.Model)
	}
}

func TestOllamaCmdRechazarDescarga(t *testing.T) {
	h := newHarness(t)
	h.mgr.Remote["grande:70b"] = 40 << 30
	h.run("n\n", "ollama", "grande:70b")
	if len(h.mgr.Pulled) != 0 || h.cfg.Model != "modelo-test" {
		t.Errorf("no debería descargar ni cambiar el modelo: pulled=%v model=%q", h.mgr.Pulled, h.cfg.Model)
	}
	h.wantOut(t, "(40.00 GB)", "No se ha descargado nada.")
}

func TestOllamaCmdModeloInexistente(t *testing.T) {
	h := newHarness(t)
	if code := h.run("", "ollama", "no-existe"); code != 1 || len(h.mgr.Pulled) != 0 {
		t.Errorf("código %d, pulled %v", code, h.mgr.Pulled)
	}
	h.wantOut(t, "no existe", "ollama.com/search")
}

func TestOllamaCmdEmbeddingsRecienDescargadoSeBorraPorDefecto(t *testing.T) {
	h := newHarness(t)
	h.mgr.Remote["all-minilm"] = 45 << 20
	h.mgr.NotChat["all-minilm"] = true
	code := h.run("s\n\n", "ollama", "all-minilm") // sí a descargar, Enter a borrar
	if code != 1 || len(h.mgr.Deleted) != 1 || h.cfg.Model != "modelo-test" {
		t.Errorf("código %d, borrados %v, modelo %q", code, h.mgr.Deleted, h.cfg.Model)
	}
	h.wantOut(t, "¿Borrarlo para liberar espacio? [S/n]")
}

func TestOllamaCmdEmbeddingsQueYaTeniasNoSeBorraPorDefecto(t *testing.T) {
	h := newHarness(t)
	h.mgr.Models["all-minilm"] = 1
	h.mgr.NotChat["all-minilm"] = true
	h.run("\n", "ollama", "all-minilm")
	if len(h.mgr.Deleted) != 0 {
		t.Error("un modelo que ya tenías no debería borrarse con Enter")
	}
	h.wantOut(t, "[s/N]")
}

func TestOllamaCmdAvisaDeVeraModel(t *testing.T) {
	h := newHarness(t)
	h.cfg.Getenv = func(k string) string {
		if k == "VERA_MODEL" {
			return "otro"
		}
		return ""
	}
	h.mgr.Models["llama3.2"] = 1
	h.run("", "ollama", "llama3.2")
	h.wantOut(t, "VERA_MODEL=otro, que tiene prioridad")
}

func TestOllamaCmdErrorAlInstalar(t *testing.T) {
	h := newHarness(t)
	h.mgr.Up, h.mgr.Installed = false, false
	h.mgr.InstallErr = errors.New("no se pudo instalar Ollama: sin red")
	if code := h.run("", "ollama"); code != 1 {
		t.Errorf("código %d", code)
	}
	h.wantOut(t, "No se pudo instalar Ollama: sin red")
}

// ---------- vera config ----------

func TestConfigMenu(t *testing.T) {
	h := newHarness(t)
	h.mgr.Models = map[string]int64{"a:1b": 1 << 30, "b:7b": 5 << 30, "emb": 1}
	h.mgr.NotChat["emb"] = true
	h.mgr.Auto = setup.Autostart{State: "no", CanChange: true}

	// 9 → no válida; 2 → desactivar confirmación; 1 → elegir el 2.º modelo de chat; 3 → activar arranque; 0 → salir
	h.run("9\n2\n1\n2\n3\n0\n", "config")

	h.wantOut(t, "Opción no válida", "Confirmar antes de descargar: no", "Modelo por defecto: b:7b", "Abrir Ollama al iniciar sesión: sí")
	if strings.Contains(h.out.String(), "3) emb") {
		t.Error("un modelo de embeddings no debería ofrecerse como modelo por defecto")
	}
	saved, _ := config.Load(h.cfg.Path)
	if saved.Model != "b:7b" || saved.Confirm() {
		t.Errorf("guardado: %+v", saved.File)
	}
	if len(h.mgr.AutostartSet) != 1 || !h.mgr.AutostartSet[0] {
		t.Errorf("autostart = %v", h.mgr.AutostartSet)
	}
}

func TestConfigBorrarModelos(t *testing.T) {
	h := newHarness(t)
	h.mgr.Models = map[string]int64{"a:1b": 1 << 30, "modelo-test": 2 << 30, "emb": 1}
	h.mgr.NotChat["emb"] = true

	// 4 → borrar; «1, 3» → a:1b y modelo-test; s → confirmar; 0 → salir
	h.run("4\n1, 3\ns\n0\n", "config")
	if len(h.mgr.Deleted) != 2 {
		t.Errorf("borrados = %v", h.mgr.Deleted)
	}
	h.wantOut(t, "(no sirve para vera)", "← modelo por defecto", "Se liberarán 3.00 GB", "Era tu modelo por defecto")
}

func TestConfigSinOllama(t *testing.T) {
	h := newHarness(t)
	h.mgr.Up, h.mgr.Installed = false, false
	h.run("1\n0\n", "config")
	h.wantOut(t, "Ollama no está instalado. Instálalo con: vera ollama")
}

func TestConfigAutostartNoModificable(t *testing.T) {
	h := newHarness(t)
	h.mgr.Auto = setup.Autostart{State: "automático (lo hace la app de Ollama)", Detail: "Lo controla la app."}
	h.run("3\n0\n", "config")
	h.wantOut(t, "automático (lo hace la app de Ollama)", "Lo controla la app.")
	if len(h.mgr.AutostartSet) != 0 {
		t.Error("no debería intentar cambiarlo")
	}
}

func TestPreRoute(t *testing.T) {
	tests := []struct{ in, want string }{
		{"ollama", "ollama"},
		{"ollama llama3.2", "ollama llama3.2"},
		{"ollama no arranca", "-- ollama no arranca"},
		{"ollama -h", "ollama -h"},
		{"config", "config"},
		{"config de nginx", "-- config de nginx"},
		{"ver puertos", "ver puertos"},
	}
	for _, tt := range tests {
		if got := strings.Join(preRoute(strings.Fields(tt.in)), " "); got != tt.want {
			t.Errorf("preRoute(%q) = %q, quería %q", tt.in, got, tt.want)
		}
	}
}

func TestMain(m *testing.M) {
	os.Unsetenv("NO_COLOR")
	os.Exit(m.Run())
}
