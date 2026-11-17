# Tareas 006 — Búsqueda (backend)

- **Dueño:** BE-B · **Estado:** T001–T010 hechas; verificada en staging · `[P]` = paralelizable
- Puede arrancar con documentos de `items` sembrados a mano según el modelo de 005 plan §3; se cierra cuando 005 esté desplegada.

- [x] **T001** Contrato OpenAPI de búsqueda
  - Archivos: `api/paths/search.yaml`, `api/openapi.yaml` (agregar `$ref`)
  - Agente: `spec-writer` · Depende de: 005-T001
  - Aceptación: describe `GET /api/v1/items` con todos los parámetros, esquema `ItemCard` y `ItemPage`, errores de validación
  - Refs: Art. IV, XII

- [x] **T002** [P] Dominio de consulta y cursor
  - Archivos: `internal/search/domain/{query.go, card.go, cursor.go}`, `internal/search/domain/domain_test.go`
  - Agente: `dev-go` · Depende de: 002-T006
  - Aceptación: validación de parámetros (RF-08, P-02); cursor codifica/decodifica y rechaza basura; solo stdlib; ≥ 80 %
  - Refs: RF-01, RF-07, RF-08

- [x] **T003** Constructor de pipelines
  - Archivos: `internal/search/adapter/mongo/pipelines.go`, `internal/search/adapter/mongo/pipelines_test.go`
  - Agente: `modelador-mongo` · Depende de: T002
  - Aceptación: función pura que genera los 3 modos del plan §3 con cursor y disponibilidad; pruebas *golden* por modo; regex escapado
  - Refs: plan §3 · P-05

- [x] **T004** Índices de búsqueda
  - Archivos: `internal/search/adapter/mongo/search_indexes.go`, `internal/search/adapter/mongo/search_indexes_test.go`
  - Agente: `modelador-mongo` · Depende de: 002-T014
  - Aceptación: crea el índice de texto en español y el compuesto del plan §4 de forma idempotente; probado con Mongo real
  - Refs: plan §4

- [x] **T005** Repositorio de búsqueda
  - Archivos: `internal/search/adapter/mongo/search_repo.go`, `internal/search/adapter/mongo/search_repo_integration_test.go`
  - Agente: `modelador-mongo` · Depende de: T003, T004, 005-T004
  - Aceptación: ejecuta los 3 modos y devuelve tarjetas con propietario; paginación sin repetidos (CA-01); texto con acentos (CA-02); cercanía ordenada (CA-04); excluye pausados, propios y cuentas inactivas (CA-05)
  - Refs: RF-02, RF-04…RF-07

- [x] **T006** Caso de uso y handler
  - Archivos: `internal/search/usecase/{search.go, ports.go, search_test.go}`, `internal/search/adapter/http/{handler.go, dto.go, routes.go, handler_test.go}`, `cmd/api/modules.go` (una línea)
  - Agente: `dev-go` · Depende de: T001, T005, 004-T007, 003-T010
  - Aceptación: cumple el contrato; `thumbUrl` con `images.Service`; errores de validación con `details` (CA-06); módulo registrado con una línea; pruebas con repositorio falso
  - Refs: RF-01, RF-06, RF-08

- [x] **T007** Disponibilidad con reservas (prueba cruzada)
  - Archivos: `internal/search/adapter/mongo/availability_integration_test.go`
  - Agente: `qa-go` · Depende de: T005
  - Aceptación: con reservas sembradas manualmente en `bookings` (estados `accepted`, `in_progress`, `pending`, `cancelled`) solo las dos primeras bloquean; cubre CA-03 y periodos que tocan bordes
  - Refs: RF-03 · CA-03

- [x] **T008** [P] Prueba de rendimiento
  - Archivos: `internal/search/adapter/mongo/search_perf_test.go` (`//go:build perf`)
  - Agente: `qa-go` · Depende de: T005
  - Aceptación: con 5,000 objetos sembrados, p95 < 500 ms en los 3 modos; resultado anotado en `memoria.md`
  - Refs: RF-09

- [x] **T009** Memoria y revisión
  - Archivos: `memoria.md` (solo anexar)
  - Agente: `revisor-constitucion` · Depende de: T006, T007
  - Aceptación: veredicto **CONFORME**; hito H2 (backend) marcado
  - Refs: DoD

- [x] **T010** Despliegue de la spec en Azure (staging)
  - Archivos: `memoria.md` (solo anexar)
  - Agente: `devops-mcp` · Depende de: T009 (y todas las anteriores `[x]`)
  - Aceptación: `make fmt lint test` en verde; `scripts/deploy.sh staging` (construye, envía a la VM y reinicia la API); `/healthz` y `/readyz` devuelven 200 por HTTPS sin `-k`; cada endpoint nuevo de esta spec (`api/paths/<modulo>.yaml`) responde con una petición correcta y una inválida en el formato de error del Art. IV; registros sin secretos ni datos personales; resultado anotado en `memoria.md`. **La spec siguiente no empieza hasta que esta tarea pase.**
  - Refs: Art. XIII.5

| Total | 10 tareas |
|---|---|
