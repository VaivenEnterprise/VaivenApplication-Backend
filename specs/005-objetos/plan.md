# Plan 005 — Objetos (backend)

- **Spec:** `spec.md` · **Dueño:** BE-A

## 1. Estructura
```
internal/items/
  domain/{item.go, pricing.go, categories.go, errors.go}
  usecase/{create.go, update.go, get.go, pause.go, delete.go, list_mine.go, suggest.go, ports.go}
  adapter/http/{handler.go, dto.go, routes.go}
  adapter/mongo/{item_repo.go, item_doc.go}
internal/policy/policy.go            # POL-xx como constantes (se crea aquí si no existe)
api/paths/items.yaml
```

## 2. Contrato
| Método | Ruta | Cuerpo → Respuesta |
|---|---|---|
| POST | `/api/v1/items` | `ItemInput` → 201 `Item` |
| GET | `/api/v1/items/{id}` | → 200 `Item` |
| PATCH | `/api/v1/items/{id}` | campos parciales → 200 `Item` |
| POST | `/api/v1/items/{id}/pause` · `/resume` | → 200 `Item` |
| DELETE | `/api/v1/items/{id}` | → 204 |
| GET | `/api/v1/me/items` | `?status=&cursor=&limit=` → `{items:[Item], nextCursor}` |
| GET | `/api/v1/items/price-suggestion` | `?declaredValueCents=` → `{suggestedPricePerDayCents}` |
| GET | `/api/v1/categories` | → `[{key,label}]` |

```
Item = { id, owner: PublicUser, title, description, category, condition, modality,
         declaredValueCents|null,        # null si quien consulta no es el dueño
         pricePerDayCents, priceSource, usageRules|null,
         zone:{municipality, neighborhood|null}, approxLocation:{lat,lng}|null,
         photos:[{publicId,width,height,format,bytes,delivery,url,thumbUrl}],
         status, createdAt, updatedAt, version }
```
(`GET /items` de listado/búsqueda pertenece a la spec 006.)

## 3. Datos
**`items`**: `_id`, `ownerId`, `title`, `description`, `category`, `condition`, `modality`, `declaredValueCents`, `pricePerDayCents`, `priceSource`, `usageRules`, `zone{municipality,neighborhood}`, `location` (GeoJSON `Point` `[lng,lat]` redondeado), `photos[]` (≤ 5 `Image`), `status` (`active`|`paused`|`blocked`|`removed`), `createdAt`, `updatedAt`, `version`.
Índices (este módulo): `{ownerId:1,status:1,createdAt:-1}`, `2dsphere` en `location`, `{status:1,category:1,createdAt:-1}`.
Índice de texto y los de búsqueda los agrega la spec 006 en **su propio archivo**.
Validación `$jsonSchema` mínima (tipos y rangos de dinero).

## 4. Decisiones
| # | Decisión | Art. |
|---|---|---|
| P-01 | Las políticas viven en `internal/policy` (constantes con nombre `POLxx…`); un cambio = un archivo | XII |
| P-02 | Sugerencia: `max(1000, ceilTo(round(declaredValueCents*3/100), 500))` en centavos (POL-04); función pura probada | V |
| P-03 | `priceSource=suggested` se valida recalculando en el servidor | V |
| P-04 | Redondeo de coordenadas a 2 decimales con `math.Round(x*100)/100` antes de crear `location` | VI.§6.11 |
| P-05 | Edición con control optimista: `PATCH` acepta `version`; desajuste → 409 | II §6.4 |
| P-06 | Fotos: `images.Service.Resolve(ctx,ownerID,"item_photo",refs)` en crear/editar; las nuevas sustituyen la lista completa | XII |
| P-07 | Puerto `ActiveBookingChecker` (de 003) para el borrado; vacío hasta 007 | XII |
| P-08 | `moderation.Checker` sobre `title`, `description`, `usageRules` | VI |
| P-09 | `declaredValueCents` se oculta en el DTO si quien consulta no es dueño ni admin | VI |

## 5. Pruebas
Unitarias: `pricing` (tabla de sugerencias y bordes), validaciones de modalidad/precio, redondeo de coordenadas, permisos de dueño, visibilidad por estado, DTO oculta valor declarado. Integración (testcontainers): crear→leer→pausar→reanudar→eliminar, índices creados, filtro por dueño y estado.

## 6. Verificación de constitución
| Art. | Cumple |
|---|---|
| II | `items` en Mongo, índices en código, fotos como `Image` |
| III | Capas; `domain` sin `mongo` |
| IV, V | Contrato en `api/paths/items.yaml`; dinero en centavos |
| VI | Ubicación aproximada, moderación, valor declarado privado |
| XII | `images.Service`, puertos, registro en `modules.go` |

## 7. Ajustes durante la implementación (2026-11-08, BE-B a pedido del responsable; pendientes de visto bueno de BE-A)
| # | Ajuste | Motivo |
|---|---|---|
| A-01 | P-07: puerto propio `usecase.BookingChecker.HasActiveBookings(ctx, itemID)` con `NoActiveBookings` hasta la 007, en lugar del `ActiveBookingChecker` de 003 | El de 003 es por persona; el borrado necesita saber si **el objeto** tiene reservas |
| A-02 | Préstamo (`loan`) exige `priceSource=manual` | La sugerencia de POL-04 es una tarifa de renta; en préstamo el precio siempre es 0 |
| A-03 | `paused`/`blocked` responden **404** a terceros (CA-06 admite 404/403) | No revelar que el objeto existe |
| A-04 | Un objeto `blocked` no se edita, pausa ni reanuda (409); sí se puede eliminar | Solo moderación (011) lo desbloquea; la baja lógica no borra evidencia |
| A-05 | `PATCH` exige `version` y al menos otro campo; `null` borra `usageRules` y `approxLocation` y es 400 en los demás campos; `photos` reemplaza la lista | P-05 y contrato `ItemUpdate` |
| A-06 | `items.Moderation` (`Block`, `Snapshot`) vive en `internal/items/port` (como `images/port`); lo implementa `usecase.Service` | Art. III: la 011 lo importa sin depender de adaptadores |
| A-07 | Archivos además de los listados: `adapter/mongo/schema.go` (validador `$jsonSchema` de items, se crea con la colección, basta `readWrite`), `adapter/mongo/item_doc_test.go`, `port/port.go`, `usecase/moderation.go`; `internal/policy` solo con POL-01, 04, 13 y 14 (POL-16 llega por `moderation.Checker`) | Plan §3 (validación mínima) y T006 |
| A-08 | `GET /me/items`: `limit` 1–50 (20 por defecto); `nextCursor` opaco = base64url(`createdAt` en ms + id), orden `createdAt` desc y `_id` desc | RF-10 |
| A-09 | `cmd/api/main.go`: `newImagesModule` devuelve también `images.Service`; `RequireActive(userRepo.ByID)` solo en `POST /items`; una sola instancia de `moderation.Checker` compartida | RF-12 y Art. XII.4 |

[NEEDS CLARIFICATION] (para la spec 007): entre `HasActiveBookings` y la baja del objeto podría aceptarse una reserva. La 007 debe hacerlo atómico: al aceptar una reserva comprobar que el objeto siga `active` (filtro por `status` en la misma escritura) o usar una transacción.
