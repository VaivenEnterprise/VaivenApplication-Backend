# Tareas 013 — Entorno de producción (backend)

- **Dueño:** BE-B · **Estado:** sin ejecutar (spec aprobada el 2026-11-11) · `[P]` = paralelizable
- Las tareas en la VM, Cloudinary o Better Stack las ejecuta `devops-mcp` o una persona, y se registran en `docs/produccion.md` y `memoria.md` sin valores secretos (Art. XIII.2 y XIII.4).
- **Requisitos previos (fase 0):** P-1 (rotar la llave de Cloudinary) y P-2 (rotar el `JWT_ACCESS_SECRET` de staging) de `docs/seguridad/remediacion-2026-11.md`, y la spec 012 desplegada en staging.

## Fase 1 — Entorno

- [x] **T001** Entorno de datos de producción en la VM: su MongoDB y su Redis
  - Archivos: `deploy/production/.env.example`, `deploy/production/docker-compose.yml`, `deploy/production/init-env.sh`, `deploy/production/provision-data.sh`, `deploy/docker-compose.yml` (red configurable, `VAIVEN_NETWORK`), `docs/produccion.md` (sección servicio y capacidad)
  - Agente: `devops-mcp` (infraestructura) y `dev-go` (archivos y documentación) · Depende de: 002-T031, fase 0
  - Aceptación: MongoDB y Redis de producción propios (proyecto `vaiven-production-data`, volumen y red propios, sin puertos publicados); replica set iniciado; usuario `vaiven_app` solo con `readWrite` en `vaiven_prod`; `/opt/vaiven/production/.env` (600) sin ningún secreto en común con staging (CA-02); memoria medida con los dos stacks de datos encendidos (RF-12)
  - Refs: RF-01, RF-01b, RF-12 · P-01, P-02, P-08, P-10
  - Ajuste (2026-11-12): el equipo decidió desplegar `main` y la API de producción **al final** (ver T010). Por eso T001 se limita al entorno de datos, y lo que exigía la API (paso 6, CA-01, CA-07, la medición con la API y la URL para el móvil) pasa a **T010**.
  - Hecho (2026-11-11): con `provision-data.sh` por Run Command y sin SSH (`docs/produccion.md` §2.0). MongoDB de producción `healthy` y primario del replica set `rs0`. Redis `healthy`. Usuario `vaiven_app` creado. `.env` con MongoDB, Redis y JWT propios, y llave de Cloudinary propia `vaiven-production-2026-11` (ping 200, distinta de la de staging). Memoria: 592 MB disponibles con los dos stacks de datos. Corrección en el camino: MongoDB 7 exige una caché de al menos 0,25 GB.

- [x] **T002** [P] Base de datos de producción
  - Archivos: `docs/produccion.md` (sección base de datos)
  - Agente: `modelador-mongo` (documentación) y `devops-mcp` · Depende de: T010
  - Aceptación: base `vaiven_prod` con usuario de permisos mínimos y contraseña distinta de staging (CA-02); `admin indexes` en producción con código 0
  - Refs: RF-02 · P-02
  - Hecho (2026-11-16): `vaiven_prod` con el usuario `vaiven_app` (`readWrite` solo en esa base) y contraseña propia (T001). `vm-admin.sh production indexes`: 34 correctos, 0 faltantes, 0 sobrantes y código 0.

- [x] **T003** [P] Carpeta de Cloudinary de producción
  - Archivos: `docs/produccion.md` (sección imágenes)
  - Agente: `dev-go` (documentación y prueba de firma) y `devops-mcp` · Depende de: T010, P-1
  - Aceptación: `CLOUDINARY_BASE_FOLDER=vaiven/production`; `admin check-services` en `OK`; una subida de prueba con firma real, borrada después; el secreto solo en el `.env` de producción
  - Refs: RF-03 · P-03
  - Hecho (2026-11-16): `CLOUDINARY_BASE_FOLDER=vaiven/production` y llave propia `vaiven-production-2026-11`, con el secreto solo en `/opt/vaiven/production/.env`. `check-services`: MongoDB, Redis y Cloudinary en OK. `upload-test`: subida con firma real en `vaiven/production`, verificación y borrado en OK.

- [x] **T004** [P] Monitor, reinicio automático y rotación de registros
  - Archivos: `deploy/docker-compose.yml`, `deploy/base/docker-compose.yml` (rotación de registros), `docs/produccion.md` (sección monitoreo y disco)
  - Agente: `dev-go` (Compose y documentación) y `devops-mcp` · Depende de: T001
  - Aceptación: Better Stack revisa `/healthz` de producción cada 3 min y avisa por correo; `restart: unless-stopped` en todos los contenedores y Docker habilitado al arranque; rotación de registros en los dos entornos (P-09); tras reiniciar la VM ambos entornos vuelven solos (CA-05)
  - Refs: RF-06, RF-13 · P-07, P-09
  - Avance (2026-11-11): rotación de registros agregada a la API y al stack base (y al de producción). **Falta:** recrear MongoDB y Redis de staging para aplicarla (`docs/produccion.md` §3), agregar en Better Stack el monitor de producción (el de staging ya está dado de alta desde 2026-11-11, `docs/produccion.md` §4) y probar el reinicio de la VM (CA-05).
  - Hecho (2026-11-16): rotación de registros aplicada a MongoDB y Redis de staging con `scripts/update-base-stack.sh` (solo diferían las líneas de `logging`; contenedores `healthy` con `max-size 10m` y `max-file 3`; staging `ready`). La API de staging ya la tenía desde su despliegue automático. Monitor de staging dado de alta. **Falta:** monitor de producción y prueba de reinicio de la VM (CA-05), después de desplegar la API de producción.
  - Hecho (2026-11-16): monitor de Better Stack de producción (`/healthz` en el 443, cada 3 min, correo) en **Up**, junto al de staging. Rotación de registros en los dos entornos. CA-05: tras `az vm restart`, producción y staging volvieron solos (`ready`, `redis: ok`, cada uno con su entorno y su versión).

## Fase 2 — Operación (19–22 nov)

- [ ] **T005** Secretos, administrador y cuenta de revisión
  - Archivos: `docs/produccion.md` (sección secretos)
  - Agente: `dev-go` y `devops-mcp` · Depende de: T001, 003-T012
  - Aceptación: secretos nuevos y distintos de staging (CA-02); procedimiento de rotación escrito y probado en staging; `create-admin` en producción; cuenta de revisión de Google Play con datos ficticios; **sin `seed`** (RF-05)
  - Refs: RF-04, RF-05
  - Avance (2026-11-16): secretos de producción nuevos y distintos de staging (CA-02: MongoDB, Redis, JWT y Cloudinary). Rotación escrita en `docs/produccion.md` §8 y probada en staging (P-1 y P-2, 2026-11-11). Cuenta de administrador creada en producción con `create-admin` por parámetros protegidos (la restauración de CA-04 muestra `users: 1`). Sin `seed` (se niega en producción). **Falta:** la cuenta de revisión de Google Play, con datos ficticios.

- [x] **T006** Respaldo y restauración
  - Archivos: `scripts/backup.sh`, `scripts/restore.sh`, `docs/respaldos.md`
  - Agente: `dev-go` · Depende de: T002
  - Aceptación: respaldo cifrado de producción restaurado en staging con colecciones y conteos iguales (CA-04); calendario semanal y antes de cada despliegue; ningún valor secreto en los scripts
  - Refs: RF-07 · P-05
  - Hecho (2026-11-16): `backup.sh production --offsite` (cifrado, sha256, manifiesto y copia en Cloudinary raw authenticated). `restore.sh` desde la **copia de fuera de la VM** en staging (`vaiven_restore_check`): 12 colecciones, 0 diferencias, base borrada al final (CA-04). Calendario: semanal y antes de cada despliegue (`docs/respaldos.md` §2). Los scripts no contienen ningún valor secreto.

- [x] **T007** Prueba de humo que escribe datos
  - Archivos: `cmd/admin/smoke.go`, `cmd/admin/smoke_test.go`
  - Agente: `dev-go` · Depende de: — (usa la API pública ya existente)
  - Aceptación: `go run ./cmd/admin smoke --base-url <staging>` registra, inicia sesión, publica y borra, y sale con 0 limpiando lo creado; se niega a correr contra producción; pruebas con servidor simulado; en producción se usa `scripts/smoke.sh` (CA-03)
  - Refs: RF-08 · P-04
  - Hecho (2026-11-16): `cmd/admin smoke` (7 pruebas con servidor simulado; se niega en producción). En staging, por `vm-admin.sh staging smoke`: registro, inicio de sesión, firma, subida, publicar, leer y borrar, más las 3 limpiezas en OK y código 0. En producción: `scripts/smoke.sh` 4 de 4 OK (CA-03).

- [ ] **T008** Guía de operación y revisión de registros
  - Archivos: `docs/produccion.md` (despliegue, reversión, registros, memoria y disco, reinicio, rotación de secretos, vencimiento del crédito)
  - Agente: `spec-writer` · Depende de: T004, T006, T007
  - Aceptación: otra persona del equipo ejecuta una reversión (`deploy.sh production --rollback`) siguiendo la guía; registros de una sesión completa sin datos personales (CA-06); límite de tasa en Redis verificado en producción (RF-11)
  - Refs: RF-09, RF-10, RF-11
  - Avance (2026-11-16): guía en `docs/produccion.md` §5 y §9. Registros de producción sin datos personales (CA-06): 0 apariciones de contraseñas, tokens, secretos o correos en 167 líneas. Límite de tasa en Redis verificado en producción (RF-11): 120 respuestas 200 y luego `429 rate_limited` con `Retry-After: 60`. **Falta:** que **otra persona del equipo** ejecute una reversión (`deploy-production.sh --rollback`) siguiendo la guía.

- [ ] **T009** Lista de comprobación de producción y conformidad
  - Archivos: `docs/produccion.md` (lista firmada), `memoria.md` (solo anexar)
  - Agente: `revisor-constitucion` · Depende de: T008, T010, 012-T011
  - Aceptación: lista completa (secretos, índices, sin seed, monitor, respaldo, humo, registros, HTTPS, capacidad, disco); p95 < 800 ms en producción (RNF-01); costo adicional cero en *Cost Management* (RNF-02); veredicto **CONFORME**; hito H5 (producción) marcado
  - Refs: RNF-01, RNF-02
  - Avance (2026-11-16): lista de `docs/produccion.md` §10 llena. p95 del servidor en producción 1,0 ms (`/readyz`, que consulta MongoDB y Redis, y `/meta`; medido dentro de la VM, RNF-01). **Falta:** *Cost Management* sin costo adicional (RNF-02, lo revisa una persona en el portal), T005 y T008 cerrados, 012-T011 (BE-A) y el veredicto **CONFORME**.

- [x] **T010** API de producción y paso a `main` (al final)
  - Archivos: `docs/produccion.md` (paso 6 y mediciones), `memoria.md` (solo anexar); y, si se aprueba, el mecanismo de despliegue de producción sin SSH (`deploy/remote-deploy.sh` o equivalente)
  - Agente: `devops-mcp` · Depende de: T001, PR `develop` → `main` (decisión del equipo: al terminar el desarrollo)
  - Aceptación: API de producción (proyecto `vaiven-production`) desplegada en el puerto 443 desde un commit de `origin/main`; HTTPS válido sin `-k` en 443 y 8443 (CA-07); `/healthz` y `/readyz` (`redis: ok`) en 200 y `/api/v1/meta` con `environment: production` (CA-01); memoria medida con los dos entornos completos y límites fijados con esa medición (RF-12, CA-08); URL anotada para el móvil (mobile 013-T001); la limpieza de imágenes de un entorno no borra la del otro
  - Refs: RF-01, RF-12 · CA-01, CA-07, CA-08
  - Hecho (2026-11-16): PR #82 `develop` → `main` (árbol idéntico). `deploy-production.sh 18b7dce` desde Cloud Shell: comprobó en GitHub que el commit está en `main` y desplegó la imagen ya probada en staging (`VAIVEN_DEPLOY_RESULT=deployed`). HTTPS válido sin `-k` en 443 y 8443 (Let's Encrypt, vence el 1 feb 2027, CA-07). `/readyz` `ready` con `redis: ok` y `/meta` `environment: production` (CA-01). `/docs` da 404 en producción. Memoria con los dos entornos (CA-08): 534 MB disponibles; límites sin cambios. URL para el móvil: `https://vaiven-staging.westus2.cloudapp.azure.com`. La limpieza de imágenes conserva las de cada entorno.

## Trazabilidad
| Requisito | Tarea |
|---|---|
| RF-01, RF-01b, RF-12 | T001 (datos), T010 (API) |
| RF-02 | T002 |
| RF-03 | T003 |
| RF-06, RF-13 | T004 |
| RF-04, RF-05 | T005 |
| RF-07 | T006 |
| RF-08 | T007 |
| RF-09, RF-10, RF-11 | T008 |
| RNF-01, RNF-02 | T009 |

| Total | 10 tareas |
|---|---|
