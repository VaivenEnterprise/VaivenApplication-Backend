# Tareas 018 — Hardening de seguridad (backend)

- **Dueño propuesto:** BE-B · **Estado:** aprobada el 2026-11-18 (B-2, Art. I) · `[P]` = paralelizable
- Las tareas del **grupo B** están **BLOQUEADAS** hasta que se cumpla su condición. Aprobar la spec **no** las desbloquea.
- Cada tarea con código termina con `make fmt lint test` y la integración del paquete en verde.

## Grupo A — Preparación local (tras aprobar la spec)

- [ ] **T001** [P] Corregir el falso positivo CRLF de la prueba de claves
  - Archivos: `internal/suggest/integration_test.go`
  - Agente: `qa-go` · Dueño: BE-B (avisar a BE-A, dueño de la 014) · Depende de: —
  - Aceptación: CA-10; pasa en Windows y en Linux; sigue detectando una clave sembrada
  - Refs: RF-15 · C-15

- [ ] **T002** Verificar las cabeceras y HSTS que implementa la 012-T003 (solo verificación)
  - Archivos: `docs/seguridad/evidencia-018.md` (evidencia); **no** edita código: la implementación es la **012-T003 de BE-A**
  - Agente: `qa-go` · Dueño: BE-B · Depende de: 012-T003 terminada; visto bueno de BE-A sobre los criterios complementarios
  - Aceptación: CA-01 con `curl -sSI` en staging (ruta pública, autenticada, 404 y error); checklist previa a HSTS documentada (nombres de host, certificado, terminación TLS, compatibilidad con staging, comportamiento HTTP/HTTPS); HSTS **sin** `includeSubDomains` ni `preload`, y solo en staging
  - Refs: RF-01 · C-01

- [ ] **T003** [P] Eventos de seguridad en el middleware y el límite de tasa
  - Archivos: `internal/adapter/http/middleware/ratelimit.go`, `internal/auth/adapter/http/middleware.go` y sus pruebas
  - Agente: `dev-go` · Dueño: BE-B (visto bueno de BE-A para `internal/auth`) · Depende de: —
  - Aceptación: eventos `ratelimit.blocked` y `authz.forbidden`; las pruebas comprueban la ausencia de token, correo e IP completa
  - Refs: RF-02 · C-02

- [ ] **T004** Eventos de seguridad de la sesión
  - Archivos: `internal/auth/usecase/{refresh.go, logout.go, login.go}` y sus pruebas
  - Agente: `dev-go` · Dueño: BE-A (o BE-B con su visto bueno) · Depende de: T003
  - Aceptación: CA-02; `auth.refresh_reuse`, `auth.family_revoked` y `auth.login_failed` sin datos sensibles; comportamiento idéntico (RNF-02)
  - Refs: RF-02 · C-02

- [ ] **T005** [P] Trivy: criterios de la 018 sobre la 016-T011 (solo la diferencia)
  - Archivos: si el equipo **amplía la 016-T011**, ninguno (la tarea se marca «cubierta por la 016-T011»); si no, `.github/workflows/deploy-staging.yml` (escaneo del digest antes de desplegar), `.trivyignore`, `.github/dependabot.yml` (`docker`), `Dockerfile` (base por digest)
  - Agente: `dev-go` · Dueño: BE-B · Depende de: 016-T011 y la decisión del equipo sobre su alcance
  - Aceptación: CA-03 con una prueba negativa controlada en una rama; acciones fijadas por SHA; `.trivyignore` con motivo, dueño y vencimiento; **sin** duplicar lo que hace la 016-T011
  - Refs: RF-03 · C-03

- [ ] **T006** Despliegue por digest en staging y verificación del SBOM
  - Archivos: `.github/workflows/deploy-staging.yml`, `deploy/remote-deploy.sh`; el SBOM lo genera la **016-T011** (aquí solo se verifica su asociación al digest)
  - Agente: `dev-go` · Dueño: BE-B · Depende de: 016-T011 (o T005)
  - Aceptación: CA-04 (SBOM consultable por digest) y CA-06 (el servidor corre el digest; la vuelta atrás por digest se prueba a propósito)
  - Refs: RF-04, RF-07 · C-04, C-07

- [ ] **T007** Firma sin llaves y verificación obligatoria en staging
  - Archivos: `.github/workflows/deploy-staging.yml`
  - Agente: `dev-go` · Dueño: BE-B · Depende de: T006
  - Aceptación: CA-05 (imagen sin firma o de otra identidad: no se despliega) y una corrida positiva; `id-token: write` solo donde hace falta
  - Refs: RF-05, RF-06 · C-05, C-06

- [ ] **T008** Compresión HTTP segura
  - Archivos: `internal/adapter/http/middleware/{compress.go, compress_test.go}`, `internal/adapter/http/router.go`
  - Agente: `dev-go` · Dueño: BE-B · Depende de: 012-T003 (BE-A edita el mismo `router.go`; se hace después, en orden)
  - Aceptación: CA-07; una prueba recorre todas las rutas de `api/openapi.yaml` y exige clasificarlas como comprimible o excluida; k6 dentro de RNF-05
  - Refs: RF-08 · C-08

- [ ] **T009** [P] DAST activo aislado en el runner (solo si la 016-T010 no lo cubre)
  - Archivos: si el equipo **amplía la 016-T010**, ninguno (tarea «cubierta por la 016-T010»); si no, `.github/workflows/zap.yml` (*job* aislado) y `.zap/rules.tsv`
  - Agente: `qa-go` · Dueño: BE-B · Depende de: 016-T010, la decisión del equipo sobre su alcance y T002
  - Aceptación: CA-08; sin acceso de red a staging ni a producción; reporte como artefacto; prueba negativa con una regla forzada
  - Refs: RF-09 · C-09

- [ ] **T010** Seis alertas, umbrales y procedimiento de incidentes
  - Archivos: `docs/seguridad/respuesta-incidentes.md`, `scripts/check-logs.sh`
  - Agente: `spec-writer` · Dueño: BE-B con BE-A · Depende de: T003, T004 (y C-03 y C-06 para A-5 y A-6)
  - Aceptación: CA-09: las alertas A-1 a A-6 del plan (C-10) con su prueba de activación registrada; los destinatarios **pendientes** no se dan por asignados; revisado por BE-A y BE-B
  - Refs: RF-10 · C-10

## Grupo B — Bloqueadas hasta coordinación o autorización

- [ ] **T011** ⛔ Emisor y audiencia del JWT con transición
  - Archivos: `internal/platform/security/jwt.go` y su prueba, `internal/config`, `cmd/api/main.go`
  - Desbloquea: **aprobación de BE-A** (spec 003) · Depende de: T004
  - Aceptación: CA-11; ventana igual a la vida del token de acceso; regresión de autenticación completa
  - Refs: RF-11 · C-11

- [ ] **T012** ⛔ Decisión de revocación de sesiones (≤ 60 s) — documento, sin implementar
  - Archivos: `docs/seguridad/decision-revocacion.md` (decisión firmada por BE-A); la implementación (Redis o `sessionVersion`) **no** es parte de esta spec y requiere una tarea nueva después de la decisión
  - Desbloquea: **BE-A** (B-3) · Depende de: T011
  - Aceptación: BE-A aprueba o ajusta el objetivo de ≤ 60 s por evento y elige una de las alternativas (a) a (d) del plan, con su impacto en las sesiones existentes
  - Refs: RF-12 · C-12

- [ ] **T013** ⛔ DAST pasivo contra staging
  - Archivos: `.github/workflows/zap.yml` (*job* manual)
  - Desbloquea: **autorización específica del responsable** (B-4) · Depende de: T009
  - Aceptación: `zap-baseline` solo contra el host de staging, por `workflow_dispatch`; nunca producción; reporte publicado
  - Refs: RF-16 · C-09

- [ ] **T014** ⛔ Prueba móvil con compresión
  - Archivos: — (repositorio móvil, de MO-A y MO-B)
  - Desbloquea: **MO-A y MO-B** · Depende de: T008 desplegada en staging
  - Aceptación: las pantallas de lista cargan igual en Expo Go; sin errores de parseo
  - Refs: RF-18 · C-08

- [ ] **T015** ⛔ Análisis de segregación staging/producción
  - Archivos: `docs/seguridad/segregacion.md`
  - Desbloquea: **lectura en Azure (B-5)** para confirmar la configuración de red y los permisos; cualquier cambio, **equipo** (B-6)
  - Aceptación: estado real documentado en un documento de circulación interna; opción sin costo propuesta y coordinada con MO-A y MO-B
  - Refs: RF-13 · C-13

- [ ] **T016** ⛔ Bicep de la infraestructura existente (solo `what-if`)
  - Archivos: `infra/main.bicep`, `infra/params/*.bicepparam`, `docs/infra.md`
  - Desbloquea: **lectura en Azure (B-5)**; aplicar cambios requiere otra autorización
  - Aceptación: CA-13 (`what-if` sin cambios); ningún `apply`
  - Refs: RF-14 · C-14

- [ ] **T017** ⛔ Propuesta de enmienda del Art. XIV.4 y prototipo de reverse proxy
  - Archivos: `docs/enmienda-proxy-propuesta.md`; el prototipo, solo en local (`deploy/caddy/`)
  - Desbloquea: **decisión del equipo** (Art. XI) antes de cualquier despliegue
  - Aceptación: propuesta escrita con costo, consumo de recursos, riesgos y la prueba de *spoofing* de `X-Forwarded-For`
  - Refs: RF-14 · C-14

- [ ] **T018** ⛔ Alertas en Azure Monitor
  - Archivos: plantillas en `infra/` (si existe T016)
  - Desbloquea: **autorización de Azure** y del **equipo** si tiene costo
  - Aceptación: alerta probada con un evento simulado; límite diario de ingesta configurado
  - Refs: RF-17 · C-10

- [ ] **T019** ⛔ Verificación de firma en producción
  - Archivos: `scripts/deploy-production.sh`
  - Desbloquea: **autorización del responsable** y T007 validada en staging
  - Aceptación: producción solo promueve un digest con firma verificada; prueba negativa en Cloud Shell
  - Refs: RF-06 · C-06

## Cierre

- [ ] **T020** Revisión de conformidad y matriz de nivel de seguridad
  - Archivos: `docs/seguridad/matriz-nivel.md`, `memoria.md`
  - Agente: `revisor-constitucion` · Depende de: T001–T010 (y las del grupo B que se hayan desbloqueado)
  - Aceptación: matriz «Control | Estado inicial | Estado final | Evidencia | Riesgo residual» con evidencia enlazada; **se declara ALTO solo si se cumple el §6 de la spec**
  - Refs: spec §6

- [ ] **T021** Despliegue de la spec en staging
  - Archivos: `memoria.md`
  - Agente: `devops-mcp` · Depende de: T020
  - Aceptación: PR `spec/SPEC-18` → `develop` fusionado; `deploy-staging.yml` en verde **con verificación de firma**; CA-01, CA-05 y CA-06 comprobados en staging; `check-logs.sh` en 0; resultado en `memoria.md`
  - Refs: Art. XIII.5

| Total | 21 tareas (10 del grupo A, de las cuales T002 solo verifica y T005 y T009 pueden quedar cubiertas por la 016; 9 del grupo B; 2 de cierre) |
|---|---|
