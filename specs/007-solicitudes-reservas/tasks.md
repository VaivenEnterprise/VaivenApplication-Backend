# Tareas 007 — Solicitudes, precio y reservas (backend)

- **Dueño:** BE-A · **Estado:** T001–T012 hechas (BE-B a pedido del responsable; archivos consolidados según plan §8 A-07) · `[P]` = paralelizable
- **T001 se integra el primer día** para MO-A y para BE-B (008 y 006 leen `bookings`).

- [x] **T001** Contrato OpenAPI de reservas
  - Archivos: `api/paths/bookings.yaml`, `api/openapi.yaml` (agregar `$ref`)
  - Agente: `spec-writer` · Depende de: 005-T001
  - Aceptación: las 8 rutas del plan §2; esquemas `Quote`, `Booking`, `Stage` (placeholder), estados y errores `quote_changed`; ejemplos con importes en centavos
  - Refs: Art. IV, XII

- [x] **T002** [P] Dominio: fechas y cotización
  - Archivos: `internal/bookings/domain/{dates.go, quote.go}`, `internal/bookings/domain/quote_test.go`
  - Agente: `dev-go` · Depende de: 005-T002
  - Aceptación: `ParseCalendarDate`, `Days`, `Quote` con mitades `ceil/floor`; CA-01 y CA-02 pasan; zona `America/Mexico_City` con `time/tzdata`; solo stdlib; ≥ 80 %
  - Refs: RF-01, RF-05 · P-01, P-02

- [x] **T003** [P] Dominio: máquina de estados y reglas de creación
  - Archivos: `internal/bookings/domain/{booking.go, statemachine.go, errors.go}`, `internal/bookings/domain/statemachine_test.go`
  - Agente: `dev-go` · Depende de: 005-T002
  - Aceptación: todas las transiciones del plan §4 (válidas e inválidas) probadas; reglas RF-03; `statusHistory` limitada a 10; solo stdlib
  - Refs: RF-03, RF-11

- [x] **T004** Repositorio de reservas
  - Archivos: `internal/bookings/adapter/mongo/{booking_doc.go, booking_repo.go, booking_repo_integration_test.go}`
  - Agente: `modelador-mongo` · Depende de: T003, 002-T014
  - Aceptación: colección `bookings` con los 4 índices del plan §3; crear, buscar, listar por rol y estado con cursor, buscar cruces `accepted/in_progress`, expirar pendientes; prueba de integración
  - Refs: plan §3 · Art. II

- [x] **T005** Aceptación atómica
  - Archivos: `internal/bookings/adapter/mongo/accept_tx.go`, `internal/bookings/adapter/mongo/accept_tx_integration_test.go`
  - Agente: `modelador-mongo` · Depende de: T004
  - Aceptación: transacción del plan P-03 (incluye `$inc reservationSeq`); prueba de **concurrencia**: 2 aceptaciones con cruce → exactamente una gana, la otra recibe conflicto (CA-05); las pendientes cruzadas pasan a `rejected`
  - Refs: RF-07 · RNF-01

- [x] **T006** Casos de uso: cotizar y crear
  - Archivos: `internal/bookings/usecase/{ports.go, quote.go, create.go, usecase_create_test.go}`
  - Agente: `dev-go` · Depende de: T002, T003, T004, 005-T005
  - Aceptación: RF-01…RF-05; instantánea del objeto; `quote_changed`; invoca `chat.Opener` y `Notifier` (dobles); creación falla si `Opener` falla; ≥ 80 %
  - Refs: RF-02…RF-05, RF-12 · CA-01…CA-04, CA-07

- [x] **T007** Casos de uso: aceptar, rechazar, cancelar, punto de encuentro
  - Archivos: `internal/bookings/usecase/{accept.go, reject.go, cancel.go, meeting_point.go, usecase_flow_test.go}`
  - Agente: `dev-go` · Depende de: T005, T006
  - Aceptación: permisos por rol; motivo obligatorio al cancelar `accepted`; notificaciones; cancelar libera la reserva (CA-08); pruebas con repositorio falso
  - Refs: RF-07…RF-09, RF-12

- [x] **T008** Casos de uso: consultar, listar y vencer
  - Archivos: `internal/bookings/usecase/{get.go, list.go, expire.go, usecase_read_test.go}`, `internal/bookings/jobs/expirer.go`
  - Agente: `dev-go` · Depende de: T004
  - Aceptación: solo partes o admin ven la reserva; punto de encuentro oculto a terceros; una pendiente vencida se muestra `expired`; tarea cada 60 s con reloj inyectado (CA-06)
  - Refs: RF-06, RF-10 · P-04

- [x] **T009** Puerto `chat.Opener` vacío y verificador de actividad real
  - Archivos: `internal/chat/opener.go`, `internal/bookings/adapter/mongo/activity_checker.go`, `internal/bookings/adapter/mongo/activity_checker_integration_test.go`
  - Agente: `dev-go` · Depende de: T004, 003-T008, 005-T006
  - Aceptación: `Opener` con implementación vacía (devuelve id vacío); `ActivityChecker` real: ¿usuario u objeto con reservas `accepted/in_progress`?; `modules.go` reemplaza los puertos vacíos de 003 y 005 (RF-14)
  - Refs: RF-12, RF-14 · P-05

- [x] **T010** Handlers, DTO y rutas
  - Archivos: `internal/bookings/adapter/http/{handler.go, dto.go, routes.go, handler_test.go}`, `cmd/api/modules.go` (una línea)
  - Agente: `dev-go` · Depende de: T001, T007, T008, 003-T010
  - Aceptación: 8 rutas cumplen el contrato; `RequireActive` al crear; `viewerRole` correcto; módulo registrado con una línea; pruebas `httptest`
  - Refs: RF-01…RF-10

- [x] **T011** Prueba de integración del flujo completo
  - Archivos: `internal/bookings/adapter/http/flow_integration_test.go`
  - Agente: `qa-go` · Depende de: T009, T010
  - Aceptación: cotizar→solicitar→aceptar→(cruce rechazado)→cancelar→disponible en `GET /items`; cambio de precio no altera la reserva; CA-01…CA-08 cubiertos
  - Refs: CA-01…CA-08

- [x] **T012** Memoria y revisión
  - Archivos: `memoria.md` (solo anexar)
  - Agente: `revisor-constitucion` · Depende de: T011
  - Aceptación: veredicto **CONFORME**; hito H3 (backend) marcado
  - Refs: DoD

- [ ] **T013** Despliegue de la spec en Azure (staging)
  - Archivos: `memoria.md` (solo anexar)
  - Agente: `devops-mcp` · Depende de: T012 (y todas las anteriores `[x]`)
  - Aceptación: `make fmt lint test` en verde; `scripts/deploy.sh staging` (construye, envía a la VM y reinicia la API); `/healthz` y `/readyz` devuelven 200 por HTTPS sin `-k`; cada endpoint nuevo de esta spec (`api/paths/<modulo>.yaml`) responde con una petición correcta y una inválida en el formato de error del Art. IV; registros sin secretos ni datos personales; resultado anotado en `memoria.md`. **La spec siguiente no empieza hasta que esta tarea pase.**
  - Refs: Art. XIII.5

| Total | 13 tareas |
|---|---|
