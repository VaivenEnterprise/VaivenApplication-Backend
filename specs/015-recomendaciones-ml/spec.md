# Spec 015 — Recomendaciones con machine learning (backend)

- **Dueño:** BE-B (con BE-A en revisión) · **Prioridad:** P1 · **Ventana:** 23 nov–1 dic · **Depende de:** 003, 005, 006, 007, 010, 013 (y del `Meta.features` que agrega la 014)
- **Constitución:** v2.0.0 · **Estado:** Implementada, desplegada y verificada en staging (T011, 2026-11-17); aprobada por BE-B tras la revisión contra la constitución; BE-A confirmó la lectura de `bookings`, `items` y `users` (P-05)

## 1. Objetivo y por qué
La entrega oficial (1–4 dic) exige integrar **ML** además de IA generativa (spec 014). El ML de VAIVÉN son las **recomendaciones de objetos**: «Recomendado para ti» en Explorar. Se implementa un **recomendador híbrido propio** (filtrado colaborativo objeto-objeto + contenido) en Go y MongoDB, **sin servicios externos ni costo**, explicable en una defensa y **evaluado con métricas** contra una línea base de popularidad. Nunca reemplaza la búsqueda; solo sugiere.

## 2. Historias
| # | Prio. | Historia |
|---|---|---|
| H-01 | P1 | Como persona que busca, quiero ver objetos recomendados según lo que he visto y solicitado, para descubrir opciones sin escribir. |
| H-02 | P1 | Como persona usuaria, quiero saber por qué me recomiendan algo y poder borrar mi historial. |
| H-03 | P1 | Como equipo académico, queremos medir la calidad del modelo con métricas reproducibles. |
| H-04 | P1 | Como equipo, queremos apagar la función con una variable de entorno. |

## 3. Requisitos
| ID | Requisito |
|---|---|
| RF-01 | **Eventos de interacción:** `POST /api/v1/recommendations/events` `{ itemId, type: "view" }` (autenticado; la app lo envía al abrir el detalle de un objeto). Responde 204. Las **solicitudes** se leen, en solo lectura, de las reservas existentes (`bookings`: `requesterId`, `itemId`, `createdAt`). Pesos: solicitud = 3, vista = 1. Se ignoran vistas de objetos propios, de objetos inexistentes o no activos, y se cuenta como máximo **1 vista por objeto cada 10 min** (las repetidas responden 204 sin guardar nada). Límite de tasa: 60 eventos por minuto por persona con `ratelimit.Limiter` (Redis; Art. XIV.2); al excederlo, 429 `rate_limited`. |
| RF-02 | **Modelo colaborativo:** matriz persona×objeto dispersa (últimos 90 días; peso máximo por par persona-objeto); similitud **coseno objeto-objeto**; por objeto se guardan los 20 vecinos más parecidos en `reco_neighbors` (arreglo acotado, §6.2). |
| RF-03 | **Modelo de contenido:** vector por objeto con categoría, modalidad (`loan`/`rent`), rango de tarifa diaria (préstamo; ≤ $50; $50.01–$200; $200.01–$500; > $500, en centavos) y municipio (codificación one-hot); similitud coseno. Solo se comparan objetos que comparten categoría o municipio, para que el cálculo sea acotado. |
| RF-04 | **Híbrido:** para cada candidato `c`, `score(c) = Σ_h w(h)·[α·cf(h,c) + (1−α)·contenido(h,c)]` sobre los objetos `h` del historial de la persona (los 20 más recientes), con `α = 0.6` (constante documentada; era 0.7 provisional y se ajustó una sola vez con la evaluación, ver `docs/ml/evaluacion.md`). Si la persona tiene < 2 interacciones (**arranque en frío**) se usan los más populares de su municipio y de las categorías que ha visto; sin datos, los más populares. El municipio es el parámetro opcional `municipality` (lo envía la app con la zona que muestra en Explorar); **la ubicación exacta nunca se usa ni se guarda** (Art. VI.4). |
| RF-05 | `GET /api/v1/recommendations?limit=10[&municipality=…]` (autenticado; `limit` 1–20, 10 por defecto) devuelve `{ items: [{ item: ItemCard, reason }], nextCursor: null }` (formato de lista del Art. IV.4; no hay más páginas). `item` es la **misma tarjeta pública** de la búsqueda (spec 006: zona y punto aproximado, nunca la ubicación exacta). `reason` es `{ code, itemId?, itemTitle? }` con `code` = `similar_to` (con el objeto visto o solicitado que más contribuyó), `popular_in_category`, `popular_nearby` o `popular`; la app arma el texto en es-MX. Se excluyen: objetos propios, no activos (`paused`, `blocked`, `removed`), de cuentas dadas de baja o suspendidas, y los que la persona tiene en una reserva activa (`pending`, `accepted`, `in_progress`). Máximo 3 por categoría (diversidad). Si hay objetos visibles, la lista nunca está vacía. |
| RF-06 | **Recálculo del modelo:** con el comando `go run ./cmd/admin recompute-recommendations` y de forma perezosa cuando el modelo tiene más de 6 h. El recálculo perezoso **nunca bloquea la petición**: la respuesta usa el modelo vigente (o el arranque en frío si aún no hay modelo) y el recálculo corre aparte con tiempo máximo de 60 s, protegido por un **candado en MongoDB** (`reco_states`, con vencimiento de 2 min, Art. XIV.2) para que solo un proceso recalcule. Con 5 000 objetos y 50 000 interacciones tarda < 5 s y usa < 100 MB de memoria (la VM tiene ~1 GiB; Art. XIV.5). Al recalcular, se purgan los eventos de cuentas dadas de baja. |
| RF-07 | **Evaluación offline** `go run ./cmd/admin eval-recommendations`: *leave-last-out* (se oculta la última interacción de cada persona) y se calculan **Hit@10, Recall@10 y MRR** del híbrido, del colaborativo, del contenido y de la **línea base de popularidad**. Por defecto corre sobre un conjunto **sintético en memoria** con semilla fija (reproducible, sin base de datos, sin datos personales); con `--source=db` usa los eventos de un entorno que no sea producción. El resultado se guarda en `docs/ml/evaluacion.md`. |
| RF-08 | **Datos de demostración:** `seed-interactions` genera interacciones **sintéticas con patrones por categoría y municipio** sobre los objetos existentes, con identificadores de persona sintéticos (`demo-NNN`, que no son cuentas) y marcadas con `demo: true`; `seed-interactions --purge` las borra. Solo corre en development, test y staging; **se niega en producción** (igual que `seed`, spec 012). |
| RF-09 | **Privacidad:** los eventos guardan solo identificadores (persona y objeto), tipo, peso y fecha; tienen índice TTL de 90 días. `DELETE /api/v1/recommendations/history` (204) borra los de la persona; las solicitudes no se borran porque son de la spec 007, pero dejan de contar para ella (se guarda en `reco_states` la fecha de borrado y solo cuentan las solicitudes posteriores). Al recalcular se purgan los eventos de cuentas dadas de baja. Los registros no incluyen eventos, objetos vistos ni identificadores de objetos. |
| RF-10 | **Interruptor** `RECO_ENABLED` (por defecto `false`); `GET /api/v1/meta` expone `features.recommendations`. Apagado, las tres rutas responden 503 `service_unavailable` y no se recalcula nada. Los índices de sus colecciones se crean siempre al arrancar, aunque esté apagado, para que `admin indexes` no los marque como faltantes (aprendizaje de la 014). |
| RF-11 | **Documentación académica** `docs/ml/modelo.md`: datos, fórmulas, supuestos, límites (sesgo de popularidad, pocos datos, datos sintéticos), por qué no se usa una red neuronal. |
| RNF-01 | p95 de `GET /recommendations` < 400 ms con el modelo ya calculado (medido en staging con el modelo de los datos de demostración). |
| RNF-02 | Cero dependencias nuevas: álgebra con `map`/`slice` de la biblioteca estándar (Art. VIII.1). |
| RNF-03 | Cobertura ≥ 80 % en `domain` y `usecase` (Art. VII.1); la matemática del modelo, ≥ 95 %, con resultados exactos. |

## 4. Criterios de aceptación
| ID | Dado / Cuando / Entonces |
|---|---|
| CA-01 | Dadas interacciones de una persona con herramientas eléctricas, entonces las recomendaciones incluyen sobre todo esa categoría y objetos vistos por personas similares, con `reason.code = similar_to`. |
| CA-02 | Dada una persona nueva sin historial, entonces recibe populares de su municipio (arranque en frío), nunca una lista vacía si hay objetos visibles. |
| CA-03 | Dado un objeto propio, pausado, bloqueado, retirado, de cuenta suspendida o dada de baja, o en una reserva activa de la persona, entonces nunca aparece. |
| CA-04 | Dada la evaluación con los datos sintéticos, entonces el híbrido supera a la línea base de popularidad en Hit@10; si no, se documenta honestamente y se ajustan α y pesos. |
| CA-05 | Dado `DELETE history`, entonces los eventos de esa persona desaparecen, sus solicitudes anteriores dejan de contar y las recomendaciones vuelven al arranque en frío. |
| CA-06 | Dado `RECO_ENABLED=false`, entonces `meta` lo indica y las rutas responden `service_unavailable`. |
| CA-07 | Dado el repositorio, entonces no hay datos personales ni secretos; dados los registros de una petición, no aparecen eventos ni objetos vistos. |
| CA-08 | Dadas dos réplicas (o dos peticiones) que encuentran el modelo vencido al mismo tiempo, entonces solo una recalcula y ninguna petición espera el recálculo. |
| CA-09 | Dadas 61 vistas en un minuto, entonces la 61.ª recibe 429 `rate_limited`; dadas dos vistas del mismo objeto en 10 min, entonces solo se guarda una. |

## 5. Fuera de alcance
Redes neuronales, embeddings externos, entrenamiento en línea, pruebas A/B, notificaciones de recomendaciones, recomendaciones de personas, uso de la ubicación exacta, encender la función en producción (sale como v1.1 cuando el equipo lo decida, igual que la 014).

## 6. Preguntas abiertas
- Resuelto (29 oct): no hay un algoritmo obligatorio; se usa ML clásico (filtrado colaborativo objeto-objeto + contenido), lo importante es que sirva a la app y se pueda explicar y medir.
- Resuelto (17 nov, T010): con la evaluación, α pasa de 0.7 a 0.6 (mejor en Hit@10 y MRR, por poco) y los pesos 3/1 se mantienen porque los datos sintéticos no pueden medir si una solicitud pesa más que una vista. Justificación en `docs/ml/evaluacion.md` §4.
- Resuelto (17 nov): el municipio del arranque en frío lo envía la app (`municipality`); los usuarios no tienen municipio guardado y no se agrega a la spec 003.
- Resuelto (17 nov): BE-A confirmó la lectura en solo lectura de `bookings`, `items` y `users` (plan P-05).
- Pendiente para el móvil (MO-B): pantalla «Recomendado para ti», envío del evento de vista, borrar historial en Ajustes y mención en el aviso de privacidad y en «Seguridad de datos» de Play.

## 7. Revisión de conformidad (2026-11-17, BE-B)
Revisión de la spec, el plan y las tareas contra la constitución v2.0.0 antes de aprobar. Cambios hechos:

| # | Hallazgo | Art. | Cambio |
|---|---|---|---|
| 1 | Los archivos planos (`internal/reco/usecase.go`) quedaban fuera de la prueba de arquitectura, que revisa rutas `…/domain/…` y `…/usecase/…` | III | Capas `internal/reco/{domain, domain/model, usecase, adapter}`; los puertos en `usecase` |
| 2 | El recálculo perezoso dentro de la petición rompía el p95 < 400 ms | XIV.1, RNF-01 | Nunca bloquea: modelo vigente o arranque en frío, y recálculo aparte con candado en MongoDB con vencimiento |
| 3 | `POST events` no tenía límite de tasa | XIV.2 | 60 por minuto con `ratelimit.Limiter` y 429 |
| 4 | La respuesta no seguía el formato de lista | IV.4 | `{ items, nextCursor: null }` |
| 5 | No decía qué datos del objeto se exponen | VI.4, §6.11 | La misma tarjeta pública de la búsqueda (punto aproximado) |
| 6 | Las exclusiones copiarían los filtros de la búsqueda | XII.6 | La búsqueda (006, de BE-B) expone tarjetas por IDs con sus mismos filtros de visibilidad |
| 7 | `DELETE history` no podía borrar solicitudes (son de la 007) | VI, XII.1 | Se ignoran las solicitudes anteriores al borrado |
| 8 | Faltaban dependencias reales: suspensiones (010) y usuarios (003) | XII | Agregadas; todas desplegadas |
| 9 | Índices: el plan decía «en el comando `indexes`» | §6.5 | Catálogo completo (`catalog.go`, pruebas, fixture, `EnsureAll`), aprendido en la 014 |
| 10 | `createdAt`/`updatedAt` faltaban en los documentos | §6.4 | Agregados |
| 11 | La evaluación dependía de sembrar datos en una base | VII.2, VI | Por defecto, en memoria con semilla fija; sin datos personales y reproducible en la CI |
| 12 | `seed-interactions` creaba «personas» en `users` | VI, §6 | Identificadores sintéticos `demo-NNN`, sin cuentas, con `--purge` |
| 13 | T011 decía `scripts/deploy.sh staging` | XIII.5 (2.0.0) | Se verifica el despliegue automático al fusionar en `develop` |
| 14 | No había meta de cobertura | VII.1 | RNF-03 |
| 15 | Rama | XV | `spec/SPEC-15`, que parte de la rama de la 014 mientras se fusiona (XV.2), porque usa `Meta.features` |

Veredicto: **CONFORME** para aprobar, con dos condiciones que no bloquean la aprobación: BE-A confirma la lectura de `bookings`, y la implementación empieza cuando la 014 termine su despliegue verificado (Art. XIII.5).

## 8. Revisión de conformidad de la implementación (T010, 2026-11-17, BE-B)
Protocolo de `revisor-constitucion` sobre T001–T009. La hizo Claude a pedido del responsable, no el agente de OpenCode.

| Art. | Veredicto | Evidencia |
|---|---|---|
| I | ✅ | Spec revisada y aprobada (BE-B) antes del código; plan y tareas; T001 publicó el contrato antes de implementar |
| II | ✅ | Solo MongoDB (driver v2); colecciones `reco_interactions`, `reco_neighbors`, `reco_popular_lists` y `reco_states` |
| III | ✅ | `reco/domain` y `reco/usecase` no importan driver, chi, `net/http` ni Redis (`go list -deps`); la prueba de arquitectura los cubre; los puertos los declara `usecase` |
| IV | ✅ | Errores con el formato único; lista `{items, nextCursor: null}`; `redocly lint` válido |
| V | ✅ | Tarifas en centavos `int64`; motivos como códigos, textos es-MX en la app |
| VI | ⚠️ | El cuerpo de `POST events` usa `validator/v10`; los parámetros de `GET` (`limit`, `municipality`) se validan en el dominio con `validation_failed` por campo, no con `validator/v10`. Eventos con TTL de 90 días, borrado de historial y purga de cuentas dadas de baja; registros sin personas ni objetos (CA-07, comprobado sembrando un secreto falso) |
| VII | ✅ | Cobertura: domain 100 %, model 99,4 %, synth 100 %, usecase 99,6 %, http 96,6 %, search 100 %; mongo 84 % (integración). Integración con MongoDB y Redis reales; lint 0. Las pruebas de mutación de estos paquetes quedan para la spec 016 (`mutation.sh` aún no los recorre) |
| VIII | ✅ | Sin dependencias nuevas (`go.mod` sin cambios); álgebra con `map` y `slice` |
| IX | ✅ | `slog`; el recálculo registra solo conteos |
| XII | ✅ | Un módulo y una línea en `modules.go`; las tarjetas salen de la búsqueda (006, de BE-B) sin copiar filtros. BE-A confirmó la lectura en solo lectura de `bookings` (007), `items` (005) y `users` (003) el 2026-11-17 |
| XIII.5 | ✅ | T011: desplegada (`d699a05`) y verificada en staging por HTTPS el 2026-11-17 |
| XIV | ✅ | Sin estado compartido en memoria; candado en MongoDB con vencimiento (CA-08); límite de tasa en Redis (CA-09); recálculo en 0,5 s y unos 32 MB |
| XV | ✅ | Rama `spec/SPEC-15` y una rama por tarea |
| §6 | ✅ | Colecciones `snake_case` en plural (se renombraron `reco_model`, `reco_popular` y `reco_meta` en T010); `createdAt`/`updatedAt`; arreglos acotados (20 y 50); índices en el catálogo (40) |

**Veredicto: CONFORME** (actualizado el 2026-11-17 tras la confirmación de BE-A). Observaciones que no bloquean: (1) la validación de los parámetros de consulta en el dominio es la misma que usan las demás specs y no bloquea; (2) las pruebas de mutación de `reco` entran en la spec 016.
