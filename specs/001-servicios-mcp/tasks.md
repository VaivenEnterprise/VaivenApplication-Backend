# Tareas 001 — Infraestructura en Azure y servicios (backend)

- **Dueño:** BE-B · **Estado:** en curso (2026-11-04): T001–T006 y T008 cerradas; T007, T009 y T010 con pendientes anotados · `[P]` = paralelizable
- **Lo único humano (Art. XIII.2):** registrarse en Azure y Cloudinary, `az login`, definir `CLOUDINARY_URL` y aprobar las acciones. Todo lo demás lo hace `devops-mcp` desde OpenCode (primero MCP; `az` y SSH para lo que el MCP no cubra).
- Esta spec no tiene tarea «Despliegue»: todavía no hay API que desplegar (Art. XIII.5 aplica desde la 002).

- [x] **T001** Conexión de los MCP y sesión de `az` (**la hace una persona** con `devops-mcp`)
  - Archivos: `memoria.md` (solo anexar)
  - Agente: — (manual) y `devops-mcp` · Depende de: —
  - Aceptación: la cuenta Microsoft se crea con el correo de la cuenta principal del proyecto y el alta de Azure la hace un integrante del equipo que **no haya usado antes la oferta gratuita** (responsable: por definir; debe seguir en el equipo hasta el final); la suscripción queda a nombre de la cuenta del proyecto y `az login` se hace con esa cuenta, no con la personal. Cuenta de Azure creada con el correo de la cuenta principal del proyecto y cuenta de Cloudinary creada, `az login` hecho y `az account show` apunta a la suscripción gratuita; `CLOUDINARY_URL` definida en el sistema (únicos pasos manuales, anotados en `memoria.md`); `opencode mcp list` muestra `azure`, `cloudinary-assets` y `cloudinary-config` conectados (CA-01); `dev-go` rechaza una herramienta MCP y `devops-mcp` la ejecuta (CA-02)
  - Refs: RF-01, RF-02 · Art. XIII

- [x] **T002** [P] Cloudinary: verificar la nube y crear la carpeta base
  - Archivos: `memoria.md` (solo anexar)
  - Agente: `devops-mcp` · Depende de: T001
  - Aceptación: por MCP se confirma que la nube responde y se crea `vaiven/staging` (CA-06); el *API secret* no se imprime en ninguna salida
  - Refs: RF-06

- [x] **T003** Archivos de infraestructura: `bootstrap.sh` y stack base
  - Archivos: `deploy/bootstrap.sh`, `deploy/base/docker-compose.yml`, `deploy/base/tls.md`
  - Agente: `dev-go` · Depende de: T001
  - Aceptación: `bootstrap.sh` (idempotente) instala Docker, el plugin de Compose y `certbot`, crea 2 GB de swap, el usuario `deploy` y `/opt/vaiven`, y activa actualizaciones de seguridad; el Compose define `mongo` (replica set `rs0`, autenticación, volumen, sin puerto publicado) y `redis` (contraseña, `maxmemory`, sin puerto publicado) con límites de memoria; `tls.md` documenta la emisión y renovación con `certbot` y la comprobación; ningún secreto escrito (todo por `.env`)
  - Refs: RF-03, RF-04, RF-05 · P-03…P-06

- [x] **T004** Azure: región y talla, VM por MCP; IP estática y reglas
  - Archivos: `memoria.md` (solo anexar)
  - Agente: `devops-mcp` · Depende de: T001, T003
  - Aceptación: con `learn=true` se anota qué cubre el MCP de Azure; antes de crear nada se revisa con `az vm list-skus … --size Standard_B --all` qué regiones y tallas (B2ats_v2 → B1s → B2pts_v2) no tienen `Restrictions` (y, si es «Azure for Students», las regiones permitidas); el agente genera la llave SSH dedicada (`ssh-keygen`, la privada nunca se imprime), crea `rg-vaiven` y la VM con **`compute_vm_create`** (talla gratuita pasada explícitamente, Ubuntu 24.04 LTS, llave pública, **sin zonas de disponibilidad**, disco de SO Premium de hasta 64 GB; la talla por defecto del MCP, `Standard_DS1_v2`, **no** es gratuita); lo que el MCP no cubra —IP pública estándar estática con nombre DNS y reglas del NSG (80, 443 y 8443 abiertos; 22 solo desde la IP pública detectada con `curl`)— lo hace con `az`; cada creación con aprobación; se anotan región, talla y nombre DNS (sin secretos); alerta de presupuesto creada; el único cargo esperado es la IP pública
  - Refs: RF-00, RF-03 · RNF-02

- [x] **T005** Preparar y verificar la VM
  - Archivos: `memoria.md` (solo anexar)
  - Agente: `devops-mcp` · Depende de: T004
  - Aceptación: `deploy/bootstrap.sh` copiado y ejecutado por SSH; luego `docker --version`, `docker compose version`, swap de 2 GB activo, memoria y disco libres anotados; desde fuera, los puertos 27017 y 6379 no responden (CA-03)
  - Refs: RF-03

- [x] **T006** Levantar MongoDB y Redis del stack base
  - Archivos: `memoria.md` (solo anexar)
  - Agente: `devops-mcp` · Depende de: T005
  - Aceptación: se anota el consumo de memoria de Mongo y Redis (`docker stats`) y los créditos de CPU de la VM; `.env` de la VM generado en la propia VM con contraseñas fuertes (permisos 600, nunca en el repo ni en la conversación); `rs.status()` con el nodo en `PRIMARY`; usuario de aplicación con permisos solo sobre la base `vaiven_staging`; `redis-cli ping` = `PONG` autenticado (CA-04)
  - Refs: RF-04 · P-04, P-05

- [ ] **T007** HTTPS: certificado de Let's Encrypt
  - Archivos: `memoria.md` (solo anexar)
  - Agente: `devops-mcp` · Depende de: T006
  - Aceptación: `certbot --standalone` emite el certificado para el nombre DNS de la IP (puerto 80 abierto); `certbot renew --dry-run` termina bien y el temporizador de renovación está activo; una prueba temporal (`openssl s_server` en el puerto 8443, que luego se detiene) se valida desde fuera con `curl` sin `-k` y con `openssl s_client` (CA-05); las rutas de los archivos quedan anotadas para el Compose de la spec 002; la clave privada nunca se imprime
  - Refs: RF-05 · P-06 · Art. XIV.4
  - **Pendiente (2026-11-04):** todo cumplido salvo `certbot renew --dry-run`, que falla porque el entorno de pruebas de Let's Encrypt (`acme-staging-v02`) está en mantenimiento programado (letsencrypt.status.io). Cerrar en cuanto vuelva: `sudo certbot renew --dry-run` en la VM
  - **Requisitos de la renovación verificados (2026-11-04, solo lectura):** `renewal/<DNS>.conf` con `authenticator = standalone` y servidor de producción; `certbot.timer` activo y habilitado (próxima ejecución 2026-11-04 11:48 UTC); puerto 80 libre en la VM y abierto en el NSG; la VM llega al servidor de producción de Let's Encrypt (200) y no al de pruebas (000). Alternativa al dry-run, si el equipo la aprueba: `sudo certbot renew --cert-name <DNS> --force-renewal` (renovación real contra producción; 2.º certificado de los 5 por semana permitidos)

- [x] **T008** `docs/servicios.md` y `.env.example`
  - Archivos: `docs/servicios.md`, `.env.example`
  - Agente: `spec-writer` · Depende de: T002, T007
  - Aceptación: lista de servicios, región, talla, variables (sin valores reales), cómo se obtuvo y rota cada credencial, cómo recuperar el acceso SSH y riesgos conocidos (memoria justa, crédito de 30 días, secretos y contexto del modelo)
  - Refs: RF-07, RNF-01

- [ ] **T009** Repositorio en GitHub y MCP de GitHub (**persona** + `gestor-repo`)
  - Archivos: `.gitignore`, `memoria.md` (solo anexar)
  - Agente: — (la persona crea el repositorio, protege `main` y crea el token) + `gestor-repo` · Depende de: T001
  - Aceptación: repositorio `vaiven-backend` creado en GitHub con `main` protegida (PR obligatoria, sin push directo ni force-push); `GITHUB_PERSONAL_ACCESS_TOKEN` (permisos mínimos, limitado a este repositorio) definido como variable del sistema y OpenCode reiniciado; `opencode mcp list` muestra `github` conectado; un agente de desarrollo rechaza una herramienta de GitHub y `gestor-repo` la ejecuta; `.gitignore` excluye `.env`, `*.pem` y claves; `gestor-repo` abre una PR de prueba sin secretos y una persona la revisa y la fusiona
  - Refs: Art. XV
  - **Avance (2026-11-04):** `main` y `develop` protegidas; `.gitignore` cumple; `GITHUB_PERSONAL_ACCESS_TOKEN` creado (fine-grained) y `opencode mcp list` muestra `github` connected; en `opencode.json` solo `gestor-repo` tiene `github_*` (global: tools false y permission deny). **Falta:** (1) limitar el token solo a este repositorio (hoy ve también Landing-Page y Mobile; Art. XV.6); (2) ejecutar con OpenCode el rechazo de `dev-go` y la PR de prueba de `gestor-repo`, bloqueados porque la cuenta de OpenCode agotó el cupo gratuito de todos los modelos

- [ ] **T010** Revisión de conformidad
  - Archivos: ninguno (solo informe)
  - Agente: `revisor-constitucion` · Depende de: T008, T009
  - Aceptación: veredicto **CONFORME**; búsqueda de cadenas sospechosas sin hallazgos (CA-07); hito H0 (infraestructura) marcado
  - Refs: DoD
  - **Revisión del 2026-11-04: NO CONFORME (pendientes, sin hallazgos de seguridad).** CA-07 sin hallazgos (solo la suma SHA-256 pública de gitleaks en la CI); ningún archivo sensible versionado; Mongo y Redis sin puertos publicados; secretos solo en la VM; presupuesto `vaiven-5usd` creado. Bloquean el CONFORME: el dry-run de T007, el token de T009 y que el equipo acepte las desviaciones de los Art. X y XIII.4 (T005–T008 hechas con Claude Code porque OpenCode agotó su cupo gratuito). Hito H0 sin marcar
  - **Correcciones de la revisión (2026-11-04):** `az`, `ssh` y `scp` en deny para dev-go, modelador-mongo y qa-go (Art. XIII.4); `deploy/base/init-env.sh` y el procedimiento «Reconstruir desde cero» en `docs/servicios.md` (antes, el `.env` de la VM y su enlace no estaban en el repositorio); VM comprobada igual a `develop` (mismo compose, contenedores healthy, keyFile 400, certbot.timer activo). Propuesta de enmienda 1.6.0 en `docs/enmienda-1.6.0-propuesta.md` para las contradicciones de los Art. VII.4, XV.2 y XV.7, pendiente de aprobación

| Total | 10 tareas |
|---|---|
