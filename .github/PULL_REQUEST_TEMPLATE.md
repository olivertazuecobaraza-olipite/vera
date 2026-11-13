<!--
El título de la PR debe seguir Conventional Commits (feat:, fix:, docs:…): será el mensaje del commit
y decide la próxima versión. Ver CONTRIBUTING.md.
-->

## Qué cambia

<!-- Qué hace esta PR y por qué. Si cierra un issue: «Closes #123». -->

## Cómo se ha probado

<!-- Tests añadidos, pruebas a mano (sistema operativo, shell, modelo de Ollama…). -->

## Checklist

- [ ] `./build.sh test` pasa en local
- [ ] He añadido o actualizado tests
- [ ] He actualizado la documentación (README, ARQUITECTURA.md…) si hacía falta
- [ ] Si he cambiado el prompt, he regenerado los golden files (`./build.sh golden`) y revisado el diff
