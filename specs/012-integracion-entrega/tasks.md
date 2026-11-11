# Tareas 012 — Integración, endurecimiento y entrega (backend)

- **Dueños:** indicados por tarea · **Estado:** sin ejecutar · `[P]` = paralelizable
- Esta spec **no cambia contratos**; los hallazgos se corrigen en el módulo y por su dueño.
- Reparto: **BE-A** T001, T002, T003, T008, T011 · **BE-B** T004, T005, T006, T007, T009, T010 · T012 según su tarea.

- [ ] **T001** Cliente y utilidades de prueba E2E
  - Archivos: `test/e2e/helpers_test.go`, `test/e2e/fake_cloudinary_test.go`
  - Dueño: BE-A · Agente: `qa-go` · Depende de: 001–011 desplegadas
  - Aceptación: levanta router real + Mongo (Testcontainers) + Cloudinary falso, sin servicios remotos; helpers `register`, `login`, `do`; reloj ajustable
  - Refs: P-01…P-03

- [ ] **T002** Recorrido E2E principal
  - Archivos: `test/e2e/journey_test.go`
  - Dueño: BE-A · Agente: `qa-go` · Depende de: T001
  - Aceptación: RF-01 completo (registro→publicar→buscar→cotizar→solicitar→aceptar→chat→entrega con QR→pagos→devolución con QR→calificar→incidencia); pasa con `-tags e2e` en local (CA-01)
  - Refs: RF-01

- [ ] **T003** [P] Cabeceras de seguridad y límite de cuerpo
  - Archivos: `internal/adapter/http/middleware/{security_headers.go, security_headers_test.go}`, `internal/adapter/http/router.go` (una línea), `internal/<módulo>/adapter/http/{dto.go|handler.go}` donde falte distinguir el cuerpo demasiado grande (coordinado con el dueño de cada módulo)
  - Dueño: BE-A · Agente: `dev-go` · Depende de: 002-T013
  - Aceptación: `nosniff` en toda respuesta; `no-store` en respuestas autenticadas, sin duplicar el de `handover`; un cuerpo de más de 1 MiB responde 400 `validation_failed` «El cuerpo de la petición es demasiado grande.» en **todos** los módulos con cuerpo, sin códigos nuevos en el contrato (CA-05); probado en el límite exacto, con 1 byte de más y con cuerpo `chunked` (CA-08); no se crea otro limitador (el existente se documenta, P-07)
  - Refs: RF-04 · P-07, P-09, P-10

- [x] **T004** [P] `Indexes()` como fuente única de verdad y comando `indexes`
  - Archivos:
    - `cmd/admin/testdata/indexes_baseline.json` (**primero**, con el código actual, en una base limpia)
    - `internal/adapter/mongo/{indexes.go, indexes_test.go, indexes_compare.go, indexes_compare_test.go}`
    - `internal/adapter/mongo/catalog/{catalog.go, catalog_test.go, purpose_test.go, catalog_integration_test.go}` (nuevo)
    - Repositorios de BE-B (9): `search/search_indexes.go`, `chat/conversation_repo.go`, `chat/message_repo.go`, `ratings/rating_repo.go`, `incidents/incident_repo.go`, `sanctions/sanction_repo.go`, `notifications/notification_repo.go`, `notifications/token_repo.go`, `reports/report_repo.go`
    - Repositorios de BE-A (5): `auth/refresh_repo.go`, `auth/user_repo.go`, `items/item_repo.go`, `bookings/booking_repo.go`, `payments/payment_repo.go` (**con aprobación formal de BE-A** o su autorización escrita a BE-B)
    - `cmd/admin/{indexes.go, indexes_test.go, indexes_integration_test.go, indexes_command_integration_test.go, ensurers_test.go, main.go}`; `check_services_test.go` y `create_admin_test.go` solo ajustan la llamada a `run` (un parámetro más)
  - Dueño: BE-B (BE-A aprobó sus 5 archivos el 2026-11-10, comunicado por BE-B) · Agente: `modelador-mongo` · Depende de: 002-T014
  - Aceptación:
    - los 14 repositorios declaran sus 34 índices como datos (`Indexes()`), `EnsureIndexes` consume esa declaración y `catalog.All()` las reúne todas; no se acepta un catálogo parcial (plan §7.1–7.3);
    - el comando es de solo lectura, funciona con rol `read`, clasifica según plan §7.4, normaliza según §7.5 y sale con 0, 1 o 2; nunca muestra credenciales; `os.Exit` solo en `main`, tras cerrar la conexión;
    - las 15 pruebas del plan §7.6 pasan, incluida la equivalencia contra `indexes_baseline.json` (CA-03, CA-10).
  - Refs: RF-03 · P-05 · plan §7

- [x] **T005** Comando `seed`
  - Archivos: `cmd/admin/{seed.go, seed_data.go, seed_upload.go, seed_deps.go, seed_test.go, seed_integration_test.go}`, `cmd/admin/assets/*.jpg`, `cmd/admin/main.go` (subcomando), `cmd/admin/create_admin.go` (la creación de la cuenta admin pasa a `newAdminAccount`, compartida con el seed)
  - Dueño: BE-B · Agente: `dev-go` · Depende de: 002-T014 (y los módulos 003–011 ya desplegados); la rama se apila sobre T004 (comparten `cmd/admin/main.go` y usa `catalog.All()` para los índices)
  - Aceptación: crea 6 personas, 15 objetos con fotos (≤ 100 KB), 3 reservas en estados distintos y 1 admin con la lógica de `create-admin`; usa casos de uso reales; credenciales por `SEED_CREDENTIALS_FILE` (P-12), distintas por cuenta y nunca impresas; rechaza `APP_ENV=production`; idempotente sin sobrescribir (P-13, CA-02); fotos por firma + `Resolve` (P-14), sin archivos temporales y borrando de Cloudinary lo subido si algo falla después (CA-09); opción `--scale N` para el conjunto de rendimiento, que también se niega fuera de `development` y `test`
  - Refs: RF-02 · P-04, P-12…P-14

- [x] **T006** [P] Rendimiento y `explain`
  - Archivos: `tests/load/catalog.js` (nuevo escenario junto al k6 existente), `docs/rendimiento.md` (no se comparte `docs/seguridad.md`, que es de T008)
  - Cierre (2026-11-11): p95 de 13,2 ms en búsqueda y 6,7 ms en `GET /me/bookings` con `seed --scale 100` (0 errores, 0 respuestas 429); las 10 consultas más frecuentes, tomadas del profiler de MongoDB, usan índice y ninguna hace `COLLSCAN`; sin optimizaciones. Queda documentado el riesgo de que la búsqueda reciente recorra todos los objetos activos (el `$lookup` del dueño va antes del `$limit`). Los límites de staging no se pudieron aplicar en Podman/WSL; confirmar en la VM en T009.
  - Dueño: BE-B · Agente: `qa-go` · Depende de: T005
  - Aceptación: con `seed --scale 100` en MongoDB local, p95 < 500 ms en `GET /items` y `GET /me/bookings`; se anotan p50, p95, p99, CPU y memoria de la API y de MongoDB, y el `explain` de las 10 consultas más frecuentes (sin `COLLSCAN`); toda optimización registra cifras de antes y después; medición reproducible con escenarios representativos (CA-07)
  - Refs: RF-08 · P-15

- [x] **T007** [P] Validación OpenAPI y E2E en local
  - Archivos: `Makefile` (objetivos `lint-api` y `e2e`)
  - Cierre (2026-11-11): `lint-api` usa el mismo Redocly fijo del CI (1.34.5); el contrato actual es válido (2 avisos, 0 errores) y un contrato roto falla. `e2e` termina con error y un mensaje claro mientras no exista `test/e2e`; correrá el recorrido cuando BE-A cierre T001 y T002.
  - Dueño: BE-B · Agente: `dev-go` · Depende de: 002-T020 (y T002 para que `make e2e` tenga qué correr)
  - Aceptación: `make lint-api` (`npx @redocly/cli lint api/openapi.yaml`) falla ante errores; `make e2e` ejecuta `go test -tags e2e ./test/e2e/...` en local con Testcontainers y **no** apunta a staging; las pruebas de humo de staging siguen en `smoke.yml`
  - Refs: RF-06 · P-01, P-06

- [ ] **T008** Revisión de seguridad
  - Archivos: `docs/seguridad.md`, `internal/auth/adapter/http/middleware.go` (+ test), `scripts/scan-archives.py`, `scripts/scan_archives_test.py`, `docs/seguridad/remediacion-2026-11.md`
  - Dueño: BE-A · Agente: `revisor-constitucion` (revisión) y `dev-go` (correcciones) · Depende de: T003
  - Aceptación: lista de comprobación del RF-05 completa y fechada; `RequireRole` comprueba el rol en la base (AUD-017, CA-06); el escáner detecta comprimidos renombrados en el historial y falla ante formatos no soportados (AUD-015); documento de remediación corregido (AUD-018); búsqueda de secretos en el historial con gitleaks y el escáner; ningún hallazgo fuera del RF-05 se agrega a esta tarea
  - Refs: RF-05 · P-08, P-11

- [ ] **T009** Despliegue final en staging
  - Archivos: `docs/despliegue.md`
  - Dueño: BE-B · Agente: `devops-mcp` · Depende de: T004, T005, T007
  - Aceptación: versión final desplegada; `check-services`, `indexes` y `seed` ejecutados (el archivo de credenciales vive solo en quien ejecuta); `/readyz` 200; pasos documentados (RF-07)
  - Refs: RF-07

- [ ] **T010** [P] Documentación final
  - Archivos: `README.md`, `docs/api.md`, `docs/politicas.md`, `docs/demo.md`, `docs/validacion.md`
  - Dueño: BE-B · Agente: `spec-writer` · Depende de: T009
  - Aceptación: RF-09 y RF-10; `demo.md` (parte del borrador del 2026-11-09) seguido por una persona ajena al backend sin ayuda (CA-04) y sin contraseñas; presentador principal confirmado por el equipo; `politicas.md` lista el valor vigente de cada POL-xx
  - Refs: RF-09, RF-10

- [ ] **T011** Revisión final y congelamiento
  - Archivos: `memoria.md` (solo anexar)
  - Dueño: BE-A · Agente: `revisor-constitucion` · Depende de: T006, T008, T010
  - Aceptación: veredicto **CONFORME** sobre todo el repositorio; `make fmt lint test` + integración + E2E pasan; cobertura global ≥ 75 % sin pruebas artificiales (RNF-01); código congelado el 22 de nov
  - Refs: RNF-01, RNF-02

- [ ] **T012** Despliegue de la spec en Azure (staging)
  - Archivos: `memoria.md` (solo anexar)
  - Agente: `devops-mcp` · Depende de: T011 (y todas las anteriores `[x]`)
  - Aceptación: `make fmt lint test` en verde; `scripts/deploy.sh staging` (construye, envía a la VM y reinicia la API); `/healthz` y `/readyz` devuelven 200 por HTTPS sin `-k`; cada endpoint nuevo de esta spec (`api/paths/<modulo>.yaml`) responde con una petición correcta y una inválida en el formato de error del Art. IV; registros sin secretos ni datos personales; resultado anotado en `memoria.md`. **La spec siguiente no empieza hasta que esta tarea pase.**
  - Refs: Art. XIII.5

## Trazabilidad
| Requisito | Tarea |
|---|---|
| RF-01 | T001, T002 |
| RF-02 | T005 |
| RF-03 | T004 |
| RF-04 | T003 |
| RF-05 | T008 |
| RF-06 | T007 |
| RF-07 | T009 |
| RF-08 | T006 |
| RF-09, RF-10 | T010 |
| RNF-01, RNF-02 | T011 |

| Total | 12 tareas |
|---|---|
