#!/bin/sh
# Calcula la próxima versión (vX.Y.Z) a partir de los commits convencionales desde la última etiqueta:
#   feat → minor · fix y el resto → patch · «!» o BREAKING CHANGE → major
# Sin etiquetas previas, la primera versión es v1.0.0.
# La usan release.yml (para publicar) y pr.yml (para anunciarla en la PR a main).
set -eu

if [ -z "$(git tag --list 'v*')" ]; then
  echo "v1.0.0"
  exit 0
fi
# --always: también se publica versión (patch) si solo hay docs/chore/ci…
go run github.com/caarlos0/svu/v3@v3.4.4 next --always
