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
