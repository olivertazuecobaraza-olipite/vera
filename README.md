# vera

Describe en lenguaje natural lo que quieres hacer en la terminal y un modelo de IA **local** (con [Ollama](https://ollama.com)) te propone el comando. Antes de ejecutarlo te pide confirmación. No envía nada a internet.

El comando se puede usar como `vera` o como `ia`: son lo mismo.

```
$ vera qué proceso usa el puerto 3000
❯ lsof -i :3000
¿Ejecutar? [s/N] s
...
✓ terminado
```

Funciona en **macOS, Linux y Windows**. Detecta tu sistema y tu shell (zsh/bash, PowerShell o cmd) y pide comandos adecuados para ella.

## Requisitos

Hace falta [Ollama](https://ollama.com) y un modelo. vera puede instalar las dos cosas (ver abajo).

## Instalación

Descarga el binario de tu sistema de `dist/`:

| Sistema | Archivo |
|---|---|
| Mac Apple Silicon | `vera-darwin-arm64` |
| Mac Intel | `vera-darwin-amd64` |
| Linux x64 / ARM | `vera-linux-amd64` / `vera-linux-arm64` |
| Windows x64 / ARM | `vera-windows-amd64.exe` / `vera-windows-arm64.exe` |

**macOS / Linux:** `./install.sh` (lo copia a `~/.local/bin/vera` y crea el alias `ia`).

**Windows** (PowerShell, en la carpeta del proyecto o junto al `.exe`):
```powershell
powershell -ExecutionPolicy Bypass -File .\install.ps1
```
Lo copia a `%LOCALAPPDATA%\Programs\vera` (como `vera.exe` e `ia.exe`) y lo añade al PATH. Abre una terminal nueva después.

## Instalar Ollama y elegir modelo: `vera ollama`

```
vera ollama               # usa el modelo por defecto
vera ollama llama3.2      # o el que quieras de https://ollama.com/search
```

1. Si Ollama no está instalado, lo descarga e instala (con barra de progreso):
   - **macOS:** `Ollama.app` en `/Applications` (o `~/Applications` si no hay permisos).
   - **Linux:** en `~/.local/bin/ollama`, sin sudo. (Si ya tienes Ollama como servicio del sistema, vera te dirá que lo arranques con `sudo systemctl start ollama`.)
   - **Windows:** con el instalador oficial, en tu cuenta y sin permisos de administrador.
2. Si Ollama no está abierto, lo arranca.
3. Si el modelo no está descargado, te dice cuánto ocupa, pide confirmación y lo descarga (con barra de progreso). Si el nombre no existe, avisa antes de empezar.
4. Lo guarda como modelo por defecto de vera (en la carpeta de configuración del usuario). `VERA_MODEL`, si está definida, tiene prioridad.

Si pulsas Ctrl+C durante la descarga del modelo, al repetir el comando sigue donde se quedó. Los modelos que no sirven para conversar (p. ej. de embeddings) se rechazan, y vera ofrece borrarlos.

No hace falta ejecutar `vera ollama` antes de usar vera: si haces una petición y Ollama está cerrado, vera lo arranca solo; si falta Ollama o el modelo, te pregunta si quieres instalarlo.

## Ajustes: `vera config`

Abre un menú sencillo (se elige con números):

```
Configuración de vera

  1) Modelo por defecto                qwen3.5:9b-mlx
  2) Confirmar antes de descargar      sí
  3) Abrir Ollama al iniciar sesión    automático (lo hace la app de Ollama)
  4) Borrar modelos
  0) Salir
```

- **Modelo por defecto:** elige entre los modelos de chat que tienes, o escribe otro y se descarga.
- **Confirmar antes de descargar:** si está en «no», los modelos se descargan sin preguntar.
- **Abrir Ollama al iniciar sesión:** en macOS y Windows lo hace la app de Ollama. En Linux (si Ollama lo instaló vera) se activa con un servicio de usuario de systemd. Aunque esté desactivado, vera abre Ollama cuando lo necesita.
- **Borrar modelos:** muestra lo que ocupa cada uno; se pueden borrar varios a la vez (p. ej. `1,3`).

Los ajustes se guardan en `config.json` dentro de la carpeta de configuración del usuario (`~/Library/Application Support/vera` en macOS, `~/.config/vera` en Linux, `%AppData%\vera` en Windows).

## Configuración (variables de entorno)

| Variable | Para qué | Por defecto |
|---|---|---|
| `VERA_MODEL` | Modelo de Ollama (tiene prioridad sobre el elegido con `vera ollama`) | `qwen3.5:9b-mlx` en Mac Apple Silicon, `qwen3.5:9b` en el resto |
| `OLLAMA_HOST` | Dónde está Ollama (puede ser otro ordenador) | `127.0.0.1:11434` |
| `VERA_SHELL` | Shell con la que ejecutar. En Windows, `cmd` para usar cmd.exe | `$SHELL` / PowerShell |
| `NO_COLOR` | Desactiva los colores | — |

## Compilar

Hace falta [Go](https://go.dev) 1.22+:
```
./build.sh                 # genera todos los binarios en dist/
go build -o vera .         # solo para tu sistema (en Windows: go build -o vera.exe .)
```

## Aviso

Un modelo pequeño se equivoca a veces. **Lee el comando antes de pulsar `s`**, sobre todo si borra, mueve o sobrescribe archivos.
