# Tareas 011 — Notificaciones y moderación (backend)

- **Dueño:** BE-B · **Estado:** sin ejecutar · `[P]` = paralelizable
- **Línea de corte:** si no hay tiempo, se recorta primero el push (T007–T008). **No se recortan** el filtro de contenido (T009) ni los reportes (T010–T012): Google Play los exige para apps con contenido de usuarios. Los avisos dentro de la app (T002–T006) son P1.

- [ ] **T001** Contrato OpenAPI de avisos y reportes
  - Archivos: `api/paths/notifications.yaml`, `api/paths/reports.yaml`, `api/openapi.yaml` (agregar `$ref`)
  - Agente: `spec-writer` · Depende de: 010-T001
  - Aceptación: las 8 rutas del plan §2; esquemas `Notification` (con los 18 tipos), `Report`; ejemplos
  - Refs: Art. IV, XII

- [ ] **T002** [P] Plantillas de avisos
  - Archivos: `internal/notify/templates.go`, `internal/notify/templates_test.go`
  - Agente: `dev-go` · Depende de: 003-T002
  - Aceptación: título y cuerpo en español para los 18 tipos; parámetros sin PII extra; prueba que cada tipo del RF-02 tiene plantilla
  - Refs: RF-01, RF-02

- [ ] **T003** Repositorio de avisos y tokens
  - Archivos: `internal/notifications/adapter/mongo/{notification_repo.go, token_repo.go, repo_integration_test.go}`
  - Agente: `modelador-mongo` · Depende de: T002, 002-T014
  - Aceptación: índices del plan §3 incluido el TTL de 90 días; token único que se reasigna al nuevo usuario; listado con cursor; marcar leídos; conteo; prueba de integración
  - Refs: plan §3 · RNF-01

- [ ] **T004** `Notifier` real y casos de uso de avisos
  - Archivos: `internal/notify/service.go`, `internal/notifications/usecase/{ports.go, list.go, read.go, unread.go, register_token.go, usecase_test.go}`
  - Agente: `dev-go` · Depende de: T002, T003
  - Aceptación: `Notify` guarda el aviso y encola push (con doble) y no envía push a cuentas dadas de baja; `read ids|all`; `unread-count`; registrar/eliminar token; CA-07; ≥ 80 %
  - Refs: RF-01, RF-03, RF-04

- [ ] **T005** Handlers de avisos y registro del módulo
  - Archivos: `internal/notifications/adapter/http/{handler.go, dto.go, routes.go, handler_test.go}`, `cmd/api/modules.go` (una línea)
  - Agente: `dev-go` · Depende de: T001, T004, 003-T010
  - Aceptación: 5 rutas cumplen el contrato; solo mis avisos y mis tokens
  - Refs: RF-03, RF-04

- [ ] **T006** Reemplazar el `Notifier` vacío
  - Archivos: `cmd/api/modules.go` (cambio de una línea acordado con BE-A)
  - Agente: `dev-go` · Depende de: T004, 007-T006
  - Aceptación: crear una solicitud (007) genera el aviso `booking_requested` (CA-01); todas las specs que ya llamaban al puerto ahora producen avisos sin tocar su código
  - Refs: P-08

- [ ] **T007** Cliente de Expo Push y *worker*
  - Archivos: `internal/notifications/adapter/expo/{client.go, client_test.go}`, `internal/notifications/worker/worker.go`, `internal/notifications/worker/worker_test.go`, `internal/config/push.go`
  - Agente: `dev-go` · Depende de: T004
  - Aceptación: lotes ≤ 100; `DeviceNotRegistered` elimina el token (CA-02); canal con buffer 500 sin bloquear el negocio; `PUSH_ENABLED=false` lo desactiva; pruebas con servidor simulado
  - Refs: RF-05, RF-06 · P-01…P-03

- [ ] **T008** [P] Prueba de integración de push simulado
  - Archivos: `internal/notifications/adapter/http/push_integration_test.go`
  - Agente: `qa-go` · Depende de: T006, T007
  - Aceptación: crear solicitud → aviso guardado → push enviado al servidor simulado con `data.bookingId`
  - Refs: CA-01, RF-06

- [ ] **T009** Filtro de contenido prohibido
  - Archivos: `internal/moderation/{filter.go, blocklist.go, filter_test.go}`, `cmd/api/modules.go` (cambio de una línea)
  - Agente: `dev-go` · Depende de: 003-T002
  - Aceptación: normalización sin acentos; coincidencia por palabra completa y frases; devuelve categoría y motivo en español; sin falsos positivos en una lista de 20 frases normales; < 5 ms por 1,000 caracteres; CA-03 y CA-04 con los módulos 005 y 008 sin editarlos
  - Refs: RF-07 · POL-16

- [ ] **T010** Dominio y repositorio de reportes
  - Archivos: `internal/reports/domain/report.go`, `internal/reports/adapter/mongo/{report_repo.go, report_repo_integration_test.go}`, `internal/reports/domain/report_test.go`
  - Agente: `modelador-mongo` · Depende de: 002-T014
  - Aceptación: índices del plan §3; reporte duplicado devuelve el existente; listado por estado con cursor
  - Refs: RF-08 · plan §3

- [ ] **T011** Casos de uso y handlers de reportes
  - Archivos: `internal/reports/usecase/{ports.go, create.go, list.go, resolve.go, usecase_test.go}`, `internal/reports/adapter/http/{handler.go, dto.go, routes.go, handler_test.go}`, `cmd/api/modules.go` (una línea)
  - Agente: `dev-go` · Depende of: T001, T010, 005-T006, 003-T008
  - Aceptación: un mensaje solo lo reporta quien participa (CA-06); `moderation_decision` permitido; `hide_item` usa `items.Moderation.Block` y notifica `content_removed`; `suspend_user` usa `users.Admin.Suspend`; `/admin/*` con `RequireRole`
  - Refs: RF-08…RF-11

- [ ] **T012** Prueba de integración de moderación
  - Archivos: `internal/reports/adapter/http/moderation_integration_test.go`
  - Agente: `qa-go` · Depende de: T009, T011
  - Aceptación: CA-03, CA-04, CA-05, CA-06 con Mongo real
  - Refs: CA-03…CA-06

- [ ] **T013** Memoria y revisión
  - Archivos: `memoria.md` (solo anexar)
  - Agente: `revisor-constitucion` · Depende de: T008, T012
  - Aceptación: veredicto **CONFORME** (sin PII en push/logs); hito H5 (backend) marcado
  - Refs: DoD

- [ ] **T014** Despliegue de la spec en staging por MCP
  - Archivos: `memoria.md` (solo anexar)
  - Agente: `devops-mcp` · Depende de: T013 (y todas las anteriores `[x]`)
  - Aceptación: `make fmt lint test` en verde; `trigger_deploy` de `vaiven-staging` y espera con `list_deploys`; `/healthz` y `/readyz` devuelven 200; cada endpoint nuevo de esta spec (`api/paths/<modulo>.yaml`) responde con una petición correcta y una inválida en el formato de error del Art. IV; `list_logs` sin secretos ni datos personales; resultado anotado en `memoria.md`. **La spec siguiente no empieza hasta que esta tarea pase.**
  - Refs: Art. XIII.4

| Total | 14 tareas |
|---|---|
