# Plan 012 — Integración, endurecimiento y entrega (backend)

- **Spec:** `spec.md` · **Revisión arquitectónica:** 2026-11-09 (sección 6)

## 1. Archivos
```
test/e2e/{journey_test.go, helpers_test.go, fake_cloudinary_test.go}      # -tags e2e, solo local
cmd/admin/{seed.go, seed_data.go, seed_upload.go, indexes.go, assets/*.jpg} (+ tests)
cmd/admin/testdata/indexes_baseline.json                                  # T004: capturado ANTES de tocar repositorios
cmd/admin/main.go (modificar: subcomandos `seed` e `indexes`)
internal/adapter/mongo/{indexes.go, indexes_test.go, indexes_compare.go, indexes_compare_test.go} (modificar/crear: IndexSpec, IndexDeclarer, Models y comparador)
internal/adapter/mongo/catalog/{catalog.go, catalog_test.go, catalog_integration_test.go} (nuevo)
14 repositorios (modificar: declaración de índices como datos; ver §7)
internal/adapter/http/middleware/{security_headers.go, security_headers_test.go}
internal/adapter/http/router.go (modificar: montar las cabeceras)
internal/<módulo>/adapter/http/{dto.go|handler.go} (modificar solo donde falte distinguir *http.MaxBytesError; lo corrige el dueño del módulo)
internal/auth/adapter/http/middleware.go (modificar: RequireRole consulta el rol en la base; AUD-017)
scripts/scan-archives.py, scripts/scan_archives_test.py (modificar: AUD-015)
tests/load/ (extender el k6 existente con GET /items y GET /me/bookings)
Makefile (modificar: `make lint-api` y `make e2e`)
docs/{api.md, despliegue.md, politicas.md, demo.md, validacion.md, seguridad.md, rendimiento.md}
docs/seguridad/remediacion-2026-11.md (modificar: AUD-018)
README.md (actualizar)
```
No se crea `ratelimit_global.go`: el límite global ya existe (decisión P-07).

## 2. Decisiones
| # | Decisión | Art. |
|---|---|---|
| P-01 | E2E en proceso: `httptest.Server` con el router real, Mongo por Testcontainers (Docker o Podman) y Cloudinary falso (`httptest`). No se conecta a ningún servicio remoto | VII |
| P-02 | El recorrido usa un **cliente HTTP de prueba** (helpers `register`, `login`, `do`) que reutiliza los DTO del contrato | VII |
| P-03 | Reloj inyectable en jobs (expiración, retrasos) para **adelantar el tiempo** en E2E; se reutiliza `internal/platform/clock` | VII |
| P-04 | `seed` usa los casos de uso y repositorios reales (no inserta documentos a mano), excepto fechas del pasado de reservas de demostración | III |
| P-05 | **`Indexes()` es la fuente única de verdad de los índices.** `EnsureAll` (002-T014) solo crea índices y no expone una lista legible, por eso cada repositorio declara sus índices como datos: una función pura de su paquete (sin conexión, sin instancia del repositorio, sin infraestructura) que devuelve `[]adaptermongo.IndexSpec`; el método `Indexes()` la devuelve y `EnsureIndexes` la consume con `CreateMany(ctx, adaptermongo.Models(r.Indexes()))`, conservando su prefijo de error. El paquete `internal/adapter/mongo/catalog` reúne las 14 declaraciones en `All()` sin construir repositorios ni ejecutar `EnsureIndexes`. `indexes` compara `catalog.All()` con `listIndexes`. No se usa una base temporal. Detalle en §7 | II |
| P-06 | Validación OpenAPI en local con `npx @redocly/cli lint api/openapi.yaml` (no agrega dependencia al repositorio) | VIII |
| P-07 | **Se conserva el limitador existente** (`internal/adapter/http/middleware/ratelimit.go`, puerto `ratelimit.Limiter`): 120 peticiones/min por IP en Redis; si Redis falla, pasa a un límite local por proceso y lo registra una vez (Art. XIV.2). La IP sale de `RemoteAddr` y **no se confía en `X-Forwarded-For`**, porque la API recibe el tráfico directo, sin proxy (Art. XIV.4); si algún día hay un proxy, esto debe revisarse. No se crea un segundo limitador | XIV |
| P-08 | `docs/seguridad.md` es una lista de comprobación firmada por quien revisa, con la fecha | VI |
| P-09 | Cabeceras en un único middleware global: `nosniff` en todo; `no-store` cuando la petición lleva identidad autenticada. No se duplica el `no-store` que ya pone `handover`, ni las cabeceras de `/docs` | VI |
| P-10 | El límite de 1 MiB ya existe (`limitBody` en `router.go`) y el cuerpo nunca se lee más allá. T003 verifica que **cada** módulo que decodifica JSON distinga `*http.MaxBytesError` y responda 400 `validation_failed` «El cuerpo de la petición es demasiado grande.», como ya hacen auth, bookings, handover e items; chat, ratings, incidents, reports, sanctions, notifications e images hoy responden un genérico «no es un JSON válido». No se usa 413: el contrato no lo declara | IV |
| P-11 | `RequireRole` reutiliza el `UserLookup` que ya usa `RequireActive`: compara el rol del token con el de la base y responde 403 si no coinciden. Un solo punto de autorización; los módulos no cambian | VI |
| P-12 | Credenciales del `seed`: archivo `correo=contraseña` fuera del repositorio, indicado por `SEED_CREDENTIALS_FILE`; 7 entradas distintas (6 personas y el admin), contraseñas no repetidas y de largo mínimo de la política de auth. Sin archivo válido, `seed` termina con código ≠ 0 sin escribir nada | VI |
| P-13 | Idempotencia del `seed` por búsqueda, no por sobrescritura: persona por correo; objeto por dueña y título; reserva por objeto y solicitante (no por fechas: son relativas a hoy y cambiarían de un día a otro; ajuste de T005). Si ya existe, se omite y se cuenta como «existente»; solo se completa la transición de una reserva que quedó `pending` por una ejecución interrumpida. Las 3 reservas son `pending`, `accepted` y `cancelled`, todas con fechas futuras y por los casos de uso, así que no hace falta la excepción de fechas pasadas de P-04. Los avisos van a `notify.Noop`: los datos de demostración no generan avisos ni push | II |
| P-14 | Fotos del `seed`: `images.Service` solo firma (no sube desde el servidor). `seed_upload.go` pide la firma, sube el archivo a Cloudinary con esa firma y luego llama `Resolve`, igual que la app. No se agrega un método al puerto. Las fotos se leen del binario o del repositorio en memoria (sin archivos temporales); si algo falla después de subir, `seed_upload.go` borra la foto con la API firmada `destroy` de Cloudinary y lo informa sin mostrar firma ni secreto (CA-09). Con `--scale` (solo development y test) las fotos son locales (`localImages`/`localPhotos`, sin Cloudinary): referencias en la carpeta de cada persona con metadatos fijos, para no subir miles de imágenes | III, XII |
| P-15 | Rendimiento con el k6 existente (`tests/load`, spec 017) contra la API local con `seed --scale 100` en MongoDB local; nunca contra staging en esta tarea | VII |

## 3. Orden de trabajo (4 días)
| Día | Foco |
|---|---|
| 19 nov | E2E del flujo principal (RF-01), `indexes`, cabeceras y cuerpo demasiado grande |
| 20 nov | `seed`, rendimiento y `explain`, OpenAPI local |
| 21 nov | Despliegue final en staging, `seed`, revisión de seguridad |
| 22 nov | Documentación, `demo.md`, revisión de conformidad, congelar código |

## 4. Pruebas
- La propia E2E (local).
- Pruebas unitarias del middleware de cabeceras y de `RequireRole` con un rol distinto en la base (CA-06).
- `seed` dos veces sobre Mongo de Testcontainers (CA-02), con `APP_ENV=production` y con un archivo de credenciales inválido.
- `indexes`: las 15 pruebas de §7.6 (CA-03, CA-10).
- Escáner: casos «comprimido renombrado en el historial» y «formato no soportado» (AUD-015).
- Cobertura: no se agregan pruebas solo para subir el porcentaje.

## 5. Verificación de constitución
| Art. | Cumple |
|---|---|
| II | Índices declarados en código con una sola fuente (P-05); `indexes` solo lee con rol `read`; `seed` idempotente por búsqueda |
| III | `seed` usa casos de uso; sin `mongo`/`bson` en `domain` ni `usecase` |
| IV | 400 y 403 en el formato de error estándar; sin códigos nuevos fuera del contrato |
| VI | Revisión de seguridad documentada; rol de admin contra la base; sin contraseñas en el repositorio |
| VII | E2E + cobertura global |
| IX | Despliegue verificado con `/readyz` |
| XII | Sin cambiar contratos; cualquier hallazgo se corrige en el módulo dueño |
| XIV | Un solo limitador, en Redis, con respaldo local documentado |

## 6. Revisión arquitectónica (2026-11-09)
Comprobado en el código:
- Límite global de 120/min por IP en Redis con respaldo local: `cmd/api/main.go:84` y `router.go:72`. No confía en `X-Forwarded-For` (`ratelimit.go:72`).
- Límite de cuerpo de 1 MiB global (`router.go:19` y `limitBody`); 7 módulos no distinguen el cuerpo demasiado grande en su mensaje de error (P-10). Ningún módulo usa 413 y el contrato no lo declara.
- `Cache-Control: no-store` solo en `handover`; `nosniff` solo en `/docs`.
- `RequireRole` confía solo en el claim (`internal/auth/adapter/http/middleware.go:54`); ya existe `UserLookup` para reutilizar.
- `images.Service` expone `Signature`, `Resolve` y `URLs`, no una subida desde el servidor (P-14).
- 002-T014 (`EnsureAll`) está cerrada y basta para que `seed` cree colecciones e índices. **No** basta para `indexes`: los 14 `EnsureIndexes` solo crean índices y no exponen una lista legible (corregido en P-05 y §7).
- Servidor con `ReadTimeout`, `ReadHeaderTimeout` y `WriteTimeout` (`cmd/api/main.go:415`); `/healthz` y `/readyz` existen; la configuración se valida al inicio (`internal/config`).
- La prueba de arquitectura (`architecture_test.go`) vigila las dependencias entre capas.

No comprobado aquí (queda para T008 y T011): lógica de negocio en handlers, uso de contexto y cancelación en cada repositorio, y la validación de entradas módulo por módulo.

## 7. Diseño de T004: `Indexes()` como fuente única de verdad (2026-11-09)

### 7.1 Contrato
En `internal/adapter/mongo/indexes.go` (paquete que hoy contiene `EnsureAll` y no importa ningún módulo):

```go
// IndexSpec declara un índice. Usa el IndexModel del driver para no perder
// opciones (texto, 2dsphere, filtro parcial, TTL, collation…).
type IndexSpec struct {
	Collection string
	Model      drv.IndexModel // Keys: bson.D ordenado; Options: builder del driver
}

// IndexDeclarer lo implementa cada repositorio que tiene índices.
type IndexDeclarer interface {
	Indexes() []IndexSpec
}

// Models devuelve los IndexModel de specs, en el mismo orden.
func Models(specs []IndexSpec) []drv.IndexModel
```

En cada repositorio:
- una **función de paquete pura** que declara sus índices (sin conexión, sin instancia del repositorio y sin dependencias de infraestructura);
- el método `Indexes() []adaptermongo.IndexSpec`, que devuelve esa función;
- `EnsureIndexes`, que consume esa misma declaración: `CreateMany(ctx, adaptermongo.Models(r.Indexes()))`, con el mismo prefijo de error de hoy.

Los tres repositorios que hoy usan `CreateOne` (users, payments, sanctions) pasan a `CreateMany` con un solo elemento **solo si** la prueba de equivalencia (§7.6, prueba 1) demuestra el mismo resultado.

Las opciones se leen sin perder información aplicando las funciones `Opts` del `options.IndexOptionsBuilder` sobre un `options.IndexOptions`, que tiene todos los campos del driver.

### 7.2 Catálogo
`internal/adapter/mongo/catalog` expone `func All() []adaptermongo.IndexSpec`, que concatena las 14 declaraciones **sin construir repositorios ni ejecutar `EnsureIndexes`**. El catálogo debe estar **completo**: un catálogo parcial no es una solución aceptable.

Dependencias resultantes (sin ciclos):
- `internal/<módulo>/adapter/mongo` → `internal/adapter/mongo` (por `IndexSpec`);
- `internal/adapter/mongo/catalog` → los 13 paquetes `adapter/mongo` y `internal/adapter/mongo`;
- `cmd/admin` → `catalog`;
- `internal/adapter/mongo` sigue sin importar ningún módulo; `domain` y `usecase` no tocan el driver (Art. III).

### 7.3 Inventario: 14 repositorios y 34 índices
Además de estos, cada colección tiene el `_id_` de MongoDB, que el comando ignora. Ninguno declara `sparse`, `collation`, `hidden` ni `wildcardProjection`.

| # | Colección | Nombre | Claves (en orden) | Opciones | Declarado en | Dueño |
|---|---|---|---|---|---|---|
| 1 | refresh_tokens | tokenHash_unique | tokenHash:1 | unique | auth/refresh_repo.go | BE-A |
| 2 | refresh_tokens | familyId | familyId:1 | — | auth/refresh_repo.go | BE-A |
| 3 | refresh_tokens | userId | userId:1 | — | auth/refresh_repo.go | BE-A |
| 4 | refresh_tokens | expiresAt_ttl | expiresAt:1 | expireAfterSeconds 0 | auth/refresh_repo.go | BE-A |
| 5 | users | email_unique | email:1 | unique | auth/user_repo.go | BE-A |
| 6 | bookings | item_status_dates | itemId:1, status:1, startDate:1, endDate:1 | — | bookings/booking_repo.go | BE-A |
| 7 | bookings | requester_status_createdAt | requesterId:1, status:1, createdAt:-1 | — | bookings/booking_repo.go | BE-A |
| 8 | bookings | owner_status_createdAt | ownerId:1, status:1, createdAt:-1 | — | bookings/booking_repo.go | BE-A |
| 9 | bookings | status_expiresAt | status:1, expiresAt:1 | — | bookings/booking_repo.go | BE-A |
| 10 | bookings | status_lateAt | status:1, lateAt:1 | — | payments/payment_repo.go | BE-A |
| 11 | items | owner_status_createdAt | ownerId:1, status:1, createdAt:-1 | — | items/item_repo.go | BE-A |
| 12 | items | location_2dsphere | location:"2dsphere" | — | items/item_repo.go | BE-A |
| 13 | items | status_category_createdAt | status:1, category:1, createdAt:-1 | — | items/item_repo.go | BE-A |
| 14 | items | search_text | title:"text", description:"text" | default_language spanish; weights title 5, description 1 | search/search_indexes.go | BE-B |
| 15 | items | search_filters | status:1, category:1, modality:1, zone.municipality:1, createdAt:-1 | — | search/search_indexes.go | BE-B |
| 16 | items | search_recent | status:1, createdAt:-1, _id:-1 | — | search/search_indexes.go | BE-B |
| 17 | conversations | bookingId_unique | bookingId:1 | unique | chat/conversation_repo.go | BE-B |
| 18 | conversations | participant_lastMessageAt | participantIds:1, lastMessageAt:-1, _id:-1 | — | chat/conversation_repo.go | BE-B |
| 19 | messages | conversation_seq | conversationId:1, seq:-1, _id:-1 | — | chat/message_repo.go | BE-B |
| 20 | messages | conversation_sender_clientId_unique | conversationId:1, senderId:1, clientId:1 | unique; partialFilterExpression `{clientId: {$type: "string"}}` | chat/message_repo.go | BE-B |
| 21 | ratings | booking_rater_unique | bookingId:1, raterId:1 | unique | ratings/rating_repo.go | BE-B |
| 22 | ratings | ratee_id | rateeId:1, _id:-1 | — | ratings/rating_repo.go | BE-B |
| 23 | incidents | booking_status | bookingId:1, status:1 | — | incidents/incident_repo.go | BE-B |
| 24 | incidents | reporter_createdAt | reporterId:1, createdAt:-1 | — | incidents/incident_repo.go | BE-B |
| 25 | incidents | reported_createdAt | reportedId:1, createdAt:-1 | — | incidents/incident_repo.go | BE-B |
| 26 | incidents | status_createdAt | status:1, createdAt:1 | — | incidents/incident_repo.go | BE-B |
| 27 | sanctions | user_createdAt | userId:1, createdAt:-1 | — | sanctions/sanction_repo.go | BE-B |
| 28 | notifications | user_id | userId:1, _id:-1 | — | notifications/notification_repo.go | BE-B |
| 29 | notifications | user_read | userId:1, readAt:1 | — | notifications/notification_repo.go | BE-B |
| 30 | notifications | created_ttl | createdAt:1 | expireAfterSeconds `RetentionSeconds` (90 días) | notifications/notification_repo.go | BE-B |
| 31 | push_tokens | token_unique | token:1 | unique | notifications/token_repo.go | BE-B |
| 32 | push_tokens | user | userId:1 | — | notifications/token_repo.go | BE-B |
| 33 | reports | reporter_target_unique | reporterId:1, targetType:1, targetId:1 | unique | reports/report_repo.go | BE-B |
| 34 | reports | status_created | status:1, createdAt:1 | — | reports/report_repo.go | BE-B |

Colecciones con más de un declarante: `items` (items 3 + search 3) y `bookings` (bookings 4 + payments 1). Por eso los sobrantes se calculan sobre la **unión** de todos los declarantes de la colección, no por repositorio.

Archivos por dueño: **BE-A, 5** (auth/refresh_repo.go, auth/user_repo.go, items/item_repo.go, bookings/booking_repo.go, payments/payment_repo.go); **BE-B, 9** (search/search_indexes.go, chat/conversation_repo.go, chat/message_repo.go, ratings/rating_repo.go, incidents/incident_repo.go, sanctions/sanction_repo.go, notifications/notification_repo.go, notifications/token_repo.go, reports/report_repo.go). **BE-A debe aprobar formalmente el cambio de sus 5 archivos o autorizar por escrito a BE-B a hacerlo** (Art. XII). Sus repositorios no se excluyen.

### 7.4 Comparador y clasificación
Se separan tres conceptos:
- **Identidad:** el nombre del índice.
- **Equivalencia estructural:** claves (con su orden) y opciones efectivas.
- **Compatibilidad de creación:** lo que haría `createIndexes` en el servidor. El comando **no la predice** ni afirma que `EnsureIndexes` vaya a fallar.

Clasificación por colección, emparejando primero por nombre:

| Estado | Condición | ¿Discrepancia? |
|---|---|---|
| `OK` | Existe con el nombre declarado y es estructuralmente equivalente | No |
| `INCOMPATIBLE` | Existe con el nombre declarado, pero cambian claves u opciones (se dice cuál) | Sí |
| `FALTANTE` | No existe un índice con el nombre declarado; se reporta como faltante **aunque** exista otro equivalente con otro nombre | Sí |
| `SOBRANTE` | Existe, no está declarado en la colección y no es `_id_` | Sí |
| `NOMBRE DISTINTO` | Nota **informativa** que une un `FALTANTE` con un `SOBRANTE` estructuralmente equivalente; no convierte ninguno en correcto | No por sí misma (las dos discrepancias se mantienen) |
| `NO VERIFICABLE` | El índice real tiene una opción que el comparador no sabe interpretar; nunca se asume equivalencia | Sí |

Una colección declarada que no existe en la base deja todos sus índices como `FALTANTE`.

### 7.5 Normalización
- **Números BSON:** `int32`, `int64` y `double` se tratan como iguales cuando su valor es el mismo (claves, `expireAfterSeconds`, pesos).
- **Opciones efectivas** que se comparan cuando corresponden: `unique`, `sparse`, `expireAfterSeconds`, `partialFilterExpression`, `weights`, `default_language`, `language_override`, `collation`, `hidden` y `wildcardProjection`.
- **Una opción ausente no equivale automáticamente a `false`** ni a un valor por defecto: solo se iguala al valor por defecto del servidor cuando ese valor está documentado y declarado de forma explícita en el comparador; en cualquier otro caso, la diferencia se informa.
- **Filtros parciales:** se comparan preservando su semántica y los tipos relevantes (por ejemplo, `$type: "string"` no equivale a `$type: 2` salvo una regla explícita).
- **Índices de texto:** MongoDB los guarda como `{_fts: "text", _ftsx: 1}`; se comparan los campos y pesos de `weights`, `default_language` y `language_override` contra la declaración.
- **2dsphere:** se compara la clave `"2dsphere"` sin perder su semántica; `2dsphereIndexVersion` se ignora salvo que la declaración lo fije.
- **Opción desconocida:** `NO VERIFICABLE`.
- **Solo se ignoran metadatos del servidor** que no forman parte de la declaración: `v`, `ns`, y `textIndexVersion` y `2dsphereIndexVersion` cuando no se declaran.

### 7.6 Comando `indexes`, códigos de salida y pruebas
- Solo envía `ping`, `listCollections` y `listIndexes`; funciona con un usuario de rol `read`; nunca crea, modifica ni borra índices.
- Salida legible por colección y un resumen; nunca muestra la URI, contraseñas, tokens ni credenciales.
- Códigos: **0** todo correcto; **1** alguna discrepancia (`INCOMPATIBLE`, `FALTANTE`, `SOBRANTE` o `NO VERIFICABLE`); **2** error de configuración, conexión, plazo o ejecución.
- **Separación del proceso:** la función de comparación devuelve el código y solo `main` llama a `os.Exit`, después de que los `defer` cerraron la conexión a MongoDB (mismo patrón que `run` en `cmd/admin/main.go`). Se cubre con una prueba.

**Fixture de equivalencia.** Antes de modificar cualquier repositorio se captura `cmd/admin/testdata/indexes_baseline.json` ejecutando los `EnsureIndexes` **actuales** en una base **limpia** de Testcontainers (nunca en una base ya usada). Contiene los 34 índices, sin `_id_`, con colección, nombre, claves y orden, opciones, TTL, filtros parciales, y pesos y configuración de idioma de los de texto.

**Pruebas** (las de integración, con MongoDB de Testcontainers y `-tags integration`):

| # | Prueba | Tipo |
|---|---|---|
| 1 | Equivalencia de los 34 índices creados por los `EnsureIndexes` refactorizados contra `indexes_baseline.json` | integración |
| 2 | Idempotencia de `EnsureIndexes` (dos ejecuciones sin error ni cambios) | integración |
| 3 | Conservación del prefijo de error de cada `EnsureIndexes` | unitaria |
| 4 | Índice faltante → `FALTANTE`, código 1 (CA-03) | integración |
| 5 | Índice sobrante → `SOBRANTE`, código 1 | integración |
| 6 | Mismo nombre con otras claves u opciones → `INCOMPATIBLE`, nombrando la diferencia | integración |
| 7 | Equivalente con otro nombre → `FALTANTE` + `SOBRANTE` con la nota `NOMBRE DISTINTO`, código 1 | integración |
| 8 | Usuario con rol `read` sobre la base: el comando funciona | integración |
| 9 | Monitor de comandos del driver: solo `ping`, `listCollections` y `listIndexes` | integración |
| 10 | Error de conexión y plazo vencido → código 2, sin URI; la conexión se cierra antes de salir | unitaria |
| 11 | Normalización de números BSON (`int32`, `int64`, `double`) | unitaria |
| 12 | Normalización de índices `text` (`_fts`/`_ftsx`, pesos, idioma) y `2dsphere` | unitaria + integración |
| 13 | Justificación funcional: cada uno de los 34 tiene su propósito documentado y, si es de consulta u orden, un `explain` que lo usa (CA-10a) | integración |
| 14 | Análisis razonado de redundancia, documentado (CA-10b) | revisión + unitaria |
| 15 | Detección AST del desfase del catálogo: `go/parser` encuentra todo `func (…) EnsureIndexes(` del proyecto y falla si su paquete no aporta a `catalog.All()`; no modifica código ni depende de `cmd/api` | unitaria |

**Usuario de solo lectura en Testcontainers.** La prueba 8 crea un usuario con rol `read` sobre la base de la prueba y se conecta con él. La autenticación con replica set exige keyFile; si el módulo de Testcontainers no lo resuelve, esta prueba usa un contenedor MongoDB standalone con autenticación (`listIndexes` no necesita replica set). **No** se sustituye por una base temporal con permisos de escritura.

### 7.7 Riesgos
1. **Desfase del catálogo:** mitigado por la prueba 15.
2. **Normalización de texto y 2dsphere:** cubierta por la prueba 12 contra MongoDB real.
3. **Cambio sutil en `EnsureIndexes`:** mitigado por el fixture y la prueba 1.
4. **Archivos de otros dueños:** requiere la aprobación de BE-A para sus 5 archivos (§7.3).
5. **Autenticación en Testcontainers:** alternativa standalone documentada en §7.6.
