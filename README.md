# VAIVÉN — Backend — Spec Driven Development con OpenCode

Plataforma móvil de préstamo y renta de objetos entre personas (México, MXN).
Contexto completo: [`docs/VAIVEN_Idea_y_Objetivo.pdf`](docs/VAIVEN_Idea_y_Objetivo.pdf). Calendario, reparto entre personas y políticas confirmadas: [`ROADMAP.md`](ROADMAP.md).

**Stack:** Go + chi + MongoDB (NoSQL) · Redis · HTTPS con Let's Encrypt · Docker Compose en una VM de **Azure** (cuenta gratuita) · Cloudinary

Este repositorio es **independiente**. Forma parte de VAIVÉN junto con [`vaiven-frontend`](../vaiven-frontend) (app móvil) y [`vaiven-landing-page`](../vaiven-landing-page) (sitio informativo). Se asume que los tres repositorios están clonados lado a lado, porque algunos documentos del SDD apuntan al repositorio hermano con rutas `../vaiven-*`.

## Qué contiene

| Archivo / carpeta | Para qué sirve |
|---|---|
| `constitution.md` | Reglas técnicas no negociables. Máxima autoridad. |
| `AGENTS.md` | Instrucciones que OpenCode carga siempre: orden de lectura, flujo, agentes, comandos, reglas. |
| `memoria.md` | Decisiones, estado, modelos asignados, dependencias y aprendizajes. |
| `opencode.json` | Modelos gratuitos por agente, instrucciones cargadas y permisos. |
| `.opencode/agents/` y `.opencode/skills/` | Subagentes propios y skills con plantillas y listas de comprobación. |
| `specs/001…015/` y `017/` | `spec.md`, `plan.md` y `tasks.md` de cada fase: **001 infraestructura en Azure y servicios**, 002 arquitectura base (incluye Redis, HTTPS y `deploy/`), cuenta, imágenes, objetos, búsqueda, reservas, chat, entrega y pagos, calificaciones, notificaciones, integración, producción, IA y ML; la **017** es DevSecOps (CI, escaneo de seguridad y despliegue continuo a staging). Cada una termina con su tarea «Despliegue». |
| `ROADMAP.md`, `docs/` | Copia del plan maestro y del documento de idea del proyecto. |

Contrato de API: `api/openapi.yaml` (Art. IV). El móvil (`../vaiven-frontend`) desarrolla contra `api/paths/<modulo>.yaml`.

## Infraestructura (Azure)

```
Internet ─ HTTPS ─► api (Go) ─► MongoDB (rs0) · Redis      ← una VM de Azure, Docker Compose
```
Staging en el puerto 8443 y producción en el 443 (proyectos de Compose separados). La API **solo** sirve HTTPS, con un certificado de Let's Encrypt emitido con `certbot` en la VM (sin nginx). **Redis es obligatorio** (límite de tasa y caché; nunca fuente de verdad, Art. XIV). Cloudinary guarda las fotos. Una sola réplica: la VM gratuita tiene ~1 GiB de RAM.

## Flujo: OpenCode + MCP, spec por spec

1. Inicia sesión con `az login` y define `CLOUDINARY_URL`; reinicia OpenCode y comprueba con `opencode mcp list` que `azure` y los dos MCP de Cloudinary estén conectados.
2. Spec **001**: `@devops-mcp` elige región y talla gratuitas, crea la VM, la red y el stack base (MongoDB y Redis) y emite el certificado HTTPS.
3. Desde la **002**: se implementan las tareas con `@dev-go`; la última tarea de cada spec es «Despliegue» (`@devops-mcp` ejecuta `scripts/deploy.sh staging` y verifica). **La spec siguiente no empieza hasta que esa tarea pase.**
4. El despliegue manual no depende del repositorio (la imagen viaja por SSH); desde la enmienda 2.0.0, cada fusión en `develop` con la CI en verde despliega staging por GitHub Actions (OIDC, Run Command e imágenes en GHCR; ver `docs/servicios.md` §7). Repositorio en GitHub con `main` y `develop` protegidas, PR por tarea, fase o spec (formatos en `docs/devsecops.md`) y CI en GitHub Actions (política, calidad, secretos y CodeQL; spec 017).

## Cómo se usa con OpenCode

```bash
opencode          # desde la raíz de este repositorio
```
1. Ejecuta `opencode models` y confirma que los modelos de `opencode.json` existen (los gratuitos rotan); si cambias alguno, anótalo en `memoria.md`.
2. Agente `plan`: revisa y ajusta `spec.md`, `plan.md` y `tasks.md`.
3. Con la spec aprobada por una persona, cambia al agente `build` y pide @dev-go la tarea que toque.
4. Al cerrar una fase, pide `@revisor-constitucion revisa los cambios`.

## Coherencia entre repositorios

- Los Artículos **IV** (contrato de API) y **V** (dinero, fechas, idioma) deben mantenerse idénticos en las constituciones de backend y móvil. Si cambias uno, cámbialo en ambos repositorios y registra la decisión en ambos `memoria.md`.
- `ROADMAP.md` y `docs/VAIVEN_Idea_y_Objetivo.pdf` son copias en cada repositorio; si se actualiza una, hay que actualizar las demás.

## Desarrollo local

### Requisitos

| Herramienta | Versión | Para qué |
|---|---|---|
| Go | 1.26 (la línea `toolchain` de `go.mod` fija 1.26.10; Go la descarga solo) | Compilar y probar |
| Docker **o** Podman | Docker Desktop o Podman Desktop | MongoDB local y pruebas de integración |
| golangci-lint | v2.15.0 (la misma de la CI): `go install github.com/golangci/golangci-lint/v2/cmd/golangci-lint@v2.15.0` | `make lint` |
| make | opcional | Atajos; en Windows hay equivalentes abajo |

### Levantar MongoDB y la API

1. MongoDB 7 local como replica set `rs0` (sin autenticación, solo para tu máquina):
   ```bash
   make up                       # o: docker compose up -d
   ```
   El `docker-compose.yml` de la raíz es **solo para desarrollo**; el de la VM vive en `deploy/base/`.
2. Variables de entorno: copia `.env.example` a `.env` (ignorado por git) y ajusta la sección «API en desarrollo local». La API **solo lee variables de entorno**, no el archivo, así que cárgalas en la terminal:
   ```bash
   set -a; source .env; set +a   # Git Bash o WSL
   ```
   ```powershell
   Get-Content .env | Where-Object { $_ -match '^[A-Z_]+=' } | ForEach-Object { $k, $v = $_ -split '=', 2; Set-Item "env:$k" $v }   # PowerShell
   ```
3. Arranca la API (escucha en `:8080`):
   ```bash
   make run                      # o: go run ./cmd/api
   ```
   Si falta una variable obligatoria (`MONGO_URI`, `MONGO_DB` y las tres de Cloudinary), termina con código 1 y un mensaje que las nombra todas.

### Variables (spec 002, plan §4.1)

| Variable | Obligatoria | Por defecto | Descripción |
|---|---|---|---|
| `APP_ENV` | No | `development` | `development`, `test`, `staging` o `production` (logs en JSON solo en `production`) |
| `APP_VERSION` | No | `dev` | Versión que muestra `/api/v1/meta` si el build no inyecta otra |
| `HTTP_ADDR` | No | `:8080` | Dirección de escucha |
| `HTTP_READ_TIMEOUT` / `HTTP_WRITE_TIMEOUT` | No | `10s` / `15s` | Duraciones de Go (`500ms`, `10s`, `1m`) |
| `SHUTDOWN_TIMEOUT` | No | `15s` | Espera máxima del cierre ordenado |
| `MONGO_URI` | **Sí** | — | Local: `mongodb://localhost:27017/?replicaSet=rs0` |
| `MONGO_DB` | **Sí** | — | Local: `vaiven_dev` |
| `MONGO_CONNECT_TIMEOUT` | No | `10s` | Tiempo máximo de la conexión inicial |
| `CORS_ALLOWED_ORIGINS` | No | vacío | Orígenes separados por comas |
| `LOG_LEVEL` | No | `info` | `debug`, `info`, `warn` o `error` |

| `CLOUDINARY_CLOUD_NAME`, `CLOUDINARY_API_KEY`, `CLOUDINARY_API_SECRET` | **Sí**, salvo con `APP_ENV=test` | — | Credenciales de Cloudinary; el secreto nunca se imprime (`***`) |
| `CLOUDINARY_BASE_FOLDER` | No | `vaiven/<APP_ENV>` | Carpeta base de las imágenes |
| `REDIS_URL` | Sí en `staging` y `production` | vacío | `redis://:<contraseña>@<host>:6379/0`; sin ella se usa la versión en memoria |
| `TLS_CERT_FILE`, `TLS_KEY_FILE` | Sí en `staging` y `production` | vacío | Certificado y clave para HTTPS; vacías en local = HTTP |

Una variable vacía equivale a no definida. Si faltan varias, el error las lista todas juntas.

### Endpoints

| Método y ruta | Respuesta |
|---|---|
| `GET /healthz` | 200 `{"status":"ok"}` mientras el proceso vive |
| `GET /readyz` | 200 `{"status":"ready"}` si MongoDB responde; 503 `service_unavailable` si no |
| `GET /api/v1/meta` | 200 con `name`, `version`, `environment` y `serverTime` (UTC) |

Contrato completo: `api/openapi.yaml` (63 operaciones en 17 grupos); resumen por módulo en **`docs/api.md`** y políticas vigentes (POL-01 a POL-17) en **`docs/politicas.md`**. Toda respuesta trae `X-Request-Id` y todo error usa `{"error":{"code","message","details","requestId"}}`.

### Comandos

| `make` | Equivalente sin `make` | Qué hace |
|---|---|---|
| `make fmt` | `gofmt -s -w .` | Formatea el código |
| `make lint` | `golangci-lint run` | Análisis estático (`.golangci.yml`) |
| `make test` | `go test ./... -race -cover` | Pruebas unitarias con cobertura |
| `make test RACE=` | `go test ./... -cover` | Igual, sin `-race` (necesita cgo; en Windows suele no haber compilador de C) |
| `make test-integration` | `go test -tags=integration ./... -race` | Pruebas con MongoDB real (Testcontainers) |
| `make build` | `go build -ldflags "-s -w -X main.version=dev" -o bin/api ./cmd/api` | Compila el binario |
| `make up` / `make down` | `docker compose up -d` / `docker compose down` | MongoDB local |
| `make lint-api` | `npx --yes @redocly/cli@1.34.5 lint api/openapi.yaml` | Valida el contrato OpenAPI (falla ante errores) |
| `make e2e` | `go test -tags=e2e -count=1 ./test/e2e/... -race` | Recorrido de punta a punta en local (Testcontainers); nunca contra staging |

Utilidades de operación (`go run ./cmd/admin <subcomando>`):

| Subcomando | Qué hace |
|---|---|
| `check-services` | Verifica MongoDB, Redis y Cloudinary (OK/FALLA por servicio) |
| `create-admin` | Crea una cuenta de administración; lee correo y contraseña de la entrada estándar |
| `indexes` | Compara los 34 índices declarados con los reales; solo lectura (rol `read`); sale con 0, 1 o 2 |
| `seed [--scale N]` | Datos de demostración idempotentes; contraseñas del archivo de `SEED_CREDENTIALS_FILE` (fuera del repo); nunca con `APP_ENV=production` |

La CI ejecuta además `go test -race`, la cobertura mínima de `domain` y `usecase` (80 %), `govulncheck` y CodeQL.

### Pruebas de humo, carga y mutación

Workflows aparte de la CI, visibles en la pestaña *Actions* (detalle en `docs/pruebas-ci.md`):

| Workflow | Cuándo | Local |
|---|---|---|
| **Smoke**: `/healthz`, `/readyz`, `/api/v1/meta` y error 404 de staging por HTTPS | a mano, diario y tras push a `develop` | `bash scripts/smoke.sh https://vaiven-staging.westus2.cloudapp.azure.com:8443` |
| **Load**: estrés y fatiga con k6, contra la API en el runner o contra staging | solo a mano | `k6 run -e BASE_URL=... -e PROFILE=smoke tests/load/api.js` |
| **Catálogo** (spec 012, T006): búsqueda y `GET /me/bookings` con datos de `seed --scale 100`; solo local, crea datos | a mano | `k6 run -e BASE_URL=http://localhost:8080 ... tests/load/catalog.js` (detalle y resultados en `docs/rendimiento.md`) |
| **Mutation**: gremlins sobre `domain`, `usecase` y `platform/security` | a mano y semanal | `bash scripts/mutation.sh` (en Linux) |

### Pruebas de integración

Levantan MongoDB 7 con Testcontainers; necesitan Docker o Podman en marcha.

- **Docker:** `make test-integration`.
- **Podman en Windows (WSL):** el puerto publicado no llega a `localhost`, así que Testcontainers debe usar la IP de la máquina de Podman:
  ```bash
  export DOCKER_HOST=npipe:////./pipe/podman-machine-default
  export TESTCONTAINERS_RYUK_DISABLED=true
  export TESTCONTAINERS_HOST_OVERRIDE=$(wsl -d podman-machine-default -- ip -4 -o addr show eth0 | tr -d '\0' | awk '{print $4}' | cut -d/ -f1)
  go test -tags=integration ./... -count=1
  ```

### Imagen de Docker

```bash
docker build --build-arg APP_VERSION=$(git rev-parse --short HEAD) -t vaiven-api .
```
Imagen `distroless` de unos 13 MB que corre como usuario no root. El despliegue en la VM (HTTPS, Compose y `scripts/deploy.sh`) está descrito en `docs/servicios.md` §7; la guía de operación (verificar, volver atrás, utilidades de `cmd/admin` en staging y problemas conocidos) está en **`docs/despliegue.md`**.

## Estado

- **Spec 001 (infraestructura en Azure):** T001–T009 cerradas; la VM, MongoDB, Redis, el certificado HTTPS y Cloudinary están en marcha (`docs/servicios.md`). Falta T010 (revisión de conformidad), que espera dos decisiones humanas.
- **Spec 002 (arquitectura base):** completa y desplegada en staging (T001–T031).
- **Spec 003 (cuenta y autenticación):** completa y desplegada; los ajustes del plan esperan el visto bueno de BE-A.
- **Spec 004 (imágenes):** completa y desplegada.
- **Spec 005 (objetos):** completa y desplegada en staging; espera el visto bueno de BE-A.
- **Specs 006 a 011** (búsqueda, reservas, chat, entrega y pagos, calificaciones e incidencias, avisos y moderación): completas, con su tarea de despliegue en staging cerrada.
- **Spec 012 (integración y entrega):** en curso. Hechas T004 (`admin indexes`), T005 (`admin seed`), T006 (rendimiento) y T007 (`make lint-api` y `make e2e`); la documentación final (T010) espera el despliegue final (T009) y las decisiones del equipo sobre la demostración.
- **Specs 013 a 015:** sin iniciar.
- **Spec 017 (DevSecOps):** CI, pruebas de humo, carga, mutación y despliegue continuo a staging listos; faltan pasos de administración (ruleset, revisores y `STAGING_AUTODEPLOY`).
- Detalle de cada tarea en `specs/<spec>/tasks.md` y decisiones en `memoria.md`.
