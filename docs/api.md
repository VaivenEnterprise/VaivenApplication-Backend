# API de VAIVÉN — resumen por módulo

> Spec 012, T010 (RF-09). Generado del contrato `api/openapi.yaml` versión **0.11.0** (63 operaciones). El contrato manda: si este resumen y el contrato difieren, vale el contrato. La documentación interactiva está en `/docs` (Swagger UI) cuando `DOCS_ENABLED=true`.

## Convenciones (Art. IV)

- Base: `/api/v1`, JSON en `camelCase`, fechas y horas en UTC (ISO 8601); las fechas de calendario (`startDate`, `endDate`) son de `America/Mexico_City`.
- Dinero en **centavos de MXN** (enteros).
- Autenticación: `Authorization: Bearer <accessToken>`; el token de acceso dura 15 min y se renueva con `POST /api/v1/auth/refresh`.
- Errores con un solo formato: `{"error": {"code", "message", "details"?, "requestId"}}`. Códigos frecuentes: `validation_failed` (400), `unauthorized` (401), `forbidden` (403), `not_found` (404), `conflict` (409), `rate_limited` (429, con `Retry-After`).
- Listas paginadas con `limit` y `cursor`; la respuesta trae `items` y `nextCursor` (`null` = no hay más).
- Cada respuesta lleva `X-Request-Id`. Si el cliente envía uno válido (de 1 a 64 caracteres alfanuméricos o `-`), se respeta; si no, la API genera uno.
- Límites: 120 peticiones por minuto por IP y límites propios de auth, chat e imágenes (ver `docs/politicas.md`).

## Rutas

### infraestructura

Salud del proceso y de sus dependencias.

| Método | Ruta | Qué hace | Acceso |
|---|---|---|---|
| `GET` | `/healthz` | El proceso está vivo | Pública |
| `GET` | `/readyz` | El servicio puede atender peticiones | Pública |

### meta

Información del servicio para el cliente móvil.

| Método | Ruta | Qué hace | Acceso |
|---|---|---|---|
| `GET` | `/api/v1/meta` | Información del servicio | Pública |

### auth

Registro, inicio de sesión y tokens (spec 003).

| Método | Ruta | Qué hace | Acceso |
|---|---|---|---|
| `POST` | `/api/v1/auth/register` | Crear una cuenta | Pública |
| `POST` | `/api/v1/auth/login` | Iniciar sesión | Pública |
| `POST` | `/api/v1/auth/refresh` | Renovar la sesión | Pública |
| `POST` | `/api/v1/auth/logout` | Cerrar sesión | Pública |

### cuenta

La cuenta de la persona autenticada (spec 003).

| Método | Ruta | Qué hace | Acceso |
|---|---|---|---|
| `GET` | `/api/v1/me` | Mi cuenta | Sesión |
| `PATCH` | `/api/v1/me` | Editar mi perfil | Sesión |
| `DELETE` | `/api/v1/me` | Dar de baja mi cuenta | Sesión |

### usuarios

Perfiles públicos (spec 003).

| Método | Ruta | Qué hace | Acceso |
|---|---|---|---|
| `GET` | `/api/v1/users/{id}` | Perfil público de una persona | Sesión |

### imagenes

Subida de imágenes a Cloudinary con firma del servidor (spec 004).

| Método | Ruta | Qué hace | Acceso |
|---|---|---|---|
| `POST` | `/api/v1/uploads/signature` | Firma para subir una imagen | Sesión |

### objetos

Publicar, editar y pausar objetos; precio sugerido y categorías (spec 005).

| Método | Ruta | Qué hace | Acceso |
|---|---|---|---|
| `POST` | `/api/v1/items` | Publicar un objeto | Sesión |
| `GET` | `/api/v1/items/price-suggestion` | Precio sugerido por día | Sesión |
| `GET` | `/api/v1/items/{id}` | Detalle de un objeto | Sesión |
| `PATCH` | `/api/v1/items/{id}` | Editar un objeto (solo el dueño) | Sesión |
| `DELETE` | `/api/v1/items/{id}` | Eliminar un objeto (solo el dueño) | Sesión |
| `POST` | `/api/v1/items/{id}/pause` | Pausar un objeto (solo el dueño) | Sesión |
| `POST` | `/api/v1/items/{id}/resume` | Reanudar un objeto (solo el dueño) | Sesión |
| `GET` | `/api/v1/me/items` | Mis objetos | Sesión |
| `GET` | `/api/v1/categories` | Categorías | Sesión |

### busqueda

Buscar objetos por texto, categoría, zona, cercanía y fechas (spec 006).

| Método | Ruta | Qué hace | Acceso |
|---|---|---|---|
| `GET` | `/api/v1/items` | Buscar objetos | Sesión |

### reservas

Cotizar, solicitar, aceptar, rechazar y cancelar reservas (spec 007).

| Método | Ruta | Qué hace | Acceso |
|---|---|---|---|
| `GET` | `/api/v1/items/{id}/quote` | Cotizar un periodo | Sesión |
| `POST` | `/api/v1/bookings` | Solicitar un objeto | Sesión |
| `GET` | `/api/v1/bookings/{id}` | Detalle de una solicitud o reserva | Sesión |
| `GET` | `/api/v1/me/bookings` | Mis solicitudes y reservas | Sesión |
| `POST` | `/api/v1/bookings/{id}/accept` | Aceptar (propietario) | Sesión |
| `POST` | `/api/v1/bookings/{id}/reject` | Rechazar (propietario) | Sesión |
| `POST` | `/api/v1/bookings/{id}/cancel` | Cancelar | Sesión |
| `POST` | `/api/v1/bookings/{id}/meeting-point` | Punto de encuentro (propietario) | Sesión |

### chat

Conversación privada de texto de cada reserva (spec 008).

| Método | Ruta | Qué hace | Acceso |
|---|---|---|---|
| `GET` | `/api/v1/conversations` | Mis conversaciones | Sesión |
| `GET` | `/api/v1/conversations/unread-count` | Mis mensajes sin leer | Sesión |
| `GET` | `/api/v1/conversations/{id}/messages` | Mensajes de una conversación | Sesión |
| `POST` | `/api/v1/conversations/{id}/messages` | Enviar un mensaje | Sesión |
| `POST` | `/api/v1/conversations/{id}/read` | Marcar como leída | Sesión |

### entrega

Evidencia, QR de un solo uso y confirmación de la entrega y la devolución (spec 009).

| Método | Ruta | Qué hace | Acceso |
|---|---|---|---|
| `PUT` | `/api/v1/bookings/{id}/delivery/evidence` | Registrar la evidencia de la entrega (el propietario) | Sesión |
| `POST` | `/api/v1/bookings/{id}/delivery/qr` | Generar el QR de la entrega (el propietario) | Sesión |
| `POST` | `/api/v1/bookings/{id}/delivery/confirm` | Confirmar la entrega con el QR (el solicitante) | Sesión |
| `GET` | `/api/v1/bookings/{id}/delivery` | Ver la etapa de la entrega | Sesión |
| `PUT` | `/api/v1/bookings/{id}/return/evidence` | Registrar la evidencia de la devolución (el solicitante) | Sesión |
| `POST` | `/api/v1/bookings/{id}/return/qr` | Generar el QR de la devolución (el solicitante) | Sesión |
| `POST` | `/api/v1/bookings/{id}/return/confirm` | Confirmar la devolución con el QR (el propietario) | Sesión |
| `GET` | `/api/v1/bookings/{id}/return` | Ver la etapa de la devolución | Sesión |

### pagos

Reporte y confirmación del pago acordado, sin cobro (spec 009).

| Método | Ruta | Qué hace | Acceso |
|---|---|---|---|
| `POST` | `/api/v1/bookings/{id}/payments/{stage}/report` | Reportar el pago (solicitante) | Sesión |
| `POST` | `/api/v1/bookings/{id}/payments/{stage}/confirm` | Confirmar el pago recibido (propietario) | Sesión |

### calificaciones

Calificar a la otra parte de una reserva completada y ver las recibidas (spec 010).

| Método | Ruta | Qué hace | Acceso |
|---|---|---|---|
| `POST` | `/api/v1/bookings/{id}/ratings` | Calificar a la otra parte de una reserva | Sesión |
| `GET` | `/api/v1/bookings/{id}/rating-status` | Si puedo calificar esta reserva | Sesión |
| `GET` | `/api/v1/users/{id}/ratings` | Calificaciones recibidas por una persona | Sesión |

### incidencias

Reportar un incumplimiento con evidencia y consultarlo (spec 010).

| Método | Ruta | Qué hace | Acceso |
|---|---|---|---|
| `POST` | `/api/v1/bookings/{id}/incidents` | Reportar un incumplimiento de una reserva | Sesión |
| `GET` | `/api/v1/incidents/{id}` | Ver una incidencia | Sesión |
| `GET` | `/api/v1/me/incidents` | Mis incidencias | Sesión |

### administracion

Revisión de incidencias, sanciones y reportes; exige rol `admin` (specs 010 y 011).

| Método | Ruta | Qué hace | Acceso |
|---|---|---|---|
| `GET` | `/api/v1/admin/incidents` | Cola de incidencias (administración) | Sesión + rol `admin` |
| `POST` | `/api/v1/admin/incidents/{id}/notes` | Agregar una nota a una incidencia | Sesión + rol `admin` |
| `POST` | `/api/v1/admin/incidents/{id}/close` | Cerrar una incidencia con respuesta final | Sesión + rol `admin` |
| `POST` | `/api/v1/admin/users/{id}/sanctions` | Registrar una advertencia o suspensión | Sesión + rol `admin` |
| `GET` | `/api/v1/admin/reports` | Cola de reportes (administración) | Sesión + rol `admin` |
| `POST` | `/api/v1/admin/reports/{id}/resolve` | Resolver un reporte | Sesión + rol `admin` |

### avisos

Avisos dentro de la app y tokens de push (spec 011).

| Método | Ruta | Qué hace | Acceso |
|---|---|---|---|
| `GET` | `/api/v1/me/notifications` | Mis avisos | Sesión |
| `GET` | `/api/v1/me/notifications/unread-count` | Cuántos avisos no he leído | Sesión |
| `POST` | `/api/v1/me/notifications/read` | Marcar avisos como leídos | Sesión |
| `POST` | `/api/v1/me/push-tokens` | Registrar mi token de push | Sesión |
| `DELETE` | `/api/v1/me/push-tokens` | Eliminar mi token de push | Sesión |

### reportes

Reportar objetos, mensajes, perfiles y decisiones de moderación (spec 011).

| Método | Ruta | Qué hace | Acceso |
|---|---|---|---|
| `POST` | `/api/v1/reports` | Reportar un objeto, mensaje, perfil o decisión de moderación | Sesión |

## Notas

- `POST /api/v1/auth/logout` no exige el token de acceso: recibe el refresh token y lo revoca.
- `GET /api/v1/items` (búsqueda) y `POST /api/v1/items` (publicar) comparten ruta; la búsqueda nunca devuelve objetos propios.
- Las rutas `/api/v1/admin/*` exigen sesión y rol `admin`; las cuentas de administración se crean con `go run ./cmd/admin create-admin`.
- Utilidades de operación (no son rutas HTTP): `go run ./cmd/admin check-services | create-admin | indexes | seed`.
