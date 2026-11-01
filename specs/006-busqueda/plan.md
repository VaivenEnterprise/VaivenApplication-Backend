# Plan 006 — Búsqueda (backend)

- **Spec:** `spec.md` · **Dueño:** BE-B

## 1. Estructura
```
internal/search/
  domain/{query.go, card.go, cursor.go}
  usecase/{search.go, ports.go}
  adapter/http/{handler.go, dto.go, routes.go}
  adapter/mongo/{search_repo.go, pipelines.go, search_indexes.go}   # índices propios, sin tocar items_repo
api/paths/search.yaml
```
Lee las colecciones `items`, `users` y `bookings` solo en modo lectura.

## 2. Contrato
`GET /api/v1/items?q=&category=&modality=&municipality=&from=&to=&lat=&lng=&radiusKm=&limit=&cursor=`
→ `{ "items": [ItemCard], "nextCursor": "…" | null }`
```
ItemCard = { id, title, category, modality, pricePerDayCents, zone, approxLocation|null,
             distanceKm|null, thumbUrl|null, photoCount,
             owner:{ id, displayName, ratingAverage|null, ratingCount }, createdAt }
```
Se registra como `r.Get("/api/v1/items")` en el **mismo** router; `POST /items` pertenece a 005 (chi las combina por método).

## 3. Consultas (tres modos, todos con agregación)
| Modo | Cuándo | Etapas |
|---|---|---|
| Reciente | sin `q` ni `lat/lng` | `$match` → (`$lookup` disponibilidad) → `$sort {createdAt:-1,_id:-1}` → cursor → `$limit` |
| Texto | `q` sin ubicación | `$match {$text}` → `$addFields score` → `$sort {score:-1,_id:-1}` → cursor `(score,_id)` |
| Cercanía | `lat,lng` | `$geoNear` (primera etapa, `query` con filtros) → `distanceField` → `$sort {dist:1,_id:1}` → cursor `(dist,_id)` |

- `q` **con** cercanía: `$geoNear` + filtro regex escapado, insensible a mayúsculas, sobre título y descripción (limitación: `$text` y `$geoNear` no se combinan).
- Filtros comunes: `status:"active"`, `ownerId != yo`, `category`, `modality`, `zone.municipality`.
- Disponibilidad: `$lookup` a `bookings` con `pipeline` (`itemId`, `status ∈ {accepted,in_progress}`, `startDate <= to`, `endDate >= from`, `$limit:1`); se conserva el objeto si el resultado está vacío. Si `bookings` aún no existe, el `$lookup` devuelve vacío.
- Propietario: `$lookup` a `users` (campos de perfil público) y se descartan cuentas `deactivated` o suspendidas.

## 4. Índices (archivo propio `search_indexes.go`)
`{title:"text", description:"text"}` con `default_language:"spanish"`, pesos título 5 / descripción 1; `{status:1,category:1,modality:1,"zone.municipality":1,createdAt:-1}`.
(El `2dsphere` de `location` lo crea 005.) Si 007 aún no creó `{itemId:1,status:1,startDate:1,endDate:1}`, lo declara 007.

## 5. Decisiones
| # | Decisión | Art. |
|---|---|---|
| P-01 | Cursor = JSON en base64url `{k:<valor de orden>,id:<ObjectID>}`; firma HMAC no necesaria (no es dato sensible); cursor inválido → 400 | IV |
| P-02 | Las fechas del periodo son **fechas de calendario** `YYYY-MM-DD` (America/Mexico_City); `from` no puede ser anterior a hoy y `to ≥ from`; máximo 7 días (POL-02) | V |
| P-03 | `thumbUrl` se calcula con `images.Service.URLs` de la primera foto | XII |
| P-04 | Radio por defecto 25 km si hay `lat/lng` sin `radiusKm` | — |
| P-05 | El constructor de pipelines es una función pura que devuelve `[]bson.D`; así se prueba sin base de datos | VII |

## 6. Pruebas
Unitarias: validación de parámetros, codificación de cursor, constructor de pipelines (golden de cada modo). Integración (testcontainers): 30 objetos sembrados, paginación sin repetidos, texto en español (acentos), disponibilidad con reservas sembradas a mano, cercanía ordenada, objeto pausado/otro dueño excluido. Prueba de rendimiento ligera (5,000 documentos) con `-tags perf`.

## 7. Verificación de constitución
| Art. | Cumple |
|---|---|
| II | Solo MongoDB: texto, geo y `$lookup` nativos |
| III | Pipelines en `adapter`; `domain` sin `bson` |
| IV | Cursor y envoltorio estándar |
| VI | Solo tarjeta pública; ubicación aproximada |
| XII | Índices propios; sin editar `items` de 005 |
