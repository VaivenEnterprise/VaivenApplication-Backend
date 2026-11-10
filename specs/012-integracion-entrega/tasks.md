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

- [ ] **T004** [P] Comando `indexes`
  - Archivos: `cmd/admin/indexes.go`, `cmd/admin/indexes_test.go`, `cmd/admin/main.go` (subcomando)
  - Dueño: BE-B · Agente: `modelador-mongo` · Depende de: 002-T014
  - Aceptación: lista índices faltantes y sobrantes por colección; solo lee; código ≠ 0 si faltan; probado con una colección sin un índice (CA-03); pruebas de integración que muestran la utilidad de cada índice (`explain` con `IXSCAN` para su consulta) y que fallan si un índice es prefijo redundante de otro (CA-10)
  - Refs: RF-03 · P-05

- [ ] **T005** Comando `seed`
  - Archivos: `cmd/admin/{seed.go, seed_data.go, seed_upload.go, seed_test.go}`, `cmd/admin/assets/*.jpg`, `cmd/admin/main.go` (subcomando)
  - Dueño: BE-B · Agente: `dev-go` · Depende de: 002-T014 (y los módulos 003–011 ya desplegados)
  - Aceptación: crea 6 personas, 15 objetos con fotos (≤ 100 KB), 3 reservas en estados distintos y 1 admin con la lógica de `create-admin`; usa casos de uso reales; credenciales por `SEED_CREDENTIALS_FILE` (P-12), distintas por cuenta y nunca impresas; rechaza `APP_ENV=production`; idempotente sin sobrescribir (P-13, CA-02); fotos por firma + `Resolve` (P-14), sin archivos temporales y borrando de Cloudinary lo subido si algo falla después (CA-09); opción `--scale N` para el conjunto de rendimiento, que también se niega fuera de `development` y `test`
  - Refs: RF-02 · P-04, P-12…P-14

- [ ] **T006** [P] Rendimiento y `explain`
  - Archivos: `tests/load/` (extender el k6 existente), `docs/rendimiento.md` (no se comparte `docs/seguridad.md`, que es de T008)
  - Dueño: BE-B · Agente: `qa-go` · Depende de: T005
  - Aceptación: con `seed --scale 100` en MongoDB local, p95 < 500 ms en `GET /items` y `GET /me/bookings`; se anotan p50, p95, p99, CPU y memoria de la API y de MongoDB, y el `explain` de las 10 consultas más frecuentes (sin `COLLSCAN`); toda optimización registra cifras de antes y después; medición reproducible con escenarios representativos (CA-07)
  - Refs: RF-08 · P-15

- [ ] **T007** [P] Validación OpenAPI y E2E en local
  - Archivos: `Makefile` (objetivos `lint-api` y `e2e`)
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
