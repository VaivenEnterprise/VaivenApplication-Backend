# Tareas 002 — Arquitectura base del backend

- **Spec:** `spec.md` · **Plan:** `plan.md`
- **Estado:** en curso. Spec y plan aprobados el 2026-11-05; T001 y T002 hechas antes de la aprobación (ver `memoria.md`).
- Leyenda: `[P]` = puede hacerse en paralelo con otras `[P]` de la misma fase (no comparten archivos). Una tarea = un cambio pequeño y verificable.
- Agentes: `dev-go`, `modelador-mongo`, `qa-go`, `spec-writer`, `revisor-constitucion`.

## Fase 0 — Preparación

- [x] **T001** Inicializar módulo y estructura de carpetas
  - Archivos: `go.mod`, `.gitignore`, `.editorconfig`, carpetas vacías con `.gitkeep` según plan §2
  - Agente: `dev-go`
  - Depende de: —
  - Aceptación: `go mod tidy` sin error; `go build ./...` pasa (aún sin paquetes ejecutables); `.gitignore` excluye `.env`, `bin/`, `coverage.out`
  - Refs: plan §2 · Art. VI, VIII

- [x] **T002** [P] Makefile con objetivos estándar
  - Archivos: `Makefile`
  - Agente: `dev-go`
  - Depende de: T001
  - Aceptación: existen `run`, `up`, `down`, `test`, `lint`, `fmt`, `build`; `make build` inyecta `APP_VERSION` con `-ldflags`; `make test` ejecuta `go test ./... -race -cover`
  - Refs: AGENTS.md §6

- [x] **T003** [P] Entorno local de MongoDB (replica set) y plantilla de entorno
  - Archivos: `docker-compose.yml`, `.env.example`
  - Agente: `modelador-mongo`
  - Depende de: T001
  - Aceptación: `docker compose up -d` deja MongoDB 7 con replica set `rs0` en estado PRIMARY; `.env.example` lista todas las variables de plan §4.1 **sin secretos**
  - Refs: plan §4.1, §4.5 · Art. II, VI

- [x] **T004** [P] Configuración de lint
  - Archivos: `.golangci.yml`
  - Agente: `dev-go`
  - Depende de: T001
  - Aceptación: habilita al menos `errcheck`, `govet`, `staticcheck`, `revive`, `gofmt`, `gosec`; `golangci-lint run` pasa sobre el esqueleto
  - Refs: Art. VII

## Fase 1 — Fundamentos

- [x] **T005** Paquete de configuración
  - Archivos: `internal/config/config.go`, `internal/config/config_test.go`
  - Agente: `dev-go`
  - Depende de: T001
  - Aceptación: `Load()` aplica defaults de plan §4.1; falla con error que lista **todas** las variables inválidas/faltantes; pruebas de tabla cubren válido, faltante `MONGO_URI`, duración inválida, `LOG_LEVEL` inválido; cobertura ≥ 80 %
  - Refs: plan §4.1 · RF-01 · Art. VI

- [x] **T006** [P] Errores de dominio
  - Archivos: `internal/domain/errors.go`, `internal/domain/errors_test.go`
  - Agente: `dev-go`
  - Depende de: T001
  - Aceptación: tipo `Error` con `Unwrap`; constructores para los 5 códigos de dominio; `errors.As` funciona; el paquete solo importa stdlib
  - Refs: plan §4.2 · Art. III, IV

- [x] **T007** [P] Logger estructurado
  - Archivos: `internal/platform/logger/logger.go`, `internal/platform/logger/logger_test.go`
  - Agente: `dev-go`
  - Depende de: T001
  - Aceptación: `New(level, env)` devuelve `*slog.Logger` JSON en `production` y texto en otro caso; nivel inválido → error; prueba verifica el formato de salida
  - Refs: plan P-04 · Art. IX

- [x] **T008** Cliente de MongoDB
  - Archivos: `internal/platform/mongodb/client.go`, `internal/platform/mongodb/client_test.go`
  - Agente: `modelador-mongo`
  - Depende de: T005
  - Aceptación: `Connect(ctx, uri, timeout)` conecta y hace *ping*; `Ping(ctx)` con timeout de 2 s; URI inválida o sin servidor → error envuelto con `%w`; prueba unitaria de validación de parámetros (la de conexión real va en T015)
  - Refs: plan §4.5 · RF-03 · Art. II

- [x] **T009** Respuestas HTTP y mapeo de errores
  - Archivos: `internal/adapter/http/respond/respond.go`, `internal/adapter/http/respond/respond_test.go`
  - Agente: `dev-go`
  - Depende de: T006
  - Aceptación: `JSON` y `Error` según plan §4.2; `domain.Error` → 400/401/403/404/409; cualquier otro error → 500 `internal_error` con mensaje genérico; el envoltorio incluye `requestId` del contexto; pruebas de tabla por código
  - Refs: RF-08, RF-10 · Art. IV

- [x] **T010** Middleware `RequestID` y `Logging`
  - Archivos: `internal/adapter/http/middleware/requestid.go`, `logging.go`, `requestid_test.go`, `logging_test.go`
  - Agente: `dev-go`
  - Depende de: T007, T009
  - Aceptación: reutiliza `X-Request-Id` válido (≤ 64 caracteres alfanuméricos y `-`), si no genera 32 hex; lo guarda en el contexto y lo devuelve en la respuesta; el log incluye método, ruta, estado, duración y `requestId`, **sin** cuerpo ni encabezados sensibles
  - Refs: RF-07, RF-11 · Art. VI, IX

- [x] **T011** Middleware `Recover` y `CORS`
  - Archivos: `internal/adapter/http/middleware/recover.go`, `cors.go`, `recover_test.go`, `cors_test.go`
  - Agente: `dev-go`
  - Depende de: T009, T010
  - Aceptación: un *panic* produce 500 `internal_error` y queda en el log con el `requestId`; el servidor sigue vivo; CORS solo permite orígenes listados, responde preflight `OPTIONS` y no refleja orígenes no listados
  - Refs: RF-10, RF-12 · Art. VI

## Fase 2 — Núcleo HTTP

- [x] **T012** Handlers de infraestructura
  - Archivos: `internal/adapter/http/handler/health.go`, `meta.go`, `health_test.go`
  - Agente: `dev-go`
  - Depende de: T009
  - Aceptación: `Healthz`, `Readyz(pinger)` y `Meta(info)`; `Readyz` devuelve 503 `service_unavailable` si el *pinger* falla; `Meta` entrega `serverTime` en UTC formato RFC 3339; pruebas con `httptest` y *pinger* falso
  - Refs: plan §4.4 · RF-04, RF-05, RF-06

- [x] **T013** Router
  - Archivos: `internal/adapter/http/router.go`, `internal/adapter/http/router_test.go`
  - Agente: `dev-go`
  - Depende de: T010, T011, T012
  - Aceptación: monta middleware en el orden de plan §4.3 y las rutas de plan §4.4; 404 y 405 con envoltorio de error; límite de cuerpo 1 MiB; prueba de las 3 rutas + 404 + 405
  - Refs: RF-09 · Art. IV

- [x] **T014** Registro de índices
  - Archivos: `internal/adapter/mongo/indexes.go`, `internal/adapter/mongo/indexes_test.go`
  - Agente: `modelador-mongo`
  - Depende de: T008
  - Aceptación: interfaz `IndexEnsurer`; `EnsureAll(ctx, ...IndexEnsurer)` ejecuta todos, acumula errores y es idempotente; prueba unitaria con ensurers falsos (la prueba con Mongo real va en T015)
  - Refs: plan P-06 · RF-13 · Art. II

- [x] **T015** `main` con cableado y cierre ordenado
  - Archivos: `cmd/api/main.go`, `internal/usecase/doc.go`
  - Agente: `dev-go`
  - Depende de: T005, T007, T008, T013, T014
  - Aceptación: `main` carga config → logger → Mongo → `EnsureAll` → router → servidor con timeouts; `SIGTERM` cierra el servidor y Mongo y sale con 0; falta de `MONGO_URI` sale con código ≠ 0 y mensaje claro; sin lógica de negocio ni globales mutables
  - Refs: plan §4.6 · RF-01, RF-03, RF-14 · Art. III, IX

## Fase 3 — Calidad y contrato

- [x] **T016** Pruebas de integración con MongoDB real
  - Archivos: `internal/platform/mongodb/client_integration_test.go`, `internal/adapter/mongo/indexes_integration_test.go`, `internal/adapter/http/readyz_integration_test.go` (todas con `//go:build integration`)
  - Agente: `qa-go`
  - Depende de: T008, T012, T014
  - Aceptación: con Docker, `go test -tags integration ./...` pasa; cubre conexión+ping, `EnsureAll` dos veces sin error y un solo índice, `/readyz` 200 con Mongo activo y 503 tras detener el contenedor
  - Refs: plan §6 · CA-03, CA-04, CA-09 · Art. VII

- [x] **T017** [P] Prueba de arquitectura
  - Archivos: `internal/architecture_test.go`
  - Agente: `qa-go`
  - Depende de: T006
  - Aceptación: falla si `internal/domain` o `internal/usecase` dependen (directa o transitivamente) de `net/http`, `go-chi`, `mongo-driver` o `bson`; pasa en el estado actual
  - Refs: RNF-07 · Art. III

- [x] **T018** [P] Contrato OpenAPI
  - Archivos: `api/openapi.yaml`
  - Agente: `spec-writer`
  - Depende de: T013
  - Aceptación: OpenAPI 3.1 válido; describe `/healthz`, `/readyz`, `/api/v1/meta` y el esquema `Error` con los 8 códigos; ejemplos con `requestId`
  - Refs: RF-15 · Art. IV

- [x] **T019** [P] Dockerfile
  - Archivos: `Dockerfile`
  - Agente: `dev-go`
  - Depende de: T015
  - Aceptación: build multi‑etapa; binario estático; imagen final `distroless`, usuario no root; `docker build .` termina sin errores
  - Refs: plan P-09 · Art. VI

- [x] **T020** [P] Verificación local de calidad
  - Archivos: `go.mod`, `.golangci.yml` (ya previsto), `Makefile` (ajustes mínimos)
  - Agente: `dev-go`
  - Depende de: T002, T004
  - Aceptación: en local pasan `gofmt` sin cambios, `go mod tidy` sin diferencias, `go vet`, `golangci-lint run`, `go test ./... -race` con ≥ 80 % en `domain` y `usecase` y las pruebas con etiqueta `integration` (Docker); `make fmt lint test` pasa de punta a punta
  - Refs: RF-15 · H-08 · Art. VII

## Fase 4 — Cierre

- [x] **T021** README del backend
  - Archivos: `README.md`
  - Agente: `spec-writer`
  - Depende de: T015, T019
  - Aceptación: explica requisitos, cómo levantar MongoDB y la API, variables de entorno, comandos `make`, equivalentes sin `make` para Windows, y cómo ejecutar pruebas unitarias e integración; un compañero puede seguirlo sin ayuda (CA-01)
  - Refs: H-01

- [x] **T022** Actualizar memoria y verificar criterios de aceptación
  - Archivos: `memoria.md`
  - Agente: `spec-writer`
  - Depende de: T016, T017, T018, T020, T021
  - Aceptación: estado de la spec 002 actualizado; dependencias reales registradas; CA-01…CA-10 comprobados y anotados
  - Refs: Art. XI

- [x] **T023** Revisión de conformidad con la constitución
  - Archivos: ninguno (solo informe)
  - Agente: `revisor-constitucion`
  - Depende de: T022
  - Aceptación: veredicto **CONFORME** o hallazgos corregidos y revisados de nuevo
  - Refs: DoD de la constitución §7

## Fase 5 — Servicios, Redis, HTTPS y despliegue (Art. XIII y XIV)

- [x] **T024** Config modular: Cloudinary, Redis y TLS
  - Archivos: `internal/config/cloudinary.go`, `internal/config/redis.go`, `internal/config/tls.go`, `internal/config/config_services_test.go`, `internal/config/config.go`, `.env.example`
  - Agente: `dev-go` · Depende de: T005, 001-T008
  - Aceptación: `Load()` incluye `Cloudinary`, `Redis` (`REDIS_URL`) y `TLS` (`TLS_CERT_FILE`, `TLS_KEY_FILE`, obligatorios solo con `APP_ENV=staging|production`); faltantes listados todos juntos; `APP_ENV=test` y `development` no los exigen; los secretos imprimen `***`; pruebas de tabla; cobertura ≥ 80 %
  - Refs: RF-S1, RF-S2, RF-S5, RF-S7 · Art. VI, XII

- [x] **T025** [P] Cliente de ping a Cloudinary
  - Archivos: `internal/platform/cloudinary/ping.go`, `internal/platform/cloudinary/ping_test.go`
  - Agente: `dev-go` · Depende de: T024
  - Aceptación: `Ping(ctx, cfg)` llama a `/v1_1/<cloud>/ping` con autenticación básica y timeout 5 s; 200 → nil; 401 → error que **no contiene** el secreto; pruebas con `httptest`
  - Refs: RF-S3 · P-S3

- [x] **T026** Adaptador de Redis y puertos `Limiter` y `Cache`
  - Archivos: `internal/platform/redis/client.go`, `internal/platform/redis/limiter.go`, `internal/platform/redis/cache.go`, `internal/platform/redis/memory.go`, `internal/platform/redis/redis_test.go`
  - Agente: `dev-go` · Depende de: T024
  - Aceptación: puertos `ratelimit.Limiter` (`Allow(ctx, key, limit, window)`) y `cache.Cache` (`Get/Set/Delete` con TTL) declarados donde se consumen; implementación Redis (ventana fija o deslizante con script atómico) y una **en memoria**; claves con prefijo `vaiven:<APP_ENV>:`; `Ping(ctx)` para `/readyz`; solo este paquete importa `go-redis`; pruebas contra Redis real (`-tags integration`) y contra la versión en memoria; cobertura ≥ 80 %
  - Refs: RF-S5 · P-S5 · Art. III, XIV

- [x] **T027** Middleware de límite de tasa y `/readyz` con Redis
  - Archivos: `internal/adapter/http/middleware/ratelimit.go`, `internal/adapter/http/middleware/ratelimit_test.go`, `internal/adapter/http/health.go`
  - Agente: `dev-go` · Depende de: T012, T026
  - Aceptación: límite por IP (rutas públicas) y por usuario (autenticadas) con 429 en el formato de error del Art. IV; si Redis falla usa el limitador local y registra una sola vez; `/readyz` devuelve 503 solo si Mongo falla y `redis: "degradado"` (200) si Redis falla (CA-S5, CA-S6)
  - Refs: RF-05, RF-S6 · P-S5

- [x] **T028** Servidor HTTPS con certificado recargable
  - Archivos: `internal/adapter/http/tlsserver.go`, `internal/adapter/http/tlsserver_test.go`, `cmd/api/main.go`
  - Agente: `dev-go` · Depende de: T015, T024
  - Aceptación: con `APP_ENV=staging|production` la API sirve **solo HTTPS** con `TLS_CERT_FILE` y `TLS_KEY_FILE` y rechaza arrancar sin ellos; relee el par de archivos cuando cambia su fecha de modificación (≤ 1 min) y la prueba con un certificado autofirmado demuestra que el nuevo se sirve sin reiniciar (CA-S4); en `development` y `test` puede servir HTTP; cierre ordenado intacto; cobertura ≥ 80 %
  - Refs: RF-S7 · P-S8 · Art. XIV.4

- [x] **T029** Comando `check-services`
  - Archivos: `cmd/admin/main.go`, `cmd/admin/check_services.go`, `cmd/admin/check_services_test.go`
  - Agente: `dev-go` · Depende de: T024, T025, T026, T008
  - Aceptación: imprime `OK`/`FALLA` por servicio (MongoDB, Redis y Cloudinary); código ≠ 0 si alguno falla; no imprime secretos; prueba con dobles (CA-S1, CA-S2)
  - Refs: RF-S3

- [x] **T030** Artefactos de despliegue: Compose y `deploy.sh`
  - Archivos: `deploy/docker-compose.yml`, `deploy/.env.example`, `scripts/deploy.sh`
  - Agente: `dev-go` · Depende de: T019, T028, 001-T007
  - Aceptación: Compose con `api`, `mongo` (`rs0`) y `redis`, límites de memoria y `GOMEMLIMIT`; solo la API publica puerto (8443 en staging, 443 en producción) y monta de solo lectura el certificado; `deploy.sh <entorno>` compila para la arquitectura de la VM (`amd64`/`arm64`), envía la imagen por SSH, reinicia la API y espera su `/healthz` por HTTPS; `--rollback` vuelve a la imagen anterior; ningún secreto escrito; `shellcheck` sin errores
  - Refs: RF-S8 · P-S6, P-S7 · Art. XIII, XIV

- [x] **T031** Despliegue de la spec en Azure (staging)
  - Archivos: `docs/servicios.md` (anexar la URL de staging), `memoria.md` (solo anexar)
  - Agente: `devops-mcp` · Depende de: T019, T020, T023, T029, T030, 001-T010
  - Aceptación: `make fmt lint test` en verde; `.env` de staging completo en la VM (nunca en el repo); `scripts/deploy.sh staging` termina bien; `/healthz`, `/readyz` y `/api/v1/meta` devuelven 200 en `https://<host>:8443` **sin `-k`** (CA-S3); no hay respuesta HTTP en claro (CA-S4); el límite de tasa responde 429 al pasarse (CA-S6); con Redis detenido la API responde y `/readyz` marca `degradado` (CA-S5); registros sin secretos ni datos personales; URL anotada para el móvil. **La spec 003 no empieza hasta que esta tarea pase.**
  - Refs: RF-S4 · Art. XIII.5

## Resumen

| Fase | Tareas | Total |
|---|---|---|
| 0 Preparación | T001–T004 | 4 |
| 1 Fundamentos | T005–T011 | 7 |
| 2 Núcleo HTTP | T012–T015 | 4 |
| 3 Calidad y contrato | T016–T020 | 5 |
| 4 Cierre | T021–T023 | 3 |
| 5 Servicios, Redis, HTTPS y despliegue | T024–T031 | 8 |
| **Total** | | **31** |
