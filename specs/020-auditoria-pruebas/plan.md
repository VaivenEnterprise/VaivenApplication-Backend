# Plan 020 — Bitácora de auditoría inmutable y pruebas de seguridad avanzadas (backend)

- **Spec:** `spec.md` · **Estado:** aprobada el 2026-11-26 (responsable, BE-B) · **Constitución:** v2.0.0
- **Fuente:** `docs/seguridad/plan-nivel-bancario.md` §3.6 y §3.8; estado de `docs/seguridad/matriz-018.md`.

## 1. Enfoque

- **Dos fases.** La **Fase 1, «pruebas de seguridad»**, es solo código de prueba, flujos de CI y documentos: no toca los repositorios de MongoDB ni el contrato, así que se implementa ya. La **Fase 2, «bitácora inmutable»**, agrega una colección y toca los casos de uso y los repositorios de varios módulos: **queda BLOQUEADA hasta que la spec 021 se fusione**, porque 021 modifica los mismos repositorios, los esquemas `$jsonSchema`, `user_repo.go` y `cmd/admin/main.go` y no se deben mezclar los cambios (conflictos y riesgo de regresión).
- **La parte E (C-10)** depende de personas (suplente de guardia, confirmaciones de recepción, simulacro): son tareas humanas, con su condición de bloqueo explícita.
- **Sin dependencias nuevas** (RNF-02). Fuzzing con `testing.F`; las pruebas por ruta reutilizan `libopenapi` (ya usada por `internal/adapter/http/contract`); la bitácora usa `crypto/sha256`.
- **Sin cambios de contrato** (RNF-01): ningún endpoint ni campo nuevos.

## 2. Fase 1 — pruebas de seguridad

### 2.1 Fuzzing nativo (RF-10 a RF-13)

| Objetivo | Paquete | Función bajo prueba | Propiedad |
|---|---|---|---|
| `FuzzDecodeCursor` (bookings, items, chat, incidents, notificaciones) | `internal/<modulo>/adapter/http` | `decodeCursor` | Sin pánico; aceptado ⇒ id ObjectID válido, ≤ límite, ida y vuelta estable |
| `FuzzDecodeCreateRequest`, `FuzzDecodeUpdateRequest` (items), `FuzzDecodeCreateRequest` (bookings), `FuzzDecodeAuthBodies` | idem y `internal/auth/adapter/http` | decodificadores y validación de DTO | Sin pánico; inválido ⇒ `validation_failed`; aceptado ⇒ las conversiones (`input()`, `changes()`) no desreferencian nulos |
| `FuzzJWTParse` | `internal/platform/security` | `JWT.Parse` | Sin pánico; solo se acepta el token que firmó el emisor, con su misma identidad |
| `FuzzParsePayload` | `internal/handover/domain` | lectura del QR de entrega | Sin pánico; aceptado ⇒ reserva, etapa válida y token; ida y vuelta estable |
| `FuzzClientIP` | `internal/adapter/http/middleware` | `ClientIP` y `X-Forwarded-For` | Sin pánico; sin proxy de confianza no cambia la IP; la IP elegida es válida y no de confianza |

- **Corpus semilla:** `f.Add` en cada objetivo (versionado con el código) más los casos mínimos que Go guarda en `testdata/fuzz/<Objetivo>/` cuando encuentra un fallo; ambos se ejecutan como pruebas normales en `go test` y `make test` (RF-13).
- **Ejecución:** `scripts/fuzz.sh [tiempo]` recorre los 12 objetivos (`go test -fuzz` admite uno por paquete). Local: 20 s por objetivo. CI nocturna: `fuzz-nightly.yml`, 2 min por objetivo = 24 min (≥ 10 min, RF-12), sin servicios de pago, acciones fijadas por SHA, cron propio y `workflow_dispatch`. Un crash deja el trabajo en rojo y sube el caso como artefacto; se versiona como regresión.

### 2.2 Pruebas por ruta (RF-14 a RF-16)

- **Tabla de clasificación** (`routeClasses` en `cmd/api/routes_security_test.go`): cada una de las 69 operaciones es pública, de sesión, de dueño o de administración. La prueba `TestEveryOperationIsClassified` compara la tabla con el contrato empaquetado y con la seguridad que declara cada operación (la global es bearer; `security: []` deja pública); una operación sin clasificar, o una clase que contradice el contrato, rompe la CI (CA-09).
- **401 por ruta:** el router es el **real** (`apihttp.NewRouter` con `modules(...)` de `cmd/api/modules.go`, el middleware real de sesión y de rol); solo los casos de uso son dobles vacíos (interfaz embebida en nil), de modo que una ruta que llegara al caso de uso sin pasar por la sesión entraría en pánico. Para cada operación protegida se prueban 9 variantes de sesión inválida y la respuesta se valida contra el contrato. No requiere Testcontainers, así que corre en cada PR.
- **403 de administración:** una sesión válida de persona usuaria en las 6 operaciones `/admin/` ⇒ 403.
- **Brecha conocida:** prueba propia de 401 para `reject` y `cancel` (RF-16, CA-10).
- **IDOR (RF-15):** requiere casos de uso y repositorios reales, así que son **pruebas de integración** (etiqueta `integration`, MongoDB con Testcontainers) por módulo, junto a las existentes: `idor_integration_test.go` en bookings, items, chat y notifications. Las operaciones de dueño de entrega, devolución, pagos, calificaciones, incidencias y asistente quedan listadas como pendientes en `docs/seguridad/pentest-interno.md` (M).
- **Swagger UI (RF-17):** `cmd/api/docs_routes_test.go` arma lo mismo que `cmd/api` (`config.Load` + `docsHandler` + router) para development, staging y production.

### 2.3 ZAP autenticado (RF-18)

Se extiende `zap-isolated.yml`: se registra una persona sintética en la API efímera (correo y contraseña aleatorios, red `--internal`), se obtiene su token, se comprueba que abre `/api/v1/me` y se entrega a ZAP como cabecera `Authorization` con un *replacer* (`-z`), enmascarado con `::add-mask::`. `JWT_ACCESS_TTL=3h` en la API del runner para que el token dure todo el escaneo. Un paso posterior cuenta en el registro de la API las rutas protegidas con 2xx y exige al menos 5 (CA-12). La puerta sigue en `High` con `.zap/exceptions.json` y su prueba negativa. **Exclusiones:** `DELETE /me`, `POST /auth/logout` y `DELETE /me/push-tokens` se quitan de la copia del contrato que recibe ZAP (destruyen la sesión sintética; 66 de 69 operaciones se escanean). **Límite:** el rol `admin` exige sembrarlo en la base del runner (mejora). **No se pudo validar en local** (necesita Docker con red interna y la imagen de ZAP): se valida en la primera ejecución de Actions.

### 2.4 Autoevaluación de pentest (`docs/seguridad/pentest-interno.md`)

Checklist por categorías de OWASP WSTG y capítulos de ASVS L2, con la clase de cada una de las 69 operaciones, la prueba automática que la cubre y lo que sigue siendo manual, más un registro de evidencias y de hallazgos (sin secretos ni datos personales, RNF-06).

## 3. Fase 2 — bitácora inmutable (BLOQUEADA hasta fusionar la spec 021)

### 3.1 Diseño

- **Puerto** `audit.Recorder` en `internal/audit` (capa `usecase`, sin `mongo` ni `bson`): `Record(ctx, Entry) error`, con `Entry{Actor, Action, Target, At, RequestID, Outcome}`. `Action` es un catálogo cerrado (RF-03); `Outcome` ∈ {success, denied, error}. Sin valores personales ni secretos (RF-04): el constructor rechaza campos con forma de correo o de token.
- **Hash.** `hash = SHA-256("v1" | seq | actor | action | target | at(ms UTC) | requestId | outcome | prevHash)` con un separador que no puede aparecer en los campos; el primer registro apunta al **génesis** `SHA-256("vaiven-audit-genesis-v1")`.
- **Documento** `audit_log`: `{ _id, seq (int64), actor, action, target, at, requestId, outcome, prevHash, hash }`.
- **Concurrencia y réplicas (RF-02, Art. XIV).** El estado vive en MongoDB: se lee la cabeza (`seq` máximo), se calcula y se inserta con `seq+1`; un **índice único en `seq`** hace que, si dos réplicas insertan a la vez, una falle por clave duplicada y **reintente** con la nueva cabeza (acotado a 5 intentos). Índice único también en `prevHash`. Sin bloqueos en memoria.
- **Solo anexar (RF-01, CA-06).** El adaptador de MongoDB solo expone `Insert` y lectura. El usuario de la API recibe un **rol personalizado** con `insert` y `find` sobre `audit_log` y sin `update` ni `remove` (hay que listar las demás colecciones explícitamente: `readWrite` los incluye); la creación del rol y del usuario es de `@devops-mcp`/`docs/servicios.md`. Una prueba de integración intenta actualizar y borrar con ese usuario y debe fallar.
- **Fallo cerrado (RF-05, resuelto):** en administración, sanciones, pagos y cuenta, si `Record` falla la operación devuelve 503 sin efectos (el registro se escribe **antes** de confirmar la operación o dentro de la misma transacción cuando la operación ya usa una); se emite un evento de seguridad y se prueba la degradación.
- **Retención (RF-07, resuelto):** 12 meses, sin purga automática antes; documentado en `docs/seguridad/` y en `docs/servicios.md`.
- **Verificación (RF-06, RF-08, CA-02 a CA-04).** `cmd/admin audit-verify [--head <hash>] [--export-head]`: recorre por `seq`, recalcula, y detecta registro modificado, eliminado, insertado fuera de orden y cadena cortada; informa posición e identificador del primer punto de ruptura y sale con código ≠ 0. `--export-head` imprime el hash de cabeza y el total (sin datos) para anclarlo fuera de la base; `--head` detecta un borrado de la cola. Solo lectura.
- **Latencia (RNF-04):** meta orientativa ≤ 10 ms por registro, **no bloqueante**; se mide en la integración y se informa.

### 3.2 Conflictos con la spec 021 (por qué se bloquea)

021 edita `internal/*/adapter/mongo/*_repo.go`, los esquemas `$jsonSchema`, `user_repo.go` y `cmd/admin/main.go`. La Fase 2 necesita instrumentar casos de uso de cuenta, pagos, sanciones e incidencias, un repositorio y un esquema nuevos, e incorporar un subcomando a `cmd/admin/main.go`. Se parte de `develop` **después** de fusionar 021 para no resolver conflictos a ciegas.

## 4. Parte E — cierre de C-10 (tareas humanas)

RF-19 a RF-22: evidencias de entrega A3 a A6, suplente de guardia con nombre, simulacro de mesa trimestral y cadencia de detección. **Decisión de cadencia (resuelta):** meta realista ≤ 6 h con el cron de GitHub, sin cambios en la VM; el disparador externo o el temporizador en la VM quedan como mejora futura, y el informe de C-10 declara la cadencia real medida (últimas 20 ejecuciones). **El suplente de guardia sigue sin definir** (`[NEEDS CLARIFICATION]` en la spec, decisión humana) y bloquea RF-20.

## 5. Riesgos y mitigación (resumen)

| Riesgo | Mitigación |
|---|---|
| El fuzzing de CI da falsos bloqueos o consume minutos | Solo nocturno y en el repositorio, 24 min en total, casos mínimos versionados |
| La sesión sintética de ZAP caduca | TTL largo en el runner y paso de cobertura con mínimo de rutas |
| La tabla de clasificación se desactualiza | La prueba compara con el contrato y rompe la CI |
| La Fase 2 choca con 021 | Bloqueada hasta fusionar 021 |
| El fallo cerrado de la bitácora bloquea acciones legítimas | Evento de seguridad, alerta y prueba de degradación |

## 6. Constitución y reglas

Art. III (el puerto vive en `usecase`; solo el adaptador importa el driver), Art. VI.5 (sin datos personales ni secretos), Art. VII (pruebas, cobertura ≥ 80 % en paquetes nuevos de `domain` y `usecase`, `gofmt`, `vet`, `golangci-lint`), Art. VIII (sin dependencias nuevas), Art. XIII.1 (costo cero), Art. XIV (sin estado en memoria).
