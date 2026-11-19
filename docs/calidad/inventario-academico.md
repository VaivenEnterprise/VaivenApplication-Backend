# Inventario académico y matriz de controles de seguridad

- **Fecha:** 2026-11-17 · **Base:** `develop` en `8b96890`, mismo código que `a1d447a` (spec 016, PR #104 y #105)
- **Alcance:** documental. La spec 018 está en **PROPUESTA, no aprobada** (Art. I), así que este documento **no implementa** ninguna de sus tareas.
- **Reglas:**
  - cada dato cita su evidencia;
  - lo que no se midió dice «no medido»;
  - no incluye el diagnóstico interno de hardening ni detalles operativos de debilidades abiertas.

## 1. Evidencia usada

| ID | Qué | Dónde | Fecha |
|---|---|---|---|
| E1 | `go test -count=1 -json ./...`: 779 aprobadas, 0 fallidas, 2 omitidas | Local (Windows, Go 1.26.11) | 2026-11-17 |
| E2 | `go test -tags=integration -json ./...` (Podman, `mongo:7`): 914 aprobadas en total, de ellas 135 solo de integración | Local | 2026-11-17 |
| E3 | Contrato: 69/69 operaciones ejercidas y 0 incumplimientos (`internal/adapter/http/contract`) | Local, dentro de E2 | 2026-11-17 |
| E4 | CI de la PR de la 016: Quality, Secrets (Gitleaks), CodeQL, **Trivy sin críticas ni altas con corrección** y SBOM CycloneDX como artefacto (`sbom-cyclonedx`, 30 días) | [Actions, run 44236290100](https://github.com/VaivenEnterprise/VaivenApplication-Backend/actions/runs/44236290100) | 2026-11-18 UTC |
| E5 | `govulncheck ./...`: 0; Gitleaks del árbol de trabajo: 0; `golangci-lint`: 0 | Local | 2026-11-17 |
| E6 | Rutas: comparación estática de los registros de chi con `openapi.yaml` (método y ruta) | Local, script de comparación | 2026-11-17 |
| E7 | Despliegues de staging en verde por `deploy-staging.yml` (último: `fa686a5`) | Actions | 2026-11-18 UTC |

Las 2 pruebas omitidas no ocultan fallas:
- `TestCoverageReport` solo corre si se le da un archivo de cobertura;
- `TestConnectAndPingReal` necesita un MongoDB externo, y su equivalente con Testcontainers corre en E2.

## 2. Inventario

| Pregunta | Respuesta | Evidencia |
|---|---|---|
| Nube | Microsoft Azure, cuenta gratuita: una VM con Docker Compose | `AGENTS.md` §5b; Art. XIII |
| ¿Despliegue en la nube? | Sí. Staging automático al fusionar en `develop` (`deploy-staging.yml`); producción manual (`scripts/deploy-production.sh`) en la misma VM, con proyecto de Compose aparte | E7; spec 013 |
| Servicios externos | Cloudinary (imágenes), Groq, OpenRouter y Google Generative Language (IA), Expo Push (avisos), Let's Encrypt (certificados), GitHub y GHCR (código, CI e imágenes) | URLs en `internal/**`; `docs/servicios.md` |
| Nivel de seguridad | **MEDIO-ALTO** (sección 4) | Matriz de la sección 3 |
| Vulnerabilidades críticas | **0** conocidas: Trivy sin críticas ni altas con corrección; `govulncheck` 0 | E4, E5 |
| Herramientas de CI | GitHub Actions: Policy (ramas y PR), Quality (gofmt, vet, pruebas con `-race`, integración, cobertura ≥ 80 %, bundle OpenAPI, golangci-lint, govulncheck), Secrets (Gitleaks y escáner de archivos), CodeQL, Trivy y SBOM, Smoke, Load (k6), Mutation (gremlins), ZAP baseline (manual, staging) | `.github/workflows/*.yml` |
| Herramientas de CD | `deploy-staging.yml` (OIDC a Azure, imagen en GHCR, `az vm run-command`) y scripts de producción manuales | `.github/workflows/deploy-staging.yml`, `scripts/` |
| Endpoints | **69** operaciones de la API (método y ruta) en OpenAPI; chi registra **72** (las 69 más 3 de Swagger UI en `/docs`) | E6 |
| Endpoints sin spec | **0** de la API. Las 3 de `/docs` son de documentación, no están en OpenAPI y no se montan en producción | E6; `internal/adapter/http/docs/docs.go` |
| Agentes | **7** propios (`spec-writer`, `dev-go`, `modelador-mongo`, `qa-go`, `gestor-repo`, `devops-mcp`, `revisor-constitucion`) más 4 nativos de OpenCode (`plan`, `build`, `explore`, `general`) | `.opencode/agents/`; `AGENTS.md` §4 |
| Skills | **5** (`sdd-workflow`, `go-clean-architecture`, `mongodb-go-patterns`, `api-rest-conventions`, `mcp-deploy`) | `.opencode/skills/` |
| Memoria del proyecto | `memoria.md`, solo anexar, con etiquetas `[BE-A]` y `[BE-B]` | `memoria.md` |
| MCP | **4** servidores: `azure`, `cloudinary-assets`, `cloudinary-config` y `github` | `opencode.json` |
| Pruebas unitarias | 779 aprobadas, 0 fallidas, 2 omitidas; `domain` y `usecase` ≥ 91,2 % de cobertura | E1 |
| Pruebas de integración | 135 aprobadas, 0 fallidas, 0 omitidas (MongoDB real) | E2 |
| Pruebas de seguridad | 135 casos con foco de seguridad, contados por paquete (`platform/security`, `adapter/http/middleware`, `auth/adapter/http`) o por nombre (401, 403, roles, cuenta activa, secretos, tokens, límite de tasa, privacidad). Es un **subconjunto** de E1 y E2, no se suma aparte. Además (herramientas, no casos): Gitleaks, govulncheck, CodeQL, gosec y Trivy | E1, E2, E4, E5 |
| Endpoints sin prueba de integración | **0** de 69 | E3 |
| Endpoints sin prueba unitaria o de seguridad por operación | **No medido** por operación. Cada módulo tiene pruebas de handler con `httptest` y de «ruta sin sesión → 401» o «sin rol → 403», pero no hay un conteo por operación como el de E3 | — |
| Pruebas adicionales | Contrato (E3), mutación (gremlins; ampliada a 32 paquetes, corrida pendiente), carga (k6; p95 de 13,2 ms y 6,7 ms en local), humo en staging (diaria), trazabilidad (156 de 287 IDs citados por una prueba) | `docs/calidad/pruebas.md`, `trazabilidad.md` |
| Réplicas del backend | **1** API por entorno | `deploy/*/docker-compose.yml`; Art. XIV |
| Caché | Redis por el puerto `cache.Cache`. Hoy lo usa el asistente de IA; hay respaldo en memoria | `internal/platform/redis/cache.go`, `cmd/api/main.go` |
| Redis | Obligatorio: límite de tasa (`ratelimit.Limiter`) y caché. Nunca es fuente de verdad; si cae, la API degrada sin caerse | Art. XIV; `/readyz` |
| Logout | `POST /api/v1/auth/logout` revoca la **familia** del refresh token; es idempotente | `internal/auth/usecase/logout.go` |
| Bases de datos | MongoDB 7 (replica set de un nodo por entorno) | `deploy/*/docker-compose.yml` |
| NoSQL | Sí: MongoDB, documental | — |
| Load balancer | **No hay.** La API sirve HTTPS directamente; un balanceador o proxy requiere enmienda constitucional | Art. XIV.4; spec 018 §7 |
| IaC | **Parcial:** Docker Compose y scripts versionados. Sin Bicep ni Terraform (propuesto en la 018, C-14, bloqueado) | `deploy/`, `scripts/` |
| TLS | HTTPS obligatorio servido por la API con certificado de Let's Encrypt | `internal/adapter/http/tlsserver.go`; `deploy/base/tls.md` |
| ¿API o microservicios? | API monolítica modular (Clean Architecture, un módulo por paquete), no microservicios | `cmd/api/modules.go` |
| CDN | Solo para imágenes: entrega de Cloudinary (`res.cloudinary.com`). La API no usa CDN | `internal/images` |
| Observabilidad | Registros JSON con `slog` y `X-Request-Id`; `/healthz` y `/readyz`; revisión con `scripts/check-logs.sh`; humo diario. Sin métricas ni alertas automáticas | `internal/adapter/http/middleware`; spec 018 C-10 |
| JWT y Redis | Token de acceso JWT HS256 sin estado. El refresh token y su familia viven en **MongoDB**, no en Redis | `internal/platform/security/jwt.go`; `auth/adapter/mongo` |
| Caching HTTP | Sin ETag. `Cache-Control: no-store` al emitir el QR | `internal/handover/adapter/http/handler.go` |
| Rate limiting | Global: 120 por minuto por IP, en Redis con respaldo local, responde 429 con `Retry-After`. Además hay límites por función (chat 30 por minuto, cuenta, IA) | `middleware/ratelimit.go`; specs 002, 008, 014 |
| Middleware | `RequestID → Logging → Recover → CORS → RateLimit → límite de cuerpo (1 MiB)`; por ruta: `Authenticate`, `RequireActive` y `RequireRole` | `internal/adapter/http/router.go` |
| Paginación | Por cursor opaco (`nextCursor`) en las listas | `api/paths/*.yaml` (10 archivos con cursor) |
| Índices | Declarados por módulo y verificados con `cmd/admin indexes`; el profiler no muestra `COLLSCAN` en las consultas frecuentes | `docs/rendimiento.md` |
| Serialización JSON | `encoding/json` de la biblioteca estándar, con `DisallowUnknownFields` donde el contrato pone `additionalProperties: false` | Handlers |
| Compresión del cuerpo | **No hay** (ni gzip ni brotli). Propuesta en la 018 (C-08), condicionada a 012-T003 y a la app móvil | Búsqueda sin resultados en `internal/` |
| Versionado | Por URL: `/api/v1`; versión del contrato en `info.version` (0.11.0) | `api/openapi.yaml` |

## 3. Matriz de controles (spec 018)

| Control | Estado inicial | Implementación | Prueba | Evidencia | Riesgo residual | Estado |
|---|---|---|---|---|---|---|
| C-01 Cabeceras y HSTS | `nosniff` solo en `/docs` | 012-T003 (BE-A), abierta | — | conformidad.md (DT-k) | Sin cabeceras de endurecimiento en la API | **Bloqueado** (BE-A) |
| C-02 Eventos de seguridad | No hay eventos tipificados | 018-T003 y T004 propuestas | — | — | Detección manual por registros | **Propuesto** (la 018 no está aprobada; T004 necesita a BE-A) |
| C-03 Escaneo de contenedores | No había | 016-T011: Trivy en la CI, falla con críticas o altas con corrección | Corrida real | E4 | Sin escaneo por digest antes de desplegar; imágenes base por etiqueta, no por digest; sin Dependabot para Docker | **Implementado y validado** (alcance de la 016). Lo que agrega la 018: **propuesto** |
| C-04 SBOM | No había | 016-T011: CycloneDX como artefacto | Corrida real | E4 | No está asociado al digest desplegado | **Implementado y validado** (016). Asociación al digest: **propuesto** |
| C-05 Firma | No hay | 018-T007 | — | — | Imagen no firmada | **Propuesto** |
| C-06 Verificación de firma | No hay | 018-T007 | — | — | Se puede desplegar una imagen sin firma | **Propuesto** |
| C-07 Despliegue por digest | Por etiqueta | 018-T006 | — | — | Etiqueta mutable | **Propuesto** |
| C-08 Compresión | No hay | 018-T008 | — | — | Ninguno de seguridad (es rendimiento) | **Bloqueado** (012-T003 y móvil) |
| C-09 DAST | No había | Pasivo: `zap.yml` contra staging (016-T010). Activo aislado: 018-T009 | — | `.github/workflows/zap.yml` | Sin análisis dinámico ejecutado | Pasivo: **implementado sin validar** (falta autorización para correrlo). Activo: **propuesto** |
| C-10 Alertas | No hay | 018-T010 | — | — | Incidentes detectados tarde | **Propuesto**. Destinatarios sin asignar: pendiente de operación |
| C-11 Emisor y audiencia del JWT | No hay | 018-T011 (BE-A) | — | — | Token aceptado sin `iss` ni `aud` | **Bloqueado** (BE-A) |
| C-12 Revocación ≤ 60 s | Revocación por familia de refresh; el acceso dura hasta vencer | 018-T012 (BE-A) | — | — | Ventana de un token de acceso robado | **Bloqueado** (BE-A) |
| C-13 Segregación de entornos | Staging y producción en la misma VM, con secretos y redes distintos | 018-T015 | — | spec 013 | Una sola VM | **Bloqueado** (equipo y costo) |
| C-14 Bicep y proxy | No hay | 018-T016 y T017 | — | — | Infraestructura sin IaC completa | **Bloqueado** (enmienda) |
| C-15 Falso positivo CRLF | La prueba fallaba en Windows | Corregida en la 016-T007 (la misma corrección que pide la 018-T001) | Windows: pasa sin clave y falla con una clave sembrada. Linux: integración en la CI | E2, E4 | — | **Implementado y validado** |
| Validación en la frontera (Art. VI.2) | Faltaba en 10 módulos | 016-T005 y T006 | Unitarias e integración | E1, E2 | — | **Implementado y validado** |
| Pruebas de contrato | No había | 016-T008 | 69/69 operaciones | E3 | — | **Implementado y validado** |

## 4. Dictamen

**MEDIO-ALTO.** No se puede declarar ALTO, porque la spec 018 §6 exige que estén implementados y validados:
- C-01 (bloqueado, BE-A);
- C-02 (propuesto);
- C-05, C-06 y C-07 (propuestos);
- C-09 en modo aislado (propuesto);
- C-10 (propuesto);
- C-11 (bloqueado, BE-A).

Lo que ya sube el nivel frente al diagnóstico del 2026-11-17:
- Trivy y SBOM en la CI, con corrida real en verde;
- validación en la frontera en todos los módulos;
- pruebas de contrato 69/69;
- portabilidad de la prueba de claves.
