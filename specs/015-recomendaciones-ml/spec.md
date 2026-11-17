# Spec 015 — Recomendaciones con machine learning (backend)

- **Dueño:** BE-B (con BE-A en revisión) · **Prioridad:** P1 · **Ventana:** 23 nov–1 dic · **Depende de:** 005, 006, 007, 013
- **Constitución:** v2.0.0 · **Estado:** Borrador

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
| RF-01 | **Eventos de interacción:** `POST /api/v1/recommendations/events` `{ itemId, type: "view" }` (autenticado). Las **solicitudes** se leen, en solo lectura, de las reservas existentes (`bookings`: solicitante, objeto, fecha). Pesos: solicitud = 3, vista = 1. Se ignoran vistas de objetos propios y se limita a 1 vista por objeto cada 10 min. |
| RF-02 | **Modelo colaborativo:** matriz persona×objeto dispersa (últimos 90 días); similitud **coseno objeto-objeto**; por objeto se guardan los 20 vecinos más parecidos en la colección `reco_model` (arreglo acotado). |
| RF-03 | **Modelo de contenido:** vector por objeto con categoría, modalidad, rango de tarifa y municipio (codificación one-hot); similitud coseno. |
| RF-04 | **Híbrido:** `score = α·colaborativo + (1−α)·contenido` con `α = 0.7` (constante documentada); si la persona tiene < 2 interacciones (**arranque en frío**) se usan los más populares de su municipio y de las categorías que ha visto; sin datos, los más populares. |
| RF-05 | `GET /api/v1/recommendations?limit=10` devuelve objetos activos con `reason` (`similar_to` con el título de un objeto visto, `popular_in_category`, `popular_nearby` o `popular`). Se excluyen: objetos propios, pausados o eliminados, de cuentas suspendidas o eliminadas, y reservas activas de la persona. Máximo 3 por categoría (diversidad). |
| RF-06 | **Recalculo del modelo:** al arrancar el comando `recompute-recommendations` y de forma perezosa si el modelo tiene más de 6 h (con candado para que solo un proceso lo recalcule). Con 5 000 objetos y 50 000 interacciones tarda < 5 s. |
| RF-07 | **Evaluación offline** `go run ./cmd/admin eval-recommendations`: *leave-last-out* (se oculta la última interacción de cada persona) y se calculan **Hit@10, Recall@10 y MRR** del híbrido, del colaborativo, del contenido y de la **línea base de popularidad**; el resultado se guarda en `docs/ml/evaluacion.md`. |
| RF-08 | **Datos de demostración:** `seed-interactions` genera personas e interacciones **sintéticas con patrones por categoría**, solo en staging y marcadas como demo; sirve para evaluar y demostrar. Nunca se ejecuta en producción. |
| RF-09 | **Privacidad:** los eventos tienen índice TTL de 90 días; `DELETE /api/v1/recommendations/history` borra los de la persona; al anonimizar una cuenta los eventos dejan de asociarse a datos personales; los registros no incluyen eventos ni objetos vistos. |
| RF-10 | **Interruptor** `RECO_ENABLED`; `GET /api/v1/meta` expone `features.recommendations`. Apagado, `GET /recommendations` responde `service_unavailable`. |
| RF-11 | **Documentación académica** `docs/ml/modelo.md`: datos, fórmulas, supuestos, límites (sesgo de popularidad, pocos datos), por qué no se usa una red neuronal. |
| RNF-01 | p95 de `GET /recommendations` < 400 ms con el modelo ya calculado. |
| RNF-02 | Cero dependencias nuevas: álgebra con `map`/`slice` de la biblioteca estándar. |

## 4. Criterios de aceptación
| ID | Dado / Cuando / Entonces |
|---|---|
| CA-01 | Dadas interacciones de una persona con herramientas eléctricas, entonces las recomendaciones incluyen sobre todo esa categoría y objetos vistos por personas similares. |
| CA-02 | Dada una persona nueva sin historial, entonces recibe populares de su municipio (arranque en frío), nunca una lista vacía si hay objetos. |
| CA-03 | Dado un objeto propio, pausado o de cuenta suspendida, entonces nunca aparece. |
| CA-04 | Dada la evaluación con los datos de demostración, entonces el híbrido supera a la línea base de popularidad en Hit@10; si no, se documenta honestamente y se ajustan α y pesos. |
| CA-05 | Dado `DELETE history`, entonces los eventos de esa persona desaparecen y las recomendaciones vuelven al arranque en frío. |
| CA-06 | Dado `RECO_ENABLED=false`, entonces `meta` lo indica y el endpoint responde `service_unavailable`. |
| CA-07 | Dado el repositorio, entonces no hay datos personales ni secretos. |

## 5. Fuera de alcance
Redes neuronales, embeddings externos, entrenamiento en línea, pruebas A/B, notificaciones de recomendaciones, recomendaciones de personas.

## 6. Preguntas abiertas
- Resuelto (29 oct): no hay un algoritmo obligatorio; se usa ML clásico (filtrado colaborativo objeto-objeto + contenido), lo importante es que sirva a la app y se pueda explicar y medir.
- [NEEDS CLARIFICATION: pesos y α definitivos; provisional 3/1 y 0.7, a ajustar con la evaluación (RF-07).]
