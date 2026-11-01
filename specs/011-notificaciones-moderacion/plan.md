# Plan 011 — Notificaciones y moderación (backend)

- **Spec:** `spec.md` · **Dueño:** BE-B

## 1. Estructura
```
internal/notify/                      # puerto (003) + implementación real
  notifier.go (interfaz, existente) · service.go · templates.go
internal/notifications/{domain, usecase, adapter/{http,mongo,expo}, worker}
internal/moderation/                  # puerto (003) + implementación real
  checker.go (interfaz) · filter.go · blocklist.go
internal/reports/{domain, usecase, adapter/{http,mongo}}
api/paths/notifications.yaml · api/paths/reports.yaml
```

## 2. Contrato
| Método | Ruta | Cuerpo → Respuesta |
|---|---|---|
| GET | `/api/v1/me/notifications` | `?cursor&limit` → `{items:[Notification],nextCursor}` |
| GET | `/api/v1/me/notifications/unread-count` | → `{count}` |
| POST | `/api/v1/me/notifications/read` | `{ids?:[…],all?:true}` → 204 |
| POST | `/api/v1/me/push-tokens` | `{token,platform}` → 204 |
| DELETE | `/api/v1/me/push-tokens` | `{token}` → 204 |
| POST | `/api/v1/reports` | `{targetType,targetId,reason,details?}` → 201 `{id}` |
| GET | `/api/v1/admin/reports` | `?status&cursor` → página de `Report` |
| POST | `/api/v1/admin/reports/{id}/resolve` | `{action,note}` → `Report` |

```
Notification = { id, type, title, body, data:{bookingId?,itemId?,conversationId?,incidentId?}, readAt|null, createdAt }
Report       = { id, targetType, targetId, reason, details|null, reporter:PublicUser,
                 status:"open"|"actioned"|"dismissed", target:{ summary }, resolution|null, createdAt }
```

## 3. Datos
**`notifications`**: `_id`, `userId`, `type`, `title`, `body`, `data`, `readAt`, `createdAt`. Índices: `{userId:1,_id:-1}`; `{userId:1,readAt:1}`; **TTL** `{createdAt:1}` 90 días.
**`push_tokens`**: `_id`, `userId`, `token` (único), `platform`, `createdAt`, `lastSeenAt`. Índice `{userId:1}`.
**`reports`**: `_id`, `reporterId`, `targetType`, `targetId`, `reason`, `details`, `status`, `resolution{action,note,byAdminId,at}`, `createdAt`. Índices: único `{reporterId:1,targetType:1,targetId:1}`; `{status:1,createdAt:1}`.

## 4. Decisiones
| # | Decisión | Art. |
|---|---|---|
| P-01 | `Notify` guarda el aviso de forma síncrona y encola el push en un canal con buffer (500); un *worker* lo envía; si el canal está lleno se descarta el push (el aviso ya existe) | IX |
| P-02 | Expo Push: `POST https://exp.host/--/api/v2/push/send` con lotes ≤ 100, JSON, timeout 10 s; sin SDK; `PUSH_ENABLED=false` en pruebas | VIII |
| P-03 | Errores por ticket `DeviceNotRegistered` → borrar token; otros errores → registrar sin datos personales | VI |
| P-04 | Plantillas con `text/template`: título y cuerpo por tipo con parámetros (nombre, título del objeto); sin PII adicional en el push | VI |
| P-05 | Filtro: normaliza con `golang.org/x/text/unicode/norm` **no** (evitar dependencia); se usa un mapa de acentos propio y minúsculas; coincidencia por **palabra completa** y por frases | VIII |
| P-06 | Listas por categoría en `blocklist.go` (sexual, armas, drogas, ilegal), revisables por el equipo | VI |
| P-07 | `reports/resolve`: `hide_item` → `items.Moderation.Block`; `suspend_user` → `users.Admin.Suspend` (7 días provisional) | XII |
| P-08 | Reemplazar los puertos vacíos en `modules.go` con **una línea** cada uno | XII |
| P-09 | Variables: `PUSH_ENABLED`, `EXPO_PUSH_URL`, `EXPO_ACCESS_TOKEN` (opcional, secreta) | VI |

## 5. Pruebas
Unitarias: plantillas por cada tipo, filtro (acentos, palabra completa, falsos positivos comunes), worker con servidor Expo simulado (lote, `DeviceNotRegistered`), reglas de reportes. Integración: aviso + token + push simulado, TTL creado, reporte duplicado, `hide_item` saca el objeto de `GET /items`.

## 6. Verificación de constitución
| Art. | Cumple |
|---|---|
| II | Mongo, TTL e índices en código |
| VI | Sin PII en push/logs; rutas admin protegidas |
| VIII | Sin SDK ni dependencias nuevas |
| IX | Push asíncrono que no bloquea el negocio |
| XII | Reemplaza puertos vacíos con una línea |
