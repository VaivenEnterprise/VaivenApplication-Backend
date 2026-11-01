# Tareas 014 — Sugerencias con IA (backend)

- **Dueño:** BE-A · **Estado:** sin ejecutar · `[P]` = paralelizable
- Se ejecuta **después** del congelamiento de la 1.0 (15 nov); no toca el AAB en revisión.

- [ ] **T001** Contrato OpenAPI
  - Archivos: `api/paths/ai.yaml`, `api/openapi.yaml` (referencia), `/meta` con `features.aiSuggestions`
  - Agente: `spec-writer` · Depende de: 005-T001
  - Aceptación: ruta, cuerpos, errores (`validation_failed`, `forbidden`, `rate_limited`, `service_unavailable`) y campo de `meta`; MO-A lo consume el mismo día
  - Refs: RF-01, RF-06, RF-08

- [ ] **T002** [P] Dominio, puerto y configuración
  - Archivos: `internal/suggest/{domain.go, usecase.go}`, `internal/config/…` (variables `AI_*`), `.env.example`
  - Agente: `dev-go` · Depende de: T001
  - Aceptación: interfaz `Suggester`; validación de salida (categoría POL-14, título ≤ 80); variables con valores por defecto seguros (`AI_ENABLED=false`); sin secretos
  - Refs: RF-03, RF-04, RF-06 · P-01, P-03

- [ ] **T003** [P] Adaptador del proveedor
  - Archivos: `internal/suggest/adapter/provider/{client.go, client_test.go}`
  - Agente: `dev-go` · Depende de: T002
  - Aceptación: `net/http` con espera de 10 s y un reintento; pide JSON estructurado; maneja errores y respuestas inválidas; prueba con `httptest`; la clave nunca se registra
  - Refs: RF-03, RF-07 · P-05, P-06

- [ ] **T004** [P] Contador de uso en MongoDB
  - Archivos: `internal/suggest/adapter/mongo/usage.go`, `internal/suggest/adapter/mongo/usage_test.go`, índice en el comando `indexes`
  - Agente: `modelador-mongo` · Depende de: T002
  - Aceptación: incremento atómico por persona y día; índice único `{userId, day}` y TTL; prueba de concurrencia
  - Refs: RF-05 · P-04

- [ ] **T005** Caso de uso completo
  - Archivos: `internal/suggest/usecase.go`, `internal/suggest/usecase_test.go`
  - Agente: `dev-go` · Depende de: T003, T004, 004-T007, 011 (filtro)
  - Aceptación: verifica que la imagen es `item_photo` de esa persona (CA-02); construye la URL; aplica límite (CA-05) y filtro (CA-03); apagado/caída → `service_unavailable` (CA-04)
  - Refs: RF-02, RF-04, RF-05, RF-06

- [ ] **T006** Handler, rutas y registro
  - Archivos: `internal/suggest/adapter/http/{handler.go, dto.go, routes.go}`, `cmd/api/modules.go` (una línea)
  - Agente: `dev-go` · Depende de: T001, T005
  - Aceptación: cumple el contrato; autenticada; límite de tasa por ruta; registros sin imagen ni texto sugerido (RF-07)
  - Refs: RF-01, RF-07

- [ ] **T007** Prueba de integración
  - Archivos: `internal/suggest/integration_test.go`
  - Agente: `qa-go` · Depende de: T006
  - Aceptación: CA-01 a CA-06 con proveedor falso; sin red; limpia sus datos
  - Refs: RNF-01

- [ ] **T008** Variables en producción y revisión
  - Archivos: `docs/produccion.md` (sección IA), `memoria.md` (solo anexar)
  - Agente: `revisor-constitucion` · Depende de: T007, 013-T008
  - Aceptación: pasos para definir `AI_API_KEY` y `AI_ENABLED` en Render (sin valores); procedimiento de apagado; **CONFORME**
  - Refs: RF-06, CA-06

- [ ] **T009** Despliegue de la spec en staging por MCP
  - Archivos: `memoria.md` (solo anexar)
  - Agente: `devops-mcp` · Depende de: T008 (y todas las anteriores `[x]`)
  - Aceptación: `make fmt lint test` en verde; `trigger_deploy` de `vaiven-staging` y espera con `list_deploys`; `/healthz` y `/readyz` devuelven 200; cada endpoint nuevo de esta spec (`api/paths/<modulo>.yaml`) responde con una petición correcta y una inválida en el formato de error del Art. IV; `list_logs` sin secretos ni datos personales; resultado anotado en `memoria.md`. **La spec siguiente no empieza hasta que esta tarea pase.**
  - Refs: Art. XIII.4

| Total | 9 tareas |
|---|---|
