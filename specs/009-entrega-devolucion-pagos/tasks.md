# Tareas 009 — Entrega, devolución y pagos (backend)

- **Dueño:** BE-A · **Estado:** sin ejecutar · `[P]` = paralelizable
- **T001 se integra el primer día.** Si el calendario se atrasa, ver línea de corte (ROADMAP §6): primero se entregan T001–T009 (evidencia + QR); pagos y retrasos son T010–T013.

- [x] **T001** Contrato OpenAPI de entrega, devolución y pagos
  - Archivos: `api/paths/handover.yaml`, `api/paths/payments.yaml`, `api/openapi.yaml` (agregar `$ref`)
  - Agente: `spec-writer` · Depende de: 007-T001, 004-T001
  - Aceptación: rutas del plan §2 con esquemas `Stage`, `Adjustment`, `QrToken`; `Booking` completa `delivery`, `return`, `payments`, `isLate`, `adjustments`, `affectedBy`
  - Refs: Art. IV, XII

- [x] **T002** [P] Dominio: etapa y QR
  - Archivos: `internal/handover/domain/{stage.go, qr.go, errors.go}`, `internal/handover/domain/domain_test.go`
  - Agente: `dev-go` · Depende de: 007-T003
  - Aceptación: token seguro (32 bytes) + hash SHA-256 + comparación en tiempo constante; reglas de vigencia, uso único, etapa y versión; roles del plan §4; ventana de entrega P-04; solo stdlib; ≥ 80 %
  - Refs: RF-02, RF-03, RF-06 · P-01

- [x] **T003** Repositorio de etapas
  - Archivos: `internal/handover/adapter/mongo/{stage_repo.go, stage_repo_integration_test.go}`
  - Agente: `modelador-mongo` · Depende de: T002, 007-T004
  - Aceptación: `$set` de `delivery`/`return` con filtro por estado; `findOneAndUpdate` atómico de confirmación (P-02); prueba de **concurrencia**: 2 confirmaciones con el mismo token → una gana (RNF-02)
  - Refs: RF-03 · P-02, P-09

- [x] **T004** Casos de uso: evidencia y QR
  - Archivos: `internal/handover/usecase/{ports.go, record_evidence.go, issue_qr.go, usecase_evidence_test.go}`
  - Agente: `dev-go` · Depende de: T002, T003, 004-T007
  - Aceptación: valida roles por etapa; usa `images.Service.Resolve` (doble); reemplazar evidencia sube `evidenceVersion` e invalida el QR (CA-04); QR nuevo invalida el anterior; ≥ 80 %
  - Refs: RF-01, RF-02, RF-04

- [x] **T005** Casos de uso: confirmar y consultar
  - Archivos: `internal/handover/usecase/{confirm.go, get_stage.go, usecase_confirm_test.go}`
  - Agente: `dev-go` · Depende de: T004
  - Aceptación: CA-01, CA-02, CA-03, CA-05; entrega → `in_progress`, devolución → `completed`; URLs firmadas de 10 min solo para partes (CA-09); notifica a la otra parte (`Notifier` doble)
  - Refs: RF-03, RF-05, RF-06, RF-12

- [x] **T006** Handlers y rutas de entrega y devolución
  - Archivos: `internal/handover/adapter/http/{handler.go, dto.go, routes.go, handler_test.go}`, `cmd/api/modules.go` (una línea)
  - Agente: `dev-go` · Depende de: T001, T005, 003-T010
  - Aceptación: 8 rutas cumplen el contrato; errores con el envoltorio; `Booking` devuelve las etapas; pruebas `httptest` por rol y por error
  - Refs: RF-01…RF-06

- [x] **T007** Prueba de integración del flujo entrega→devolución
  - Archivos: `internal/handover/adapter/http/handover_integration_test.go`
  - Agente: `qa-go` · Depende de: T006
  - Aceptación: reserva `accepted` → evidencia → QR → confirmar → `in_progress` → evidencia de devolución → QR → confirmar → `completed`; CA-01…CA-05 y CA-09 con Mongo real y `images.Service` simulado
  - Refs: CA-01…CA-05, CA-09

- [x] **T008** Revisión intermedia (corte de seguridad)
  - Archivos: ninguno
  - Agente: `revisor-constitucion` · Depende de: T007
  - Aceptación: veredicto **CONFORME** en token hasheado, URLs de evidencia y roles; esto es el mínimo entregable de la spec
  - Refs: Art. VI

- [x] **T009** [P] Dominio de pagos y retraso
  - Archivos: `internal/payments/domain/{payment.go, lateness.go}`, `internal/payments/domain/domain_test.go`
  - Agente: `dev-go` · Depende de: 007-T003
  - Aceptación: máquina `pending→reported→confirmed`; `LateDays` y ajuste con la tabla de CA-07 (tolerancia 2 h, 1 y 2 días, préstamo sin ajuste); solo stdlib; ≥ 80 %
  - Refs: RF-07, RF-10 · P-06

- [x] **T010** Repositorio y casos de uso de pagos
  - Archivos: `internal/payments/adapter/mongo/payment_repo.go`, `internal/payments/usecase/{ports.go, report.go, confirm.go, usecase_test.go}`
  - Agente: `dev-go` · Depende de: T009, 007-T004
  - Aceptación: `report` solo solicitante; `confirm` solo propietario y solo si `reported` (CA-06); notifica; pruebas; integración de `$set` por etapa
  - Refs: RF-07, RF-08, RF-12

- [x] **T011** Detector de retrasos y reservas afectadas
  - Archivos: `internal/payments/jobs/late_detector.go`, `internal/payments/usecase/late_adjust.go`, `internal/payments/usecase/late_test.go`, `internal/payments/jobs/late_detector_integration_test.go`
  - Agente: `dev-go` · Depende de: T009, T010
  - Aceptación: tarea cada 5 min con reloj inyectado marca `isLate`/`lateAt`; ajuste registrado al confirmar la devolución (CA-07); reserva afectada marcada y notificada (CA-08)
  - Refs: RF-09…RF-11 · P-05, P-07

- [x] **T012** Handlers de pagos y registro del módulo
  - Archivos: `internal/payments/adapter/http/{handler.go, dto.go, routes.go, handler_test.go}`, `cmd/api/modules.go` (una línea)
  - Agente: `dev-go` · Depende de: T001, T010
  - Aceptación: 2 rutas cumplen el contrato; roles verificados; `Booking` muestra `payments`, `isLate`, `adjustments`
  - Refs: RF-07

- [x] **T013** Prueba de integración de pagos y retrasos
  - Archivos: `internal/payments/adapter/http/payments_integration_test.go`
  - Agente: `qa-go` · Depende de: T011, T012
  - Aceptación: CA-06, CA-07, CA-08 con Mongo real y reloj falso; el QR no depende del pago y el pago no cambia el estado de la reserva
  - Refs: CA-06…CA-08

- [x] **T014** Memoria y revisión final
  - Archivos: `memoria.md` (solo anexar)
  - Agente: `revisor-constitucion` · Depende de: T013
  - Aceptación: veredicto **CONFORME**; hito H4 (backend) marcado
  - Refs: DoD

- [ ] **T015** Despliegue de la spec en Azure (staging)
  - Archivos: `memoria.md` (solo anexar)
  - Agente: `devops-mcp` · Depende de: T014 (y todas las anteriores `[x]`)
  - Aceptación: `make fmt lint test` en verde; `scripts/deploy.sh staging` (construye, envía a la VM y reinicia la API); `/healthz` y `/readyz` devuelven 200 por HTTPS sin `-k`; cada endpoint nuevo de esta spec (`api/paths/<modulo>.yaml`) responde con una petición correcta y una inválida en el formato de error del Art. IV; registros sin secretos ni datos personales; resultado anotado en `memoria.md`. **La spec siguiente no empieza hasta que esta tarea pase.**
  - Refs: Art. XIII.5

| Total | 15 tareas |
|---|---|
