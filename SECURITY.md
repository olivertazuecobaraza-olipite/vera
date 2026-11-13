# Política de seguridad

## Versiones con soporte

Solo la **última release** recibe correcciones de seguridad. Actualiza antes de reportar un problema:

```sh
curl -fsSL https://raw.githubusercontent.com/olivertazuecobaraza-olipite/vera/main/install.sh | sh
```

## Cómo reportar una vulnerabilidad

**No abras un issue público.** Usa el reporte privado de GitHub:

1. Ve a la pestaña [**Security → Report a vulnerability**](https://github.com/olivertazuecobaraza-olipite/vera/security/advisories/new).
2. Describe el problema, cómo reproducirlo, la versión de vera (`vera --version`), tu sistema operativo y el impacto que crees que tiene.

Solo el mantenedor puede ver el reporte.

## Qué esperar

| Paso | Plazo orientativo |
|---|---|
| Acuse de recibo | 3 días |
| Primera evaluación (¿es una vulnerabilidad?, gravedad) | 7 días |
| Corrección publicada en una release | según la gravedad; las críticas, lo antes posible |

Cuando la corrección esté publicada se abrirá un aviso de seguridad (GitHub Security Advisory) y, si quieres, se te mencionará como descubridor.

## Qué entra en el alcance

- Que vera ejecute un comando **sin** la confirmación explícita del usuario.
- Que `IsDangerous` no marque un comando claramente destructivo (borrado recursivo, formateo, ejecutar scripts descargados…).
- Problemas en la descarga o instalación de Ollama o de los binarios de vera (integridad, rutas, permisos).
- Fugas de datos: vera no debe enviar nada a ningún servidor salvo a Ollama (local o el que indique `OLLAMA_HOST`) y a ollama.com para descargas.

Fuera del alcance: que el modelo proponga un comando incorrecto o poco útil (es inherente a los modelos de lenguaje; vera siempre pide confirmación), y vulnerabilidades del propio Ollama, que se reportan a [su proyecto](https://github.com/ollama/ollama/security).

## Verificar los binarios

Cada release incluye `checksums.txt` y atestaciones de procedencia firmadas por GitHub Actions:

```sh
gh attestation verify vera_<versión>_<so>_<arquitectura>.tar.gz --repo olivertazuecobaraza-olipite/vera
```
