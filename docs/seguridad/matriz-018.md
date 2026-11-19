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
