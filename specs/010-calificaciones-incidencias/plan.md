# Plan 010 — Calificaciones e incidencias (backend)

- **Spec:** `spec.md` · **Dueño:** BE-B

## 1. Estructura
```
internal/ratings/{domain, usecase, adapter/{http,mongo}}
internal/incidents/{domain, usecase, adapter/{http,mongo}}
internal/sanctions/{domain, usecase, adapter/{http,mongo}}
api/paths/ratings.yaml · api/paths/incidents.yaml · api/paths/admin.yaml
scripts/admin.http
```

## 2. Contrato
| Método | Ruta | Cuerpo → Respuesta |
|---|---|---|
| POST | `/api/v1/bookings/{id}/ratings` | `{score,comment?}` → 201 `Rating` |
| GET | `/api/v1/bookings/{id}/rating-status` | → `{canRate,myRating,expiresAt}` |
| GET | `/api/v1/users/{id}/ratings` | `?cursor&limit` → `{items:[Rating],nextCursor}` |
| POST | `/api/v1/bookings/{id}/incidents` | `IncidentInput` → 201 `Incident` |
| GET | `/api/v1/incidents/{id}` | → `Incident` |
| GET | `/api/v1/me/incidents` | `?status&cursor` → `{items:[Incident],nextCursor}` |
| GET | `/api/v1/admin/incidents` | `?status&cursor` → página de `Incident` (vista admin) |
| POST | `/api/v1/admin/incidents/{id}/notes` | `{text,visibleToParties}` → `Incident` |
| POST | `/api/v1/admin/incidents/{id}/close` | `{finalResponse,outcome}` → `Incident` |
| POST | `/api/v1/admin/users/{id}/sanctions` | `{type,reason,endsAt?,incidentId?}` → 201 `Sanction` |

```
Rating   = { id, bookingId, score, comment|null, ratedRole:"owner"|"requester", rater:{id,displayName}, createdAt }
Incident = { id, bookingId, reporter:PublicUser, reported:PublicUser, reason, description,
             evidence:[Image+url], status:"open"|"closed",
             notes:[{text,at}],                       # solo las visibles; el admin ve todas con "visibleToParties"
             finalResponse:{text,outcome,at}|null, createdAt, closedAt|null }
Sanction = { id, userId, type, reason, endsAt|null, incidentId|null, createdAt }
```

## 3. Datos
**`ratings`**: `_id`, `bookingId`, `raterId`, `rateeId`, `ratedRole`, `score`, `comment`, `createdAt`. Índices: **único** `{bookingId:1,raterId:1}`; `{rateeId:1,_id:-1}`.
**`incidents`**: `_id`, `bookingId`, `reporterId`, `reportedId`, `reason`, `description`, `evidence[≤5 Image]`, `status`, `notes[≤10]{text,at,byAdminId,visibleToParties}`, `finalResponse{text,outcome,byAdminId,at}`, `createdAt`, `closedAt`. Índices: `{bookingId:1,status:1}`; `{reporterId:1,createdAt:-1}`; `{reportedId:1,createdAt:-1}`; `{status:1,createdAt:1}`.
**`sanctions`**: `_id`, `userId`, `type`, `reason`, `endsAt`, `incidentId`, `createdByAdminId`, `createdAt`. Índice `{userId:1,createdAt:-1}`.
`users.ratingSum/ratingCount/suspendedUntil` se modifican **solo** mediante `users.Admin` (003).

## 4. Decisiones
| # | Decisión | Art. |
|---|---|---|
| P-01 | Calificar: `WithTransaction` — insertar `rating` (el índice único evita duplicados) y `users.Admin.AddRating`; si falla uno, ninguno | II |
| P-02 | Ventana de 14 días desde `completedAt` (el estado `completed` lo registra 009 en `statusHistory`) | V |
| P-03 | `rating-status.canRate` = reserva `completed` ∧ parte ∧ sin calificación mía ∧ dentro de ventana | III |
| P-04 | Abrir incidencia no toca `bookings.status` | III |
| P-05 | Las incidencias leen la reserva con un puerto `BookingReader` (solo lectura) | XII |
| P-06 | Rutas admin bajo `/api/v1/admin` con `RequireRole("admin")` | VI |
| P-07 | Sanción de suspensión llama `users.Admin.Suspend(userID, endsAt)`; advertencia solo notifica | XII |
| P-08 | Evidencia: `images.Service.Resolve(ctx, userID, "evidence", refs)`; lectura con URLs firmadas de 10 min para partes y admin | VI.7 |
| P-09 | `scripts/admin.http` con ejemplos de cada ruta admin para uso con una extensión REST | — |

## 5. Pruebas
Unitarias: ventanas y permisos, estados de incidencia, límite de 3 abiertas, cálculo de promedio, notas visibles/ocultas por rol. Integración: calificación atómica y concurrente (índice único), flujo abrir→nota→cerrar, suspensión impide crear objeto (usa 005), 403 a no admin.

## 6. Verificación de constitución
| Art. | Cumple |
|---|---|
| II | Transacción para calificar; índices únicos en código |
| III | Reglas en `domain`/`usecase` |
| IV | Contrato por módulo |
| VI | Rutas admin, evidencia privada, sin PII en logs |
| XII | `users.Admin` y `BookingReader` sin editar otros módulos |
