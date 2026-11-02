# Tareas 015 — Recomendaciones con ML (backend)

- **Dueño:** BE-B · **Estado:** sin ejecutar · `[P]` = paralelizable
- Se ejecuta **después** del congelamiento de la 1.0 (22 nov).

- [ ] **T001** Contrato OpenAPI
  - Archivos: `api/paths/recommendations.yaml`, `api/openapi.yaml` (referencia), `/meta` con `features.recommendations`
  - Agente: `spec-writer` · Depende de: 006-T001
  - Aceptación: `POST events`, `GET recommendations`, `DELETE history`, errores y `reason`; MO-B lo consume el mismo día
  - Refs: RF-01, RF-05, RF-09, RF-10

- [ ] **T002** [P] Dominio, parámetros y configuración
  - Archivos: `internal/reco/{domain.go, params.go}`, config (`RECO_ENABLED`, por defecto `false`)
  - Agente: `dev-go` · Depende de: T001
  - Aceptación: constantes con nombre (`WeightRequest=3`, `WeightView=1`, `Alpha=0.7`, `Neighbors=20`, `WindowDays=90`, `MaxPerCategory=3`); interfaces `InteractionStore`, `ModelStore`, `RequestSource`
  - Refs: RF-02, RF-04 · P-01

- [ ] **T003** [P] Repositorios MongoDB
  - Archivos: `internal/reco/adapter/mongo/{interactions.go, model_store.go, bookings_source.go}`, índices en el comando `indexes`
  - Agente: `modelador-mongo` · Depende de: T002
  - Aceptación: TTL de 90 días; límite de 1 vista por objeto cada 10 min; `bookings` solo lectura; arreglos acotados; pruebas de integración
  - Refs: RF-01, RF-09 · P-03, P-05

- [ ] **T004** Modelo colaborativo y de contenido
  - Archivos: `internal/reco/model/{cooccurrence.go, content.go, hybrid.go}` y pruebas
  - Agente: `dev-go` · Depende de: T002
  - Aceptación: coseno objeto-objeto y de contenido con resultados exactos en matrices pequeñas; híbrido α·cf + (1−α)·contenido; deterministas
  - Refs: RF-02, RF-03, RF-04 · P-01, P-02

- [ ] **T005** Caso de uso «recomendar»
  - Archivos: `internal/reco/usecase.go`, `internal/reco/usecase_test.go`
  - Agente: `dev-go` · Depende de: T003, T004, 005-T006 (objetos), 011 (suspendidos)
  - Aceptación: CA-01 a CA-03; arranque en frío; exclusiones; diversidad (≤ 3 por categoría); `reason`; recalculo perezoso con candado
  - Refs: RF-04, RF-05, RF-06 · P-04

- [ ] **T006** Handlers, rutas y registro
  - Archivos: `internal/reco/adapter/http/{handler.go, dto.go, routes.go}`, `cmd/api/modules.go` (una línea)
  - Agente: `dev-go` · Depende de: T001, T005
  - Aceptación: cumple el contrato; interruptor (CA-06); borrar historial (CA-05); registros sin eventos ni objetos vistos
  - Refs: RF-01, RF-09, RF-10

- [ ] **T007** Comandos de administración
  - Archivos: `cmd/admin/{recompute_reco.go, seed_interactions.go, eval_reco.go}`
  - Agente: `dev-go` · Depende de: T005
  - Aceptación: `recompute-recommendations`; `seed-interactions` (sintético con patrones, se niega a correr en producción); `eval-recommendations` con Hit@10, Recall@10 y MRR de híbrido, colaborativo, contenido y popularidad; semilla fija
  - Refs: RF-06, RF-07, RF-08 · P-06, P-07

- [ ] **T008** Evaluación y documentación académica
  - Archivos: `docs/ml/evaluacion.md`, `docs/ml/modelo.md`
  - Agente: `spec-writer` · Depende de: T007
  - Aceptación: resultados reales de la evaluación (CA-04), fórmulas, supuestos, límites y declaración de que los datos son sintéticos; si el híbrido no supera a popularidad se explica y se ajustan α y pesos
  - Refs: RF-07, RF-11

- [ ] **T009** Prueba de integración
  - Archivos: `internal/reco/integration_test.go`
  - Agente: `qa-go` · Depende de: T006
  - Aceptación: CA-01 a CA-07 de punta a punta; limpia sus datos
  - Refs: RNF-01

- [ ] **T010** Memoria y revisión
  - Archivos: `memoria.md` (solo anexar), `docs/produccion.md` (sección `RECO_ENABLED`)
  - Agente: `revisor-constitucion` · Depende de: T008, T009
  - Aceptación: **CONFORME**; BE-A confirma la lectura de `bookings`
  - Refs: DoD

- [ ] **T011** Despliegue de la spec en Azure (staging)
  - Archivos: `memoria.md` (solo anexar)
  - Agente: `devops-mcp` · Depende de: T010 (y todas las anteriores `[x]`)
  - Aceptación: `make fmt lint test` en verde; `scripts/deploy.sh staging` (construye, envía a la VM y reinicia la API); `/healthz` y `/readyz` devuelven 200 por HTTPS sin `-k`; cada endpoint nuevo de esta spec (`api/paths/<modulo>.yaml`) responde con una petición correcta y una inválida en el formato de error del Art. IV; registros sin secretos ni datos personales; resultado anotado en `memoria.md`. **La spec siguiente no empieza hasta que esta tarea pase.**
  - Refs: Art. XIII.5

| Total | 11 tareas |
|---|---|
