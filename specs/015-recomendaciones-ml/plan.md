# Plan 015 — Recomendaciones con ML (backend)

- **Spec:** `spec.md` · **Dueño:** BE-B · **Constitución:** v2.0.0 · Revisado el 2026-11-17

## 1. Archivos
```
api/paths/recommendations.yaml · api/openapi.yaml (referencia y esquemas) · internal/adapter/http/docs/openapi.yaml (make docs-bundle)
internal/config/reco.go (+ prueba)                         RECO_ENABLED, por defecto false
internal/reco/domain/{event.go, params.go, reason.go, errors.go}
internal/reco/domain/model/{cosine.go, cooccurrence.go, content.go, hybrid.go, popular.go, metrics.go, *_test.go}   (matemática pura, sin E/S ni reloj)
internal/reco/domain/synth/{synth.go, synth_test.go}       conjunto sintético con semilla fija (evaluación y demostración)
internal/reco/usecase/{ports.go, events.go, recommend.go, history.go, recompute.go, eval.go, *_test.go}
internal/reco/adapter/mongo/{interactions.go, model_store.go, meta.go, requests_source.go, items_source.go, *_integration_test.go}
internal/reco/adapter/search/cards.go                      tarjetas por IDs con la búsqueda (spec 006)
internal/reco/adapter/http/{handler.go, dto.go, routes.go, handler_test.go}
internal/reco/integration_test.go
internal/search/... (CardsByIDs: repositorio, caso de uso y prueba; spec 006, también de BE-B)
internal/adapter/mongo/catalog/{catalog.go, purpose_test.go, catalog_test.go} · fixture de índices · cmd/admin (pruebas de índices)
cmd/admin/{reco_recompute.go, reco_seed.go, reco_eval.go, main.go (3 casos), *_test.go}
cmd/api/main.go (EnsureAll, cableado) · cmd/api/modules.go (una línea) · internal/adapter/http/handler/meta.go (features.recommendations)
docs/ml/{modelo.md, evaluacion.md} · docs/produccion.md (§12 RECO_ENABLED) · .env.example y deploy/*/.env.example
```

## 2. Colecciones MongoDB
| Colección | Documento | Índices |
|---|---|---|
| `reco_interactions` | `{ userId, itemId, type: "view", weight, bucket, demo, createdAt, updatedAt }` (`bucket` = inicio del bloque de 10 min) | único `{userId, itemId, type, bucket}` (límite de vistas: el duplicado se ignora); `{userId, createdAt: -1}` (historial y borrado); TTL en `createdAt` (90 d) |
| `reco_model` | `{ _id: itemId, neighbors: [{itemId, cf, content}] (≤ 20), buildId, createdAt, updatedAt }` | solo `_id` (toda lectura es por `_id`) |
| `reco_popular` | `{ _id: "all" \| "m:<municipio>" \| "c:<categoría>", items: [itemId] (≤ 50), buildId, createdAt, updatedAt }` | solo `_id` |
| `reco_meta` | `{ _id: "model", buildId, builtAt, lockedUntil, lockOwner, createdAt, updatedAt }` y `{ _id: "reset:<userId>", resetAt, createdAt, updatedAt }` | solo `_id` |

- Las solicitudes se leen de `bookings` en **solo lectura** (`requesterId`, `itemId`, `createdAt`, `status`). La exclusión de reservas activas usa el índice existente `requester_status_createdAt`. El recálculo recorre `bookings` de los últimos 90 días una vez cada 6 h como máximo: no es una consulta frecuente (§6.5) y no se agrega un índice a una colección de BE-A.
- Los objetos (categoría, modalidad, tarifa, municipio, estado, dueño) se leen de `items` en solo lectura para el modelo de contenido y para validar el evento.
- El catálogo de índices pasa de 37 a 40 (+3 en `reco_interactions`: único, historial y TTL; las demás colecciones solo usan `_id`).

## 3. Decisiones
| # | Decisión | Art. |
|---|---|---|
| P-01 | La matemática vive en `internal/reco/domain/model`: funciones puras y deterministas (sin MongoDB, sin hora actual, desempates por `itemId`) para pruebas exactas; al estar bajo `domain/`, la prueba de arquitectura la cubre | III, VII |
| P-02 | Similitud coseno sobre vectores dispersos con `map[string]float64`; sin bibliotecas | VIII.1 |
| P-03 | Arreglos acotados (20 vecinos, 50 populares) dentro del documento; el modelo se escribe con `buildId` nuevo y al final se borran los documentos de otro `buildId`, así nunca se lee un modelo a medias | §6.2 |
| P-04 | Recálculo perezoso que **no bloquea**: la petición que encuentra el modelo vencido intenta tomar el candado (`FindOneAndUpdate` en `reco_meta` con `lockedUntil < ahora`) y, si lo logra, lanza el recálculo en una gorrutina con contexto propio de 60 s; el candado vence a los 2 min por si el proceso muere. No hay trabajos programados ni estado compartido en memoria | XIV.1, XIV.2 |
| P-05 | Lectura de `bookings` mediante el puerto `RequestSource`, implementado en `reco/adapter/mongo`; solo lectura y solo los campos citados; **excepción documentada que confirma BE-A** (dueño de la 007), igual que la búsqueda (006) ya lee `bookings` | XII.1 |
| P-06 | Las tarjetas y los filtros de visibilidad salen de la búsqueda (spec 006): se agrega `CardsByIDs` con los mismos filtros que `Search` (objeto activo, dueño activo y no suspendido) y la respuesta usa `searchhttp.ToCard`. No se copia código | XII.6, VI.4 |
| P-07 | El modelo se evalúa con *leave-last-out* y se compara siempre contra popularidad; por defecto sobre datos sintéticos en memoria con semilla fija, para que la evaluación sea reproducible y corra en las pruebas | VII |
| P-08 | Los datos sintéticos tienen patrones (grupos por categoría y municipio) para que la evaluación sea significativa, y se declaran como sintéticos en el informe; las personas sintéticas son identificadores `demo-NNN`, no cuentas | VI |
| P-09 | El historial borrado: se eliminan los eventos y se guarda `reset:<userId>` en `reco_meta`; las solicitudes anteriores a esa fecha no cuentan | VI, XII.1 |
| P-10 | Límite de tasa de `POST events` con el `ratelimit.Limiter` (Redis) y su respaldo local, igual que la 014; sin caché de recomendaciones (no hace falta para el p95 y evita invalidaciones; Art. VIII.2) | XIV.2, VIII.2 |
| P-11 | Memoria del recálculo acotada: se leen como máximo 200 000 interacciones (las más recientes) y se mide con un *benchmark* (5 000 objetos y 50 000 interacciones: < 5 s y < 100 MB) | XIV.5 |
| P-12 | `RECO_ENABLED=false` por defecto; los índices se crean siempre | — |

## 4. Pruebas
- Unitarias exactas: coseno, coocurrencia y contenido con matrices pequeñas calculadas a mano; híbrido; popularidad; Hit@10, Recall@10 y MRR con casos conocidos; arranque en frío; exclusiones; diversidad; `reason`.
- Caso de uso con dobles de los puertos: el recálculo perezoso no bloquea y solo uno toma el candado (CA-08); límite de vistas; borrado de historial (CA-05).
- Integración con MongoDB real (testcontainers): repositorios, índices, TTL, candado concurrente y `CardsByIDs`.
- Integración de punta a punta (`internal/reco/integration_test.go`) con MongoDB y Redis reales: CA-01 a CA-09.
- Evaluación reproducible con semilla fija (dos ejecuciones dan el mismo informe).

## 5. Verificación de constitución
| Art. | Cumple |
|---|---|
| I, X.5 | Spec, plan y tareas revisados; se implementa solo tras la aprobación |
| III | Capas `domain` → `usecase` → `adapter` → `cmd`; puertos en `usecase`; sin `bson`/`mongo` en `domain` ni `usecase` |
| IV | Contrato antes del código (T001); errores con el formato único; lista con `nextCursor` |
| V.4 | `reason` como código; los textos en es-MX los arma la app; mensajes de error en es-MX |
| VI | Solo identificadores en los eventos; TTL de 90 días; borrado de historial; tarjeta pública sin ubicación exacta; registros sin eventos ni objetos vistos |
| VII | Cobertura ≥ 80 % en `domain` y `usecase`; integración con MongoDB real |
| VIII | Sin dependencias nuevas |
| XII | Módulo `reco`, una línea en `modules.go`; lectura de `bookings` confirmada por BE-A; tarjetas desde la búsqueda sin copiar código |
| XIII.5 | Termina con el despliegue verificado en staging (T011) |
| XIV | Sin estado en memoria; candado en MongoDB; límite de tasa en Redis; memoria acotada |
| XV | Rama `spec/SPEC-15` (parte de la de la 014 mientras se fusiona) |
