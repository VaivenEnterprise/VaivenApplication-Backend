# Spec 010 — Calificaciones e incidencias (backend)

- **Dueño:** BE-B · **Prioridad:** P1 · **Ventana:** 13–17 nov · **Depende de:** 007, 009 (parcial)
- **Constitución:** v1.4.0 · **Políticas:** POL-08, 09, 10, 13, 15 · **Estado:** Borrador

## 1. Objetivo y por qué
Construir confianza: cada persona puede calificar a su contraparte al concluir una operación, y cuando hay un desacuerdo se abre una **incidencia** vinculada a la reserva, con evidencia y seguimiento manual. Las sanciones se registran aparte y no cambian el promedio.

## 2. Historias
| # | Prio. | Historia |
|---|---|---|
| H-01 | P1 | Como participante, quiero calificar (1–5, comentario opcional) a la otra parte tras la devolución, para construir referencias. |
| H-02 | P1 | Como persona usuaria, quiero ver promedio y cantidad de calificaciones de otra persona, para decidir si confío. |
| H-03 | P1 | Como participante, quiero reportar un incumplimiento con motivo, descripción y evidencia, para que se revise. |
| H-04 | P2 | Como administrador, quiero revisar incidencias abiertas, pedir información, responder y cerrar, para resolver desacuerdos. |
| H-05 | P2 | Como administrador, quiero registrar una advertencia o suspensión, para aplicar consecuencias con política conocida. |

## 3. Requisitos
| ID | Requisito |
|---|---|
| RF-01 | `POST /api/v1/bookings/{id}/ratings` con `score` 1–5 y `comment` ≤ 300: solo reservas `completed`, solo partes, una calificación por participante y reserva, hasta 14 días tras completar (POL-10); la calificada es la otra parte. |
| RF-02 | Al calificar, el perfil de la calificada actualiza `ratingSum` y `ratingCount` de forma atómica; el promedio se muestra con 1 decimal. |
| RF-03 | `GET /api/v1/bookings/{id}/rating-status` → `{canRate, myRating|null, expiresAt|null}`. |
| RF-04 | `GET /api/v1/users/{id}/ratings?cursor&limit` lista calificaciones (puntaje, comentario, quién califica por nombre visible, fecha), recientes primero. |
| RF-05 | `POST /api/v1/bookings/{id}/incidents` con `reason` ∈ {`no_show`,`late_return`,`damaged`,`not_returned`,`payment_dispute`,`other`}, `description` 10–1000 y `evidence` 0–5 (`{publicId}`, propósito `evidence`). Solo partes; reserva `accepted`, `in_progress` o `completed` dentro de 72 h tras completar (POL-08); `no_show` solo si ya llegó la fecha de inicio; máximo 3 incidencias abiertas por reserva. |
| RF-06 | Una incidencia tiene **solo dos estados**: `open` y `closed`. Abrirla **no** cambia el estado de la reserva (no borra una entrega ni cierra una devolución pendiente). |
| RF-07 | `GET /api/v1/incidents/{id}` y `GET /api/v1/me/incidents?status=&cursor` (partes y admin); la evidencia con URLs firmadas de 10 min. |
| RF-08 | Admin: `GET /api/v1/admin/incidents?status=open&cursor`, `POST /api/v1/admin/incidents/{id}/notes` (`text`, `visibleToParties`; ≤ 10 notas) y `POST /api/v1/admin/incidents/{id}/close` (`finalResponse` 5–1000 y `outcome` ∈ {`none`,`monetary_adjustment`,`restriction`}). Sin apelaciones ni etapas extra. |
| RF-09 | Admin: `POST /api/v1/admin/users/{id}/sanctions` (`type` ∈ {`warning`,`suspension`}, `reason`, `endsAt` para suspensión, `incidentId` opcional). Una suspensión vigente impide **crear** objetos y solicitudes (RF-13 de 003). Las sanciones **no** modifican calificaciones. |
| RF-10 | Notificaciones: calificación recibida, incidencia abierta (a la parte reportada), nota visible, incidencia cerrada, sanción aplicada. |
| RF-11 | Las rutas `/admin/*` exigen rol `admin`; para el resto → 403. |
| RNF-01 | Una calificación duplicada concurrente: solo una se guarda. |
| RNF-02 | Cobertura ≥ 80 % en `domain` y `usecase` de `ratings` e `incidents`. |

## 4. Criterios de aceptación
| ID | Dado / Cuando / Entonces |
|---|---|
| CA-01 | Dada una reserva `completed`, cuando califico con 5, entonces el perfil de la otra parte muestra promedio y cantidad actualizados. |
| CA-02 | Dada una reserva no completada, cuando califico, entonces 409. |
| CA-03 | Dada una segunda calificación mía a la misma reserva, entonces 409. |
| CA-04 | Dada una incidencia abierta, cuando consulto la reserva, entonces su estado no cambió. |
| CA-05 | Dado un usuario no admin, cuando llamo a una ruta de admin, entonces 403. |
| CA-06 | Dada una suspensión vigente, cuando intento publicar un objeto, entonces 403. |
| CA-07 | Dada una incidencia cerrada con respuesta final, entonces ambas partes la ven con evidencia y respuesta. |
| CA-08 | Dados 4 intentos de abrir incidencias en una reserva, entonces el 4.º falla (máx. 3 abiertas). |

## 5. Fuera de alcance
Apelaciones, escalamiento, cobros automáticos, panel visual de administración (se usa `scripts/admin.http`), edición o borrado de calificaciones, respuesta pública a calificaciones.

## 6. Preguntas abiertas
- [NEEDS CLARIFICATION: ¿Quién es administrador y cómo se contacta a las partes (seguimiento manual)? Provisional: el admin agrega notas visibles y las partes son avisadas por notificación.]
