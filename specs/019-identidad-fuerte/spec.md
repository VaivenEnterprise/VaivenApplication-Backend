# Spec 019 — Identidad fuerte: MFA, step-up, sesiones por dispositivo y contraseñas (backend)

- **Dueño sugerido:** BE-A (auth, spec 003) · **Con:** BE-B (eventos, alertas y pruebas de carga) y móvil MO-A / MO-B (pantallas y flujos) · **Prioridad:** P1
- **Depende de:** 002, 003, 011 (puerto `notify.Notifier`), 017 y 018 (formato de eventos, emisor/audiencia y revocación) · **Ventana:** por decidir, después de la 018
- **Constitución:** v2.0.0 · **Estado:** **Borrador** (2026-11-21). No se implementa nada hasta su aprobación (Art. I).
- **Fuente:** `docs/seguridad/plan-nivel-bancario.md`, §3.1 (identidad y acceso) y §3.2 (tokens).

## 1. Objetivo y por qué

Hoy una cuenta depende solo de correo y contraseña, y las acciones más sensibles (cambiar credenciales, dar de baja la cuenta, confirmar pagos, sancionar, cerrar incidencias) se autorizan con el mismo token de acceso de 15 minutos. Robar un teléfono desbloqueado, un token o una contraseña filtrada basta para causar daño grave. Esta spec define **qué** controles de identidad se agregan, de modo que:

- una cuenta administradora no se pueda tomar solo con su contraseña;
- las acciones irreversibles exijan una prueba reciente de que quien actúa sigue siendo la persona dueña;
- cada persona vea y controle sus sesiones por dispositivo;
- los ataques de fuerza bruta y las contraseñas ya filtradas dejen de funcionar;
- todo esto quede registrado como eventos de seguridad verificables.

**Costo cero:** sin servicios de pago ni dependencias de Go nuevas, salvo justificación en el plan (Art. VIII, RNF-03). Se reafirma (decisión del responsable, 2026-11-21): esta spec **no usa servicios de pago ni servicios externos** de ningún tipo (ni desafíos de verificación humana, ni proveedor de correo, ni Key Vault).

## 2. Historias

| # | Prio. | Historia |
|---|---|---|
| H-01 | P1 | Como **administrador**, quiero que mi sesión exija un segundo factor (TOTP), para que robar mi contraseña no dé acceso a las herramientas de moderación. |
| H-02 | P2 | Como persona usuaria, quiero activar un segundo factor opcional y tener códigos de recuperación, para proteger mi cuenta sin quedar fuera si pierdo el teléfono. |
| H-03 | P1 | Como persona usuaria, quiero que cambiar mi correo o contraseña, borrar mi cuenta o confirmar un pago me pida confirmar que soy yo, para que una sesión abierta no baste para hacer daño. |
| H-04 | P1 | Como administrador, quiero confirmar mi identidad antes de sancionar o cerrar una incidencia, para que una sesión robada no pueda usarse en decisiones irreversibles. |
| H-05 | P2 | Como persona usuaria, quiero ver mis dispositivos con sesión abierta, cerrar uno o todos los demás, y recibir un aviso si entran desde un dispositivo nuevo. |
| H-06 | P1 | Como equipo, queremos frenar ataques repetidos de credenciales con bloqueo progresivo por cuenta y por IP. |
| H-07 | P2 | Como persona usuaria, quiero que no me dejen elegir una contraseña que ya apareció en filtraciones conocidas. |
| H-08 | P1 | Como equipo, queremos que la app móvil actual siga funcionando sin cambios hasta que el móvil adopte los flujos nuevos. |

## 3. Requisitos funcionales

### 3.1 MFA TOTP (RFC 6238)

| ID | Requisito |
|---|---|
| RF-01 | Una persona autenticada puede **enrolar** un segundo factor TOTP: el backend genera un secreto y lo entrega **una sola vez** (como URI `otpauth://` para código QR) junto con su estado «pendiente». El factor solo se activa tras **verificar** un primer código válido. |
| RF-02 | El secreto TOTP se guarda cifrado en reposo y **nunca** se devuelve de nuevo (ni en el perfil, ni en listados, ni en logs). El cifrado usa una **llave de aplicación** entregada por variable de entorno en el `.env` de cada entorno, como los demás secretos (Art. VI.1); nunca en el repositorio ni en logs. La custodia de la llave es de la persona responsable (BE-B) mediante el manejo de secretos por entorno ya existente. El procedimiento de rotación de la llave (sin dejar fuera a las personas ya enroladas) se define en `plan.md`. Si la persona pierde el secreto, debe desactivar y volver a enrolar. |
| RF-03 | La verificación acepta un desfase de ±1 paso de 30 s. Un código ya aceptado **no puede reutilizarse** dentro de su ventana (anti-replay), también entre peticiones concurrentes. |
| RF-04 | Al activar el factor se emiten **códigos de recuperación** de un solo uso (cantidad por definir en el plan), mostrados una sola vez y guardados solo como hash. Usar un código lo consume. Se puede regenerar el conjunto con *step-up* (invalida el anterior). |
| RF-05 | Con el factor activo, el inicio de sesión exige el código TOTP o un código de recuperación en un segundo paso; sin él no se emiten tokens. |
| RF-06 | La cuenta con rol `admin` **debe** tener MFA activo: un administrador sin factor no obtiene tokens de acceso de administración (401) hasta enrolarlo. `create-admin` deja la cuenta lista para el enrolamiento inicial. |
| RF-07 | Una persona puede **desactivar** su factor con *step-up*. La desactivación **no** se permite a una cuenta `admin`. |
| RF-08 | Los intentos fallidos de TOTP cuentan para el bloqueo progresivo (RF-18) por cuenta. |

### 3.2 Reautenticación reciente (step-up)

| ID | Requisito |
|---|---|
| RF-09 | Las acciones **sensibles** exigen una prueba reciente: código TOTP válido o contraseña verificada en los **últimos 5 minutos**. La lista inicial es: cambiar correo, cambiar contraseña, `DELETE /me`, confirmar o reportar un pago, sanciones de administración y cierre de incidencias. |
| RF-10 | Si falta la prueba reciente, la ruta responde **401** con `error.code = "unauthorized"` (código ya permitido por el Art. IV.3, sin código nuevo) y el motivo en `error.details`: `[{ "field": "stepUp", "issue": "required" }]` (mensaje en español), y no ejecuta ninguna acción. La app móvil distingue el *step-up* de una sesión vencida por `details[].field`, no por un código nuevo; es compatible hacia atrás (un cliente que ignora `details` ve el 401 de siempre). Cumplida la prueba, el cliente repite la petición. |
| RF-11 | La prueba reciente queda ligada a la **sesión** (no a la persona en general) y expira sola; cerrar la sesión o cambiar la contraseña la invalida. |
| RF-12 | Los administradores con MFA obligatorio cumplen el *step-up* con TOTP; la contraseña sola no basta para sanciones ni cierre de incidencias. |

### 3.3 Sesiones por dispositivo

| ID | Requisito |
|---|---|
| RF-13 | Una persona puede **listar** sus sesiones activas (dispositivo o etiqueta legible, fecha de creación, último uso, IP truncada, marca de «sesión actual») sin exponer tokens ni identificadores internos sensibles. |
| RF-14 | Puede **revocar** una sesión concreta y **revocar todas las demás**. La revocación invalida el refresh de esa sesión y deja inválido su token de acceso en **≤ 60 s** (objetivo de revocación de la spec 018, RF-12). |
| RF-15 | El refresh tiene **vida absoluta de 30 días** desde el inicio de sesión: la rotación no la extiende. |
| RF-16 | Un inicio de sesión desde un **dispositivo nuevo** genera un aviso push por el puerto `notify.Notifier`. Es el **único** canal de aviso: el aviso por correo queda diferido y fuera de alcance (§7; no existe proveedor de correo y rige el costo cero). |

### 3.4 Bloqueo progresivo y contraseñas

| ID | Requisito |
|---|---|
| RF-17 | Las contraseñas del registro y del cambio se comparan con una **lista local de las 100 000 más filtradas**, incluida en el repositorio; sin llamadas externas. Una contraseña de la lista se rechaza con 400 `validation_failed`. |
| RF-18 | Los fallos de inicio de sesión y de TOTP producen **demora y bloqueo crecientes** por cuenta y por IP, sobre el límite de tasa actual (POL-11 / spec 003 RF-07) en Redis, con degradación local si Redis cae (Art. XIV.2). Una cuenta bloqueada responde con 429 `rate_limited` e igual mensaje exista o no el correo. |
| RF-19 | **No** se usa desafío de verificación humana (Cloudflare Turnstile u otro; fuera de alcance, §7). La protección contra relleno de credenciales descansa solo en la demora y el bloqueo progresivos por cuenta y por IP en Redis (RF-18), que debe cumplirse por sí solo. |

### 3.5 Eventos de seguridad y compatibilidad

| ID | Requisito |
|---|---|
| RF-20 | Cada flujo nuevo registra eventos con `middleware.LogSecurityEvent` (formato común de la 018): enrolamiento iniciado, activado y desactivado; código TOTP fallido y reutilizado; uso de código de recuperación; MFA exigido y no cumplido; *step-up* exigido (`unauthorized` con `details` `stepUp`); sesión listada, revocada y revocación masiva; dispositivo nuevo; bloqueo progresivo; contraseña filtrada rechazada. Sin tokens, contraseñas, secretos, códigos, correos ni IP completa. |
| RF-21 | Todo el comportamiento nuevo va detrás de **banderas de funcionalidad apagadas por defecto**, activables por entorno y por separado (MFA opcional, MFA de administración, *step-up*, avisos). Con las banderas apagadas la API se comporta como hoy. |
| RF-22 | Las personas **sin MFA** siguen iniciando sesión como hasta ahora (excepto administradores, RF-06, cuando su bandera esté encendida). |
| RF-23 | Las rutas nuevas y cambios de contrato (§6) se publican en `api/paths/` y `api/openapi.yaml` **en la primera tarea** (Art. IV.6). |

## 4. Requisitos no funcionales

| ID | Requisito |
|---|---|
| RNF-01 | Con las banderas apagadas, el contrato y el comportamiento actuales no cambian (regresión en verde de `auth`, 018 RNF-02). |
| RNF-02 | Sin estado en memoria del proceso (Art. XIV.1): anti-replay, contadores y marcas de *step-up* viven en Redis o MongoDB. **Redis no es la fuente de verdad** de sesiones ni de factores (Art. XIV.2); si Redis cae, el anti-replay de TOTP se apoya en MongoDB. |
| RNF-03 | Costo cero; solo biblioteca estándar para TOTP (HMAC-SHA1 de RFC 6238); cualquier dependencia nueva exige plan y `memoria.md` (Art. VIII). |
| RNF-04 | Secretos TOTP y códigos de recuperación protegidos en reposo (cifrado con la llave de aplicación de RF-02 y argon2id respectivamente, Art. VI.3); nada de esto en logs ni en errores. |
| RNF-05 | El paso adicional de MFA y el chequeo de *step-up* añaden ≤ 20 ms al p95 de `login` y de las rutas sensibles en staging. |
| RNF-06 | Cobertura ≥ 80 % en `auth` (usecase y domain) y pruebas de abuso para TOTP, recuperación y bloqueo. |
| RNF-07 | Los mensajes de error no revelan si el correo existe ni si la cuenta tiene MFA hasta superar la contraseña. |

## 5. Criterios de aceptación

| ID | Dado / Cuando / Entonces |
|---|---|
| CA-01 | Dado un enrolamiento, cuando se completa, entonces el secreto no vuelve a aparecer en ninguna respuesta ni log (búsqueda automatizada en respuestas y logs de la prueba). |
| CA-02 | Dado un código TOTP ya aceptado, cuando se envía de nuevo dentro de su ventana, entonces se rechaza y se registra el evento de reutilización; lo mismo con dos envíos simultáneos. |
| CA-03 | Dado un código con desfase de ±1 paso, entonces se acepta; con ±2 pasos, se rechaza. |
| CA-04 | Dada una cuenta `admin` sin MFA activo (bandera encendida), cuando inicia sesión, entonces recibe 401 y no obtiene tokens de administración. |
| CA-05 | Dado un código de recuperación, cuando se usa una vez entonces inicia sesión; cuando se usa otra vez, se rechaza; el hash nunca sale de la base. |
| CA-06 | Dada una ruta sensible de RF-09 sin prueba reciente, cuando se llama, entonces responde 401 `unauthorized` con `details` `[{ "field": "stepUp", "issue": "required" }]`, no cambia ningún dato y registra el evento; tras cumplir el *step-up*, la repetición funciona. |
| CA-07 | Dada una prueba de contraseña de hace más de 5 minutos, entonces no cuenta como reciente; dada una de hace menos, sí. |
| CA-08 | Dada una sanción o cierre de incidencia por una cuenta `admin`, cuando solo hay contraseña reciente, entonces se exige TOTP (401 `unauthorized` con `details` `stepUp`). |
| CA-09 | Dadas dos sesiones, cuando se revoca una, entonces su refresh falla de inmediato y su token de acceso se rechaza en **≤ 60 s**; la otra sigue válida. «Revocar las demás» deja solo la actual. |
| CA-10 | Dada una sesión con 30 días desde su inicio, cuando se refresca, entonces se rechaza aunque el refresh anterior sea válido. |
| CA-11 | Dado un inicio de sesión desde un dispositivo no visto antes, entonces se emite una llamada a `notify.Notifier` (verificable con un doble de prueba) sin datos personales en el cuerpo del evento. |
| CA-12 | Dada una prueba de credenciales contra una cuenta y desde una IP, entonces tras el umbral los intentos adicionales reciben 429 y el éxito posterior al bloqueo es **0**; con Redis apagado el límite local sigue activo. |
| CA-13 | Dada cualquiera de las 100 000 contraseñas del archivo, cuando se registra o se cambia, entonces se rechaza; una que no está, se acepta si cumple la política vigente. |
| CA-14 | Dado cada evento de RF-20, entonces aparece con el formato común, sin PII (revisión de logs de prueba). |
| CA-15 | Dadas todas las banderas apagadas, cuando corre la suite actual y las pruebas de contrato, entonces pasan sin modificación; un usuario sin MFA inicia sesión como hoy. |
| CA-16 | Dado el contrato nuevo, entonces `api/openapi.yaml` es válido y documenta el 401 `unauthorized` con `details` de *step-up*, rutas de MFA y de sesiones antes de cualquier implementación. |

## 6. Cambios de contrato previstos (no se escriben en esta spec)

Se publicarán en la **primera tarea** del `tasks.md` (Art. IV.6); aquí solo se listan:

- **Rutas nuevas (nombres tentativos):** enrolar, verificar y desactivar MFA; regenerar códigos de recuperación; segundo paso de `login` con TOTP o recuperación; confirmar identidad para *step-up*; `GET /me/sessions`, `DELETE /me/sessions/{id}` y revocar las demás.
- **Rutas existentes con nueva variante de error 401 `unauthorized` con `details` `[{ "field": "stepUp", "issue": "required" }]`:** cambio de correo y de contraseña, `DELETE /me`, confirmar y reportar pago, sanciones y cierre de incidencias.
- **Respuesta de `login`:** un estado «segundo factor requerido» para cuentas con MFA, sin romper clientes actuales mientras la bandera esté apagada.
- **Constitución:** el *step-up* reutiliza `unauthorized`, ya permitido por el Art. IV.3, con el formato de `details` del mismo artículo; no hay código nuevo ni enmienda. La 019 no modifica `constitution.md`.

## 7. Fuera de alcance

- **Play Integrity** (integridad del dispositivo Android): futuro, no se implementa ni se diseña aquí.
- JWT asimétrico (ES256, `kid`, JWKS) y rotación de llaves: es la otra mitad del plan §3.2 y se aborda aparte; esta spec solo usa el emisor, la audiencia y la revocación de la 018.
- Biometría, passkeys/WebAuthn, SMS u OTP por correo como segundo factor.
- Consulta externa de contraseñas filtradas (HIBP) y cualquier servicio de pago.
- Cambios en el repositorio móvil (se coordinan con MO-A / MO-B, solo el contrato).
- Cifrado de datos por campo con Key Vault (plan §3.3); aquí el secreto TOTP se protege con una llave de aplicación por variable de entorno (RF-02).
- **Desafío de verificación humana (Cloudflare Turnstile u otro):** es un servicio externo; queda fuera por el Art. VIII y la regla de costo cero (decisión del responsable, 2026-11-21).
- **Avisos por correo** de inicio de sesión desde dispositivo nuevo: diferidos, no existe proveedor de correo y rige el costo cero; solo se mantiene el aviso push (RF-16).
- Recuperación de cuenta por correo («olvidé mi contraseña») y verificación de correo.

## 8. Dependencias y riesgos

**Dependencias**
- Spec 003 (sesiones, familias de refresh, `DELETE /me`), 018 (formato de eventos, objetivo de revocación ≤ 60 s, emisor y audiencia) y 011 (`notify.Notifier`).
- Llave de aplicación para cifrar el secreto TOTP, en el `.env` de cada entorno (custodia: BE-B); sin servicios externos.
- Redis (Art. XIV) para contadores y anti-replay; MongoDB como fuente de verdad.
- Móvil: pantallas de enrolamiento, segundo paso y reintento tras el 401 `unauthorized` con `details` `stepUp`.

**Riesgos**
- **Bloqueo de administradores** (pérdida de teléfono sin recuperación): mitigar con códigos de recuperación y un procedimiento documentado de restablecimiento por una segunda persona administradora.
- **Reloj desfasado del servidor**: el TOTP depende de la hora; verificar sincronización NTP en la VM.
- **Relleno de credenciales sin desafío humano**: al no usar Turnstile, todo recae en el bloqueo progresivo (RF-18); sus umbrales deben validarse con pruebas de abuso (RNF-06).
- **Pérdida de la llave de cifrado TOTP**: sin ella las personas enroladas no podrían verificar; mitigar con custodia de BE-B, copia por el manejo de secretos por entorno y el procedimiento de rotación del plan.
- **Bloqueo por IP compartida** (NAT móvil): la demora por IP debe ser menos agresiva que la de cuenta para no castigar a personas inocentes.
- **Denegación de servicio por bloqueo de cuenta**: un atacante puede bloquear a otra persona a propósito; el bloqueo es temporal y progresivo, no permanente.
- **Móvil no listo**: por eso las banderas nacen apagadas (RF-21); encenderlas sin la app lista rompería flujos.
- **Cambio de contrato** es prerrequisito y puede retrasar el inicio.
- **Clientes que tratan todo 401 como cierre de sesión:** deben revisar `error.details` (`field: "stepUp"`) antes de cerrar la sesión; coordinar con MO-A / MO-B para que el reintento por *step-up* no expulse a la persona.

## 9. Preguntas pendientes

- Resuelto (2026-11-21, responsable): se reutiliza `unauthorized` (401) con `details` `[{ "field": "stepUp", "issue": "required" }]`; no se enmienda el Art. IV.
- Resuelto (2026-11-21, responsable): **no** se usa desafío de verificación humana (Cloudflare Turnstile); es un servicio externo y contradice el Art. VIII y el costo cero. Se confía solo en el bloqueo progresivo por cuenta e IP (RF-18, RF-19). Pasa a «Fuera de alcance».
- Resuelto (2026-11-21, responsable): el aviso por **correo** de dispositivo nuevo se **difiere** (no hay proveedor de correo; costo cero). Solo queda el aviso push por `notify.Notifier` (RF-16). Pasa a «Fuera de alcance».
- Resuelto (2026-11-21, responsable): el secreto TOTP se cifra en reposo con una llave de aplicación por variable de entorno en el `.env` de cada entorno (Art. VI.1), nunca en repositorio ni logs; custodio: BE-B mediante el manejo de secretos por entorno existente; Key Vault sigue fuera de alcance; la rotación se define en `plan.md` (RF-02, RNF-04).
- [NEEDS CLARIFICATION: número de códigos de recuperación, umbrales y tiempos del bloqueo progresivo, y qué se considera «dispositivo nuevo» (sin datos personales).]
- [NEEDS CLARIFICATION: BE-A confirma si el cambio de correo existe como ruta hoy o es un requisito futuro; si no existe, RF-09 lo cubre solo cuando se cree.]
- [NEEDS CLARIFICATION: MO-A / MO-B confirman ventana para el flujo móvil antes de encender las banderas en producción.]
