# Plan 015 — Recomendaciones con ML (backend)

- **Spec:** `spec.md` · **Dueño:** BE-B

## 1. Archivos
```
api/paths/recommendations.yaml
internal/reco/{domain.go, params.go, usecase.go, usecase_test.go}
internal/reco/model/{cooccurrence.go, content.go, hybrid.go, *_test.go}   (matemática pura, sin E/S)
internal/reco/adapter/mongo/{interactions.go, model_store.go, bookings_source.go}
internal/reco/adapter/http/{handler.go, dto.go, routes.go}
cmd/admin/{recompute_reco.go, seed_interactions.go, eval_reco.go}
cmd/api/modules.go (una línea) · config (RECO_ENABLED)
docs/ml/{modelo.md, evaluacion.md}
```

## 2. Colecciones MongoDB
| Colección | Documento | Índices |
|---|---|---|
| `reco_interactions` | `{ userId, itemId, type, weight, at }` | `{userId, at}`; TTL en `at` (90 d); único `{userId,itemId,type,bucket10min}` para limitar vistas |
| `reco_model` | `{ itemId, neighbors:[{itemId, cf, content}] (≤ 20), builtAt }` | `{itemId}` único |
| `reco_meta` | `{ _id:"model", builtAt, lock }` | — |

Las solicitudes se leen de `bookings` (solo lectura, campos estables según la spec 007).

## 3. Decisiones
| # | Decisión | Art. |
|---|---|---|
| P-01 | Matemática en el paquete `model`, funciones puras y deterministas (sin Mongo, sin hora actual) para pruebas exactas | Arquitectura limpia |
| P-02 | Similitud coseno sobre vectores dispersos con `map[string]float64`; sin librerías | II |
| P-03 | Arreglos acotados (20 vecinos) dentro del documento del objeto | Modelo Mongo |
| P-04 | Recalculo perezoso con candado en `reco_meta`; en Render gratuito no hay trabajos programados | — |
| P-05 | Lectura de `bookings` mediante un puerto `RequestSource` implementado en `reco/adapter/mongo`; excepción documentada y revisada por BE-A | XIII |
| P-06 | El modelo se evalúa con *leave-last-out* y se compara siempre contra popularidad | — |
| P-07 | Los datos sintéticos incluyen patrones reales (grupos por categoría y municipio) para que la evaluación sea significativa, y se declaran como sintéticos en el informe | — |

## 4. Pruebas
Pruebas exactas de coseno y del híbrido con matrices pequeñas; arranque en frío; exclusiones; integración con Mongo de pruebas; evaluación reproducible con semilla fija.

## 5. Verificación de constitución
| Art. | Cumple |
|---|---|
| II | Sin dependencias nuevas |
| VI | Sin datos personales en registros; TTL y borrado de historial |
| XIII | Módulo `reco`, una línea en `modules.go`, excepción de lectura documentada |
