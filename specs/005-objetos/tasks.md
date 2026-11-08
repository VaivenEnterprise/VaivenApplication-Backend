# Tareas 005 — Objetos (backend)

- **Dueño:** BE-A · **Estado:** sin ejecutar · `[P]` = paralelizable
- **T001 se integra el primer día** para que MO-A trabaje contra el contrato.

- [x] **T001** Contrato OpenAPI de objetos
  - Archivos: `api/paths/items.yaml`, `api/openapi.yaml` (agregar `$ref`)
  - Agente: `spec-writer` · Depende de: 003-T001, 004-T001
  - Aceptación: las 8 rutas del plan §2, esquemas `Item`, `ItemInput`, `Category`; ejemplos con precios en centavos
  - Refs: Art. IV, XII

- [x] **T002** [P] Políticas como constantes
  - Archivos: `internal/policy/policy.go`, `internal/policy/policy_test.go`
  - Agente: `dev-go` · Depende de: 002-T001
  - Aceptación: constantes con nombre para POL-01…POL-17 del ROADMAP §8 (solo las que el backend usa); prueba que fija los valores provisionales
  - Refs: ROADMAP §8 · Art. XII

- [x] **T003** Dominio: objeto, categorías y precios
  - Archivos: `internal/items/domain/{item.go, pricing.go, categories.go, errors.go}`, `internal/items/domain/domain_test.go`
  - Agente: `dev-go` · Depende de: T002
  - Aceptación: validación de campos y de modalidad/precio; `SuggestPrice(declaredCents)` cumple CA-02 y bordes (mínimo $10, múltiplos de $5); `RoundCoord`; solo stdlib; ≥ 80 %
  - Refs: RF-01…RF-04 · POL-04

- [ ] **T004** Repositorio de objetos en MongoDB
  - Archivos: `internal/items/adapter/mongo/{item_doc.go, item_repo.go, item_repo_integration_test.go}`
  - Agente: `modelador-mongo` · Depende de: T003, 002-T014
  - Aceptación: colección `items`; índices del plan §3 vía `EnsureIndexes`; crear, buscar por id, actualizar con `version`, listar por dueño y estado con cursor; prueba de integración
  - Refs: plan §3 · Art. II

- [ ] **T005** Casos de uso: crear, editar y obtener
  - Archivos: `internal/items/usecase/{ports.go, create.go, update.go, get.go, suggest.go, usecase_test.go}`
  - Agente: `dev-go` · Depende de: T003, 004-T007, 003-T002
  - Aceptación: usa `images.Service` y `moderation.Checker` (dobles en pruebas); reglas RF-02, RF-03, RF-05, RF-06, RF-07; conflicto de versión → 409; ≥ 80 %
  - Refs: RF-01…RF-07

- [ ] **T006** Casos de uso: pausar, reanudar, eliminar y listar
  - Archivos: `internal/items/usecase/{pause.go, delete.go, list_mine.go, lifecycle_test.go}`
  - Agente: `dev-go` · Depende de: T005
  - Aceptación: solo el dueño; `delete` consulta `ActiveBookingChecker` (409 si hay compromisos); estados válidos; expone `items.Moderation` con `Block(ctx, itemID)` y `Snapshot(ctx, itemID)` para la moderación de 011; pruebas
  - Refs: RF-08…RF-10 · CA-05, CA-07

- [ ] **T007** Handlers, DTO y rutas
  - Archivos: `internal/items/adapter/http/{handler.go, dto.go, routes.go, handler_test.go}`, `cmd/api/modules.go` (una línea)
  - Agente: `dev-go` · Depende de: T001, T006, 003-T010
  - Aceptación: las 8 rutas cumplen el contrato; `declaredValueCents` oculto a terceros; `RequireActive` en crear; fotos con `url` y `thumbUrl`; ruta estática `price-suggestion` antes de `{id}`; pruebas `httptest`
  - Refs: RF-06, RF-11, RF-12 · RNF-02

- [ ] **T008** Prueba de integración del ciclo de vida
  - Archivos: `internal/items/adapter/http/items_integration_test.go`
  - Agente: `qa-go` · Depende de: T007
  - Aceptación: crear→leer→editar→pausar→reanudar→eliminar con Mongo real y `images.Service` simulado; CA-01…CA-07 cubiertos (los que dependen de 007/011 con puertos vacíos)
  - Refs: CA-01…CA-07

- [ ] **T009** Memoria y revisión
  - Archivos: `memoria.md` (solo anexar)
  - Agente: `revisor-constitucion` · Depende de: T008
  - Aceptación: veredicto **CONFORME**; hito H2 (backend) marcado
  - Refs: DoD

- [ ] **T010** Despliegue de la spec en Azure (staging)
  - Archivos: `memoria.md` (solo anexar)
  - Agente: `devops-mcp` · Depende de: T009 (y todas las anteriores `[x]`)
  - Aceptación: `make fmt lint test` en verde; `scripts/deploy.sh staging` (construye, envía a la VM y reinicia la API); `/healthz` y `/readyz` devuelven 200 por HTTPS sin `-k`; cada endpoint nuevo de esta spec (`api/paths/<modulo>.yaml`) responde con una petición correcta y una inválida en el formato de error del Art. IV; registros sin secretos ni datos personales; resultado anotado en `memoria.md`. **La spec siguiente no empieza hasta que esta tarea pase.**
  - Refs: Art. XIII.5

| Total | 10 tareas |
|---|---|
