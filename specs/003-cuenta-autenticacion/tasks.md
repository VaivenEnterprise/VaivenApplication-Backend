# Tareas 003 — Cuenta y autenticación (backend)

- **Dueño:** BE-A · **Estado:** sin ejecutar · `[P]` = paralelizable
- **T001 va primero (contrato publicado el mismo día)**, para que MO-A y los demás lean el contrato.

- [x] **T001** Contrato OpenAPI de auth y usuarios
  - Archivos: `api/paths/auth.yaml`, `api/paths/users.yaml`, `api/openapi.yaml` (agregar `$ref`)
  - Agente: `spec-writer` · Depende de: 002-T018
  - Aceptación: describe las 8 rutas del plan §2 con esquemas `User`, `PublicUser`, `Tokens` y errores; YAML válido
  - Refs: Art. IV, XII

- [x] **T002** [P] Registro de módulos y puertos vacíos
  - Archivos: `cmd/api/modules.go`, `internal/notify/notifier.go`, `internal/moderation/checker.go`
  - Agente: `dev-go` · Depende de: 002-T015
  - Aceptación: `Module` con `Name()` y `Register(chi.Router)`; `main` recorre `modules`; `Notifier` y `Checker` con implementación vacía y pruebas de que no fallan
  - Refs: RF-11, RF-12 · Art. XII

- [x] **T003** [P] Seguridad: contraseñas y tokens
  - Archivos: `internal/platform/security/password.go`, `jwt.go`, `tokens.go`, `security_test.go`
  - Agente: `dev-go` · Depende de: 002-T005
  - Aceptación: `argon2id` con parámetros del plan P-01 (hash/verify, formato PHC); JWT HS256 con expiración y rechazo de otro algoritmo; generador de refresh + SHA-256; pruebas de tabla
  - Refs: RF-03, RF-05 · Art. VI

- [x] **T004** [P] Configuración de auth
  - Archivos: `internal/config/auth.go`, `internal/config/auth_test.go`, `.env.example`
  - Agente: `dev-go` · Depende de: 002-T024
  - Aceptación: variables del plan §5; `JWT_ACCESS_SECRET` < 32 caracteres → error; `Load()` con una línea nueva
  - Refs: P-02 · Art. VI

- [x] **T005** Dominio y repositorio de usuarios
  - Archivos: `internal/auth/domain/user.go`, `internal/auth/domain/errors.go`, `internal/auth/adapter/mongo/user_repo.go`, `internal/auth/adapter/mongo/user_repo_integration_test.go`
  - Agente: `modelador-mongo` · Depende de: T003, 002-T014
  - Aceptación: colección `users` con índice único de correo (`EnsureIndexes`); creación y búsqueda por correo/id; duplicado → error de dominio `conflict`; prueba de integración
  - Refs: plan §3 · Art. II

- [x] **T006** Repositorio de refresh tokens
  - Archivos: `internal/auth/adapter/mongo/refresh_repo.go`, `internal/auth/adapter/mongo/refresh_repo_integration_test.go`
  - Agente: `modelador-mongo` · Depende de: T005
  - Aceptación: índices único de hash, por familia y TTL; marcar usado, revocar familia, buscar por hash; prueba de integración
  - Refs: P-03, P-04

- [x] **T007** Casos de uso: registro y login
  - Archivos: `internal/auth/usecase/ports.go`, `register.go`, `login.go`, `auth_usecase_test.go`
  - Agente: `dev-go` · Depende de: T005, T006
  - Aceptación: validaciones de POL-12; correo normalizado; login con error genérico; emite tokens; pruebas con repos falsos; ≥ 80 %
  - Refs: RF-01…RF-04 · Art. III

- [x] **T008** Casos de uso: refresh, logout, perfil y baja
  - Archivos: `internal/auth/usecase/refresh.go`, `logout.go`, `profile.go`, `deactivate.go`, `session_usecase_test.go`
  - Agente: `dev-go` · Depende de: T007
  - Aceptación: rotación y detección de reutilización; baja bloqueada si `ActiveBookingChecker` devuelve true; la baja **anonimiza** correo (se sustituye por un valor único no enrutable), nombre («Cuenta eliminada») y teléfono, y revoca todas las familias de refresh tokens (prueba: tras la baja el correo original puede volver a registrarse); perfil público con «Cuenta eliminada»; expone `users.Directory` con `PublicUsers(ctx, ids) → map[id]PublicUser` (lectura por lote que usarán 005, 006, 007, 008 y 010) y `users.Admin` con `Suspend(ctx, userID, until)` y `AddRating(ctx, userID, score)` (`$inc` de `ratingSum`/`ratingCount`; los usan 010 y 011), todo con implementación en Mongo; pruebas
  - Refs: RF-05, RF-06, RF-08, RF-09 · POL-17

- [x] **T009** Límite de tasa
  - Archivos: `internal/platform/ratelimit/limiter.go`, `internal/platform/ratelimit/limiter_test.go`
  - Agente: `dev-go` · Depende de: 002-T011
  - Aceptación: middleware por IP, ventana de 1 min, 429 `rate_limited` con envoltorio; probado con reloj inyectado
  - Refs: RF-07

- [x] **T010** Handlers, middleware de autenticación y rutas
  - Archivos: `internal/auth/adapter/http/{handler.go, dto.go, middleware.go, routes.go, handler_test.go}`
  - Agente: `dev-go` · Depende de: T001, T002, T008, T009
  - Aceptación: las 8 rutas cumplen el contrato; `Authenticate`, `RequireRole`, `RequireActive`; `auth.UserID(ctx)`; el módulo se registra con **una línea** en `modules.go`; pruebas `httptest` por ruta y por código de error
  - Refs: RF-10, RF-13 · Art. IV, XII

- [ ] **T011** [P] Comando `create-admin`
  - Archivos: `cmd/admin/create_admin.go`, `cmd/admin/create_admin_test.go`
  - Agente: `dev-go` · Depende de: T005, 002-T029
  - Aceptación: crea usuario `admin` leyendo correo y contraseña por entrada estándar; no acepta la contraseña por argumento; idempotente (existe → mensaje y código ≠ 0)
  - Refs: RF-10

- [ ] **T012** Pruebas de integración del flujo de sesión
  - Archivos: `internal/auth/adapter/http/flow_integration_test.go`
  - Agente: `qa-go` · Depende de: T010
  - Aceptación: registro→`/me`→refresh→reutilización revoca familia→logout; CA-01…CA-07 cubiertos (con `-tags integration`)
  - Refs: CA-01…CA-07

- [ ] **T013** Memoria, dependencias y revisión
  - Archivos: `memoria.md` (solo anexar)
  - Agente: `revisor-constitucion` · Depende de: T012
  - Aceptación: dependencias registradas; veredicto **CONFORME**; hito H1 (backend) marcado
  - Refs: DoD

- [ ] **T014** Despliegue de la spec en Azure (staging)
  - Archivos: `memoria.md` (solo anexar)
  - Agente: `devops-mcp` · Depende de: T013 (y todas las anteriores `[x]`)
  - Aceptación: `make fmt lint test` en verde; `scripts/deploy.sh staging` (construye, envía a la VM y reinicia la API); `/healthz` y `/readyz` devuelven 200 por HTTPS sin `-k`; cada endpoint nuevo de esta spec (`api/paths/<modulo>.yaml`) responde con una petición correcta y una inválida en el formato de error del Art. IV; registros sin secretos ni datos personales; resultado anotado en `memoria.md`. **La spec siguiente no empieza hasta que esta tarea pase.**
  - Refs: Art. XIII.5

| Total | 14 tareas |
|---|---|
