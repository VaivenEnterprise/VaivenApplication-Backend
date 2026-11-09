# Tareas 010 — Calificaciones e incidencias (backend)

- **Dueño:** BE-B · **Estado:** T001–T012 cerradas (2026-11-09); spec desplegada y verificada en staging · `[P]` = paralelizable
- Prioridad de entrega (ROADMAP §6): **primero calificaciones (T002–T006)**, después incidencias y sanciones.

- [x] **T001** Contrato OpenAPI de calificaciones, incidencias y administración
  - Archivos: `api/paths/ratings.yaml`, `api/paths/incidents.yaml`, `api/paths/admin.yaml`, `api/openapi.yaml` (agregar `$ref`)
  - Agente: `spec-writer` · Depende de: 009-T001
  - Aceptación: las 10 rutas del plan §2; esquemas `Rating`, `Incident`, `Sanction`; marca las rutas admin con seguridad `admin`
  - Refs: Art. IV, XII

- [x] **T002** [P] Dominio de calificaciones
  - Archivos: `internal/ratings/domain/{rating.go, rules.go}`, `internal/ratings/domain/domain_test.go`
  - Agente: `dev-go` · Depende de: 002-T006
  - Aceptación: validación de puntaje 1–5 y comentario ≤ 300; `CanRate(booking, userID, now)` (completada, parte, ventana de 14 días); promedio a 1 decimal; solo stdlib
  - Refs: RF-01, RF-03 · P-03

- [x] **T003** Repositorio de calificaciones y calificación atómica
  - Archivos: `internal/ratings/adapter/mongo/{rating_repo.go, rating_repo_integration_test.go}`
  - Agente: `modelador-mongo` · Depende de: T002, 003-T008
  - Aceptación: índices único `{bookingId,raterId}` y `{rateeId,_id}`; `Create` en transacción con `users.Admin.AddRating`; concurrencia: 2 calificaciones iguales → una gana (RNF-01)
  - Refs: RF-02 · P-01

- [x] **T004** Casos de uso y handlers de calificaciones
  - Archivos: `internal/ratings/usecase/{ports.go, rate.go, status.go, list.go, usecase_test.go}`, `internal/ratings/adapter/http/{handler.go, dto.go, routes.go, handler_test.go}`, `cmd/api/modules.go` (una línea)
  - Agente: `dev-go` · Depende de: T001, T003, 007-T004, 003-T010
  - Aceptación: CA-01, CA-02, CA-03; notifica «calificación recibida»; el perfil público (`/users/{id}`) ya refleja promedio y cantidad; pruebas `httptest`
  - Refs: RF-01…RF-04, RF-10

- [x] **T005** [P] Dominio de incidencias y sanciones
  - Archivos: `internal/incidents/domain/{incident.go, rules.go}`, `internal/sanctions/domain/sanction.go`, `internal/incidents/domain/domain_test.go`
  - Agente: `dev-go` · Depende de: 002-T006
  - Aceptación: motivos y reglas de apertura (estado de reserva, 72 h, `no_show` solo desde `startDate`, máx. 3 abiertas); solo dos estados; notas ≤ 10; solo stdlib; ≥ 80 %
  - Refs: RF-05, RF-06, RF-08, RF-09

- [x] **T006** Repositorios de incidencias y sanciones
  - Archivos: `internal/incidents/adapter/mongo/{incident_repo.go, incident_repo_integration_test.go}`, `internal/sanctions/adapter/mongo/{sanction_repo.go, sanction_repo_integration_test.go}`
  - Agente: `modelador-mongo` · Depende de: T005, 002-T014
  - Aceptación: colecciones e índices del plan §3; listados por estado/persona con cursor; pruebas de integración
  - Refs: plan §3

- [x] **T007** Casos de uso de incidencias (partes)
  - Archivos: `internal/incidents/usecase/{ports.go, open.go, get.go, list_mine.go, usecase_test.go}`
  - Agente: `dev-go` · Depende de: T005, T006, 004-T007
  - Aceptación: CA-04 y CA-08; evidencia con `images.Service` (doble); notifica a la parte reportada; la reserva no cambia
  - Refs: RF-05…RF-07, RF-10

- [x] **T008** Casos de uso de administración
  - Archivos: `internal/incidents/usecase/{admin_list.go, add_note.go, close.go, admin_test.go}`, `internal/sanctions/usecase/{apply.go, apply_test.go}`
  - Agente: `dev-go` · Depende de: T007, 003-T008
  - Aceptación: notas con visibilidad; cierre con respuesta final y `outcome`; suspensión llama `users.Admin.Suspend`, advertencia solo notifica; notificaciones a las partes
  - Refs: RF-08, RF-09, RF-10

- [x] **T009** Handlers de incidencias y administración
  - Archivos: `internal/incidents/adapter/http/{handler.go, dto.go, routes.go, handler_test.go}`, `internal/sanctions/adapter/http/{handler.go, routes.go}`, `scripts/admin.http`, `cmd/api/modules.go` (una línea por módulo)
  - Agente: `dev-go` · Depende de: T001, T007, T008, 003-T010
  - Aceptación: 7 rutas cumplen el contrato; `/admin/*` con `RequireRole("admin")` (CA-05); evidencia con URLs firmadas; `scripts/admin.http` con un ejemplo por ruta admin
  - Refs: RF-07, RF-08, RF-11 · P-09

- [x] **T010** Prueba de integración de punta a punta
  - Archivos: `internal/incidents/adapter/http/trust_integration_test.go`
  - Agente: `qa-go` · Depende de: T004, T009
  - Aceptación: calificar tras completar; abrir→nota→cerrar incidencia; suspensión impide crear objeto (CA-06); respuesta final visible (CA-07); CA-01…CA-08
  - Refs: CA-01…CA-08

- [x] **T011** Memoria y revisión
  - Archivos: `memoria.md` (solo anexar)
  - Agente: `revisor-constitucion` · Depende de: T010
  - Aceptación: veredicto **CONFORME**; hito H4 (backend) marcado
  - Refs: DoD

- [x] **T012** Despliegue de la spec en Azure (staging)
  - Archivos: `memoria.md` (solo anexar)
  - Agente: `devops-mcp` · Depende de: T011 (y todas las anteriores `[x]`)
  - Aceptación: `make fmt lint test` en verde; `scripts/deploy.sh staging` (construye, envía a la VM y reinicia la API); `/healthz` y `/readyz` devuelven 200 por HTTPS sin `-k`; cada endpoint nuevo de esta spec (`api/paths/<modulo>.yaml`) responde con una petición correcta y una inválida en el formato de error del Art. IV; registros sin secretos ni datos personales; resultado anotado en `memoria.md`. **La spec siguiente no empieza hasta que esta tarea pase.**
  - Refs: Art. XIII.5

| Total | 12 tareas |
|---|---|
