# Tareas 013 — Entorno de producción (backend)

- **Dueño:** BE-B · **Estado:** sin ejecutar (spec aprobada el 2026-11-11) · `[P]` = paralelizable
- Las tareas en la VM, Cloudinary o Better Stack las ejecuta `devops-mcp` o una persona, y se registran en `docs/produccion.md` y `memoria.md` sin valores secretos (Art. XIII.2 y XIII.4).
- **Requisitos previos (fase 0):** P-1 (rotar la llave de Cloudinary) y P-2 (rotar el `JWT_ACCESS_SECRET` de staging) de `docs/seguridad/remediacion-2026-11.md`, y la spec 012 desplegada en staging.

## Fase 1 — Entorno

- [ ] **T001** Entorno de producción en la VM, con su MongoDB y su Redis
  - Archivos: `deploy/.env.production.example`, `deploy/production/docker-compose.yml`, `deploy/docker-compose.yml` (red configurable, `VAIVEN_NETWORK`), `docs/produccion.md` (sección servicio y capacidad)
  - Agente: `devops-mcp` (infraestructura) y `dev-go` (archivos y documentación) · Depende de: 002-T031, fase 0
  - Aceptación: proyecto `vaiven-production` con API, MongoDB y Redis propios (volúmenes y red propios, sin puertos publicados); `scripts/deploy.sh production` (puerto 443) desde `origin/main`; HTTPS válido sin `-k` en 443 y 8443 (CA-07); `/healthz` y `/readyz` (`redis: ok`) en 200 y `/api/v1/meta` con `environment: production` (CA-01); **memoria medida** con los dos entornos encendidos y límites fijados con esa medición (RF-12, CA-08); URL anotada para el móvil (mobile 013-T001)
  - Refs: RF-01, RF-01b, RF-12 · P-01, P-02, P-08, P-10
  - Avance (2026-11-11): archivos listos y validados con `docker compose config` (staging sin cambios; producción en `vaiven-production-net` y puerto 443; MongoDB y Redis propios sin puertos). Se agregó `deploy/production/init-env.sh` (contraseñas y keyFile propios). **Falta (VM, `devops-mcp`):** fase 0 (P-1 y P-2), crear el entorno siguiendo `docs/produccion.md` §2, verificar CA-01 y CA-07 y medir la capacidad (CA-08).

- [ ] **T002** [P] Base de datos de producción
  - Archivos: `docs/produccion.md` (sección base de datos)
  - Agente: `modelador-mongo` (documentación) y `devops-mcp` · Depende de: T001
  - Aceptación: base `vaiven_prod` con usuario de permisos mínimos y contraseña distinta de staging (CA-02); `admin indexes` en producción con código 0
  - Refs: RF-02 · P-02

- [ ] **T003** [P] Carpeta de Cloudinary de producción
  - Archivos: `docs/produccion.md` (sección imágenes)
  - Agente: `dev-go` (documentación y prueba de firma) y `devops-mcp` · Depende de: T001, P-1
  - Aceptación: `CLOUDINARY_BASE_FOLDER=vaiven/production`; `admin check-services` en `OK`; una subida de prueba con firma real, borrada después; el secreto solo en el `.env` de producción
  - Refs: RF-03 · P-03

- [ ] **T004** [P] Monitor, reinicio automático y rotación de registros
  - Archivos: `deploy/docker-compose.yml`, `deploy/base/docker-compose.yml` (rotación de registros), `docs/produccion.md` (sección monitoreo y disco)
  - Agente: `dev-go` (Compose y documentación) y `devops-mcp` · Depende de: T001
  - Aceptación: Better Stack revisa `/healthz` de producción cada 3 min y avisa por correo; `restart: unless-stopped` en todos los contenedores y Docker habilitado al arranque; rotación de registros en los dos entornos (P-09); tras reiniciar la VM ambos entornos vuelven solos (CA-05)
  - Refs: RF-06, RF-13 · P-07, P-09
  - Avance (2026-11-11): rotación de registros agregada a la API y al stack base (y al de producción). **Falta:** recrear MongoDB y Redis de staging para aplicarla (`docs/produccion.md` §3), agregar en Better Stack el monitor de producción (el de staging ya está dado de alta desde 2026-11-11, `docs/produccion.md` §4) y probar el reinicio de la VM (CA-05).

## Fase 2 — Operación (19–22 nov)

- [ ] **T005** Secretos, administrador y cuenta de revisión
  - Archivos: `docs/produccion.md` (sección secretos)
  - Agente: `dev-go` y `devops-mcp` · Depende de: T001, 003-T012
  - Aceptación: secretos nuevos y distintos de staging (CA-02); procedimiento de rotación escrito y probado en staging; `create-admin` en producción; cuenta de revisión de Google Play con datos ficticios; **sin `seed`** (RF-05)
  - Refs: RF-04, RF-05

- [ ] **T006** Respaldo y restauración
  - Archivos: `scripts/backup.sh`, `scripts/restore.sh`, `docs/respaldos.md`
  - Agente: `dev-go` · Depende de: T002
  - Aceptación: respaldo cifrado de producción restaurado en staging con colecciones y conteos iguales (CA-04); calendario semanal y antes de cada despliegue; ningún valor secreto en los scripts
  - Refs: RF-07 · P-05

- [ ] **T007** Prueba de humo que escribe datos
  - Archivos: `cmd/admin/smoke.go`, `cmd/admin/smoke_test.go`
  - Agente: `dev-go` · Depende de: — (usa la API pública ya existente)
  - Aceptación: `go run ./cmd/admin smoke --base-url <staging>` registra, inicia sesión, publica y borra, y sale con 0 limpiando lo creado; se niega a correr contra producción; pruebas con servidor simulado; en producción se usa `scripts/smoke.sh` (CA-03)
  - Refs: RF-08 · P-04

- [ ] **T008** Guía de operación y revisión de registros
  - Archivos: `docs/produccion.md` (despliegue, reversión, registros, memoria y disco, reinicio, rotación de secretos, vencimiento del crédito)
  - Agente: `spec-writer` · Depende de: T004, T006, T007
  - Aceptación: otra persona del equipo ejecuta una reversión (`deploy.sh production --rollback`) siguiendo la guía; registros de una sesión completa sin datos personales (CA-06); límite de tasa en Redis verificado en producción (RF-11)
  - Refs: RF-09, RF-10, RF-11

- [ ] **T009** Lista de comprobación de producción y conformidad
  - Archivos: `docs/produccion.md` (lista firmada), `memoria.md` (solo anexar)
  - Agente: `revisor-constitucion` · Depende de: T008, 012-T011
  - Aceptación: lista completa (secretos, índices, sin seed, monitor, respaldo, humo, registros, HTTPS, capacidad, disco); p95 < 800 ms en producción (RNF-01); costo adicional cero en *Cost Management* (RNF-02); veredicto **CONFORME**; hito H5 (producción) marcado
  - Refs: RNF-01, RNF-02

## Trazabilidad
| Requisito | Tarea |
|---|---|
| RF-01, RF-01b, RF-12 | T001 |
| RF-02 | T002 |
| RF-03 | T003 |
| RF-06, RF-13 | T004 |
| RF-04, RF-05 | T005 |
| RF-07 | T006 |
| RF-08 | T007 |
| RF-09, RF-10, RF-11 | T008 |
| RNF-01, RNF-02 | T009 |

| Total | 9 tareas |
|---|---|
