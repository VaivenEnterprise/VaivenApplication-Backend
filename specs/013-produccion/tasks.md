# Tareas 013 — Entorno de producción (backend)

- **Dueño:** BE-B · **Estado:** sin ejecutar · `[P]` = paralelizable
- Las tareas «manual» son trámites en Render, Atlas, Cloudinary o el monitor; se registran en `docs/produccion.md` y `memoria.md` sin valores secretos.

## Fase 1 — Entorno (30 oct–1 nov)

- [ ] **T001** Servicio de producción en Render (**manual**)
  - Archivos: `config/prod.example.env`, `docs/produccion.md` (sección servicio)
  - Agente: `dev-go` (documentación) · Depende de: 002-T027
  - Aceptación: `vaiven-prod` creado por MCP (`create_web_service`, solo `devops-mcp`, Art. XIII.5); variables definidas (nombres en `prod.example.env`); `/healthz` y `/readyz` responden 200 (CA-01); la URL queda anotada para el móvil (mobile 013-T001)
  - Refs: RF-01 · P-01

- [ ] **T002** [P] Base de datos de producción en Atlas (**manual**)
  - Archivos: `docs/produccion.md` (sección base de datos)
  - Agente: `modelador-mongo` (documentación e índices) · Depende de: 002-T026
  - Aceptación: base `vaiven_prod`, usuario de permisos mínimos, contraseña distinta de staging (CA-02); `indexes` sin faltantes tras el primer arranque; riesgo de `0.0.0.0/0` anotado (P-02)
  - Refs: RF-02 · P-02

- [ ] **T003** [P] Carpeta de Cloudinary de producción (**manual**)
  - Archivos: `docs/produccion.md` (sección imágenes)
  - Agente: `dev-go` (documentación y prueba de firma) · Depende de: 004-T009
  - Aceptación: `CLOUDINARY_FOLDER=vaiven/production`; subida de prueba con firma real; secreto solo en Render
  - Refs: RF-03 · P-03

- [ ] **T004** [P] Monitor de disponibilidad (**manual**)
  - Archivos: `docs/produccion.md` (sección monitoreo y arranque en frío)
  - Agente: `dev-go` (documentación) · Depende de: T001
  - Aceptación: servicio gratuito de ping a `/healthz` cada 5–10 min; documentado cuál es y cómo pausarlo; CA-05
  - Refs: RF-06 · P-07

## Fase 2 — Operación (12–15 nov)

- [ ] **T005** Secretos, administrador y cuenta de revisión
  - Archivos: `docs/produccion.md` (sección secretos)
  - Agente: `dev-go` · Depende de: T001, 003-T012
  - Aceptación: `JWT_SECRET` y credenciales propios (CA-02); procedimiento de rotación escrito y probado en staging; `create-admin` ejecutado en producción; cuenta de revisión de Google Play creada con datos ficticios; **sin `seed`** (RF-05)
  - Refs: RF-04, RF-05

- [ ] **T006** Respaldo y restauración
  - Archivos: `scripts/backup.sh`, `scripts/restore.sh`, `docs/respaldos.md`
  - Agente: `dev-go` · Depende de: T002
  - Aceptación: respaldo cifrado de producción restaurado en staging, colecciones y conteos coinciden (CA-04); calendario semanal y previo a despliegues; nada de valores secretos en los scripts
  - Refs: RF-07 · P-05

- [ ] **T007** Prueba de humo
  - Archivos: `cmd/admin/smoke.go`, `cmd/admin/smoke_test.go`
  - Agente: `dev-go` · Depende de: 012-T002
  - Aceptación: `go run ./cmd/admin smoke --base-url <url>` registra, inicia sesión, publica y borra, y sale con 0 sin datos residuales (CA-03); pruebas con servidor simulado
  - Refs: RF-08 · P-04

- [ ] **T008** Guía de operación y revisión de registros
  - Archivos: `docs/produccion.md` (despliegue, reversión, logs, límites, Atlas M0 al 80 %)
  - Agente: `spec-writer` · Depende de: T004, T006, T007
  - Aceptación: otra persona del equipo ejecuta una reversión siguiendo la guía; revisión de los registros de una sesión completa sin datos personales (CA-06); límite de tasa verificado en producción
  - Refs: RF-09, RF-10, RF-11

- [ ] **T009** Lista de comprobación de producción y conformidad
  - Archivos: `docs/produccion.md` (lista firmada), `memoria.md` (solo anexar)
  - Agente: `revisor-constitucion` · Depende de: T008, 012-T011
  - Aceptación: lista completa (secretos, índices, sin seed, monitor, respaldo, humo, registros); veredicto **CONFORME**; hito H5 (producción) marcado
  - Refs: RNF-01, RNF-02

| Total | 9 tareas |
|---|---|
