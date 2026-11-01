# Spec 011 — Notificaciones y moderación (backend)

- **Dueño:** BE-B · **Prioridad:** P1 (solo el push es recortable) · **Ventana:** 10–12 nov · **Depende de:** 007–010
- **Constitución:** v1.3.0 · **Políticas:** POL-16 · **Estado:** Borrador

## 1. Objetivo y por qué
Mantener a las personas informadas sobre solicitudes, acuerdos, encuentros, devoluciones e incidencias (en la app y por notificación push), y limitar objetos y contenido prohibido mediante un filtro automático, reportes y revisión humana. Reemplaza los puertos vacíos `notify.Notifier` y `moderation.Checker`.

## 2. Historias
| # | Prio. | Historia |
|---|---|---|
| H-01 | P1 | Como persona usuaria, quiero ver una lista de avisos dentro de la app, para no perder novedades. |
| H-02 | P2 | Como persona usuaria, quiero recibir notificaciones push, para enterarme sin abrir la app. |
| H-03 | P1 | Como persona usuaria, quiero que no se publiquen objetos o mensajes prohibidos, para un entorno seguro. |
| H-04 | P1 | Como persona usuaria, quiero reportar un objeto, mensaje o perfil, para que se revise. |
| H-05 | P2 | Como administrador, quiero revisar reportes y ocultar un objeto o suspender a alguien, para aplicar las reglas. |

## 3. Requisitos
| ID | Requisito |
|---|---|
| RF-01 | `notify.Notifier` real: `Notify(ctx, userID, Event)` guarda un aviso (`notifications`) con título y cuerpo en español a partir de una plantilla del tipo de evento. |
| RF-02 | Tipos de evento: `booking_requested`, `booking_accepted`, `booking_rejected`, `booking_cancelled`, `booking_expired`, `message_received`, `delivery_confirmed`, `return_confirmed`, `payment_reported`, `payment_confirmed`, `late_return`, `booking_affected`, `incident_opened`, `incident_note`, `incident_closed`, `rating_received`, `sanction_applied`, `content_removed`. |
| RF-03 | `GET /api/v1/me/notifications?cursor&limit`, `GET /api/v1/me/notifications/unread-count` y `POST /api/v1/me/notifications/read` (`ids` o `all:true`). |
| RF-04 | `POST /api/v1/me/push-tokens` (`token` de Expo, `platform`) registra o reasigna el token a la persona; `DELETE /api/v1/me/push-tokens` lo elimina (al cerrar sesión). |
| RF-05 | Cada aviso se envía además por push (API de Expo) a los tokens de la persona, en segundo plano y por lotes de hasta 100; un token inválido (`DeviceNotRegistered`) se elimina. El fallo de push **no** afecta la operación de negocio. |
| RF-06 | El push incluye `data` con ids para abrir la pantalla correcta (`bookingId`, `itemId`, `conversationId`, `incidentId`). |
| RF-07 | `moderation.Checker` real: normaliza el texto (minúsculas, sin acentos) y busca términos prohibidos de sexual, armas, drogas e ilegal (POL-16); devuelve categoría y motivo en español. 005 y 008 ya lo invocan. |
| RF-08 | `POST /api/v1/reports` con `targetType` ∈ {`item`,`message`,`user`,`moderation_decision`}, `targetId`, `reason` ∈ {`prohibited_content`,`inappropriate`,`spam`,`other`}, `details` ≤ 500. Un mensaje solo lo reporta quien participa en su conversación. Repetir el mismo reporte devuelve el existente. |
| RF-09 | `moderation_decision` permite reportar un rechazo automático para **seguimiento manual**, sin proceso de apelaciones. |
| RF-10 | Admin: `GET /api/v1/admin/reports?status=open&cursor` y `POST /api/v1/admin/reports/{id}/resolve` con `action` ∈ {`dismiss`,`hide_item`,`suspend_user`} y `note`; `hide_item` bloquea el objeto (`items.Moderation.Block`) y notifica `content_removed`. |
| RF-11 | Cada decisión conserva motivo y quién la tomó. |
| RNF-01 | Los avisos se conservan 90 días (índice TTL). |
| RNF-02 | El filtro responde en < 5 ms por texto de 1,000 caracteres. |
| RNF-03 | Cobertura ≥ 80 % en `domain` y `usecase`. |

## 4. Criterios de aceptación
| ID | Dado / Cuando / Entonces |
|---|---|
| CA-01 | Dada una solicitud creada, entonces el propietario tiene un aviso `booking_requested` y recibe push si tiene token. |
| CA-02 | Dado un token no registrado en Expo, cuando falla el envío, entonces se elimina y la operación de negocio no falla. |
| CA-03 | Dado un título con una palabra de la lista, cuando publico, entonces 400 con categoría y motivo (005). |
| CA-04 | Dado un mensaje de chat prohibido, cuando lo envío, entonces 400 (008). |
| CA-05 | Dado un reporte de objeto, cuando el admin elige `hide_item`, entonces el objeto sale de la búsqueda. |
| CA-06 | Dado que no soy participante, cuando reporto un mensaje, entonces 404. |
| CA-07 | Dado `read all`, entonces `unread-count` es 0. |

## 5. Fuera de alcance
Moderación de imágenes, apelaciones, escalamiento, correos, SMS, preferencias de notificación, panel visual de administración.

## 6. Preguntas abiertas
- [NEEDS CLARIFICATION: lista de palabras en español para POL-16; provisional: lista corta mantenida en `blocklist.go` y revisada por el equipo.]
- [NEEDS CLARIFICATION: ¿`EXPO_ACCESS_TOKEN` (seguridad mejorada de Expo) se usará? Provisional: opcional.]
