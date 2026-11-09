# Plan 008 — Chat privado (backend)

- **Spec:** `spec.md` · **Dueño:** BE-B

## 1. Estructura
```
internal/chat/
  domain/{conversation.go, message.go, errors.go}
  usecase/{open.go, list.go, messages.go, send.go, read.go, unread.go, ports.go}
  adapter/http/{handler.go, dto.go, routes.go}
  adapter/mongo/{conversation_repo.go, message_repo.go, docs.go}
  opener.go                # reemplaza la implementación vacía de 007 (una línea en modules.go)
api/paths/chat.yaml
```

## 2. Contrato
| Método | Ruta | Respuesta |
|---|---|---|
| GET | `/api/v1/conversations` | `?cursor&limit` → `{items:[Conversation],nextCursor}` |
| GET | `/api/v1/conversations/unread-count` | `{count}` |
| GET | `/api/v1/conversations/{id}/messages` | `?limit&before` → `{items:[Message],nextCursor}` · `?after` → `{items:[Message]}` ascendente |
| POST | `/api/v1/conversations/{id}/messages` | `{body,clientId?}` → 201/200 `Message` |
| POST | `/api/v1/conversations/{id}/read` | 204 |

```
Conversation = { id, bookingId, bookingStatus, counterpart:PublicUser,
                 item:{id,title,thumbUrl|null}, canWrite,
                 lastMessage:{preview,at,senderId}|null, unreadCount }
Message      = { id, conversationId, senderId, body, createdAt, clientId|null }
```

## 3. Datos
**`conversations`**: `_id`, `bookingId` (único), `participantIds[2]`, `unread{<userId>:int}`, `lastMessage{preview,at,senderId}`, `lastMessageAt`, `createdAt`.
Índices: único `{bookingId:1}`; `{participantIds:1,lastMessageAt:-1}`.
**`messages`**: `_id` (ObjectID, ordena por tiempo), `conversationId`, `senderId`, `body`, `clientId`, `createdAt`.
Índices: `{conversationId:1,_id:-1}`; único parcial `{conversationId:1,senderId:1,clientId:1}` donde `clientId` es cadena.

## 4. Decisiones
| # | Decisión | Art. |
|---|---|---|
| P-01 | Cursor `before/after` = `_id` del mensaje (ObjectID crece con el tiempo) | IV |
| P-02 | Consulta periódica en el cliente (5 s con el chat abierto); sin WebSockets | VIII |
| P-03 | `canWrite` se calcula del estado de la reserva (lectura de `bookings` vía puerto `BookingReader`) y de la baja de la contraparte | III |
| P-04 | `unread.<userId>` se incrementa con `$inc` al enviar y se pone en 0 con `$set` al leer; nunca se expone el de la otra parte | VI |
| P-05 | Notificación solo si el contador previo del receptor era 0 | IX |
| P-06 | Límite de mensajes con `ratelimit` (clave userId, 30/min) | VI |
| P-07 | `Opener.Open` usa `findOneAndUpdate` con `upsert` por `bookingId` | II |
| P-08 | El cuerpo se normaliza (`strings.TrimSpace`, se rechaza vacío o > 1000 runas) y se trata siempre como texto plano | VI |

## 5. Pruebas
Unitarias: validación de cuerpo (runas), `canWrite` por estado, idempotencia por `clientId`, contadores, notificación solo en el primer no leído. Integración: `Opener` idempotente, paginación `before/after` sin huecos ni repetidos, acceso de ajenos = 404, límite de tasa.

## 6. Verificación de constitución
| Art. | Cumple |
|---|---|
| II | Mongo, índices en código |
| III | Reglas en `usecase`; Mongo en `adapter` |
| IV | Contrato, cursor, errores estándar |
| VI | Privacidad: solo participantes; sin lectura de la otra parte |
| XII | Reemplaza `chat.Opener` sin tocar 007 |

## 8. Ajustes tras la revisión T009 (2026-11-09, BE-B)
| # | Ajuste | Motivo |
|---|---|---|
| A-01 | Enviar es **una transacción** (`adapter/mongo.Sender`): `$inc` de `conversations.seq` y de `unread.<receptor>`, `$set` del último mensaje y luego el `insert` del mensaje con ese `seq`. La escritura de la conversación la bloquea hasta el commit, así que los `seq` se confirman en orden; `before`/`after` siguen recibiendo el id del mensaje (P-01) pero ordenan por `seq` (índice `conversation_seq`) | Observaciones 1 y 2: con ObjectID generado en la app, dos envíos simultáneos podían guardarse en desorden y `after` saltarse uno; y si fallaba el contador, el mensaje quedaba sin contar ni avisar |
| A-02 | Un `clientId` repetido aborta la transacción (no suma no leídos ni consume `seq`); si en la carrera no se encuentra el mensaje existente se responde error interno, nunca un mensaje vacío | Observación 3 |
| A-03 | `unread.<userId>` solo acepta ids de 24 caracteres hexadecimales | Observación 4 (Art. VI) |
| A-04 | Contrato: la consulta periódica de `/conversations` vuelve a pedir la primera página; `before`/`after` de otra conversación → 400 | Observación 5 |
| A-05 | Archivos consolidados respecto a §1: casos de uso en `list.go`, `messages.go` y `service.go`; HTTP en `handler.go`; el `chat.Opener` real es `usecase.Service`, conectado en `cmd/api/main.go` (el `Noop` de 007 se conserva) | Registro de desviaciones |

Los mensajes guardados antes de A-01 no tienen `seq` (cuentan como 0 y quedan antes de los nuevos); solo existían datos de prueba en staging. El índice anterior `conversation_id` puede quedar en staging sin efecto.
