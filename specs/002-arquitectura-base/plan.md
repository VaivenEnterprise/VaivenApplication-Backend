# Plan 002 — Arquitectura base del backend

- **Spec:** `spec.md` (mismo directorio)
- **Constitución:** v1.0.0
- **Estado:** Borrador, pendiente de aprobación humana

## 1. Enfoque

Construir el esqueleto en capas de afuera hacia adentro solo en lo imprescindible: configuración → infraestructura (logger, Mongo) → núcleo HTTP (errores, middleware, router) → endpoints de infraestructura → registro de índices → pruebas, contrato y automatización. No se crean entidades ni casos de uso de negocio; se deja la carpeta `domain`/`usecase` con la base de errores y el contrato de puertos de ejemplo mínimo.

## 2. Estructura de archivos a crear

```
vaiven-backend/
├─ cmd/api/main.go
├─ internal/
│  ├─ config/{config.go, config_test.go}
│  ├─ domain/{errors.go, errors_test.go}
│  ├─ usecase/doc.go
│  ├─ adapter/
│  │  ├─ http/
│  │  │  ├─ router.go
│  │  │  ├─ respond/{respond.go, respond_test.go}
│  │  │  ├─ middleware/{requestid.go, logging.go, recover.go, cors.go, *_test.go}
│  │  │  └─ handler/{health.go, meta.go, health_test.go}
│  │  └─ mongo/{indexes.go, indexes_test.go}
│  └─ platform/
│     ├─ logger/{logger.go, logger_test.go}
│     └─ mongodb/{client.go, client_test.go}
├─ api/openapi.yaml
├─ docker-compose.yml · Dockerfile · Makefile
├─ .env.example · .gitignore · .golangci.yml · .editorconfig
└─ README.md
```

## 3. Decisiones técnicas

| # | Decisión | Motivo | Art. |
|---|---|---|---|
| P-01 | Módulo Go `github.com/vaiven/backend` (provisional) | Pregunta abierta; se ajusta antes de T001 | VIII |
| P-02 | Router `chi/v5` con `middleware` propios | Ligero y estándar | III, VIII |
| P-03 | Configuración con `os.LookupEnv` + validación propia (sin biblioteca) | Menos dependencias | VIII |
| P-04 | `log/slog`; JSON si `APP_ENV=production`, texto en otro caso | Observabilidad estándar | IX |
| P-05 | `domain.Error{Code, Message, Details, Err}` y traducción única en `respond.Error` | Un solo punto de mapeo HTTP | III, IV |
| P-06 | Registro de índices: interfaz `IndexEnsurer{ EnsureIndexes(ctx) error }`; `indexes.go` guarda una lista y `EnsureAll` las ejecuta | Extensible sin tocar `main` por cada repo | II |
| P-07 | `requestId` con `crypto/rand` (16 bytes hex) | Sin dependencia extra | VIII |
| P-08 | Pruebas de integración etiquetadas `//go:build integration` con testcontainers | Aísla las pruebas lentas | VII |
| P-09 | Dockerfile multi‑etapa (`golang` → `gcr.io/distroless/static`) | Imagen pequeña y segura | VI |

## 4. Diseño por componente

### 4.1 Configuración (`internal/config`)
| Variable | Obligatoria | Por defecto | Descripción |
|---|---|---|---|
| `APP_ENV` | No | `development` | `development` · `test` · `production` |
| `APP_VERSION` | No | `dev` | Inyectada con `-ldflags` en el build |
| `HTTP_ADDR` | No | `:8080` | Dirección de escucha |
| `HTTP_READ_TIMEOUT` | No | `10s` | Duración Go |
| `HTTP_WRITE_TIMEOUT` | No | `15s` | Duración Go |
| `SHUTDOWN_TIMEOUT` | No | `15s` | Espera de cierre ordenado |
| `MONGO_URI` | **Sí** | — | Cadena de conexión |
| `MONGO_DB` | **Sí** | — | Nombre de la base |
| `MONGO_CONNECT_TIMEOUT` | No | `10s` | Tiempo de conexión inicial |
| `CORS_ALLOWED_ORIGINS` | No | vacío | Lista separada por comas |
| `LOG_LEVEL` | No | `info` | `debug` · `info` · `warn` · `error` |

`config.Load()` devuelve `(Config, error)`; el error lista **todas** las variables inválidas, no solo la primera.

### 4.2 Errores y respuestas
- `domain.Error` con constructores `NewValidation`, `NewNotFound`, `NewConflict`, `NewUnauthorized`, `NewForbidden`.
- `respond.JSON(w, status, v)` y `respond.Error(w, r, err)`: mapea `domain.Code` → HTTP (400/401/403/404/409; otros → 500 `internal_error`) y escribe el envoltorio con `requestId` tomado del contexto.
- Los mensajes de `internal_error` son genéricos: "Ocurrió un error inesperado. Intenta de nuevo."

### 4.3 Middleware (orden de ejecución)
1. `RequestID` → 2. `Logging` → 3. `Recover` → 4. `CORS` → 5. `http.MaxBytesReader` global (1 MiB) → handlers.

### 4.4 Rutas
| Método | Ruta | Respuesta |
|---|---|---|
| GET | `/healthz` | `200 {"status":"ok"}` |
| GET | `/readyz` | `200 {"status":"ready"}` · `503` con envoltorio de error |
| GET | `/api/v1/meta` | `200 {"name":"vaiven-backend","version":"…","environment":"…","serverTime":"…Z"}` |
| * | otras | `404`/`405` con envoltorio de error |

### 4.5 Datos (MongoDB)
- Sin colecciones de negocio en esta spec.
- `mongodb.Connect(ctx, uri)` con *ping*; `mongodb.Ping(ctx)` para `/readyz` con timeout de 2 s.
- `mongo.EnsureAll(ctx, ensurers ...IndexEnsurer)` se invoca en `main` tras conectar; lista vacía en esta spec.
- `docker-compose.yml`: imagen `mongo:7`, comando `--replSet rs0 --bind_ip_all`, *healthcheck* que ejecuta `rs.initiate()` si hace falta, volumen con nombre, puerto `27017`.

### 4.6 Cierre ordenado
`signal.NotifyContext` (SIGINT, SIGTERM) → `server.Shutdown(ctx con SHUTDOWN_TIMEOUT)` → `client.Disconnect(ctx)` → código de salida 0.

## 5. Dependencias (registradas también en `memoria.md`)

| Módulo | Uso |
|---|---|
| `github.com/go-chi/chi/v5` | Router |
| `go.mongodb.org/mongo-driver/v2` | MongoDB |
| `github.com/stretchr/testify` | Pruebas |
| `github.com/testcontainers/testcontainers-go` y `.../modules/mongodb` | Pruebas de integración |

`github.com/go-playground/validator/v10` forma parte del stack fijo de la constitución, pero **no se agrega en esta spec** (Art. VIII: no hay entrada de usuario que validar todavía). Se incorpora en la primera spec funcional.

## 6. Estrategia de pruebas

| Nivel | Qué | Herramienta |
|---|---|---|
| Unitaria | `config` (tabla de casos válidos/ inválidos), `domain.Error`, `respond`, cada middleware, handlers con `httptest` y un *pinger* falso | `testing` + `testify` |
| Integración | Conexión y `Ping` reales; `EnsureAll` idempotente con un repo de prueba; `/readyz` contra Mongo real | testcontainers (`-tags integration`) |
| Arquitectura | Prueba que recorre imports de `domain` y `usecase` y falla si aparece `net/http`, `chi`, `mongo` o `bson` | `go list -deps` en un test |
| Calidad | `gofmt`, `go vet`, `golangci-lint` | Makefile (local) |

## 7. Verificación de constitución

| Art. | ¿Cumple? | Cómo |
|---|---|---|
| I | Sí | Spec → plan → tasks antes de código |
| II | Sí | Solo MongoDB v2, replica set, índices en código |
| III | Sí | Capas definidas; prueba de arquitectura (RNF-07) |
| IV | Sí | Envoltorio de error, `/api/v1`, `openapi.yaml` |
| V | N/A | Sin dinero en esta spec; reglas heredadas |
| VI | Sí | Env vars, CORS, límites, sin PII en logs |
| VII | Sí | Cobertura ≥ 80 %, lint |
| VIII | Sí | Solo 4 dependencias justificadas; `validator` se difiere |
| IX | Sí | slog, requestId, healthz/readyz, graceful shutdown |
| X | Sí | Tareas pequeñas con agente asignado |
| XI | Sí | Cambios de principios pasan por `memoria.md` |

## 8. Fases

1. **Fase 0 — Preparación:** módulo, carpetas, herramientas, MongoDB local.
2. **Fase 1 — Fundamentos:** config, logger, Mongo, errores, respuestas, middleware.
3. **Fase 2 — Núcleo HTTP:** handlers, router, `main`, cierre ordenado, índices.
4. **Fase 3 — Calidad y contrato:** pruebas de integración y arquitectura, OpenAPI, Docker, verificación local.
5. **Fase 4 — Cierre:** README, memoria, revisión de constitución.

## 9. Riesgos

| Riesgo | Mitigación |
|---|---|
| El modelo gratuito alucina la API del driver v2 | La skill `mongodb-go-patterns` fija imports; exigir `go build` y `go doc` ante dudas |
| `make` no existe en Windows | Documentar Git Bash/WSL y comandos equivalentes en el README |
| Replica set local falla al iniciar | Healthcheck con `rs.initiate()` idempotente y documentación de diagnóstico |
| Docker no disponible para pruebas de integración | Etiqueta `integration`; las unitarias siguen pasando sin Docker |

## 9. Servicios externos y despliegue (movido desde la antigua spec de servicios)
Archivos: `internal/config/{cloudinary.go, cloudinary_test.go}` · `cmd/admin/{main.go, check_services.go, check_services_test.go}` · `internal/platform/cloudinary/{ping.go, ping_test.go}` · `.env.example` (modificar).

| # | Decisión | Art. |
|---|---|---|
| P-S1 | `config.Load()` llama a `loadCloudinary(getenv)`; futuras specs agregan su `loadX` en su propio archivo y **una línea** en `Load()` | XII |
| P-S2 | `Cloudinary` expone `CloudName`, `APIKey`, `APISecret`, `BaseFolder`; el secreto es un tipo `Secret` con `String() = "***"` | VI |
| P-S3 | Ping a Cloudinary con `net/http`, `http.NewRequestWithContext`, autenticación básica y timeout de 5 s; `cmd/admin` usa subcomandos con `os.Args` (sin librería) | VIII |
| P-S4 | Staging: el `Dockerfile` (T019) se construye localmente y se envía a la VM de Azure con `scripts/deploy.sh` (`devops-mcp`); las variables viven solo en el `.env` de la VM, nunca en el repositorio | VI, XIII |
| P-S5 | Redis detrás de puertos (`ratelimit.Limiter`, `cache.Cache`); implementaciones Redis y en memoria; el middleware de límite de tasa degrada al limitador local si Redis falla | III, XIV |
| P-S6 | `deploy/docker-compose.yml` parametrizado por entorno (`-p vaiven-<entorno>`): `api`, `mongo` y `redis`; solo la API publica puerto y monta el certificado (`/etc/letsencrypt/live/<host>` de solo lectura); Mongo y Redis nunca publican puertos; límites de memoria y `GOMEMLIMIT` | XIII, XIV |
| P-S7 | `deploy.sh` compila con `GOOS=linux GOARCH=<arquitectura de la VM>`, envía la imagen por `docker save \| ssh … docker load`, reinicia la API y espera `/healthz`; con `--rollback` vuelve a la imagen etiquetada anterior | XIII.5 |
| P-S8 | Servidor HTTPS: `http.Server` con `TLSConfig.GetCertificate` que relee el par de archivos cuando cambia su fecha de modificación (comprobación cada ≤ 1 min); sin dependencias nuevas | VIII, XIV.4 |

Variables nuevas (se suman a las anteriores): `REDIS_URL`, `TLS_CERT_FILE`, `TLS_KEY_FILE` (obligatorias en `staging` y `production`), `CLOUDINARY_CLOUD_NAME`, `CLOUDINARY_API_KEY`, `CLOUDINARY_API_SECRET` (secreto), `CLOUDINARY_BASE_FOLDER` (opcional). Pruebas: unitarias de `config` (válido, faltantes, `test`), `ping` con `httptest`, `check-services` con dobles.
