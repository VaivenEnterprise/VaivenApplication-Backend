# Spec 005 — Objetos (backend)

- **Dueño:** BE-A · **Prioridad:** P0 · **Ventana:** 3–5 nov · **Depende de:** 003, 004
- **Constitución:** v1.3.0 · **Políticas:** POL-01, POL-04, POL-13, POL-14, POL-16 · **Estado:** Borrador

## 1. Objetivo y por qué
Permitir que una persona publique objetos disponibles (préstamo gratuito o renta), los edite y los pause para controlar futuras solicitudes. Es el inventario sobre el que se construyen búsqueda, solicitudes y reservas.

## 2. Historias
| # | Prio. | Historia |
|---|---|---|
| H-01 | P1 | Como propietario, quiero publicar un objeto con fotos, estado, zona, modalidad y precio, para ofrecerlo. |
| H-02 | P1 | Como propietario, quiero editar o pausar mi publicación, para controlar nuevas solicitudes sin afectar compromisos existentes. |
| H-03 | P1 | Como propietario, quiero una sugerencia de precio, para no adivinarlo; y confirmarla antes de publicar. |
| H-04 | P1 | Como persona usuaria, quiero ver el detalle de un objeto con sus condiciones y quién lo ofrece, para decidir. |
| H-05 | P2 | Como propietario, quiero ver todos mis objetos por estado, para administrarlos. |

## 3. Requisitos
| ID | Requisito |
|---|---|
| RF-01 | `POST /api/v1/items` crea un objeto con: `title` (3–80), `description` (≤ 1000), `category` (POL-14), `condition` (`new`,`like_new`,`good`,`fair`,`worn`), `modality` (`loan`,`rent`), `declaredValueCents` ($50–$100,000), `pricePerDayCents`, `priceSource` (`manual`,`suggested`), `usageRules` (≤ 500, opcional), `zone` (`municipality` 2–60, `neighborhood` opcional), `approxLocation` (`lat`,`lng`, opcional), `photos` (1–5 `{publicId}`). |
| RF-02 | Préstamo (`loan`) exige `pricePerDayCents = 0`. Renta exige entre $10 y el valor declarado. |
| RF-03 | `GET /api/v1/items/price-suggestion?declaredValueCents=` devuelve la sugerencia de POL-04; **no** publica nada. Si `priceSource=suggested`, el precio enviado debe coincidir con la sugerencia (la persona la confirma enviándola). |
| RF-04 | Las coordenadas se **redondean a 2 decimales** en el servidor antes de guardar; la ubicación exacta no se almacena en este módulo. |
| RF-05 | Título, descripción y reglas pasan por `moderation.Checker`; si se rechaza → 400 `validation_failed` con `details` (campo y motivo en español). |
| RF-06 | `GET /api/v1/items/{id}` devuelve el detalle con `owner` (perfil público) y fotos con `url` y `thumbUrl`. Visible para cualquier persona autenticada si está `active`; `paused`/`blocked` solo para el dueño y admin; `removed` → 404. |
| RF-07 | `PATCH /api/v1/items/{id}` (solo dueño) edita campos; cambiar el precio **no** altera reservas existentes (viven con instantánea, spec 007). |
| RF-08 | `POST /api/v1/items/{id}/pause` y `/resume` (solo dueño). Un objeto pausado no admite solicitudes nuevas y conserva compromisos. |
| RF-09 | `DELETE /api/v1/items/{id}` marca `removed`; si hay reservas aceptadas o en curso → 409 `conflict`. |
| RF-10 | `GET /api/v1/me/items?status=&cursor=&limit=` lista mis objetos por estado (todos menos `removed`), más recientes primero. |
| RF-11 | `GET /api/v1/categories` devuelve `[{key,label}]` de POL-14. |
| RF-12 | Un usuario suspendido no puede crear objetos (`RequireActive`). |
| RNF-01 | Cobertura ≥ 80 % en `domain` y `usecase` de `items`. |
| RNF-02 | Ninguna respuesta pública incluye coordenadas con más de 2 decimales ni datos de contacto del dueño. |

## 4. Criterios de aceptación
| ID | Dado / Cuando / Entonces |
|---|---|
| CA-01 | Dado un préstamo con precio > 0, cuando lo publico, entonces 400 con detalle en `pricePerDayCents`. |
| CA-02 | Dado `declaredValueCents=100000` ($1,000), cuando pido sugerencia, entonces `suggestedPricePerDayCents=3000` ($30). |
| CA-03 | Dado `lat=19.123456`, cuando publico, entonces se guarda `19.12` y la respuesta muestra `19.12`. |
| CA-04 | Dado un título con contenido prohibido (cuando exista 011), cuando publico, entonces 400 con el motivo. |
| CA-05 | Dado otro usuario, cuando intento editar mi objeto, entonces 403. |
| CA-06 | Dado un objeto pausado, cuando otro usuario pide su detalle, entonces 404/403 según RF-06. |
| CA-07 | Dado un objeto con reserva aceptada, cuando lo elimino, entonces 409. |

## 5. Fuera de alcance
Búsqueda pública (006), reservas (007), calificaciones, fotos de perfil, categorías editables, borrado físico.

## 6. Preguntas abiertas
- Categorías confirmadas el 28 de oct (POL-14): herramientas manuales y eléctricas, construcción y jardinería, movilidad (patines, patinetas, bicicletas), deportes, hogar y otros.
- El valor declarado **no** es visible para otras personas (confirmado el 28 de oct).
