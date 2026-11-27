# Plan 021 — Baja de cuenta limpia y datos sin `null` (backend)

- **Spec:** `spec.md` · **Estado:** aprobada por el responsable (BE-B) el 2026-11-26; visto bueno de BE-A pendiente para la parte de auth (spec 003) · **Constitución:** v2.0.0
- **Sin dependencias nuevas** (Art. VIII): solo el driver oficial de MongoDB v2, `slog` y la biblioteca estándar.

## 1. Enfoque

1. **Primero «sin null»** (Fase 1): el código deja de escribir `null`, con validadores todavía tolerantes; después la migración y la verificación; al final se endurecen los validadores. Así el despliegue no tiene corte (§9).
2. **Luego la baja atómica** (Fase 2): un orquestador en `internal/auth/usecase` abre **una** transacción y llama a un puerto pequeño que cada módulo implementa sobre sus propias colecciones (Art. II y XII).
3. **Al final lo externo y la retención** (Fase 3): fotos en Cloudinary (outbox con reintento), `purgeAt` con TTL, lecturas de contrapartes y documentación.

## 2. Verificación de lo leído en el código (impacta el diseño)

| Hallazgo | Consecuencia |
|---|---|
| Solo `users`, `refresh_tokens` e `items` tienen `$jsonSchema` (`EnsureSchemas` en `internal/auth/adapter/mongo/schema.go` e `items/adapter/mongo/schema.go`); el nivel es `moderate`, la acción `error` | «Todos los `$jsonSchema`» son esos tres. Las demás colecciones **no tienen validador**: se les agrega uno solo de campos opcionales (§5.3) |
| El `$jsonSchema` de `users` **no** tiene el estado `suspended`: el enum es `active` / `deactivated` y la suspensión es `suspendedUntil` | La spec §5.2 habla de «active o suspended»; el esquema condicional usa `status` `active` / `deactivated` |
| Un *slice* o *map* `nil` se codifica en BSON como `null` por defecto (`Payments`, `StatusHistory`, `Adjustments`, `Unread`, `Data`, `Turns`, `Neighbors`…) | No basta con `omitempty` en punteros: se activa `NilSliceAsEmpty` y `NilMapAsEmpty` en el cliente (`internal/platform/mongodb/Connect`, hoy sin `BSONOptions`), y los opcionales sin significado de «vacío» llevan `omitempty` |
| La transacción ya se usa con `client.StartSession()` + `WithTransaction` y el `ctx` de la transacción viaja por los repositorios (`bookings/accept_tx.go`, `chat/sender.go`, `ratings/rating_repo.go`) | Mismo patrón: el helper `Tx` recibe un `func(ctx)` y cada puerto usa ese `ctx`, sin importar `mongo` en `usecase` (Art. III.2) |
| `images.Service` (`internal/images/port`) **no** tiene operación de borrado y `AdminClient` solo tiene `GetResource` | Se agrega un puerto **separado** `images/port.Destroyer` (no se rompen los *fakes* de `Service`) y `AdminClient.DeleteResource` |
| `HasActiveBookings` solo mira aceptadas y en curso; las pendientes no se tratan | El eraser de `bookings` las cierra dentro de la misma transacción (§4, paso 3) |
| `chat.conversations.lastMessage.preview` guarda un **extracto del texto** del último mensaje | Además de `messages.body`, hay que reemplazar el `preview` si el último mensaje es de la cuenta eliminada |
| `bookings.itemSnapshot.firstPhoto` guarda el `publicId` de la foto del objeto | Al borrar la foto en Cloudinary el historial ajeno mostraría imagen rota. Decisión **propuesta** en §8 (R-02) |
| `ratings.AddRating` (`UserRepo.AddRating`) hace `$inc` de `ratingSum`/`ratingCount` y `UserRepo.Suspend` fija `suspendedUntil` sin filtrar por estado | Sobre una cuenta eliminada **recrearían** campos que la baja quitó. Se filtra `status = active` (§6) |
| `reports.Snapshot` (items `port.Moderation`) devuelve el objeto «incluso eliminado» (`removed`); con borrado físico dejará de existir | La moderación debe tolerar `not_found` en un reporte de un objeto borrado (R-03) |
| Las evidencias de incidencias son imágenes de Cloudinary (`evidence[]`) | Un TTL borra el documento pero no la imagen: se encolan los borrados con fecha `purgeAt` (§7) |
| `reco_states` guarda el reinicio como documento `_id = "reset:<userId>"` | Se borra por `_id`, no por `userId` |
| La API corre con una réplica; el worker de reservas usa `RunExpirer` con *ticker* y es idempotente (Art. XIV) | El worker de bajas copia ese patrón |

## 3. Arquitectura y puertos (domain → usecase → adapter → cmd)

```
cmd/api/main.go            arma: TxRunner, erasers de cada módulo, outbox, worker
  └ internal/auth/adapter/mongo   UserRepo (baja), RefreshRepo, DeletionJobs (outbox), pending de users
  └ internal/<modulo>/adapter/mongo/erase.go   Eraser de cada módulo (solo sus colecciones)
internal/auth/usecase      DeleteAccount (orquestador) + puertos que declara (Art. III.3)
internal/auth/domain       reglas: AnonymizedEmail, ErasePlan, errores
```

Puertos declarados en `internal/auth/usecase/ports.go` (el consumidor los declara; los adaptadores los implementan):

```go
// TxRunner ejecuta fn en una sola transacción; fn puede repetirse (TransientTransactionError).
type TxRunner interface { Do(ctx context.Context, fn func(ctx context.Context) error) error }

// Eraser lo implementa cada módulo sobre SUS colecciones. Se llama dentro de la transacción.
type Eraser interface {
    Name() string
    Erase(ctx context.Context, userID string, at time.Time) (EraseResult, error)
}
// EraseResult trae lo que debe hacerse DESPUÉS del commit: borrados de imágenes y avisos.
type EraseResult struct { Images []ImageRef; Notices []Notice; Counts map[string]int64 }

// Jobs es la bandeja de salida (outbox) de pasos posteriores al commit.
type Jobs interface {
    Enqueue(ctx context.Context, jobs []Job) error          // dentro de la transacción
    Claim(ctx context.Context, now time.Time, limit int) ([]Job, error)
    Done(ctx context.Context, id string) error
    Fail(ctx context.Context, id string, next time.Time, reason string) error
}
// PendingDeletions lee/limpia la marca deletionPending de users.
type PendingDeletions interface {
    ListPending(ctx context.Context, limit int) ([]string, error)
    ClearIfDone(ctx context.Context, userID string, now time.Time) (bool, error)
}
```

- `ImageRef` y `Notice` son tipos propios de `usecase` (identificador, entrega; usuario destino, tipo, título, cuerpo, `data`); no importan `images/domain`. El adaptador de `cmd/api` los convierte.
- **Dueño de cada módulo (Art. XII.1):** cada `erase.go` lo escribe la persona dueña de ese módulo o con su visto bueno; la 021 no edita rutas ni otros archivos del módulo. Registro en `cmd/api/main.go` con **una línea** por eraser.
- **Ningún módulo escribe colecciones ajenas.** `auth` borra `users` y `refresh_tokens` (suyos); las demás colecciones las toca el eraser del módulo dueño:

| Eraser (módulo, dueño original) | Colecciones y acción dentro de la transacción |
|---|---|
| `auth` (003, BE-A) | `refresh_tokens`: borrar; `users`: anonimizar (§4, paso final) |
| `bookings` (007, BE-A) | Cuenta de bloqueantes (409) y cierra pendientes (§4); desprende `itemSnapshot.firstPhoto` (R-02) |
| `items` (005, BE-A) | Borra los objetos de la persona; devuelve sus fotos (`ImageRef`) |
| `chat` (008, BE-B) | `messages.body` = «Mensaje eliminado»; `conversations.lastMessage.preview` si el último es suyo |
| `ratings` (010, BE-B) | `$unset comment` en las suyas (por `raterId`); las estrellas no cambian |
| `incidents`, `reports`, `sanctions` (010/011, BE-B) | `purgeAt` (§7); evidencias como `ImageRef` con fecha |
| `notifications` (011, BE-B) | `notifications` y `push_tokens` de la persona: borrar |
| `ai` (014, BE-A) y `assistant` (014, BE-A) | `ai_usage` y `ai_conversations`: borrar |
| `reco` (015, BE-B) | `reco_interactions` por `userId`; `reco_states` con `_id = "reset:<userId>"` |

## 4. Transacción única (RF-01, RF-02, CA-03 a CA-05)

`DeleteAccount(ctx, userID, password)`:

1. **Fuera de la transacción (barato):** `Me`, verificar contraseña (`ErrWrongPassword` → 403) y una primera comprobación de reservas activas para fallar rápido (409). Todo lo caro (argon2id) ocurre **antes** de abrir la transacción.
2. **`TxRunner.Do` con `ReadConcern` *snapshot* y `WriteConcern` *majority***; dentro, en este orden (el orden está fijo por el orquestador, no por el mapa de módulos):
   1. **Cerrojo de la cuenta:** `findOneAndUpdate({_id, status:"active"}, {$inc:{version:1}})`. Si no coincide, la baja ya ocurrió (RF-05: idempotente, se devuelve éxito sin repetir efectos). Serializa dos bajas simultáneas.
   2. **Comprobación de reservas activas** (aceptadas o en curso, dueña o solicitante) con el `ctx` de la transacción. Si hay, devuelve `ErrActiveBookings` → la transacción se **aborta** sin cambios (CA-03).
   3. `bookings`: pendientes donde es solicitante → `cancelled` (rol solicitante) y donde es dueña → `rejected` (rol dueño), motivo «cuenta eliminada» (la máquina de estados ya permite esas dos transiciones; no hace falta un rol nuevo). Devuelve un `Notice` por contraparte.
   4. `chat`, `ratings`, `items`, `notifications`, `ai`, `assistant`, `reco`, `incidents`, `reports`, `sanctions` (cada eraser) y `refresh_tokens`.
   5. **Outbox:** `Jobs.Enqueue` con los `ImageRef` y `Notice` devueltos (misma transacción).
   6. **`users` al final:** `$set` {`email` anonimizado, `displayName` «Cuenta eliminada», `status:"deactivated"`, `deactivatedAt`, `updatedAt`, `deletionPending:true`} y `$unset` {`passwordHash`, `phone`, `suspendedUntil`, `role`, `ratingSum`, `ratingCount`}, `$inc version`. `acceptedTermsAt` se conserva (D-6).
3. **Por qué CA-04 se cumple:** `Accept` (`bookings/accept_tx.go`) escribe el documento de la reserva y el del objeto. La baja escribe esos mismos documentos (cierra la pendiente y borra el objeto), así que si `Accept` confirma después de la lectura de la baja, la baja choca (`WriteConflict`), el driver repite la función y la comprobación del paso 2 ya ve la reserva aceptada → 409 sin cambios. Se prueba con un entrelazado forzado (T014).
4. **Fallo a media transacción (CA-05):** cualquier error aborta todo; la cuenta queda activa y completa. El `Eraser` no debe tener efectos fuera de MongoDB (los avisos y fotos salen en `EraseResult`).
5. **Repetición del callback:** cada eraser debe ser **idempotente y sin estado local** (el driver puede ejecutar la función varias veces); el orquestador limpia sus acumuladores al inicio de cada intento.
6. **Tamaño y tiempo (RNF-02):** `messages`, `reco_interactions` y `notifications` son lo único con volumen. Se mide con una cuenta de 10 000 mensajes (T014). Si supera el p95 de 2 s o el límite de 60 s de transacción, el eraser de ese módulo pasa a un **barrido por lotes reanudable**: queda marcado en `deletionPending` con el paso pendiente (`steps`) y lo completa el worker. No se implementa de entrada (evitar sobrediseño); es la salida si la medición lo exige.

### Después del commit (RF-03, RF-04, RF-05; N-1)

- **En línea:** el caso de uso intenta, sin bloquear la respuesta 204 en caso de fallo: (a) `Revocations.RevokeUserBefore(userID, deactivatedAt, TTL del acceso)`, (b) borrar claves de caché ligadas a la persona (`cache.Cache.Delete` solo borra por clave exacta; las claves de límite de tasa vencen por TTL y no guardan datos personales; se revisa en T012 si el asistente guarda claves por persona), (c) procesar sus jobs. Un error se registra (sin datos personales) y **no** revierte la baja.
- **`deletionPending`:** es un booleano que existe **solo** mientras haya pasos pendientes; al terminar se hace `$unset` (nunca `null`, RF-10). Sirve para el paso de Redis y como candado lógico de «hay jobs de esta cuenta». Índice **parcial** `{deletionPending:1}` con `partialFilterExpression {deletionPending:true}`.
- **Outbox `deletion_jobs`** (colección nueva; la dueña es `auth`): `{_id, userId, kind: "image"|"notice", payload…, attempts, nextAttemptAt, createdAt, updatedAt}`; al tener éxito el documento **se borra** (no se conserva historial). Reintento con espera exponencial (1 min × 2^n, tope 6 h) y, tras 20 intentos, `failedAt` y un evento `deletion.job_failed` en el registro para operación. Índices: `{nextAttemptAt:1}` parcial sobre `failedAt` ausente y `{userId:1}`. Se eligió **outbox en MongoDB** y no una cola en Redis porque Redis nunca es fuente de verdad (Art. XIV) y porque insertar el job **en la misma transacción** garantiza que no haya fotos huérfanas aunque el proceso caiga justo después del *commit*.
- **Worker `RunDeletionWorker`** (en `internal/auth/jobs`, mismo patrón que `bookings/jobs.RunExpirer`, intervalo 60 s, arranque con una pasada): (1) cuentas con `deletionPending` → repite (a) y (b); (2) reclama jobs vencidos con `findOneAndUpdate` y un arrendamiento (`leaseUntil`), así dos réplicas no repiten; (3) `ClearIfDone` quita `deletionPending` cuando no quedan jobs de esa cuenta. Todo es idempotente (borrar una imagen que ya no existe en Cloudinary, 404, cuenta como hecho).
- **Comando `admin resume-deletions`:** ejecuta una pasada del mismo caso de uso desde la CLI (reanudación manual, N-1).
- **Avisos a contrapartes:** salen como jobs `notice` (`notify.Notifier`), con los tipos ya existentes `booking.rejected` / `booking.cancelled` y su texto, para no crear plantillas nuevas. Un fallo de `Notify` se reintenta como cualquier job.

## 5. Regla «sin null» (RF-10, RF-11; Fase 1)

### 5.1 Escritura

- **`omitempty`** en cada campo opcional de cada documento BSON: `users` (`phone`, `suspendedUntil`, `deactivatedAt`), `refresh_tokens` (`usedAt`, `revokedAt`), `bookings` y subdocumentos (`expiresAt`, `meetingPoint`, `cancelReason`, `cancelledBy`, `rejectReason`, `conversationId`, `delivery`, `return`, `lateAt`, `affectedBy`, `handoverDoc.*`, `qr`), `items` (`usageRules`, `zone.neighborhood`, `itemSnapshot.firstPhoto`), `ratings.comment`, `reports` (`details`, `resolution`), `incidents` (`finalResponse`, `closedAt`), `sanctions` (`endsAt`, `incidentId`), `chat` (`lastMessage`), `notifications.readAt`, `reco_states.builtAt`. El **inventario exacto** lo da la verificación (T002), no esta lista.
- **Opciones globales del cliente** (`internal/platform/mongodb.Connect`): `NilSliceAsEmpty` y `NilMapAsEmpty` (nombres a confirmar en el paquete `options` de la versión fijada en `go.mod` al implementar). Red de seguridad: ningún *slice* o *map* `nil` produce `null`.
- **Actualizaciones:** quitar un valor es `$unset`, nunca `$set: null` (p. ej. `ClearPhone` en `UpdateProfile`, hoy `phone: nil`; el reemplazo de la baja).
- **Lectura:** Go trata un campo ausente igual que `nil`/cero, así que los lectores no cambian. Se prueba con documentos sin el campo (riesgo de la spec §10).

### 5.2 Validadores

Se generaliza el aplicador de `$jsonSchema` en un helper único `adaptermongo.EnsureValidator(ctx, db, collection, schema, level, action)` (`internal/adapter/mongo`), que usa `collMod` (idempotente y tolerante a la falta de rol `dbAdmin`, como `EnsureSchemas`). `auth` y `items` migran a él; cada módulo con campos opcionales declara su `Schema()` mínimo.

- **Tolerante primero:** durante el despliegue 1 los esquemas siguen aceptando `null` donde hoy lo aceptan y se agrega el validador de las demás colecciones en `validationAction: "warn"` (solo registra). Se usa una bandera de código (constante) y no una variable de entorno nueva.
- **Estricto después** (despliegue 2, tras migrar y verificar en cero): opcionales con su tipo real y **sin** `"null"`, `validationLevel: "moderate"`, `validationAction: "error"` (código 121 ante `null`, CA-14).
- **`users` condicional:** `anyOf` de dos ramas por `status` (MongoDB no tiene `if/then`): `active` exige el conjunto completo; `deactivated` exige solo `_id, email, displayName, status, deactivatedAt, acceptedTermsAt, createdAt, updatedAt, version`, y permite `deletionPending` booleano.
- **Cuidado:** con `moderate`, los documentos existentes inválidos (con `null`) se pueden seguir actualizando; por eso la migración es obligatoria antes del estricto, no un efecto del validador.

### 5.3 Migración y verificación (`cmd/admin`)

Los subcomandos se registran en `run` de `cmd/admin/main.go` como los existentes (fábrica inyectable para pruebas, como `indexes` y `seed`), y se ejecutan en la VM como el resto de `admin` (ver `docs/respaldos.md` y `docs/despliegue.md`).

| Comando | Comportamiento |
|---|---|
| `admin verify-no-nulls` | **Solo lectura** (rol `read`). Recorre todas las colecciones (salvo `system.*`) con cursor por lotes; recorre cada documento con `bson.Raw` en **cualquier nivel** (subdocumentos y arreglos) y cuenta los valores BSON `null`. Imprime por colección y por **ruta normalizada** (índices de arreglo como `[]`) cuántos hay, **sin imprimir valores ni ids**. Salida 0 si no hay, 1 si hay, 2 ante error (misma convención que `indexes`) |
| `admin migrate-remove-nulls` | **Por omisión es `--dry-run`**: descubre rutas con el mismo recorrido y muestra cuántos documentos cambiaría por colección. Con `--apply --backup-ref <texto>` ejecuta; sin `--backup-ref` se niega (el respaldo previo es obligatorio, `docs/respaldos.md`, RF-12). Idempotente: filtra con `{ruta: {$type: "null"}}` (no `{ruta: null}`, que también coincide con ausente), quita con `$unset`; en arreglos de subdocumentos usa `arrayFilters`; un `null` suelto dentro de un arreglo de escalares usa `$pull`. Procesa por lotes de 1 000 `_id` (RNF-05, reanudable). Registra el `backup-ref` y los conteos por colección. Una ruta más profunda que un nivel de arreglo la **reporta y no la toca** (se añade regla). Se niega con `APP_ENV=production` si no recibe además `--yes-production` |
| `admin resume-deletions` | Una pasada del worker de bajas (§4) |

## 6. Efectos laterales sobre otros puertos

- `users.Admin.AddRating` y `Suspend` (`UserRepo`): filtran `status = "active"`. Sobre una cuenta eliminada no hacen nada y no son error (la estrella se guarda en `ratings`, pero no se acumula en un perfil que ya no existe; los promedios ajenos no cambian, CA-08). Se prueba.
- `users.Directory.PublicUsers` y `domain.User.Public`: una cuenta `deactivated` devuelve «Cuenta eliminada», `RatingAverage` nulo y `RatingCount` 0 aunque los campos no existan. **N-2:** no se toca `api/`; la respuesta JSON conserva `ratingAverage: null` y `ratingCount: 0` porque el contrato los exige.
- Búsqueda (`search`, pipelines con `$_owner.ratingSum`): los objetos de la persona ya no existen; se verifica que el *lookup* con campos ausentes no falle.
- `chat` (`AccountReader.Active`): ya cierra la conversación si la contraparte no está activa (`ErrClosed`, 409 «La conversación está cerrada.», `api/paths/chat.yaml`), lo que cumple RF-08; solo se agrega la prueba. **N-4:** `Message` no necesita campo nuevo.
- Reservas con objeto borrado: `Reject`/`Cancel` leen el título desde `itemSnapshot`, no desde `items`.
- `cmd/admin smoke` (hace una baja): se ajusta a lo nuevo.

## 7. Retención de 12 meses (RF-04 de D-4; CA-11)

- Campo `purgeAt` (fecha; **ausente** si ninguna cuenta implicada se dio de baja) en `incidents`, `reports` y `sanctions`. El eraser lo fija con `$min` (la fecha más temprana si se dan de baja las dos partes) a `at + 12 meses`, calculada con el reloj inyectable (`Now()`, RNF-06; se suman 12 meses de calendario, no 365 días).
- Qué «involucra» a la persona: `incidents.reporterId` o `reportedId`; `sanctions.userId`; `reports.reporterId` o `targetId` con `targetType = "user"`.
- **Índice TTL** `{purgeAt:1}` con `expireAfterSeconds: 0` en las tres colecciones (los documentos sin el campo no se ven afectados). Entran al catálogo (`internal/adapter/mongo/catalog`) y a `cmd/admin/testdata/indexes_baseline.json`. El TTL de MongoDB corre cada ~60 s, así que las pruebas con reloj falso verifican la **fecha** (`purgeAt`) y una prueba de integración lo comprueba con un `purgeAt` en el pasado.
- **Evidencias en Cloudinary:** al fijar `purgeAt`, las imágenes de `incidents.evidence` se encolan como jobs `image` con `nextAttemptAt = purgeAt`, así el borrado de fotos coincide con el del documento.
- **N-3:** no hay redacción anticipada; el texto no se toca antes de `purgeAt`.
- **Índices nuevos** que exige la baja (`catalog` y *baseline*): `ratings {raterId:1}`; `reports {targetType:1, targetId:1}`; `deletion_jobs` (dos) y `users {deletionPending:1}` parcial; más los TTL de arriba. Conteo final y nombres se fijan en T005/T016 contra el baseline actual.

## 8. Riesgos

| ID | Riesgo | Mitigación |
|---|---|---|
| R-01 | Transacción lenta con cuentas muy grandes | Medición en T014 y salida por lotes reanudables (§4.6) |
| R-02 | `itemSnapshot.firstPhoto` apunta a una foto ya borrada: imagen rota en el historial ajeno | **Decisión propuesta, requiere visto bueno del responsable y de BE-A (dueño de 007):** el eraser de `bookings` hace `$unset itemSnapshot.firstPhoto` de las reservas del objeto borrado; la app ya trata la foto como opcional (el campo es `*imageDoc`). Alternativa: conservar el *snapshot* de la foto en Cloudinary, lo que contradice D-2 |
| R-03 | Reportes de moderación sobre un objeto borrado: `Snapshot` devolverá `not_found` | La moderación trata `not_found` como «objeto eliminado» y cierra el reporte; prueba en T020. Los textos del reporte se conservan 12 meses (N-3) |
| R-04 | Código viejo y nuevo conviven un instante al desplegar y el viejo escribe `null` | Una sola réplica (Art. XIII); validadores tolerantes en el primer despliegue; la migración se ejecuta **después** de que corre el código nuevo y la verificación debe dar 0 (§9) |
| R-05 | `deletionPending` visible en `users` rompe CA-13 si se comprueba antes de terminar | CA-13 se verifica tras la pasada del worker; se documenta en la prueba (el campo es transitorio y no es dato personal) |
| R-06 | Retirar `role` de la cuenta eliminada rompe un lector (p. ej. una comprobación de administrador) | Un token de acceso emitido antes de la baja deja de valer por la revocación; T012 revisa los lectores de `role` y la prueba de lectura de cuentas eliminadas (RF-07) corre antes de quitar el campo |
| R-07 | Cambiar el puerto compartido `images` afecta a todos los módulos | Puerto aparte `Destroyer`; `Service` no cambia (Art. XII.4) |
| R-08 | La migración quita un `null` que significa algo | La verificación imprime rutas; antes de aplicar, T003 revisa cada ruta contra su lector |
| R-09 | Respaldos conservan datos de cuentas ya eliminadas | Fuera de alcance (spec §8); se documenta en POL-17 y en `docs/respaldos.md` |
| R-10 | Si dos cuentas implicadas en una incidencia se borran en fechas distintas, el documento se purga 12 meses tras la **primera** baja | Decisión de diseño (privacidad primero); se documenta en POL-17. Cámbiese a la última baja si el responsable lo pide |
| R-11 | Tareas de BE-A (auth, items, bookings, ai, assistant) y de BE-B (chat, ratings, moderación, notificaciones, reco, migración) se cruzan | Cada eraser lo implementa el dueño de su módulo o con su visto bueno (Art. XII.1); la 021 no edita archivos ajenos fuera de `erase.go` y la línea de registro |

## 9. Orden de despliegue (sin corte)

1. **Despliegue A** (código «sin null» + erasers + worker, **validadores tolerantes**, índices nuevos): staging primero. Escrituras nuevas ya no traen `null`.
2. **Respaldo** (`docs/respaldos.md`) → `admin migrate-remove-nulls` (dry-run, revisar conteos) → `--apply --backup-ref …` → `admin verify-no-nulls` debe salir 0. Primero en una **copia** y luego en staging; en producción solo tras la orden del responsable.
3. **Despliegue B** (validadores **estrictos** sin `null`). Si la verificación no dio 0, no se despliega B.
4. Producción repite A → respaldo → migración → verificación → B, **solo cuando el responsable lo indique**. Este plan no propone `develop → main`.

## 10. Estrategia de pruebas (Art. VII; RNF-07)

- **Unitarias con *fakes*** (`usecase` ≥ 80 %): orden de pasos, 403/409, idempotencia, error de un eraser que aborta, reintentos del worker, cálculo de `purgeAt` con reloj falso, normalización de `null`.
- **Integración con Testcontainers** (MongoDB **replica set**, Redis y Cloudinary falso): transacción completa (CA-01, CA-03 a CA-05, CA-12, CA-16), entrelazado con `Accept`, escaneo de `null` sobre todas las colecciones tras una baja y tras la siembra (CA-02, CA-14), migración sobre una copia con `null` (CA-15), TTL con `purgeAt` pasado, índices contra el baseline (`admin indexes`).
- **Mutación y contrato:** la 016 ya valida `api/openapi.yaml`; esta spec no lo cambia. Se agrega una prueba de que `GET /users/{id}` de una cuenta eliminada responde 200 con «Cuenta eliminada», `ratingAverage: null`, `ratingCount: 0`.
- **Seguridad de registros:** una prueba captura `slog` durante una baja y comprueba que no aparecen correo, teléfono, contraseña ni el texto borrado (RNF-03).

## 11. Verificación de constitución

| Artículo | Cómo se cumple |
|---|---|
| I (SDD) | Spec aprobada → plan → tareas; contrato verificado en T001 |
| II (MongoDB, replica set, transacciones) | Una transacción para toda la baja (la spec la exige); outbox en MongoDB; sin otra base |
| III (Clean Architecture) | Puertos en `usecase`; solo los adaptadores importan `mongo`/`bson`; el orquestador no importa adaptadores |
| IV (contrato) | Sin cambios en `api/` (T001 lo deja por escrito); 204/403/409 igual |
| V (dinero y fechas) | No toca importes; los 12 meses se calculan en UTC con el reloj inyectable |
| VI/VII (privacidad y pruebas) | Sin datos personales en registros; cobertura ≥ 80 %; integración real |
| XII (paralelo) | Un eraser por módulo, registro de una línea; dueños y visto bueno explícitos |
| XIII (despliegue) | Última tarea «Despliegue» con A → migración → B |
| XIV (sin estado) | Estado de reanudación en MongoDB; Redis solo revoca y se rehace |

## 12. Verificación del contrato (T001, Art. IV.6) — 2026-11-26

Revisado `api/openapi.yaml` y `api/paths/{auth,chat,users}.yaml` contra esta spec. Resultado: **no se cambia `api/paths/*` ni `api/openapi.yaml`**.

- `DELETE /api/v1/me` (`api/paths/auth.yaml`) sigue en **204 / 403 / 409** (RF-06).
- `PublicUser.ratingAverage` es obligatorio y admite `null` (`type: [number, 'null']`) y `ratingCount` es entero ≥ 0: se conservan; una cuenta eliminada responde `ratingAverage: null` y `ratingCount: 0` (N-2). El `null` es solo de la respuesta JSON; MongoDB no guarda esos campos.
- `Message` no necesita campo nuevo: «Mensaje eliminado» viaja como texto normal en `body` (N-4, RF-09).
- El 409 «La conversación está cerrada.» ya existe en `POST /conversations/{id}/messages` (`api/paths/chat.yaml`, RF-08).
- La Fase 1 («sin null») solo cambia cómo se **guarda** en MongoDB (omitir campos opcionales); las respuestas JSON no cambian, porque los DTO HTTP son independientes de los documentos BSON y los campos `null` del contrato (`clientId`, `lastMessage`, `ratingAverage`…) los produce la capa HTTP.

Si alguien propone un cambio de contrato, se detiene y se publica aquí **antes** del código.

## 13. Resumen para el dueño de la 003 (T029, RF-14) — 2026-11-26

Esta spec **no edita** la spec 003 ni `constitution.md`. Lo que cambia la baja en `auth`, para que su dueño (BE-A) lo revise:

- `DELETE /api/v1/me` conserva 204 / 403 / 409. La contraseña se verifica **antes** de la transacción; dentro, cerrojo de la cuenta, segunda comprobación de reservas activas, `Eraser` de cada módulo en un orden fijo, borrado de sesiones, bandeja de salida (`deletion_jobs`) y `users` al final. Lo que no es MongoDB (Redis y Cloudinary) se hace después del *commit*, con reintento (worker y `admin resume-deletions`).
- `users` queda solo con `_id`, `status`, `deactivatedAt`, `email` anonimizado, `displayName` «Cuenta eliminada», `acceptedTermsAt`, `createdAt`, `updatedAt` y `version` (más `deletionPending` mientras haya pasos pendientes). Se quitan `passwordHash`, `phone`, `suspendedUntil`, `role`, `ratingSum` y `ratingCount`.
- **Corrección propuesta para P-08** (`specs/003-cuenta-autenticacion/plan.md`, la edita su dueño): donde dice «el correo se conserva (impide reusarlo)», debe decir «la baja **anonimiza** el correo (`deleted+<id>@vaiven.invalid`), borra la contraseña, el teléfono y las sesiones, y conserva solo lo de §5.2 de la spec 021; el correo original puede registrarse otra vez». Es lo que ya dicen A-04 y RF-08 y lo que hace el código; P-08 es el texto que quedó desactualizado.
- Un puerto nuevo compartido, `internal/usecase/erasure` (`Eraser`, `EraseResult`, `Notice`, `ImageRef`), evita que un módulo importe el `usecase` de otro (Art. III y XII).
- Pendiente de visto bueno de BE-A: R-02 (el eraser de `bookings` quita `itemSnapshot.firstPhoto` de las reservas de objetos borrados) y los erasers de `items`, `ai` y `assistant`.
