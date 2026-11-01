---
name: api-rest-conventions
description: Convenciones de la API REST de VAIVÉN - rutas, verbos, JSON camelCase, envoltorio de errores, paginación por cursor, encabezados y OpenAPI. Úsala al crear o modificar endpoints, respuestas de error o api/openapi.yaml.
---

# Skill: Convenciones de API REST

> Estas reglas son compartidas con la app móvil (Art. IV de ambas constituciones). No las cambies de un solo lado.

## Rutas y verbos
- Prefijo `/api/v1`. Infraestructura fuera: `/healthz`, `/readyz`.
- Recursos en plural y minúsculas: `/api/v1/things`, `/api/v1/things/{id}`.
- `GET` lee, `POST` crea (201 + `Location`), `PUT` reemplaza, `PATCH` modifica parcialmente, `DELETE` elimina (204).
- Acciones que no encajan en CRUD: `POST /api/v1/things/{id}/acciones` con verbo en el subrecurso (definido en la spec).

## JSON
- Campos `camelCase`; fechas ISO-8601 UTC; ids como cadena; dinero `…Cents` entero (MXN).
- Respuesta de éxito: el recurso directamente (sin envoltorio). Lista: `{ "items": [...], "nextCursor": "..." | null }`.
- Paginación por cursor opaco: `?limit=20&cursor=...` (límite por defecto 20, máximo 100).

## Errores (envoltorio único)
```json
{ "error": { "code": "validation_failed", "message": "Datos inválidos.",
             "details": [{ "field": "name", "issue": "es obligatorio" }],
             "requestId": "..." } }
```
| code | HTTP |
|---|---|
| `validation_failed` | 400 (o 422 si la spec lo define) |
| `unauthorized` | 401 |
| `forbidden` | 403 |
| `not_found` | 404 |
| `conflict` | 409 |
| `rate_limited` | 429 |
| `internal_error` | 500 |
| `service_unavailable` | 503 |
- `message` en español, sin detalles internos ni trazas. `details` solo en validación.
- Nunca filtres información sensible en errores.

## Encabezados
- Respuesta siempre con `X-Request-Id` y `Content-Type: application/json; charset=utf-8`.
- Si la petición trae `X-Request-Id` válido, se reutiliza; si no, se genera.

## OpenAPI
- Todo endpoint se describe en `api/openapi.yaml` **antes** de implementarlo: ruta, parámetros, cuerpos, respuestas y esquema `Error`.
- Un cambio incompatible se publica en una nueva versión de ruta.

## Lista de comprobación
- [ ] ¿Ruta, verbo y código HTTP correctos?
- [ ] ¿Respuesta de error con el envoltorio único y `requestId`?
- [ ] ¿`openapi.yaml` actualizado?
- [ ] ¿Validación en la frontera con mensajes en español?
- [ ] ¿Sin datos sensibles en la respuesta?
