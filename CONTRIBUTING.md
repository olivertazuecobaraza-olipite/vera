# Cómo contribuir a vera

¡Gracias por querer ayudar! Esta guía explica cómo se trabaja en el proyecto.

## Ramas

```
feat/…, fix/…, docs/…  ──PR (squash)──▶  develop  ──PR (merge)──▶  main  ──▶  release automática
```

- **`main`** es producción: cada fusión publica una versión nueva. Solo recibe PR desde `develop`.
- **`develop`** es la rama de integración y la rama por defecto. Todas las PR van aquí.
- Para cada cambio, crea una rama desde `develop`: `feat/nombre-corto`, `fix/…`, `docs/…`, `chore/…`.

Nadie (tampoco el mantenedor) puede subir directamente a `develop` ni a `main`: todo entra por PR, con todas las verificaciones en verde y aceptada por el mantenedor.

## Commits convencionales

El **título de la PR** debe seguir [Conventional Commits](https://www.conventionalcommits.org/es/), porque al fusionar con *squash* se convierte en el mensaje del commit, y de ahí se calcula la versión:

| Tipo | Cuándo | Efecto en la versión |
|---|---|---|
| `feat:` | nueva funcionalidad | sube la **minor** (1.2.0 → 1.3.0) |
| `fix:` | corrección de un fallo | sube la **patch** (1.2.0 → 1.2.1) |
| `docs:`, `test:`, `refactor:`, `perf:`, `chore:`, `ci:`, `build:`, `style:` | lo demás | sube la **patch** |
| `feat!:` / `fix!:` o `BREAKING CHANGE:` en el cuerpo | cambio incompatible | sube la **major** (1.2.0 → 2.0.0) |

Ejemplos: `feat: copiar el comando con la tecla c`, `fix(setup): reintentar la descarga si se corta`, `docs: instalación en Windows`.

El CI comprueba el título; si no cumple el formato, la PR no se puede fusionar.

## Antes de abrir la PR

Necesitas [Go](https://go.dev) (la versión de `go.mod`).

```sh
./build.sh test          # go vet + tests con detector de carreras
./build.sh cover         # opcional: cobertura (cover.html)
```

- Añade o actualiza tests para lo que cambies. La arquitectura y qué probar en cada paquete están en [ARQUITECTURA.md](ARQUITECTURA.md).
- Si cambias la plantilla del prompt (`internal/suggest/templates/`), regenera los golden files con `./build.sh golden` y revisa el `git diff`.
- Si añades un patrón de comando peligroso, añade primero el caso a la tabla de `internal/suggest/danger_test.go`.
- Con Ollama instalado puedes lanzar también los tests con un modelo real: `VERA_TEST_MODEL=qwen3:0.6b ./build.sh integration`.

## Qué comprueba el CI

En cada PR: formato (`gofmt`), `go vet`, golangci-lint, `go mod tidy`, tests en Linux, macOS y Windows, compilación de los 6 binarios, vulnerabilidades conocidas (govulncheck), CodeQL, revisión de dependencias y seguridad de los workflows.

En las PR de `develop` a `main`, además: una prueba de extremo a extremo en un Ubuntu limpio (instala Ollama con vera, descarga un modelo y hace peticiones reales) y un comentario con la versión que se publicará.

Las PR de colaboradores externos necesitan que el mantenedor apruebe la ejecución del CI antes de que corra.

## Releases

Son automáticas: al fusionar `develop` en `main`, se calcula la versión a partir de los commits, se crea la etiqueta `vX.Y.Z` y se publican los binarios con sus checksums y atestaciones de procedencia. No hay que crear etiquetas a mano (y no se puede).

Cada release también publica vera en los gestores de paquetes:

| Canal | Dónde | Cómo |
|---|---|---|
| `.deb`, `.rpm`, `.apk` | adjuntos a la release de GitHub | GoReleaser (`nfpms`) |
| Homebrew | `olivertazuecobaraza-olipite/homebrew-tap` (`Casks/vera.rb`) | GoReleaser (`homebrew_casks`) |
| Scoop | `olivertazuecobaraza-olipite/scoop-bucket` (`vera.json`) | GoReleaser (`scoops`) |
| winget | PR a `microsoft/winget-pkgs` desde el fork propio (la revisa Microsoft) | GoReleaser (`winget`) |
| AUR (en pausa) | `vera-bin` | GoReleaser (`aurs`) genera el PKGBUILD pero no lo sube (`skip_upload: true`) hasta que haya cuenta de AUR |
| npm | `vera-ia` + `@olipite/vera-<so>-<arq>` | `.github/scripts/npm-packages.mjs` en `release.yml` |

Los secretos están en el environment `release` (que solo puede usar `main`):

- `PACKAGES_TOKEN`: token *fine-grained* con permiso de escritura en Contents y Pull requests, limitado a `homebrew-tap`, `scoop-bucket` y el fork de `winget-pkgs`.
- `WINGET_TOKEN`: token clásico con el scope `public_repo`, solo para abrir la PR en `microsoft/winget-pkgs` (los tokens fine-grained no pueden contribuir a repos públicos ajenos).
- `AUR_KEY`: clave SSH privada (sin contraseña) cuya parte pública está en la cuenta de AUR.

npm no usa ningún secreto: cada paquete (`vera-ia` y `@olipite/vera-*`) tiene configurado *trusted publishing* en npmjs.com (repositorio `olivertazuecobaraza-olipite/vera`, workflow `release.yml`, environment `release`), así que solo ese workflow puede publicarlos. Un paquete nuevo hay que publicarlo la primera vez a mano y configurarle lo mismo.

Las versiones previas (`vX.Y.Z-rc1`) solo se publican en GitHub, no en los gestores. En cada PR, el CI genera todos los paquetes sin publicarlos y prueba a instalar el `.deb` y el de npm.

## Seguridad

Las vulnerabilidades **no** se reportan en issues: sigue [SECURITY.md](SECURITY.md).

## Código de conducta

Este proyecto sigue el [Código de conducta](CODE_OF_CONDUCT.md). Al participar, te comprometes a respetarlo.

## Paso a paso

```sh
git switch develop && git pull
git switch -c feat/mi-cambio            # o fix/…, docs/…
# … cambios …
./build.sh test
git commit -am "feat: lo que hace el cambio"
git push -u origin feat/mi-cambio
gh pr create --base develop --title "feat: lo que hace el cambio" --fill
```

Cuando el CI esté en verde y la PR aceptada, se fusiona con *squash* y la rama se borra sola.
Para publicar, el mantenedor abre una PR de `develop` a `main`: el CI añade la prueba de extremo
a extremo y un comentario con la versión que saldrá; al fusionarla se publica la release.
