# Plan 018 — Hardening de seguridad (backend)

- **Spec:** `spec.md` · **Estado:** PROPUESTA, borrador local, no aprobada; preguntas cerradas con propuesta el 2026-11-17 · **Constitución:** v2.0.0
- **Evidencia base:** Diagnóstico interno del 17 de noviembre de 2026, disponible a solicitud del equipo. No se publica aquí; los detalles operativos de los hallazgos se revisa por canal privado.

## 1. Enfoque

- **Primero lo local y reversible (grupo A)**, en la API y en los workflows, validado en staging. Producción no cambia hasta tener evidencia.
- **La cadena de suministro se arma de punta a punta, en orden:** construir → escanear → SBOM → firmar → verificar → desplegar por digest. Un eslabón no se activa como bloqueante hasta que el anterior funcione.
- **Grupo B:** cada control tiene su dueño y su autorización. Mientras no los tenga, sus tareas están **bloqueadas**.
- **Herramientas** (gratuitas y fijadas por versión o SHA): Trivy, Sigstore/cosign (firma sin llaves con el OIDC de GitHub), atestados SBOM y de procedencia de BuildKit, y OWASP ZAP. En Go, solo la biblioteca estándar (`compress/gzip`); **ninguna dependencia nueva**.

## 2. Fichas por control

Cada ficha tiene los 10 campos: requisito, archivos o componentes, amenaza, pruebas, evidencia, compatibilidad, riesgo residual, responsable, dependencias y rollback.

### C-01 Cabeceras de seguridad y HSTS (grupo A; **implementa la 012-T003 de BE-A**, la 018 solo verifica)

| Campo | Contenido |
|---|---|
| Requisito | RF-01. Criterios **complementarios** a la 012-T003, que no sustituyen los suyos: en toda respuesta, `X-Content-Type-Options: nosniff`, `X-Frame-Options: DENY`, `Referrer-Policy: no-referrer` y `Cross-Origin-Resource-Policy: same-origin`; `Cache-Control: no-store` en las rutas autenticadas, sin duplicar el de `handover`; `/docs` conserva su CSP. **HSTS**, en este orden: (1) validar los nombres de host reales de cada entorno, el certificado, dónde termina TLS, la compatibilidad con staging (Expo Go y Swagger UI) y el comportamiento HTTP/HTTPS (la configuración concreta se revisa por canal privado); (2) en **staging**, `Strict-Transport-Security: max-age=300` **sin** `includeSubDomains` y **sin** `preload`, enviado solo sobre TLS; (3) subirlo a `31536000` solo después de 7 días sin incidencias. En producción, **no se configura dentro de esta spec** (B-8) |
| Archivos | **Los de la 012-T003** (`internal/adapter/http/middleware/security_headers.go` y su prueba, `router.go`), que edita su dueño. La 018 solo agrega la evidencia en `docs/seguridad/evidencia-018.md` |
| Amenaza | *Sniffing* MIME, *clickjacking*, fuga del *referrer*, *downgrade* a HTTP y cacheo de datos privados |
| Pruebas | Las de la 012-T003. La T002 de la 018 verifica en staging con `curl -sSI` una ruta pública, una autenticada, un 404 y un error: todas las cabeceras presentes, HSTS solo por HTTPS y `no-store` en las privadas |
| Evidencia | Salida de `curl -sSI` (la petición autenticada usa una cuenta de prueba y no se guarda el `Authorization`) y la checklist de la validación previa a HSTS |
| Compatibilidad | La app nativa ignora estas cabeceras y HSTS. Swagger UI sigue funcionando. **HSTS es por nombre de host:** si dos entornos comparten nombre, HSTS de uno aplica también al otro en los navegadores; con `max-age=300` el efecto dura 5 min. Si es el caso se revisa por canal privado |
| Riesgo residual | Sin *preload* (el dominio actual no es propio del proyecto); el primer acceso de un navegador no queda protegido por HSTS |
| Responsable | Implementa **BE-A** (012-T003); verifica BE-B (T002) |
| Dependencias | 012-T003 terminada; visto bueno de BE-A sobre los criterios complementarios (no se asume) |
| Rollback | Lo revierte su dueño en la 012; HSTS empezó con `max-age=300`, así que un error dura como máximo 5 min |

### C-02 Eventos de seguridad auditables (grupo A)

| Campo | Contenido |
|---|---|
| Requisito | RF-02: `slog` con `event` (`auth.refresh_reuse`, `auth.family_revoked`, `auth.login_failed`, `authz.forbidden`, `ratelimit.blocked`), `requestId` y `outcome`. Sin token, hash, correo, contraseña, cuerpo ni IP completa (el `userId` interno solo si hace falta) |
| Archivos | `internal/auth/usecase/{refresh.go, logout.go, login.go}` (BE-A, solo registro); `internal/adapter/http/middleware/ratelimit.go`; `internal/auth/adapter/http/middleware.go`; y sus pruebas |
| Amenaza | Ataques que no se detectan: robo de refresh token, fuerza bruta, escalada |
| Pruebas | Unitarias que capturan el registro y comprueban el evento y la **ausencia** de datos sensibles (como CA-07 de la 015); integración de la reutilización de un refresh |
| Evidencia | Prueba en verde; en staging, `check-logs.sh` muestra el conteo de eventos y 0 tokens o correos |
| Compatibilidad | Sin cambios de contrato ni de comportamiento |
| Riesgo residual | Hasta C-10 o RF-17, la retención y la consulta de registros son limitadas (el detalle se revisa por canal privado) |
| Responsable | BE-B (middleware) y BE-A (visto bueno en `internal/auth`) |
| Dependencias | Ninguna |
| Rollback | Revertir; solo agrega registros |

### C-03 Escaneo de contenedores con Trivy (grupo A)

> **Trazabilidad:** implementa **016-T011** (BE-B; se conserva). Si el equipo **amplía la 016-T011** con estos criterios, la T005 de la 018 se marca como «cubierta por la 016-T011» y no se ejecuta. Si no, la T005 implementa **solo la diferencia**: escaneo del digest antes de desplegar, imágenes base por digest, `.trivyignore` con vencimiento y la prueba negativa.

| Campo | Contenido |
|---|---|
| Requisito | RF-03: `trivy fs` en la CI (dependencias, secretos y configuración de Dockerfile y Compose) y `trivy image` sobre el **digest** construido, **antes** de desplegar; falla con HIGH/CRITICAL que tengan corrección (`--ignore-unfixed`). `.trivyignore` con CVE, motivo, dueño y vencimiento. Imágenes base (`golang`, `distroless`, `mongo`, `redis`) fijadas por digest; Dependabot para `docker` |
| Archivos | `.github/workflows/ci.yml`, `.github/workflows/deploy-staging.yml`, `.trivyignore` (nuevo), `.github/dependabot.yml`, `Dockerfile`, `deploy/base/docker-compose.yml`, `deploy/production/docker-compose.yml` (solo el `image:` por digest) |
| Amenaza | Componentes vulnerables en la imagen o en la base; mala configuración del contenedor |
| Pruebas | Prueba negativa controlada en una rama: una imagen base vieja con un CVE crítico hace fallar el *job*. Prueba positiva: la imagen actual pasa |
| Evidencia | Enlace a las ejecuciones fallida y exitosa; reporte SARIF o JSON como artefacto |
| Compatibilidad | Sin efecto en la app |
| Riesgo residual | CVE sin corrección (se reportan, no bloquean); falsos negativos de la base de datos de vulnerabilidades |
| Responsable | BE-B |
| Dependencias | Ninguna; aplicar las imágenes de datos por digest en el servidor requiere autorización (**grupo B**) |
| Rollback | Quitar el paso o marcarlo como no bloqueante **solo con una decisión escrita** (RNF-06) |

### C-04 SBOM (grupo A)

> **Trazabilidad:** implementa **016-T011** (BE-B; se conserva). El SBOM lo genera la **016-T011**. La 018 exige que quede **asociado al digest** y sea consultable (atestado de BuildKit o de cosign). La T006 solo verifica eso y agrega el despliegue por digest (C-07), que es nuevo.

| Campo | Contenido |
|---|---|
| Requisito | RF-04: atestados SBOM (SPDX) y de procedencia de BuildKit (`--sbom=true --provenance=mode=max`) asociados al digest en GHCR, más un **CycloneDX** con Trivy como artefacto y como atestado de cosign |
| Archivos | `.github/workflows/deploy-staging.yml` (*job* `build`: `docker buildx build` en lugar de `docker build` + `push`) |
| Amenaza | No saber qué componentes corren; respuesta lenta ante un CVE nuevo |
| Pruebas | `docker buildx imagetools inspect <img@digest> --format '{{json .SBOM}}'` muestra módulos Go y paquetes; Gitleaks sobre el SBOM: 0 secretos |
| Evidencia | Artefacto CycloneDX de una ejecución y la salida de `imagetools inspect` |
| Compatibilidad | Ninguna |
| Riesgo residual | El SBOM refleja el momento del build; los CVE se reevalúan escaneando el SBOM guardado |
| Responsable | BE-B |
| Dependencias | C-03 (mismo *job*) |
| Rollback | Volver a `docker build` + `push` (la imagen es la misma) |

### C-05 Firma de imágenes mediante OIDC (grupo A)

| Campo | Contenido |
|---|---|
| Requisito | RF-05: `cosign sign --yes <img@digest>` sin llaves (Fulcio y Rekor) con `id-token: write` **solo** en el *job* `build`; `cosign attest` del CycloneDX |
| Archivos | `.github/workflows/deploy-staging.yml` |
| Amenaza | Imagen sustituida en el registro |
| Pruebas | `cosign verify` con la identidad correcta pasa y con otra falla |
| Evidencia | Entrada en el registro de transparencia (Rekor) y salida de `cosign verify` |
| Compatibilidad | Ninguna |
| Riesgo residual | Depende de la disponibilidad de Sigstore; la firma queda en un registro público (solo metadatos, no secretos) |
| Responsable | BE-B |
| Dependencias | C-04 |
| Rollback | Quitar el paso; C-06 deja de ser bloqueante |

### C-06 Verificación obligatoria de firmas (grupo A en staging; producción, grupo B)

| Campo | Contenido |
|---|---|
| Requisito | RF-06: en el *job* `deploy`, `cosign verify <img@digest> --certificate-oidc-issuer https://token.actions.githubusercontent.com --certificate-identity https://github.com/VaivenEnterprise/VaivenApplication-Backend/.github/workflows/deploy-staging.yml@refs/heads/develop`; si falla, no hay Run Command |
| Archivos | `.github/workflows/deploy-staging.yml`; después, `scripts/deploy-production.sh` (grupo B) |
| Amenaza | Desplegar una imagen no producida por el pipeline |
| Pruebas | **Negativa:** despliegue manual (`workflow_dispatch`) con una imagen sin firma, que se detiene. **Positiva:** el flujo normal |
| Evidencia | Ejecución fallida esperada y ejecución exitosa |
| Compatibilidad | Ninguna |
| Riesgo residual | La identidad la controla quien pueda modificar el *workflow* en `develop` (lo mitigan las reglas de ramas y la revisión de PR) |
| Responsable | BE-B; producción, con autorización del responsable |
| Dependencias | C-05 y C-07 |
| Rollback | Pasar la verificación a advertencia **solo** con una decisión escrita |

### C-07 Despliegue por digest (grupo A en staging)

| Campo | Contenido |
|---|---|
| Requisito | RF-07: el *job* `build` publica el digest; `deploy` envía `PULL_IMAGE=ghcr.io/…@sha256:…`; `remote-deploy.sh` descarga por digest y guarda el digest en `current-tag` y `previous-tag` para la vuelta atrás |
| Archivos | `.github/workflows/deploy-staging.yml`, `deploy/remote-deploy.sh` |
| Amenaza | Sustitución de la imagen entre el build y el despliegue |
| Pruebas | `docker inspect` en el servidor muestra el `RepoDigest` igual al verificado; vuelta atrás probada a propósito |
| Evidencia | Salida del paso de despliegue con el digest y del Run Command |
| Compatibilidad | Ninguna; la imagen local se sigue llamando `vaiven-api:<sha>` |
| Riesgo residual | La promoción a producción se verifica por digest en una fase posterior (T019, grupo B) |
| Responsable | BE-B |
| Dependencias | Ninguna (se puede hacer antes de C-05) |
| Rollback | Volver a descargar por etiqueta |

### C-08 Compresión HTTP segura (grupo A; prueba móvil en el grupo B)

| Campo | Contenido |
|---|---|
| Requisito | RF-08: gzip (`compress/gzip`, nivel 5) si `Accept-Encoding` incluye gzip, el tipo es `application/json` y la respuesta pesa ≥ 1 KB; `Vary: Accept-Encoding`. **Lista de exclusión** (BREACH): `/api/v1/auth/*`, `/api/v1/me` (todos los métodos), `/api/v1/uploads/signature`, `*/qr`, `/docs/*`. Sin Brotli (dependencia nueva) |
| Archivos | `internal/adapter/http/middleware/compress.go` (nuevo) y su prueba; `internal/adapter/http/router.go` |
| Amenaza | No es de seguridad (rendimiento móvil). Mitiga el riesgo **que crea**: BREACH en respuestas con secretos |
| Pruebas | El JSON es idéntico con y sin compresión en todas las rutas de lista; las rutas excluidas nunca llevan `Content-Encoding`; < 1 KB sin comprimir; cliente sin `Accept-Encoding`; k6 antes y después (RNF-05) |
| Evidencia | Pruebas en verde; `curl --compressed` en staging; reporte de k6 |
| Compatibilidad | OkHttp (Android) y `fetch` de React Native descomprimen solos; **validar en Expo Go** (RF-18, MO-A y MO-B) |
| Riesgo residual | Una ruta nueva con secretos que no se agregue a la exclusión: se mitiga con una prueba que recorre `openapi.yaml` y exige clasificar cada ruta |
| Responsable | BE-B; validación móvil de MO-A y MO-B |
| Dependencias | C-01 (mismo `router.go`, en orden) |
| Rollback | Revertir el middleware (una línea en `router.go`) |

### C-09 DAST en entornos autorizados (aislado: grupo A; staging pasivo: grupo B)

> **Trazabilidad:** implementa **016-T010** (BE-B; se conserva). El escaneo *baseline* lo hace la **016-T010**. La 018 agrega el **modo activo aislado** en el runner y los umbrales. Si el equipo amplía la 016-T010, la T009 de la 018 queda «cubierta por la 016-T010».

| Campo | Contenido |
|---|---|
| Requisito | RF-09: `zap-api-scan` con `api/openapi.yaml` contra una API **levantada en el runner** (Compose con MongoDB y Redis efímeros), con una cuenta de prueba registrada en ese momento y el token por *replacer*; `-m 5`, un hilo; `.zap/rules.tsv` con excepciones justificadas; falla con High. RF-16 (grupo B): `zap-baseline` **pasivo** contra staging por `workflow_dispatch` |
| Archivos | `.github/workflows/zap.yml` (nuevo), `.zap/rules.tsv` (nuevo), `docs/calidad/deuda.md` (falsos positivos; spec 016) |
| Amenaza | Fallas que solo aparecen en ejecución: inyección, cabeceras, divulgación de información |
| Pruebas | Ejecución en el runner sin acceso a staging ni a producción; prueba negativa con una regla forzada que debe fallar |
| Evidencia | Reporte HTML/JSON como artefacto y enlace a la ejecución |
| Compatibilidad | Ninguna |
| Riesgo residual | Cobertura limitada a lo descrito en OpenAPI; sin pruebas de lógica de negocio |
| Responsable | BE-B; RF-16 con autorización del responsable |
| Dependencias | C-01 (para no reportar lo que C-01 ya corrige); la T008 de la 016 ayuda |
| Rollback | El *workflow* es manual o programado; se desactiva sin tocar la API |

### C-10 Observabilidad y alertas (local: grupo A; Azure: grupo B)

| Campo | Contenido |
|---|---|
| Requisito | RF-10: seis alertas iniciales (tabla de abajo), documento de respuesta (`docs/seguridad/respuesta-incidentes.md`) y conteo de eventos en `check-logs.sh`. RF-17 (grupo B): Azure Monitor Agent y Log Analytics con límite diario de ingesta |
| Archivos | `docs/seguridad/respuesta-incidentes.md` (nuevo), `scripts/check-logs.sh`; en el grupo B, plantillas de Azure Monitor |
| Amenaza | Detección tardía; incidentes sin procedimiento |
| Pruebas | Cada alerta tiene su **prueba de activación** (tabla) |
| Evidencia | Registro de cada prueba de activación (fecha, alerta recibida y canal) |
| Compatibilidad | Ninguna |
| Riesgo residual | Sin una plataforma central (grupo B), las alertas A-1 a A-4 dependen de una revisión programada de los registros y no son en tiempo real |
| Responsable | BE-B (infraestructura) y BE-A (autenticación); **destinatarios pendientes** donde se indica |
| Dependencias | C-02 (eventos), C-03 y C-06 |
| Rollback | Documental |

**Alertas iniciales.** Los umbrales son propuestos y se ajustan después de 2 semanas con datos reales. «Pendiente» = decisión del equipo; no se inventa un responsable.

| # | Alerta | Fuente | Umbral | Severidad | Destinatario | Canal | Prueba de activación | Respuesta | Procedimiento |
|---|---|---|---|---|---|---|---|---|---|
| A-1 | Reutilización de refresh tokens | Evento `auth.refresh_reuse` (C-02) en los registros de la API | ≥ 3 en 10 min en el entorno, o ≥ 2 de la misma familia en 1 h | Alta | **Pendiente** (propuesta: BE-A) | Correo del equipo; alerta automática con la plataforma central (B-5) | Reutilizar a propósito un refresh token de prueba 3 veces en staging | ≤ 4 h hábiles | Confirmar que no es un error de la app; si es un ataque, revocar las familias afectadas; si se sospecha fuga del secreto, rotar el secreto de firma de tokens (cierra todas las sesiones) |
| A-2 | Fallos repetidos de inicio de sesión | Evento `auth.login_failed` | ≥ 50 en 10 min en el entorno, o el límite por cuenta alcanzado por ≥ 5 cuentas en 1 h | Media | **Pendiente** (propuesta: BE-A) | Correo del equipo | Script con 51 inicios fallidos de cuentas de prueba en staging | ≤ 1 día hábil | Determinar si es fuerza bruta o *credential stuffing*; el límite de tasa ya frena; si sigue, bajar el límite o bloquear la IP en el NSG (B-5) |
| A-3 | Errores HTTP 5xx | Registro de acceso (`status` ≥ 500) | > 2 % de las peticiones en 5 min con ≥ 50 peticiones, o ≥ 10 en 5 min | Alta | **Pendiente** (propuesta: BE-B) | Correo del equipo; Better Stack ya avisa si `/healthz` cae | Forzar un 500 en una instancia local o efímera (**nunca en producción**) | ≤ 2 h hábiles | Revisar los registros con `requestId`; si viene de un despliegue, volver atrás |
| A-4 | Reinicios anómalos de contenedores | `docker inspect` (`RestartCount`, `OOMKilled`) por Run Command o `check-logs.sh` | ≥ 3 reinicios en 30 min, o cualquier `OOMKilled` | Alta | **Pendiente** (propuesta: BE-B) | Correo del equipo | Reiniciar a propósito la API de staging 3 veces (con B-5) | ≤ 2 h hábiles | Revisar el uso de memoria y los registros; subir el límite o reducir la carga; si es por un ataque, ver A-2 y A-3 |
| A-5 | Vulnerabilidades críticas | Trivy, CodeQL y govulncheck en la CI | Cualquier **CRITICAL** con corrección, en la imagen o en el código | Crítica | **Pendiente** (propuesta: BE-B) | PR bloqueado y correo de GitHub Actions al autor y a quienes revisan | Prueba negativa de C-03 (imagen base vieja) | ≤ 2 días hábiles para corregir o registrar la excepción con vencimiento | Actualizar la dependencia o la imagen base; si no hay corrección, excepción con vencimiento en `.trivyignore` |
| A-6 | Fallo de verificación de firma | Paso `cosign verify` del despliegue (C-06) | Cualquier fallo | Crítica | **Pendiente** (propuesta: BE-B) | *Workflow* fallido y correo de GitHub Actions | Prueba negativa de C-06 (imagen sin firma) | ≤ 4 h hábiles | **No reintentar sin investigar**: averiguar quién publicó la imagen y por qué no tiene firma; tratarlo como posible compromiso de la cadena de suministro |

### C-11 Emisor y audiencia del JWT (grupo B: BE-A)

| Campo | Contenido |
|---|---|
| Requisito | RF-11: `iss = vaiven-<entorno>` y `aud = vaiven-app`, validados con `jwt.WithIssuer` y `jwt.WithAudience`. **Transición:** durante la vida de un token de acceso (definida en la spec 003) también se aceptan tokens sin esos claims; después, se rechazan |
| Archivos | `internal/platform/security/jwt.go` y su prueba; `internal/config` (emisor por entorno); `cmd/api/main.go` |
| Amenaza | Tokens de un entorno o de otra aplicación aceptados por error |
| Pruebas | Emisor o audiencia incorrectos dan 401; sin claims dentro de la ventana se aceptan y fuera se rechazan; regresión completa de autenticación |
| Evidencia | Pruebas en verde; staging con un token de otro emisor → 401 |
| Compatibilidad | Transparente para la app (el token es opaco para ella) |
| Riesgo residual | Riesgos de gestión de la llave de firma de tokens (el detalle se revisa por canal privado); se mitigan con su rotación y el procedimiento de incidentes |
| Responsable | **BE-A** |
| Dependencias | Aprobación de BE-A |
| Rollback | Volver a aceptar tokens sin claims (bandera de configuración) |

### C-12 Estrategia de revocación de sesiones (grupo B: **pendiente de BE-A**)

| Campo | Contenido |
|---|---|
| Requisito | RF-12. **Objetivo propuesto: ≤ 60 s** según el evento: logout (la familia de esa sesión), baja de cuenta (todas las sesiones) y reutilización confirmada (la familia afectada). **En esta spec no se implementa Redis ni `sessionVersion`**: solo se documenta y se decide |
| Alternativas e impacto en las sesiones existentes | (a) **TTL de acceso de 60 s**: cumple el objetivo sin estado nuevo, pero aumenta varias veces la frecuencia de refresh (más carga y más tráfico móvil); las sesiones existentes no se cortan, solo hacen refresh más seguido. (b) **`sessionVersion` en la cuenta**, en el claim y comparado en cada petición: revocación inmediata por cuenta; una lectura de MongoDB por `_id` por petición; los tokens existentes sin el claim necesitan una **ventana de transición igual a la vida del token de acceso**, o cierran sesión una vez. (c) (b) más una **caché en Redis** con TTL ≤ 60 s: cumple el objetivo con menos lecturas a MongoDB; si Redis cae, se lee de MongoDB, así que no se crea un punto único de fallo nuevo; agrega complejidad. (d) **Lista de `jti` revocados** en Redis o MongoDB con TTL igual a la vida del token: precisa por sesión, pero cada logout escribe y cada petición lee |
| Recomendación (no aprobada) | (b) con transición; (c) solo si la medición de p95 lo justifica. Redis **no** se necesita para el JWT |
| Archivos (cuando se apruebe) | `internal/auth/{domain, usecase, adapter/mongo, adapter/http/middleware.go}` y sus pruebas |
| Amenaza | Uso de una sesión después de revocada |
| Pruebas | Un token emitido antes de un logout, una baja o una reutilización recibe 401 en ≤ 60 s; concurrencia de refresh (ya existe); sin Redis la API sigue funcionando; p95 sin regresión |
| Evidencia | Pruebas en verde; k6 antes y después |
| Compatibilidad | La app ya hace refresh cuando recibe 401: con (a), más refresh; con (b) o (c), sin cambio visible |
| Riesgo residual | (a) más carga; (b) y (c) más lecturas; ventana de hasta 60 s en (a) y (c) |
| Responsable | **BE-A** |
| Dependencias | Que BE-A apruebe el objetivo y la alternativa (B-3) |
| Rollback | Bandera de configuración que vuelve al comportamiento actual |

### C-13 Segregación staging y producción (grupo B: equipo y Azure)

| Campo | Contenido |
|---|---|
| Requisito | RF-13. **Sin costo:** restringir el acceso a las herramientas de prueba de staging, mantener secretos y redes separados por entorno y aplicar cuotas de recursos que protejan a producción. **Con costo:** infraestructura separada para staging (estimación que se revisa por canal privado). La topología actual se revisa por canal privado |
| Archivos | Documental (`docs/seguridad/segregacion.md`, de circulación interna); si se aprueba, reglas de red en Azure |
| Amenaza | Que un compromiso o el agotamiento de recursos de un entorno afecte a otro |
| Pruebas | Acceso desde fuera de lo permitido rechazado; carga sobre staging sin efecto en el p95 de producción |
| Evidencia | `az network nsg rule show` (con autorización) y reporte de k6 |
| Compatibilidad | Las restricciones de acceso a staging se coordinan con MO-A y MO-B, que lo usan para las pruebas móviles |
| Riesgo residual | Disponibilidad sin redundancia mientras no se apruebe la opción con costo |
| Responsable | Equipo |
| Dependencias | Autorización de Azure (B-5) y de costo (B-6) |
| Rollback | Quitar la regla de red |

### C-14 Evaluación de Bicep y reverse proxy (grupo B)

| Campo | Contenido |
|---|---|
| Requisito | RF-14. **Bicep:** `az group export` (solo lectura) → `az bicep decompile` → `infra/main.bicep`, ajustado hasta que `az deployment group what-if` muestre **0 cambios**; siempre modo incremental; `@secure()`; en la CI, `what-if` con una identidad `Reader`. **Proxy:** Caddy en el mismo servidor (TLS ACME, HSTS, gzip, límites), con la API solo en la red interna y confianza en `X-Forwarded-*` **solo** desde la red del proxy (el límite de tasa debe usar la IP real del cliente detrás del proxy) |
| Archivos | `infra/main.bicep` e `infra/params/*.bicepparam` (nuevos), `docs/infra.md`; proxy: `deploy/caddy/Caddyfile`, Compose y una propuesta de enmienda en `docs/` |
| Amenaza | Deriva de configuración sin control; exposición directa de la API |
| Pruebas | `what-if` sin cambios (CA-13); con el proxy, regresión completa, *spoofing* de `X-Forwarded-For` ignorado y renovación del certificado |
| Evidencia | Salida de `what-if`; prueba de *spoofing* |
| Compatibilidad | Mismas URL y certificado público; la app no cambia |
| Riesgo residual | Un `apply` mal revisado puede recrear recursos (solo con `what-if` limpio); el proxy consume recursos del servidor |
| Responsable | Equipo |
| Dependencias | **Proxy: enmienda del Art. XIV.4 (B-1).** Bicep: lectura en Azure (B-5) |
| Rollback | Bicep: no aplicar. Proxy: volver a publicar la API directo |

### C-15 Corrección de la prueba CRLF (grupo A)

| Campo | Contenido |
|---|---|
| Requisito | RF-15: la expresión de `TestIntegrationNoProviderKeyInRepo` excluye `\r` del valor (`[^ \t\r$"'{]+`) |
| Archivos | `internal/suggest/integration_test.go` |
| Amenaza | Ninguna (falso positivo en Windows) |
| Pruebas | Pasa en Windows y en Linux; sigue detectando una clave sembrada (prueba manual, como en la 014) |
| Evidencia | Corrida local en Windows y la CI |
| Compatibilidad | Ninguna |
| Riesgo residual | Ninguno |
| Responsable | BE-B (la prueba es de la 014, de BE-A: avisar) |
| Dependencias | Ninguna |
| Rollback | Revertir |

## 3. Dependencias nuevas

- **Go:** ninguna.
- **CI:** acciones de Trivy (`aquasecurity/trivy-action`), cosign (`sigstore/cosign-installer`), Buildx (`docker/setup-buildx-action`) y ZAP (`zaproxy/action-api-scan`), todas **fijadas por SHA**.
- Se registran en `memoria.md` al instalarse (Art. VIII.1).

## 4. Estrategia de pruebas (regresión)

- `go test -race -tags=integration ./...` completo (hoy: 760 unitarias y 135 de integración).
- **Autenticación:** registro, inicio de sesión, refresh (incluido el concurrente), logout, reutilización, roles, cuenta suspendida y baja.
- **Contrato:** el JSON es idéntico con y sin gzip.
- **HTTPS:** cabeceras y TLS 1.2+.
- **Despliegue:** verificación negativa de la firma; vuelta atrás.
- **Carga:** k6 antes y después.
- **Móvil:** Expo Go (con autorización).

## 5. Verificación de constitución

| Art. | Cumple |
|---|---|
| I | Esta spec se aprueba antes de implementar; el grupo B queda bloqueado hasta su autorización |
| IV | Contrato intacto (RNF-01) |
| VI | Refuerza: cabeceras, eventos sin datos personales, cadena de suministro |
| VII | Cada control con pruebas y evidencia |
| VIII | Sin dependencias de Go nuevas |
| XII | Las partes de otros dueños (003, 012-T003, 014) requieren su visto bueno |
| XIII.1 | Todo lo del grupo A cuesta cero; lo que tiene costo queda en el grupo B |
| XIII.4 | Solo `devops-mcp` o la persona responsable toca Azure; aquí no se toca |
| XIV.4 | **El reverse proxy contradice «sin proxy»: requiere enmienda (C-14)** |
| XV | Rama `spec/SPEC-18` y PR revisados por otra persona |

## 6. Riesgos

| Riesgo | Mitigación |
|---|---|
| HSTS mal configurado | `max-age` corto al principio |
| La firma o la verificación bloquean despliegues legítimos | Activar como bloqueante solo después de una corrida positiva y una negativa en staging |
| CVE sin corrección bloquean | `--ignore-unfixed` y excepciones con vencimiento |
| Se comprime una ruta con secretos | Lista de exclusión y una prueba que recorre todas las rutas |
| Sesiones cerradas por C-11 | Ventana de transición |
