# Pruebas de humo, carga y mutación en GitHub Actions

Spec 017, T010. Tres workflows aparte de `ci.yml` (que no cambia). Ninguno usa secretos ni corre en los PR: no son controles obligatorios para fusionar.

| Workflow | Archivo | Cuándo corre | Qué prueba |
|---|---|---|---|
| Smoke | `.github/workflows/smoke.yml` | a mano, diario (13:23 UTC) y tras cada push a `develop` | Que staging responde bien por HTTPS |
| Load | `.github/workflows/load.yml` | **solo a mano** | Estrés y fatiga (soak) con k6 |
| Mutation | `.github/workflows/mutation.yml` | a mano y semanal (domingo) | Que las pruebas detectan cambios en el código |

Los resultados aparecen en el resumen de cada ejecución (pestaña *Actions*) y, en carga y mutación, como artefacto descargable (30 días).

## Pruebas de humo

`scripts/smoke.sh <url-base>` comprueba, validando el certificado (sin `-k`):

1. `GET /healthz` → 200 y `{"status":"ok"}`.
2. `GET /readyz` → 200 y `{"status":"ready"}`.
3. `GET /api/v1/meta` → 200 con `name`, `version`, `environment` y `serverTime`.
4. Una ruta inexistente con `X-Request-Id` propio → 404 con `{"error":{"code":"not_found","message","requestId"}}` y el mismo `X-Request-Id` en el encabezado y en el cuerpo (Art. IV).

```bash
SMOKE_EXPECT_ENV=staging bash scripts/smoke.sh https://vaiven-staging.westus2.cloudapp.azure.com:8443
```

Solo acepta `https://`; para la API local usar `SMOKE_ALLOW_HTTP=1`. Usa `jq` si está instalado y, si no, expresiones regulares (sirve en Git Bash de Windows). La URL de staging se puede cambiar con la variable de repositorio `STAGING_URL` (Settings → Variables) o con la entrada `base_url` al lanzarlo a mano.

El despliegue es manual (`scripts/deploy.sh`), así que el disparo por push a `develop` comprueba el estado actual de staging, no el commit recién fusionado.

## Carga: estrés y fatiga (k6)

Script: `tests/load/api.js` (k6 2.3.0). Pide `/healthz` y `/api/v1/meta` al azar y valida el cuerpo.

**Límite de tasa.** La API permite 120 peticiones por minuto por IP y responde 429 `rate_limited` con `Retry-After` (Art. XIV.2). Ese 429 es **esperado**: no cuenta como error en `http_req_failed` y se mide aparte en `rate_limited` y `rate_limited_rate`. Sí es error cualquier otro código, un cuerpo inválido o un 429 sin el envoltorio del Art. IV.

Umbrales: errores reales < 1 %, `checks` > 99 %, cuerpos inválidos < 1 % y p95 de las respuestas esperadas < 800 ms en staging o < 300 ms en local (`P95_MS` lo cambia). En smoke y soak de staging, además, menos del 5 % de 429.

Entradas del workflow:

| Entrada | Valores | Nota |
|---|---|---|
| `target` | `local` (defecto), `staging` | Dónde va la carga |
| `profile` | `stress` (defecto), `soak`, `smoke` | Perfil de k6 |
| `duration` | `1m` a `60m` (defecto `10m`) | Solo para `soak` |
| `confirm_staging` | casilla | Obligatoria con `target=staging` |

Perfiles:

| Perfil | `staging` (VM de ~1 GiB, API con 160 MiB) | `local` (API dentro del runner) |
|---|---|---|
| `smoke` | 1 petición/s durante 30 s | 5/s durante 30 s |
| `stress` | sube de 1 a 6/s en 3 min, 1 min arriba y baja: cruza el límite de 120/min a propósito | sube de 10 a 600/s en 5 min |
| `soak` | 1.5/s (90/min, bajo el límite) durante `duration` | 100/s durante `duration` |

- **`local`** levanta la API compilada del commit con MongoDB 7 y Redis 7 como servicios del job, `APP_ENV=test` (sin Cloudinary ni TLS), `GOMEMLIMIT=120MiB` y `GOMAXPROCS=1` como en staging. Al final informa el pico de memoria residente (`VmHWM`) y **falla si supera 160 MiB** o si la API murió durante la carga: así el soak detecta fugas de memoria. Como todo sale de una IP, con el límite de tasa activo la mayoría de las peticiones del estrés reciben 429: se mide cuánto aguanta la API (y Redis) mientras limita.
- **`staging`** corre las pruebas de humo antes y después (tras esperar 65 s a que pase la ventana del límite) para confirmar que staging sigue sano. Un solo run a la vez por destino (`concurrency`).

En local, con Podman:

```bash
podman run --rm -v "$PWD/tests/load:/s:ro" docker.io/grafana/k6:2.3.0 run \
  -e BASE_URL=https://vaiven-staging.westus2.cloudapp.azure.com:8443 -e PROFILE=smoke /s/api.js
```

## Mutación (gremlins)

`scripts/mutation.sh` no instala nada: necesita `go` y `gremlins` v0.6.0 (`go install github.com/go-gremlins/gremlins/cmd/gremlins@v0.6.0`). Recorre cada paquete de `internal/domain`, `internal/usecase` e `internal/platform/security` (los que no existen se omiten), ejecuta `gremlins unleash` y escribe en `reports/mutation/` un JSON y un log por paquete y `summary.md`.

- **Eficacia** = eliminados / (eliminados + vivos). Umbral por defecto: 75 %.
- **Cobertura de mutantes** = (eliminados + vivos) / total. Umbral por defecto: 70 %.
- Los paquetes sin mutantes (solo interfaces) se informan como «sin mutantes» y no fallan.
- Los mutantes con tiempo agotado (`TIMED OUT`, típicos en bucles de argon2) no cuentan en la eficacia; `MUTATION_TIMEOUT_COEFFICIENT=5` da margen a las pruebas lentas.
- gremlins v0.6.0 ignora `--threshold-*` en la línea de comandos (sale con 0 aunque no se cumpla), por eso el script comprueba los umbrales por su cuenta.

Medición de referencia (Linux, código de la spec 003 en curso): `domain` 100 %, `platform/security` ~90 % de eficacia y ~92 % de cobertura de mutantes. Los mutantes vivos señalan pruebas que conviene reforzar.

```bash
MUTATION_EFFICACY=75 MUTATION_MCOVER=70 bash scripts/mutation.sh
```

En Windows gremlins falla al borrar sus carpetas temporales y marca casi todo como tiempo agotado: correrlo en Linux (la CI o `podman run docker.io/library/golang:1.26.10`).
