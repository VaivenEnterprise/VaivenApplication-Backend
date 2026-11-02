# Spec 008 — Chat privado (backend)

- **Dueño:** BE-B · **Prioridad:** P1 · **Ventana:** 10–13 nov · **Depende de:** 007 (puerto `chat.Opener`)
- **Constitución:** v1.4.0 · **Políticas:** POL-11, POL-16 · **Estado:** Borrador

## 1. Objetivo y por qué
Que propietario y solicitante coordinen hora, punto de encuentro y medio de pago por una conversación privada ligada a la reserva. Solo texto: sin adjuntos, audios, llamadas, indicadores de escritura ni confirmaciones de lectura.

## 2. Historias
| # | Prio. | Historia |
|---|---|---|
| H-01 | P1 | Como parte de una reserva, quiero enviar y recibir mensajes de texto, para coordinar la entrega. |
| H-02 | P1 | Como persona usuaria, quiero ver mi historial y la lista de mis conversaciones, para retomar acuerdos. |
| H-03 | P2 | Como persona usuaria, quiero saber cuántos mensajes tengo sin leer, para no perder avisos. |

## 3. Requisitos
| ID | Requisito |
|---|---|
| RF-01 | Cada reserva tiene **una** conversación con exactamente dos participantes, creada al enviar la solicitud mediante `chat.Opener` (idempotente por `bookingId`). |
| RF-02 | `GET /api/v1/conversations?cursor&limit` lista las conversaciones de la persona, la más reciente primero, con contraparte, objeto, último mensaje (≤ 80 caracteres), `unreadCount` propio y estado de la reserva. |
| RF-03 | `GET /api/v1/conversations/{id}/messages?limit=30&before=<id>` devuelve mensajes del más nuevo al más viejo con `nextCursor`; `?after=<id>` devuelve los más nuevos que `<id>` en orden ascendente (máx. 100) para consulta periódica. |
| RF-04 | `POST /api/v1/conversations/{id}/messages` con `body` (1–1000 caracteres, texto plano, sin espacios solo) y `clientId` opcional (idempotencia: repetir el mismo `clientId` devuelve el mensaje existente, 200). |
| RF-05 | Solo los dos participantes acceden; cualquier otra persona recibe 404. |
| RF-06 | Se puede escribir mientras la reserva esté `pending`, `accepted` o `in_progress`. En otros estados el historial se conserva y escribir → 409 `conflict` («La conversación está cerrada.»). |
| RF-07 | Si la contraparte dio de baja su cuenta, se muestra «Cuenta eliminada» y no se puede escribir. |
| RF-08 | `POST /api/v1/conversations/{id}/read` (204) pone en cero **mi** contador. **No** existe ni se expone información de lectura de la otra parte. |
| RF-09 | `GET /api/v1/conversations/unread-count` → `{count}` (suma de mis no leídos). |
| RF-10 | El cuerpo pasa por `moderation.Checker`; si se rechaza → 400 `validation_failed` con motivo. |
| RF-11 | Límite de 30 mensajes por minuto por persona → 429. |
| RF-12 | Al recibir el **primer** mensaje no leído, se notifica al otro participante (`notify.Notifier`, evento `message_received`); no se notifica cada mensaje siguiente mientras haya no leídos. |
| RNF-01 | Consulta periódica ligera: `after` responde en < 100 ms con índice. |
| RNF-02 | Cobertura ≥ 80 % en `domain` y `usecase` de `chat`. |

## 4. Criterios de aceptación
| ID | Dado / Cuando / Entonces |
|---|---|
| CA-01 | Dada una solicitud creada, entonces existe su conversación con 2 participantes. |
| CA-02 | Dada una persona ajena, cuando pide mensajes, entonces 404. |
| CA-03 | Dado el mismo `clientId` enviado dos veces, entonces un solo mensaje. |
| CA-04 | Dada una reserva `cancelled`, cuando escribo, entonces 409; el historial se lee. |
| CA-05 | Dado un mensaje de 1001 caracteres, entonces 400. |
| CA-06 | Dados 3 mensajes enviados, entonces el otro participante ve `unreadCount=3` y tras `read` ve 0, sin cambiar nada para quien envió. |

## 5. Fuera de alcance
Adjuntos, audio, llamadas, indicadores de escritura, confirmaciones de lectura, tiempo real (WebSockets), búsqueda en mensajes, reporte de mensajes (se hace con la spec 011).

## 6. Preguntas abiertas
- [NEEDS CLARIFICATION: política de conservación de mensajes (PDF §04). Provisional: se conservan; sin borrado en esta versión.]
