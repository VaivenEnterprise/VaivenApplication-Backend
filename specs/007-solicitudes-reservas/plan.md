# Plan 007 — Solicitudes, precio y reservas (backend)

- **Spec:** `spec.md` · **Dueño:** BE-A

## 1. Estructura
```
internal/bookings/
  domain/{booking.go, quote.go, statemachine.go, dates.go, errors.go}
  usecase/{quote.go, create.go, accept.go, reject.go, cancel.go, meeting_point.go, get.go, list.go, expire.go, ports.go}
  adapter/http/{handler.go, dto.go, routes.go}
  adapter/mongo/{booking_doc.go, booking_repo.go, activity_checker.go}
  jobs/expirer.go
internal/chat/opener.go           # puerto chat.Opener + implementación vacía (008 la reemplaza)
api/paths/bookings.yaml
```

## 2. Contrato
| Método | Ruta | Cuerpo → Respuesta |
|---|---|---|
| GET | `/api/v1/items/{id}/quote` | `?startDate&endDate&paymentMode` → `Quote` |
| POST | `/api/v1/bookings` | `{itemId,startDate,endDate,paymentMode,acceptedTotalCents}` → 201 `Booking` |
| GET | `/api/v1/bookings/{id}` | → `Booking` |
| GET | `/api/v1/me/bookings` | `?role=owner|requester&status=&cursor=&limit=` → `{items:[Booking],nextCursor}` |
| POST | `/api/v1/bookings/{id}/accept` | `{meetingPoint?:{description}}` → `Booking` |
| POST | `/api/v1/bookings/{id}/reject` | `{reason?}` → `Booking` |
| POST | `/api/v1/bookings/{id}/cancel` | `{reason}` → `Booking` |
| POST | `/api/v1/bookings/{id}/meeting-point` | `{description}` → `Booking` |

```
Quote   = { itemId, startDate, endDate, days, modality, pricePerDayCents, totalCents, currency:"MXN",
            paymentMode, payments:[{stage:"delivery"|"return", amountCents}],
            policy:{ requestExpiresInHours, lateCancellationNotice, noShowNotice } }
Booking = { id, item:{id,title,thumbUrl|null,modality}, owner:PublicUser, requester:PublicUser,
            startDate, endDate, days,
            priceSnapshot:{modality,pricePerDayCents,totalCents,currency},
            paymentMode, payments:[{stage,amountCents,status,reportedAt|null,confirmedAt|null}],
            status, expiresAt|null, meetingPoint|null, cancelReason|null, cancelledBy|null,
            conversationId|null, delivery|null, return|null, isLate, adjustments:[],   # 009 completa estos
            viewerRole:"owner"|"requester", createdAt, updatedAt, version }
```
Fechas `startDate`/`endDate`: **fecha de calendario** `YYYY-MM-DD` interpretada en `America/Mexico_City` (excepción documentada en Art. V). Resto de instantes en ISO UTC.

## 3. Datos
**`bookings`**: `_id`, `itemId` (texto hexadecimal del `_id` del objeto, igual que `items.ownerId`; la búsqueda de la 006 lo compara así — [NEEDS CLARIFICATION] confirmar BE-A), `ownerId`, `requesterId`, `startDate`, `endDate` (cadenas `YYYY-MM-DD`), `days`, `itemSnapshot{title,usageRules,modality,firstPhoto Image}`, `priceSnapshot{modality,pricePerDayCents,totalCents,currency}`, `paymentMode`, `payments[]` (≤ 2), `status`, `statusHistory[]` (≤ 10: `{status,at,by}`), `expiresAt`, `meetingPoint{description}`, `cancelReason`, `cancelledBy`, `conversationId`, `createdAt`, `updatedAt`, `version`.
Índices: `{itemId:1,status:1,startDate:1,endDate:1}`; `{requesterId:1,status:1,createdAt:-1}`; `{ownerId:1,status:1,createdAt:-1}`; `{status:1,expiresAt:1}`.
`items` recibe un contador `reservationSeq` (int) que 007 incrementa solo dentro de la transacción de aceptar (ver P-03).

## 4. Máquina de estados
| Desde | Hacia | Quién |
|---|---|---|
| `pending` | `accepted` · `rejected` | propietario |
| `pending` | `cancelled` | solicitante |
| `pending` | `expired` | sistema |
| `accepted` | `cancelled` | cualquiera de las partes |
| `accepted` | `in_progress` | 009 (confirmación de entrega) |
| `in_progress` | `completed` | 009 (confirmación de devolución) |
Todo lo demás → 409 `conflict`.

## 5. Decisiones
| # | Decisión | Art. |
|---|---|---|
| P-01 | Cálculo: `days = (end − start) + 1`; `total = days × rate` (int64); mitades: entrega `ceil(total/2)`, devolución `floor(total/2)` (POL-01, POL-05) | V |
| P-02 | `dates.go` usa `time.LoadLocation("America/Mexico_City")` e importa `time/tzdata` para que funcione en la imagen `distroless` | V |
| P-03 | **Aceptar** en `WithTransaction`: (1) `$inc` en `items.reservationSeq` (fuerza conflicto de escritura entre transacciones del mismo objeto), (2) buscar cruces `accepted/in_progress`, (3) `findOneAndUpdate` de la solicitud con `status:"pending"` y `version`, (4) `updateMany` de pendientes cruzadas → `rejected` | II |
| P-04 | Vencimiento: `expiresAt = min(createdAt+48h, inicio del día startDate en México)`; tarea periódica (cada 60 s) `pending && expiresAt<=now → expired`; además, al leer, una pendiente vencida se presenta como `expired` | IX |
| P-05 | `activity_checker.go` implementa `ActiveBookingChecker` (usuario y objeto) y reemplaza los vacíos de 003/005 en `modules.go` | XII |
| P-06 | `chat.Opener` (puerto) se invoca al crear; si falla, la solicitud **no** se crea | XII |
| P-07 | Notificaciones por `notify.Notifier` (eventos `booking.requested|accepted|rejected|cancelled|expired|meeting_point`; la 010 consume estos tipos); si falla el aviso no revierte la operación (se registra) | IX |
| P-08 | Los textos de política salen de `internal/policy` (POL-03, POL-09) | XII |

## 6. Pruebas
Unitarias: cotización (tabla de días y mitades, bordes de 1 y 7 días, y 8 días rechazado, cambio de mes), máquina de estados (todas las transiciones válidas/ inválidas), vencimiento, fechas en zona de México (cambio de horario no aplica desde 2022). Integración con Mongo real: crear→aceptar→cruce rechazado→cancelar→disponible; **concurrencia**: 2 goroutines aceptan solicitudes que se cruzan, exactamente una gana; vencimiento con reloj falso.

## 7. Verificación de constitución
| Art. | Cumple |
|---|---|
| II | Transacción en replica set; índices en código |
| III | Reglas en `domain`/`usecase` sin `mongo` |
| IV, V | Contrato; importes en centavos; fechas de calendario documentadas |
| VI | Punto de encuentro solo para partes |
| XII | Puertos `chat.Opener`, `Notifier`; sin tocar rutas ajenas |

## 8. Ajustes durante la implementación (2026-11-08, BE-B a pedido del responsable; pendientes de visto bueno de BE-A)
| # | Ajuste | Motivo |
|---|---|---|
| A-01 | Aceptar hace `$inc` de `items.reservationSeq` **y** `items.version`: dos aceptaciones del mismo objeto chocan (el driver reintenta) y un `DELETE /items` concurrente de la 005 falla por versión; un PATCH con versión vieja recibe 409. Se acepta si el objeto está `active` o `paused` (no `blocked`/`removed`); crear exige `active`. El filtro de la transacción incluye `expiresAt > now` | RNF-01, RF-14; cierra la [NEEDS CLARIFICATION] de 005 plan §7 |
| A-02 | Si `startDate` es hoy, la solicitud vence al terminar ese día (si no, nacería vencida). [NEEDS CLARIFICATION] confirmar con BE-A | RF-03, RF-06 |
| A-03 | P-03 se implementa con `ReplaceOne` filtrado por `version` (y `status`) en lugar de `findOneAndUpdate`/`updateMany`; cada pendiente rechazada guarda su entrada de historial. **La 009 debe ampliar `bookingDoc`** (adapter/mongo) con sus campos; si escribe campos con `$set` sin ampliarlo, un `Update` de 007 los borraría | Revisión T012 |
| A-04 | `rejectReason` es un campo aparte (`no_disponible` al perder el periodo); `GET /bookings/{id}` responde 404 a quien no es parte; admin ve la reserva sin punto de encuentro; el propietario no cancela una pendiente (la rechaza) | RF-08, RF-10, Art. VI |
| A-05 | `GET /me/bookings?status=` usa el estado efectivo: `pending` excluye las ya vencidas y `expired` incluye las pendientes vencidas sin barrer | P-04 |
| A-06 | `chat.Opener.Open(ctx, bookingID, ownerID, requesterID)`; el id de la solicitud se genera antes de guardar (`Repo.NewID`). `meetingPoint.description` de 3 a 200 caracteres | P-06, RF-07, RF-09 |
| A-07 | Archivos consolidados: casos de uso en `create.go`, `flow.go` (aceptar, rechazar, cancelar, punto de encuentro) y `read.go` (consultar, listar, vencer); las pruebas de `activity_checker` viven en `accept_tx_integration_test.go`. `cmd/api/main.go` arma el módulo, sustituye los verificadores vacíos de 003 y 005 y arranca y espera la tarea de vencimiento | Tamaño del módulo |
| A-08 | Riesgo aceptado: la transacción de aceptar no revisa el estado de las cuentas; una baja (003) que pasa su verificación justo antes de un commit de aceptar podría dejar una reserva con una parte dada de baja. Ventana de milisegundos; se revisará si la 010 agrega sanciones | RF-14 |
