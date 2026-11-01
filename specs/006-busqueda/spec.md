# Spec 006 — Búsqueda (backend)

- **Dueño:** BE-B · **Prioridad:** P0 · **Ventana:** 3–5 nov · **Depende de:** 005 (modelo `items`)
- **Constitución:** v1.3.0 · **Políticas:** POL-14 · **Estado:** Borrador

## 1. Objetivo y por qué
Que una persona encuentre objetos por texto, categoría, zona, modalidad y disponibilidad en el periodo que necesita, con resultados ordenados y paginados, sin exponer datos sensibles.

## 2. Historias
| # | Prio. | Historia |
|---|---|---|
| H-01 | P1 | Como solicitante, quiero buscar por texto y filtrar por categoría y modalidad, para encontrar lo que necesito. |
| H-02 | P1 | Como solicitante, quiero ver solo objetos disponibles en mis fechas, para no perder tiempo. |
| H-03 | P1 | Como solicitante, quiero ver objetos cercanos y ordenarlos por cercanía, para recogerlos fácil. |
| H-04 | P2 | Como solicitante, quiero ver la reputación de quien ofrece, para confiar. |

## 3. Requisitos
| ID | Requisito |
|---|---|
| RF-01 | `GET /api/v1/items` con filtros opcionales: `q`, `category`, `modality`, `municipality`, `from` y `to` (fechas `YYYY-MM-DD`, ambas o ninguna), `lat`, `lng`, `radiusKm` (1–50), `limit` (1–50, por defecto 20) y `cursor`. |
| RF-02 | Solo devuelve objetos `active` de propietarios con cuenta activa; **excluye los del propio usuario**. |
| RF-03 | Con `from`/`to` excluye objetos que tengan una reserva `accepted` o `in_progress` que se cruce con el periodo (una solicitud pendiente no bloquea). |
| RF-04 | `q` busca en título y descripción en español (sin acentos ni mayúsculas) y ordena por relevancia. |
| RF-05 | Con `lat`,`lng` agrega `distanceKm` (1 decimal) y ordena por distancia; sin ellos, por fecha de publicación descendente. |
| RF-06 | Cada resultado es una **tarjeta**: `id`, `title`, `category`, `modality`, `pricePerDayCents`, `zone`, `approxLocation`, `distanceKm|null`, `thumbUrl|null`, `photoCount`, `owner{id,displayName,ratingAverage,ratingCount}`, `createdAt`. |
| RF-07 | Paginación por cursor opaco estable; `nextCursor=null` al final. |
| RF-08 | Parámetros inválidos (fechas incoherentes, `from` en el pasado, `radiusKm` fuera de rango, `lat` sin `lng`) → 400 `validation_failed` con `details`. |
| RF-09 | Tiempo de respuesta p95 < 500 ms con 5,000 objetos en entorno local. |
| RNF-01 | Cobertura ≥ 80 % en `search` (usecase y construcción de consultas). |

## 4. Criterios de aceptación
| ID | Dado / Cuando / Entonces |
|---|---|
| CA-01 | Dados 30 objetos activos, cuando pido `limit=10`, entonces recibo 10 y un `nextCursor`; con él, los 10 siguientes sin repetir. |
| CA-02 | Dado `q=taladro`, cuando busco, entonces aparecen «Taladro» y «taladros eléctricos» antes que descripciones lejanas. |
| CA-03 | Dado un objeto con reserva aceptada del 10 al 12, cuando busco `from=11&to=13`, entonces no aparece; con `from=13&to=14`, sí. |
| CA-04 | Dado `lat,lng`, cuando busco, entonces vienen ordenados por `distanceKm` ascendente. |
| CA-05 | Dado un objeto pausado, cuando busco, entonces no aparece. |
| CA-06 | Dado `lat` sin `lng`, entonces 400 con detalle. |

## 5. Fuera de alcance
Búsqueda difusa avanzada, sugerencias de autocompletado, historial de búsquedas, guardado de favoritos, IA.

## 6. Preguntas abiertas
- [NEEDS CLARIFICATION: ¿Ubicación por defecto cuando la persona no concede permiso? Provisional: sin orden por distancia.]
