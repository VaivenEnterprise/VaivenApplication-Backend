# Spec 021 — Baja de cuenta limpia y datos sin `null` (backend)

- **Dueño sugerido:** BE-A (auth, spec 003) · **Con:** BE-B (datos y migración). Es una sugerencia; el reparto lo confirma el responsable en `ROADMAP.md`.
- **Depende de:** 002, 003, 005, 007, 008, 009, 010, 011, 014, 015 (todas desplegadas) · **Prioridad:** P1 (privacidad)
- **Constitución:** v2.0.0 · **Estado:** **Aprobada** — aprobada por el responsable (BE-B) el 2026-11-26; visto bueno de BE-A pendiente para la parte de auth (spec 003) · **Fecha:** 2026-11-26
- **Decisiones del responsable (2026-11-26):** recogidas en §4 (D-1 a D-6). Las preguntas N-1 a N-4 quedaron resueltas el 2026-11-26 (§11).

## 1. Objetivo y por qué

Hoy `DELETE /api/v1/me` solo **anonimiza** la cuenta, y de forma incompleta. Quedan datos de la persona en varias colecciones (tokens de push, avisos, objetos con fotos, texto de mensajes y comentarios, uso de IA), y la baja se hace con cuatro escrituras sueltas sin transacción. Además, la base guarda valores `null` en campos opcionales, lo que complica la validación y las consultas.

Esta spec define **qué** debe quedar de una persona tras darse de baja, **cómo se demuestra** que no queda nada identificable más allá de lo conservado a propósito (POL-17), y una regla global: **ningún campo `null` en MongoDB**. Un campo opcional sin valor **no existe** en el documento.

## 2. Estado actual (verificado en el código)

| Tema | Hoy | Dónde |
|---|---|---|
| Baja | Borrado lógico en cuatro escrituras sin transacción: contraseña, conteo de reservas activas, `users`, refresh tokens y revocación de acceso en Redis. La comprobación de reservas activas y la actualización **no son atómicas** | `internal/auth/usecase/deactivate.go` |
| Documento `users` tras la baja | `email = deleted+<id>@vaiven.invalid`, `displayName = "Cuenta eliminada"`, `phone` en `null` explícito, `status = deactivated`, `deactivatedAt`. **Siguen** `passwordHash`, `ratingSum`, `ratingCount`, `role`, `acceptedTermsAt`, `suspendedUntil` | `internal/auth/adapter/mongo/user_repo.go` (`Deactivate`) |
| `null` en `users` | Todo documento nuevo guarda `phone`, `suspendedUntil` y `deactivatedAt` como `null` (etiquetas `bson` sin `omitempty`) y el `$jsonSchema` acepta `null` en ellos y en `refresh_tokens.usedAt/revokedAt` | `user_repo.go`, `internal/auth/adapter/mongo/schema.go` |
| `null` en otras colecciones | Punteros opcionales guardados como `null`: reservas (`meetingPoint`, `cancelReason`, `cancelledBy`, `conversationId`, `delivery`, `return`, `lateAt`…), valoraciones (`comment`), denuncias (`details`), incidencias y sanciones | adaptadores `internal/*/adapter/mongo` |
| Reservas activas | Cuenta reservas aceptadas o en curso como dueña o solicitante. Las solicitudes **pendientes** no se tratan | `internal/bookings/adapter/mongo/activity_checker.go` |
| `push_tokens` | **No** se borran en la baja, contra POL-17 | `internal/notifications/adapter/mongo/token_repo.go`, `docs/politicas.md` |
| Sesiones | Los refresh tokens solo se **revocan** (quedan hasta su TTL) | `RevokeAllForUser` |
| Se conservan sin tocar | `notifications`, `items` (y fotos en Cloudinary), texto de `messages`, comentarios de `ratings`, `ai_usage`, `ai_conversations`, `reco_states`. `reco_interactions` solo se purga después, por el recálculo | `internal/reco/adapter/mongo/items_source.go` (`Deactivated`) |
| Contradicción en la 003 | `plan.md` P-08 dice que la baja **conserva** el correo; A-04 y RF-08 dicen que lo **anonimiza**. Manda el código (anonimiza). Esta spec lo deja anotado; **no** edita la 003 | `specs/003-cuenta-autenticacion/plan.md` |

## 3. Historias

| # | Prio. | Historia |
|---|---|---|
| H-01 | P1 | Como persona usuaria, quiero que al darme de baja se borren mis datos personales y mi contenido, para que nada me identifique. |
| H-02 | P1 | Como contraparte (quien rentó o prestó con esa persona), quiero seguir viendo mi historial y mis chats sin errores, aunque la otra cuenta ya no exista. |
| H-03 | P1 | Como persona usuaria, quiero que la baja sea todo o nada: si algo falla a la mitad no quedo con la cuenta a medias. |
| H-04 | P1 | Como equipo, queremos que la base no tenga campos `null` ni datos huérfanos, para validar y consultar con reglas simples. |
| H-05 | P2 | Como equipo, queremos conservar 12 meses lo necesario para atender disputas y luego borrarlo solo. |

## 4. Decisiones del responsable (2026-11-26)

| ID | Decisión |
|---|---|
| D-1 | **Mensajes de chat** de la cuenta eliminada: el texto se reemplaza por «Mensaje eliminado». La conversación conserva orden y fechas; la contraparte la sigue viendo; no queda texto que identifique. |
| D-2 | **Objetos publicados**: se borran por completo junto con sus fotos en Cloudinary. Las reservas pasadas conservan su `itemSnapshot`, así que el historial ajeno no se rompe. El borrado de fotos puede ser asíncrono y reintentable; un fallo ahí **no** deja la cuenta a medias en MongoDB. |
| D-3 | **Valoraciones**: las estrellas se conservan (los promedios ajenos no cambian). El comentario escrito por la cuenta eliminada se **quita** (el campo desaparece, no queda `null`). |
| D-4 | **Incidencias, denuncias y sanciones** que involucran a la persona: se conservan **12 meses** tras la baja y luego se borran solas. |
| D-5 | **Regla global:** nada de datos sin normalizar ni `null` en MongoDB. Los campos opcionales se **omiten** (`$unset` / `omitempty`); los validadores `$jsonSchema` dejan de aceptar `null`; una migración única e idempotente, con respaldo previo, quita los `null` existentes; una verificación demuestra cero `null`. |
| D-6 | D-6 (2026-11-26, responsable): se conserva `acceptedTermsAt` como prueba de consentimiento. |

## 5. Requisitos funcionales

### 5.1 Baja atómica

| ID | Requisito |
|---|---|
| RF-01 | `DELETE /api/v1/me` valida la contraseña y rechaza con 409 si hay reservas activas (aceptadas o en curso, como dueña o solicitante). **La comprobación se repite dentro de la transacción** de RF-02. Si rechaza, **no cambia nada**. |
| RF-02 | Todas las escrituras de MongoDB de la baja (cuenta y las acciones de §5.3) ocurren en **una sola transacción** (el *replica set* ya existe). O se aplican todas o ninguna. |
| RF-03 | Lo que no es MongoDB (revocación de acceso en Redis y borrado de fotos en Cloudinary) ocurre **después** del *commit*, es idempotente y se reintenta. Un fallo ahí no revierte la baja ni deja la cuenta activa. |
| RF-04 | Si el proceso cae entre el *commit* y los pasos posteriores, la baja se **reanuda**: la cuenta queda con una marca de pendientes (p. ej. `deletionPending`) que un trabajo periódico completa. Mientras tanto la cuenta ya no inicia sesión. **Resuelto (2026-11-26, responsable):** N-1, ver §11. El campo `deletionPending` no existe cuando no hay pendientes (nunca `null`). |
| RF-05 | Repetir la baja sobre una cuenta ya eliminada es idempotente: no falla de forma distinta ni duplica efectos (la cuenta ya no inicia sesión, así que el caso práctico es un reintento con el mismo token de acceso, que Redis ya invalida). |
| RF-06 | La respuesta de `DELETE /me` no cambia: **204**, **403** contraseña incorrecta, **409** reservas activas. |

### 5.2 Cuenta (`users`) tras la baja

Se conservan solo los campos necesarios. Ningún campo queda en `null`: lo que se quita **no existe**.

| Campo | Acción | Motivo |
|---|---|---|
| `_id` | Conservar | Identidad seudónima para reservas, valoraciones, incidencias |
| `status = deactivated`, `deactivatedAt` | Conservar | Estado y base del plazo de 12 meses |
| `email` | Reemplazar por `deleted+<id>@vaiven.invalid` | Unicidad del índice y que el correo original pueda registrarse otra vez |
| `displayName` | Reemplazar por «Cuenta eliminada» | Se muestra en lecturas ajenas |
| `createdAt`, `updatedAt`, `version` | Conservar | Metadatos sin datos personales |

Campos que quedan en `users` tras la baja: `_id`, `status`, `deactivatedAt`, `email` anonimizado, `displayName` «Cuenta eliminada», `acceptedTermsAt`, `createdAt`, `updatedAt` y `version`.
| `passwordHash` | **Quitar** | Dato de autenticación sin uso |
| `phone` | **Quitar** | Dato personal |
| `acceptedTermsAt` | **Conservar** | Prueba de consentimiento; no identifica a la persona (D-6) |
| `suspendedUntil` | **Quitar** | Sin uso (la cuenta ya está inactiva). La sanción vive en `sanctions` (D-4) |
| `role` | **Quitar** | Una cuenta eliminada no autoriza nada |
| `ratingSum`, `ratingCount` | **Quitar** | Son el promedio propio de una cuenta que ya no se muestra. `GET /users/{id}` de una cuenta eliminada devuelve «Cuenta eliminada» sin promedio. Las estrellas que recibió siguen en `ratings` (D-3) |

El `$jsonSchema` de `users` exige los campos opcionales solo cuando `status` es `active` o `suspended`; para `deactivated` basta el conjunto mínimo de arriba. **Resuelto (2026-11-26, responsable):** N-2, ver §11. Una cuenta eliminada no muestra promedio. En MongoDB `ratingSum` y `ratingCount` no existen; el contrato no cambia (en `PublicUser` `ratingAverage` es obligatorio pero admite `null` en JSON, y `ratingCount` vale 0).

### 5.3 Qué pasa en cada colección

| Colección / sistema | Acción | Regla |
|---|---|---|
| `refresh_tokens` | **Borrar** los de la persona | Hoy solo se revocan. Además, revocar acceso en Redis (RF-03) |
| `push_tokens` | **Borrar** los de la persona | Cumple POL-17 |
| `notifications` | **Borrar** las dirigidas a la persona | Texto con contenido personal; el resto lo cubre el TTL de 90 días |
| `items` | **Borrar** los publicados por la persona y **sus fotos en Cloudinary** (D-2) | Las reservas pasadas conservan `itemSnapshot` |
| `bookings` | **Conservar** con ids seudónimos (POL-17). Las **solicitudes pendientes** donde la persona es solicitante o dueña pasan a cancelada o rechazada con motivo «cuenta eliminada», y se avisa a la contraparte | Cierra el hueco de las pendientes |
| `conversations` / `messages` | Reemplazar el texto de sus mensajes por «Mensaje eliminado» (D-1). El chat con una cuenta eliminada queda **de solo lectura** | La contraparte conserva el historial |
| `ratings` | Conservar estrellas; **quitar el campo `comment`** de las escritas por la persona (D-3) | Promedios ajenos intactos |
| `incidents`, `reports`, `sanctions` | **Conservar 12 meses** tras la baja y luego borrar automáticamente (D-4); p. ej. con un campo de vencimiento (`purgeAt`) y un índice TTL | Disputas y seguridad. Texto libre escrito por la persona: **Resuelto (2026-11-26, responsable):** N-3, se conserva sin cambios y se purga entero a los 12 meses. |
| `ai_usage`, `ai_conversations` | **Borrar** los de la persona | Sin base para conservarlos |
| `reco_interactions`, `reco_states` | **Borrar** los de la persona en la misma baja | Hoy `reco_interactions` espera al recálculo y `reco_states` no se toca |
| Redis | Revocar acceso del usuario; borrar claves de caché y límite ligadas a la persona | Redis nunca es fuente de verdad (Art. XIV) |

### 5.4 Lecturas que siguen funcionando

| ID | Requisito |
|---|---|
| RF-07 | Ninguna lectura devuelve 404 ni 500 porque una reserva, un chat, una valoración o un historial referencian una cuenta eliminada: se muestra «Cuenta eliminada». |
| RF-08 | En el chat con una cuenta eliminada, enviar un mensaje responde con el error de conflicto ya definido en el contrato (no se crea un código nuevo salvo necesidad) y la lectura sigue disponible. |
| RF-09 | **Contrato (Art. IV):** se prefiere **no cambiar** `openapi.yaml`: «Mensaje eliminado» viaja como texto normal. Si la app móvil necesita distinguirlo (p. ej. un indicador `deleted: true` en el mensaje), se publica como **primera tarea** de la spec, antes del código. **Resuelto (2026-11-26, responsable):** N-4, sin cambio de contrato. |

### 5.5 Regla global: ningún `null` en MongoDB

| ID | Requisito |
|---|---|
| RF-10 | Los campos opcionales **no se escriben** cuando no tienen valor (`omitempty` en la escritura y `$unset` en las actualizaciones). Aplica a todas las colecciones. |
| RF-11 | Todos los `$jsonSchema` dejan de aceptar `null`: los campos opcionales se definen como no requeridos con su tipo real. Una escritura con `null` es rechazada. |
| RF-12 | Una **migración única e idempotente** (comando de administración) quita todos los campos `null` existentes en todas las colecciones. Exige **respaldo previo** y registra cuántos documentos cambió por colección. Repetirla no cambia nada. Se prueba en una copia antes de staging. |
| RF-13 | Una **verificación** (prueba de integración y comando de administración de solo lectura) recorre todas las colecciones y falla si encuentra un campo `null` en cualquier nivel del documento. |
| RF-14 | Se corrige la contradicción P-08 / A-04 de la spec 003. La spec 003 no se edita aquí: esta spec la anota y deja la baja descrita en §5.2 como fuente de verdad; la corrección de `plan.md` la propone el dueño de la 003 por escrito. |
| RF-15 | `docs/politicas.md` (POL-17) y `memoria.md` se actualizan con el nuevo alcance de la baja y los 12 meses de conservación. |

## 6. Requisitos no funcionales

| ID | Requisito |
|---|---|
| RNF-01 | **Sin estado (Art. XIV):** ningún dato de la baja vive solo en memoria; los reintentos y la reanudación se apoyan en MongoDB. |
| RNF-02 | Latencia de la transacción de baja: p95 < 2 s para una cuenta típica (objetivo orientativo; no bloquea la aceptación). Para cuentas con muchos objetos o mensajes, el trabajo pesado se divide por lotes sin romper la atomicidad lógica (la cuenta queda marcada y se completa). |
| RNF-03 | **Privacidad (Art. VII):** ningún registro, métrica ni evento contiene correo, teléfono, contraseña ni el contenido borrado. |
| RNF-04 | **Idempotencia:** la baja, los reintentos de Redis y Cloudinary, y la migración pueden repetirse sin efectos adicionales. |
| RNF-05 | La migración no bloquea la API: corre por lotes y puede reanudarse. |
| RNF-06 | La retención de 12 meses usa el reloj inyectable (`Now()`), para probarse sin esperar. |
| RNF-07 | Cobertura de `usecase` y `domain` tocados ≥ 80 % (Art. III) y pruebas de integración con MongoDB real (replica set) para la transacción. |

## 7. Criterios de aceptación

| ID | Criterio |
|---|---|
| CA-01 | Dada una cuenta con objetos, mensajes, valoraciones y tokens, cuando se da de baja, entonces **ninguna colección** contiene un documento con su correo original, teléfono ni hash de contraseña. |
| CA-02 | Tras la baja, una consulta sobre **todas** las colecciones no encuentra ningún campo `null` (RF-13). |
| CA-03 | Dada una cuenta con una reserva aceptada, cuando pide la baja, entonces recibe 409 y **ningún documento cambia** (cuenta, objetos, mensajes, tokens). |
| CA-04 | Dada una reserva que pasa a *aceptada* entre la primera comprobación y la transacción, entonces la baja se rechaza con 409 sin cambios (comprobación dentro de la transacción). |
| CA-05 | Dada una falla simulada a media transacción, entonces la cuenta sigue activa y completa, sin cambios parciales. |
| CA-06 | Dado un fallo de Cloudinary o de Redis tras el *commit*, entonces la baja sigue válida, los pasos se reintentan y terminan; las fotos desaparecen de Cloudinary o quedan encoladas con reintento. |
| CA-07 | Dada una conversación con la cuenta eliminada, la contraparte ve el historial en orden y con las mismas fechas, y los mensajes de la cuenta eliminada dicen «Mensaje eliminado». Enviar un mensaje nuevo es rechazado. |
| CA-08 | Dadas las estrellas que la cuenta eliminada dio a otras personas, los promedios de esas personas **no cambian**, y los comentarios de la cuenta eliminada ya no existen (el campo está ausente). |
| CA-09 | Los objetos de la cuenta eliminada no existen en MongoDB ni en Cloudinary; las reservas pasadas siguen mostrando su `itemSnapshot` sin error. |
| CA-10 | Las solicitudes pendientes de la cuenta quedan canceladas o rechazadas con motivo «cuenta eliminada» y la contraparte recibe un aviso. |
| CA-11 | Con un reloj falso, incidencias, denuncias y sanciones de la cuenta existen a los 11 meses y desaparecen pasados los 12. |
| CA-12 | Dada la baja repetida o reanudada, el resultado final es idéntico y sin errores. |
| CA-13 | Tras la baja, `users` contiene solo `_id`, `status`, `deactivatedAt`, `email` anonimizado, `displayName` «Cuenta eliminada», `acceptedTermsAt`, `createdAt`, `updatedAt` y `version` (§5.2); `GET /users/{id}` devuelve «Cuenta eliminada» con 200. |
| CA-14 | Una escritura con `null` en un campo opcional de cualquier colección es rechazada por el validador. |
| CA-15 | La migración sobre una copia con `null` deja cero `null`; ejecutarla de nuevo cambia 0 documentos; el respaldo previo existe. |
| CA-16 | El correo original puede registrarse de nuevo tras la baja. |
| CA-17 | `make fmt lint test` pasa; ninguna ruta del contrato cambia salvo lo que decida N-4. |

## 8. Fuera de alcance

- Recuperar o «deshacer» una baja (la baja es definitiva).
- Exportar los datos de la persona antes de borrarlos (derecho de portabilidad): spec aparte.
- Borrar el historial de reservas ajeno o el `itemSnapshot`.
- Cambiar la política de reservas activas (POL-17 sigue rechazando con 409).
- Respaldos y retención de copias de seguridad fuera de MongoDB (ver spec 013).
- Cualquier cambio en la app móvil más allá del posible indicador de N-4.
- Editar la spec 003 o `constitution.md`.

## 9. Dependencias

- Transacciones de MongoDB (replica set, ya disponible desde la 002) y reloj inyectable.
- Puertos existentes: `images.Service` (borrado en Cloudinary), `notify.Notifier` (aviso a contrapartes), `Revocations` (Redis).
- Cada módulo (items, bookings, chat, ratings, moderación, notificaciones, IA, reco) aporta su operación de borrado o anonimización por un puerto, sin que `auth` importe sus adaptadores (Art. II).
- Despliegue en staging con la tarea «Despliegue» (Art. XIII) y respaldo previo a la migración.

## 10. Riesgos

| Riesgo | Mitigación |
|---|---|
| La transacción abarca muchas colecciones y se vuelve lenta | Medir (RNF-02); marcar la cuenta y completar por lotes si hace falta |
| La migración quita un `null` que el código lee como «sin valor» | Revisar lectores de cada campo y probar con documentos sin el campo (Go lo trata igual que `nil`) |
| Quitar `role` o `ratingSum` rompe un lector de `users` | Probar lecturas de cuentas eliminadas (RF-07) antes de borrar campos |
| Conservar `acceptedTermsAt` tras la baja | No identifica a la persona (D-6); queda junto a `_id` seudónimo y correo anonimizado |
| Fotos huérfanas si falla Cloudinary | Reintento con tarea pendiente y reconciliación periódica |
| Conservar texto libre en incidencias 12 meses | Decidido en N-3: íntegro 12 meses y purga total; solo accede la moderación |
| La migración sin respaldo es irreversible | Respaldo obligatorio (RF-12); probar en copia |

## 11. Preguntas resueltas

| ID | Decisión |
|---|---|
| N-1 | **Resuelto (2026-11-26, responsable).** Una baja interrumpida se reanuda con la marca `deletionPending` en la cuenta. El campo **no existe** cuando no hay pendientes (nunca `null`). Los pasos posteriores al *commit* se procesan de forma idempotente: en línea tras el *commit*, por un trabajo periódico y por un comando de administración. Diseño en `plan.md` |
| N-2 | **Resuelto (2026-11-26, responsable).** El perfil público de una cuenta eliminada no muestra promedio. En el contrato actual `ratingAverage` es obligatorio pero admite `null` (`PublicUser`), así que se **mantiene el contrato**: la respuesta lleva `ratingAverage: null` y `ratingCount: 0`; el `null` es solo de la respuesta JSON y MongoDB no guarda esos campos |
| N-3 | **Resuelto (2026-11-26, responsable).** Sin redacción anticipada: los textos de incidencias, denuncias y sanciones se conservan sin cambios y se **purgan por completo** a los 12 meses de la baja |
| N-4 | **Resuelto (2026-11-26, responsable).** Sin cambio de contrato: el servidor guarda el texto «Mensaje eliminado» y viaja como texto normal. `Message` no necesita campo nuevo |
