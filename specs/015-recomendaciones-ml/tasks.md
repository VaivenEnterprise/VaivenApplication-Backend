# Tareas 015 — Recomendaciones con ML (backend)

- **Dueño:** BE-B · **Estado:** completa (11/11), desplegada y verificada en staging el 2026-11-17 · `[P]` = paralelizable
- Ventana del ROADMAP: 23 nov–1 dic (v1.1, detrás de `RECO_ENABLED`). No empieza hasta que la 014 termine su despliegue verificado (Art. XIII.5) y la spec esté aprobada (Art. I).
- Cada tarea termina con `make fmt lint test` (y `go test -tags=integration` del paquete si tiene integración) en verde.

- [x] **T001** Contrato OpenAPI
  - Archivos: `api/paths/recommendations.yaml`, `api/openapi.yaml` (referencia, esquemas `RecommendationEvent`, `Recommendation`, `RecommendationReason`, lista y `Meta.features.recommendations`), `internal/adapter/http/docs/openapi.yaml` (`make docs-bundle`)
  - Agente: `spec-writer` · Depende de: 006-T001, 014-T001
  - Aceptación: `POST events` (204, 400, 401, 404, 429, 503), `GET recommendations` (`limit`, `municipality`; lista `{items, nextCursor: null}` con `ItemCard` y `reason`), `DELETE history` (204, 401, 503); errores con el formato del Art. IV; MO-B lo consume el mismo día
  - Refs: RF-01, RF-05, RF-09, RF-10
  - **Hecho (2026-11-17, BE-B):** `api/paths/recommendations.yaml` (tres rutas, etiqueta `recomendaciones`), esquemas `RecommendationEventRequest`, `RecommendationReason(Code)`, `Recommendation` y `RecommendationList` (tarjeta `ItemCard` de la 006), `Meta.features.recommendations` y bundle de `/docs` regenerado. `redocly lint` válido (solo las 2 advertencias que ya existían) y `go test ./...` en verde.

- [x] **T002** [P] Dominio, parámetros y configuración
  - Archivos: `internal/reco/domain/{event.go, params.go, reason.go, errors.go}` y pruebas, `internal/config/reco.go` y prueba (`RECO_ENABLED`, por defecto `false`), `TestLoadDefaults`, `.env.example` y `deploy/*/.env.example`
  - Agente: `dev-go` · Depende de: T001
  - Aceptación: constantes con nombre (`WeightRequest=3`, `WeightView=1`, `Alpha=0.7`, `Neighbors=20`, `HistorySize=20`, `WindowDays=90`, `MaxPerCategory=3`, `ViewBucket=10m`, `ModelMaxAge=6h`); bandas de tarifa en centavos; validación de evento y de `limit`; cobertura ≥ 80 %
  - Refs: RF-01–RF-04, RF-10 · P-01, P-12
  - **Hecho (2026-11-17, BE-B):** `internal/reco/domain` (`params.go`, `event.go`, `reason.go`, `errors.go`; 100 %) con las constantes del modelo, bandas de tarifa, `NewView` (valida `itemId` y `type`, bloque de 10 min en UTC), `ParseLimit`, `CleanMunicipality`, motivos y errores; `internal/config/reco.go` (`RECO_ENABLED`, apagado por defecto; 100 %), `TestLoadDefaults` y los tres `.env.example`. Las interfaces (`InteractionStore`, `ModelStore`, `RequestSource`) se declaran en `usecase` en T006, porque los puertos los declara quien los consume (Art. III.3; revisión de la spec). `gofmt`, `go vet`, `golangci-lint` (0), prueba de arquitectura y `go test ./...` en verde.

- [x] **T003** [P] Repositorios MongoDB e índices
  - Archivos: `internal/reco/adapter/mongo/{interactions.go, model_store.go, meta.go, requests_source.go, items_source.go}` y pruebas de integración; catálogo (`catalog.go`, `purpose_test.go`, conteo en `catalog_test.go`), fixture (`VAIVEN_UPDATE_INDEX_BASELINE=1`), pruebas de índices de `cmd/admin`, siembra genérica de `catalog_integration_test.go`, `EnsureAll` en `cmd/api/main.go`
  - Agente: `modelador-mongo` · Depende de: T002
  - Aceptación: TTL de 90 días; vista duplicada en el mismo bloque de 10 min ignorada por el índice único; candado con vencimiento y una sola toma entre varias concurrentes; escritura del modelo por `buildId` sin lecturas a medias; `bookings` e `items` solo lectura; arreglos acotados; catálogo en 40 índices con `go test -tags=integration ./internal/adapter/mongo/catalog/` en verde
  - Refs: RF-01, RF-06, RF-09 · P-03, P-04, P-05, P-09
  - **Hecho (2026-11-17, BE-B):** `internal/reco/adapter/mongo`: `InteractionRepo` (vista repetida en el mismo bloque → `false` por el índice único; historial, ventana, borrado, purga de cuentas dadas de baja, demostración con `--purge`), `ModelRepo` (vecinos ≤ 20 y populares ≤ 50 por `buildId`; el nuevo convive con el anterior hasta `DeleteOtherBuilds`), `MetaRepo` (candado atómico con vencimiento: de 20 procesos a la vez gana exactamente 1; `Publish` solo si el candado sigue siendo suyo; fechas de borrado de historial), `RequestSource` (bookings) e `ItemSource` (items y users), estos dos en solo lectura comprobada. Tipos `Neighbor` e `ItemFeatures` en el dominio. Catálogo en **40** índices (`catalog.go`, propósitos, consulta representativa, siembra genérica con `type` y `bucket`, conteos de `cmd/admin`), fixture regenerado (solo +3 entradas) y `EnsureAll`. `go test -tags=integration ./...` completo en verde, lint 0, dominio 100 %, adaptador 84 % (integración).

- [x] **T004** Modelo colaborativo, de contenido, híbrido y métricas
  - Archivos: `internal/reco/domain/model/{cosine.go, cooccurrence.go, content.go, hybrid.go, popular.go, metrics.go}`, `internal/reco/domain/synth/synth.go` y pruebas
  - Agente: `dev-go` · Depende de: T002
  - Aceptación: coseno objeto-objeto y de contenido con resultados exactos en matrices pequeñas; híbrido `α·cf + (1−α)·contenido`; populares por municipio y categoría; Hit@10, Recall@10 y MRR exactos en casos conocidos; deterministas (mismos datos y semilla → mismo resultado); *benchmark* con 5 000 objetos y 50 000 interacciones < 5 s y < 100 MB; cobertura ≥ 95 %
  - Refs: RF-02, RF-03, RF-04, RF-06, RF-07 · P-01, P-02, P-07, P-08, P-11
  - **Hecho (2026-11-17, BE-B):** `internal/reco/domain/model` (coseno, colaborativo objeto-objeto con índice invertido y acumulador denso, contenido one-hot por bloques de categoría y municipio, `MergeNeighbors`, `Rank`, `ColdStart`, `Diversify`, `Popular`, `LeaveLastOut` y `Evaluate`; 99,4 %) y `internal/reco/domain/synth` (semilla fija, gustos por categoría y municipio, popularidad sesgada; 100 %). Pruebas exactas calculadas a mano (p. ej. cf(A,B) = 4/√30). Prueba de control reproducible: híbrido Hit@10 = 0,257 contra 0,087 de popularidad. *Benchmark* (5 000 objetos, 50 000 interacciones): **0,5 s y 31,7 MB asignados por recálculo** (cota superior del pico; límite 5 s y 100 MB). `golangci-lint` 0; `math/rand/v2` con semilla fija justificado con `nolint:gosec` (no es para seguridad).

- [x] **T005** Tarjetas por IDs en la búsqueda (spec 006)
  - Archivos: `internal/search/usecase` y `internal/search/adapter/mongo` (`CardsByIDs` con los mismos filtros de visibilidad que `Search`) y pruebas, `internal/reco/adapter/search/cards.go`
  - Agente: `dev-go` · Depende de: T002
  - Aceptación: devuelve tarjetas públicas en el orden pedido; excluye objetos no activos y de dueños dados de baja o suspendidos; un ID mal formado no rompe la consulta; sin copiar el *pipeline* de la búsqueda
  - Refs: RF-05, CA-03 · P-06
  - **Hecho (2026-11-17, BE-B):** `BuildByIDsPipeline` reutiliza `ownerStages` y `projectStage` de la búsqueda (sin copiar filtros); `Repo.CardsByIDs` devuelve en el orden pedido, sin repetidos, y omite ids inválidos o no visibles; `Service.CardsByIDs` valida ids, acota a 50 y agrega la miniatura con el mismo código que `Search` (extraído a `views`); `internal/reco/adapter/search.Cards` (100 %). Integración con MongoDB real: excluye pausados, propios, cuentas dadas de baja y suspendidas. Búsqueda y asistente siguen en verde; lint 0.

- [x] **T006** Casos de uso: eventos, recomendar, historial y recálculo
  - Archivos: `internal/reco/usecase/{ports.go, events.go, recommend.go, history.go, recompute.go, eval.go}` y pruebas
  - Agente: `dev-go` · Depende de: T003, T004, T005
  - Aceptación: CA-01 a CA-03, CA-05 y CA-08 con dobles; arranque en frío; exclusiones (propios, reservas activas); diversidad (≤ 3 por categoría); `reason`; el recálculo perezoso nunca bloquea y solo uno toma el candado; purga de eventos de cuentas dadas de baja; cobertura ≥ 80 %
  - Refs: RF-01, RF-04–RF-07, RF-09 · P-04, P-09
  - **Hecho (2026-11-17, BE-B):** `internal/reco/usecase` (99,6 %): puertos declarados aquí (Art. III.3); `RecordView` y `DeleteHistory` en `service.go` (en lugar de `events.go` e `history.go`, por ser cortos); `Recommend` (historial de vistas y de solicitudes posteriores al borrado, híbrido con ≥ 2 objetos, arranque en frío por municipio, categorías vistas y generales, exclusión de historial y reservas activas, tarjetas desde la búsqueda, ≤ 3 por categoría, motivo con título; sin modelo todavía calcula populares al momento); `Recompute` (candado, ventana de 90 días, borrados de historial, purga de cuentas dadas de baja, `Build` puro, publicación solo si el candado sigue siendo suyo); recálculo perezoso con `Deps.Go`, que nunca bloquea la petición; `Evaluate` de los cuatro modelos. CA-01, CA-02, CA-03, CA-05 y CA-08 con dobles en memoria. Ajuste en T003: `Titles` pasó a `Briefs` (título y categoría).

- [x] **T007** Handlers, rutas, `/meta` y registro
  - Archivos: `internal/reco/adapter/http/{handler.go, dto.go, routes.go}` y pruebas, `internal/adapter/http/handler/meta.go` (`features.recommendations`), `cmd/api/modules.go` (una línea), cableado en `cmd/api/main.go`
  - Agente: `dev-go` · Depende de: T001, T006
  - Aceptación: cumple el contrato; `validator/v10` en la frontera; límite de 60 eventos por minuto con 429 y `Retry-After` (CA-09); interruptor (CA-06); borrar historial (CA-05); registros sin eventos, objetos vistos ni IDs de objetos
  - Refs: RF-01, RF-05, RF-09, RF-10 · P-10
  - **Hecho (2026-11-17, BE-B):** `internal/reco/adapter/http/handler.go` (96,6 %; DTO y rutas en el mismo archivo por ser cortos): `POST /recommendations/events` con `validator/v10` y `DisallowUnknownFields`, 60 por minuto por persona con 429 y `Retry-After` (CA-09); `GET /recommendations` con la tarjeta de la búsqueda y `nextCursor: null`; `DELETE /recommendations/history`; apagado, las tres responden 503 antes de validar (CA-06); los errores internos se registran sin el objeto (CA-07). `/meta` expone `features.recommendations`; una línea en `modules.go` y `newRecoModule` en `main.go`. Lint 0 y `go test -tags=integration ./...` en verde.

- [x] **T008** Comandos de administración
  - Archivos: `cmd/admin/{reco_recompute.go, reco_seed.go, reco_eval.go}`, `cmd/admin/main.go` (tres casos) y pruebas
  - Agente: `dev-go` · Depende de: T006
  - Aceptación: `recompute-recommendations`; `seed-interactions` (sintético con patrones, `demo: true`, personas `demo-NNN`, `--purge`, se niega en producción); `eval-recommendations` con Hit@10, Recall@10 y MRR de híbrido, colaborativo, contenido y popularidad, en memoria con semilla fija por defecto y `--source=db` fuera de producción; misma salida en dos ejecuciones
  - Refs: RF-06, RF-07, RF-08 · P-07, P-08
  - **Hecho (2026-11-17, BE-B):** `cmd/admin/reco_{eval,recompute,seed,deps}.go` y tres casos en `main.go`. `eval-recommendations`: por defecto sintético en memoria, sin base ni configuración, salida idéntica en dos corridas (tabla Markdown para `docs/ml/evaluacion.md`); `--source db` lee los últimos 90 días y se niega en producción. `recompute-recommendations`: usa el mismo candado y solo imprime conteos; no necesita Cloudinary. `seed-interactions`: personas `demo-NNN` sobre los objetos activos, fechas dentro de la ventana, `--purge`, y se niega en producción **antes de conectar**. Pruebas unitarias con dobles e integración real de los tres comandos (sembrar → recalcular → evaluar → purgar). Lint 0 y `go test -tags=integration ./...` en verde.

- [x] **T009** Prueba de integración de punta a punta
  - Archivos: `internal/reco/integration_test.go`
  - Agente: `qa-go` · Depende de: T007, T008
  - Aceptación: CA-01 a CA-09 con MongoDB y Redis reales; limpia sus datos; CA-07 busca secretos y datos personales con `git grep` (rutas `:(top)`, `--untracked`, se excluye a sí misma)
  - Refs: CA-01–CA-09, RNF-03
  - **Hecho (2026-11-17, BE-B):** `internal/reco/integration_test.go` con rutas reales, JWT, MongoDB 7 y Redis 7 (límite de tasa real), búsqueda de la 006 para las tarjetas; cada prueba con su base, que se borra. CA-01 (lo primero es una herramienta vista por personas parecidas, motivo con título, 3 por categoría), CA-02 (Pachuca primero; sin modelo responde al momento y el recálculo perezoso publica después), CA-03 (nunca propios, pausados, de cuentas suspendidas o dadas de baja, del historial ni en reserva pendiente), CA-04, CA-05, CA-06 (`/meta` y 503), CA-07 (registros sin objetos ni personas; `git grep` de claves y correos, comprobado con uno sembrado), CA-08 (candado ajeno: nadie publica y ninguna petición espera), CA-09 (vista repetida guardada una vez; la 61.ª da 429 en Redis) y RNF-01 (p95 local ≈ 6 ms). Lint 0; `go test -tags=integration ./...` en verde.

- [x] **T010** Evaluación, documentación académica, memoria y revisión
  - Archivos: `docs/ml/evaluacion.md`, `docs/ml/modelo.md`, `docs/produccion.md` (§12 `RECO_ENABLED` y datos de demostración), `memoria.md` (solo anexar)
  - Agente: `spec-writer` y `revisor-constitucion` · Depende de: T008, T009
  - Aceptación: resultados reales de la evaluación (CA-04) con α y pesos finales justificados, fórmulas, supuestos, límites y declaración de que los datos son sintéticos; si el híbrido no supera a popularidad, se explica y se ajustan α y pesos; veredicto **CONFORME**; BE-A confirma la lectura de `bookings`
  - Refs: RF-07, RF-11, DoD
  - **Hecho (2026-11-17, BE-B):** `docs/ml/evaluacion.md` (resultados reales; híbrido Hit@10 0,263 frente a 0,087 de popularidad; barrido de α con el pipeline real), `docs/ml/modelo.md` (datos, fórmulas, supuestos, límites y por qué no una red neuronal) y `docs/produccion.md` §12. **α pasa de 0.7 a 0.6** (mejor por poco en Hit@10 y MRR) y los pesos 3/1 se mantienen (los datos sintéticos no pueden medirlos). Colecciones renombradas en plural (§6.1). Revisión de conformidad en `spec.md` §8: **CONFORME CON OBSERVACIONES**. BE-A confirmó la lectura en solo lectura de `bookings`, `items` y `users` (el responsable lo informó el 2026-11-17: «Sí, ya confirmó»).

- [x] **T011** Despliegue de la spec en Azure (staging)
  - Archivos: `memoria.md` (solo anexar), `docs/produccion.md` (registro)
  - Agente: `devops-mcp` · Depende de: T010 (y todas las anteriores `[x]`)
  - Aceptación: `make fmt lint test` en verde; PR `spec/SPEC-15` → `develop` fusionado y `deploy-staging.yml` en verde (Art. XIII.5, enmienda 2.0.0); `RECO_ENABLED=true` en el `.env` de staging; `seed-interactions` y `recompute-recommendations` en staging; `/healthz` y `/readyz` responden 200 por HTTPS sin `-k`; cada endpoint de `api/paths/recommendations.yaml` responde con una petición correcta y una inválida en el formato del Art. IV; p95 de `GET /recommendations` < 400 ms (RNF-01); registros sin secretos ni datos personales; cuenta de prueba borrada al terminar; resultado anotado en `memoria.md`. **La spec siguiente no empieza hasta que esta tarea pase.**
  - Refs: Art. XIII.5, RNF-01
  - **Hecho (2026-11-17, BE-B):** PR #101 (`spec/SPEC-15` → `develop`); CI, Smoke, Mutation y `deploy-staging.yml` en verde para `d699a05`. En staging (Cloud Shell): `set-reco.sh staging --enable` (nuevo; solo cambia `RECO_ENABLED`, recrea la API con la misma imagen y espera `/meta`), `seed-interactions` (1 517 interacciones de 300 personas sintéticas sobre los 3 objetos activos) y `recompute-recommendations` (1 524 interacciones, 3 objetos, 4 cuentas dadas de baja purgadas). Verificación por HTTPS sin `-k`: `/healthz` y `/readyz` 200 (Redis `ok`); `/meta` con `recommendations: true`; `GET` 200 (arranque en frío, `nextCursor: null`) y 400 con `limit=0`; `POST events` 204, 400 y 404; `DELETE history` 204 y 401 sin sesión. **p95 de `GET /recommendations` = 205 ms** con conexión reutilizada (p50 149 ms; la red sola, medida con `/meta`, da p50 126 ms: el servidor tarda unos 20 ms). Registros (`check-logs.sh staging 1h`): 190 líneas, 0 contraseñas, tokens, claves, correos de prueba ni ids de objeto. Cuentas de prueba dadas de baja.

| Total | 11 tareas |
|---|---|
