# Matriz de controles de la spec 018 (seguridad ALTA)

- **Fecha:** 2026-11-18 · **Rama:** `feat/SPEC-18-Fase-1`
- **Aprobaciones:**
  - spec 018 aprobada por el responsable (B-2);
  - visto bueno de BE-A para C-01, C-11 y C-12, comunicado por el responsable el 2026-11-18 («BE-A dio el visto bueno, implementa los 3»).
- **Regla (§6):** ALTO solo con todos los obligatorios **implementados y validados con evidencia**.

## Controles obligatorios (§6)

| Control | Implementación | Prueba y evidencia | Estado |
|---|---|---|---|
| **C-01** Cabeceras y HSTS (012-T003) | `middleware.SecurityHeaders` en el router: `nosniff`, `X-Frame-Options: DENY`, `Referrer-Policy: no-referrer`, CSP `default-src 'none'; frame-ancestors 'none'` (`/docs` pone la suya), `no-store` en respuestas autenticadas sin duplicar, HSTS `max-age=86400` **solo en staging** con TLS (sin subdominios ni *preload*; nunca en producción, B-8). Además, un cuerpo de más de 1 MiB responde «demasiado grande» en los 15 módulos (`respond.InvalidBody`) | `security_headers_test.go` (en 200, 404 y 500; HSTS solo con TLS; sin duplicados); 966 pruebas de integración y 69/69 operaciones validadas contra el contrato con las cabeceras puestas | **Implementado y validado en local.** Falta la evidencia `curl -sSI` en staging tras desplegar (CA-01) |
| **C-02** Eventos | `ratelimit.blocked`, `auth.login_failed`, `auth.refresh_reuse`, `auth.family_revoked`, `auth.revoked_token_used`, `auth.revocation_unavailable` y `authz.forbidden`, todos con el formato común (`middleware.LogSecurityEvent`): evento, resultado, severidad, categoría, `requestId`, sujeto seudónimo y hora | Pruebas unitarias y de punta a punta. Ningún registro contiene correo, contraseña, token, `Bearer` ni IP completa | **Implementado y validado** |
| **C-03** Escaneo de la imagen | Trivy en la CI (016) y **por digest antes de desplegar**. Imágenes base por digest. Dependabot docker. Excepciones con CVE, dueño y vencimiento (`.trivyignore.yaml`, validado en la CI) | CI de la 016 en verde: 0 críticas o altas con corrección. Pruebas del validador de excepciones | **Implementado.** Validado en la CI; el escaneo por digest falta en su primera corrida de `deploy-staging` |
| **C-04** SBOM | CycloneDX por digest, adjunto como **atestado firmado** (`cosign attest`) y como artefacto | — | **Implementado sin validar** (primera corrida de `deploy-staging`) |
| **C-05** Firma | `cosign sign` sin llaves (OIDC del workflow) por digest | — | **Implementado sin validar** (Actions) |
| **C-06** Verificación obligatoria | `cosign verify` y `verify-attestation` con identidad restringida a este repositorio y workflow, antes de tocar la VM. Pruebas negativas: otra identidad y una imagen sin nuestra firma se rechazan. Una falla emite la alerta A6 y no despliega | — | **Implementado sin validar** (Actions) |
| **C-07** Despliegue por digest | La VM descarga `imagen@sha256:…` | — | **Implementado sin validar** (Actions) |
| **C-09** DAST activo aislado | `zap-isolated.yml`: API efímera, MongoDB y Redis en red `--internal` (sin salida a internet), ZAP 2.16.1 por digest y puerta que bloquea High, con prueba negativa | **Corrida real en local, 2026-11-18**, sobre el código de esta rama: 106 reglas en PASS, 0 advertencias, **0 High y 0 Medium**; 85 operaciones importadas, 213 URL y 13 906 peticiones. Solo notas informativas (4xx esperados y detección de rutas de sesión). El informe crudo no se versiona (lleva datos aleatorios del escaneo); se resume aquí | **Implementado y validado** (local). La corrida en Actions se dispara con la PR |
| **C-10** Alertas | Seis alertas con umbrales, evaluador y pruebas de activación (`docs/seguridad/respuesta-incidentes.md`) | `tests/security-controls.test.mjs`, 10/10 | **Implementado y probado. Destinatarios: pendientes del equipo** |
| **C-11** Emisor y audiencia | Todo token lleva `iss=vaiven-api` y `aud=vaiven-mobile`; `Parse` exige ambos (y `jti` e `iat`). Los tokens emitidos antes del despliegue se rechazan y la app renueva con su refresh | `jwt_issuer_test.go`: sin `iss`, sin `aud`, con otro emisor o con otra audiencia → rechazado | **Implementado y validado** |

## Controles que suman (no obligatorios)

| Control | Estado |
|---|---|
| **C-12** Revocación ≤ 60 s | **Implementado y validado.** El token lleva `sid` (familia). Logout y reutilización revocan la sesión, y la baja revoca todo el usuario, en Redis. Con dos réplicas reales, el logout en una se ve en la otra en **8 ms**. Con Redis caído, la ruta protegida responde **503 en 500 ms** (falla cerrada) y las públicas siguen funcionando. El contrato documenta el 503 en las 51 operaciones protegidas que no lo tenían (0.12.0, compatible) |
| **C-08** Compresión | Implementada, apagada por omisión (coordinar con móvil). 69/69 operaciones con gzip y 0 incumplimientos; −77 % de bytes |
| **C-13** Segregación y **C-14** Bicep y proxy | Balanceador local demostrado; Azure Load Balancer solo diseñado (Bicep), sin desplegar: requiere la enmienda y la aprobación del gasto |
| **C-15** CRLF | Corregido (016) |

## Dictamen

**Todos los controles obligatorios están implementados.** C-01, C-02, C-09 y C-11 están validados con evidencia local.

**Todavía no se declara ALTO**, porque §6 exige evidencia que solo existe al desplegar. Faltan:
1. **Primera corrida de `deploy-staging`** tras fusionar en `develop`: firma, verificación, atestado del SBOM, Trivy por digest y despliegue por digest (C-03 a C-07).
2. **`curl -sSI` en staging** con las cabeceras y HSTS (C-01, CA-01).
3. **Destinatarios de las alertas** (C-10), decisión del equipo.

Con esos tres puntos cumplidos, el dictamen pasa a **ALTO** sin cambios de código.

## Aviso a móvil (T014)

- Tras el despliegue, los tokens de acceso anteriores se rechazan con 401: la app debe renovar con su refresh, que sigue valiendo.
- Las rutas protegidas pueden responder 503 si el almacén de revocaciones no responde: reintentar.
- gzip sigue apagado hasta coordinarlo.

## Evidencia en staging (2026-11-19)

Tras fusionar la PR #107 en `develop` (`b819278`):

| Control | Evidencia | Estado |
|---|---|---|
| C-03 a C-07 | Ejecución de `deploy-staging` [44464389454](https://github.com/VaivenEnterprise/VaivenApplication-Backend/actions/runs/44464389454), toda en verde: digest, Trivy por digest, SBOM CycloneDX, firma sin llaves con su atestado, **verificación de firma, identidad y atestado antes de desplegar** (la alerta A6 no se disparó), despliegue por digest y humo por HTTPS. Staging responde `version b819278` | **Implementado y validado** |
| C-03 (puerta) | La PR de Dependabot que subía `golang` a 1.27.3 quedó **bloqueada por Trivy** (CVE-2026-97054, HIGH, con corrección en 1.26.11 y 1.27.4) | **Validado con un caso real** |
| C-01 | `curl -sSD -` contra `/api/v1/meta` en staging (200 y 405): `X-Content-Type-Options: nosniff`, `X-Frame-Options: DENY`, `Referrer-Policy: no-referrer`, `Content-Security-Policy: default-src 'none'; frame-ancestors 'none'`, `Strict-Transport-Security: max-age=86400` (sin subdominios ni preload) | **Implementado y validado** |

**Dictamen actualizado:** 9 de los 10 controles obligatorios están implementados y validados con evidencia. Solo falta **C-10**: asignar destinatarios a las alertas (decisión del equipo). Con eso, el nivel es **ALTO**.

## C-10: entrega de alertas (2026-11-19)

| Pieza | Estado |
|---|---|
| Detección de A1 a A4 | `security-alerts.yml` cada 30 min: cuenta señales en la VM de staging con OIDC, en solo lectura y sin copiar registros (`scripts/alert-scan.sh`) |
| Detección de A5 y A6 | `deploy-staging.yml`, en cuanto falla el escaneo por digest o la verificación de firma |
| Entrega | `scripts/notify-alert.sh`: issue `security-alert` asignado a `ALERT_ASSIGNEES` (GitHub avisa por correo y móvil) y webhook opcional (`ALERT_WEBHOOK_URL`). Sin destinatarios sale con «C-10 no operativo» |
| Vigilancia de la vigilancia | Latido de Better Stack si existe `BETTERSTACK_HEARTBEAT_URL` (Terraform en `infra/terraform/observability`) |
| Pruebas | 18/18 en Node: activación de las seis alertas, sin destinatarios → no operativo, issue asignado a cada destinatario, rechazo de valores que no son usuarios y cuerpo sin datos sensibles |
| **Destinatarios** | **Pendientes:** el responsable los entrega; un administrador fija `ALERT_ASSIGNEES` |
| **Recepción real** | **Pendiente:** ejecutar `Security alerts` con `synthetic=A1…A6` y confirmar que cada destinatario recibe el aviso |

**C-10 sigue sin acreditar** hasta tener los destinatarios y la prueba de recepción real. El dictamen sigue en 9/10.

### C-10: actualización (2026-11-19)

- **Destinatario autorizado por el responsable:** `JosephYanez27` (titular; acepta el rol). Suplente: **pendiente**. Canal: issue de GitHub asignado (correo y app de GitHub). Respuesta esperada: ≤ 15 min tras el aviso.
- **Privacidad (repositorio público):** el issue solo lleva el id, la severidad, el entorno y la hora; los conteos no salen en los registros públicos (`--quiet`). Opción de repositorio privado de operación (`ALERT_REPOSITORY` y `ALERT_REPO_TOKEN`).
- **Robustez:**
  - sin duplicados: una alerta repetida comenta el issue abierto;
  - tres reintentos ante fallas de GitHub;
  - sin destinatarios, falla con «C-10 no operativo».
- **Cierre de un incidente:** lo cierra a mano quien responde, siguiendo `respuesta-incidentes.md`. Una repetición posterior abre un issue nuevo.
- **Pruebas:** Node 21/21.
- **Para acreditar C-10 faltan:**
  1. publicar estos cambios (PR a `develop`), porque Actions solo ofrece «Run workflow» para flujos que están en la rama por defecto;
  2. fijar la variable `ALERT_ASSIGNEES=JosephYanez27`;
  3. ejecutar `Security alerts` con `synthetic=A1…A6` y enlazar los 6 issues;
  4. que el destinatario **confirme** que recibió los avisos.
