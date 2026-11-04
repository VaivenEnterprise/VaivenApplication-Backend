# Memoria del Backend — VAIVÉN

Memoria viva del proyecto. Los agentes la leen después de `constitution.md` y la **actualizan** al cerrar cada tarea relevante. No contradice la constitución: si hay conflicto, gana la constitución.

_Última actualización: 2026-10-28_

## 1. Estado actual

| Elemento | Estado |
|---|---|
| Constitución | v1.1.0 (enmendada 2026-10-28) |
| Spec 001 — Servicios con MCP | Redactada (reescrita el 2026-11-01), pendiente de aprobación humana |
| Spec 002 — Arquitectura base | Redactada, pendiente de aprobación humana |
| Specs 003–015 | Redactadas (spec, plan, tasks), pendientes de aprobación; ver `ROADMAP.md` |
| Tareas | **Sin ejecutar**; arranque planeado 2026-10-29 |
| Código | Ninguno todavía |

## 2. Decisiones (registro tipo ADR)

| # | Fecha | Decisión | Motivo | Constitución |
|---|---|---|---|---|
| D-001 | 2026-10-26 | Go + chi + Clean Architecture | Router ligero, cercano a `net/http`; capas claras para modelos con poco contexto | Art. III |
| D-002 | 2026-10-26 | MongoDB 7+ con driver oficial v2, replica set | Requisito del proyecto (NoSQL); replica set habilita transacciones futuras | Art. II |
| D-003 | 2026-10-26 | Dinero en centavos `int64`, sufijo `Cents` | Evitar errores de punto flotante; MXN único | Art. V |
| D-004 | 2026-10-26 | JSON y campos BSON en `camelCase`; colecciones `snake_case` plural | Un solo nombre por campo, sin mapeos intermedios | Art. IV, §6 |
| D-005 | 2026-10-26 | Errores con envoltorio único `{error:{code,message,details,requestId}}` | El móvil los normaliza en un solo lugar | Art. IV |
| D-006 | 2026-10-26 | Solo OpenCode + modelos gratuitos de Zen | Requisito del curso | Art. X |
| D-007 | 2026-10-26 | Documentos y prompts de agentes en español | Preferencia del equipo | Art. X |
| D-008 | 2026-10-26 | `task.md` se nombra `tasks.md` | Convención estándar de SDD | Art. I |
| D-009 | 2026-10-28 | Imágenes en Cloudinary (capa gratuita); MongoDB guarda solo referencias; sin SDK | Decisión del equipo: servicio gratuito y rápido | Art. II.4, VI.7 (enmienda 1.1.0) |
| D-010 | 2026-10-28 | Trabajo en paralelo: dueño por spec, contrato primero, módulos con una línea de registro, puertos compartidos | 4 personas con OpenCode propio sin pisarse | Art. XII (enmienda 1.1.0) |
| D-011 | 2026-10-28 | Autenticación con JWT de acceso + refresh rotativo, `argon2id` | Estándar para API móvil sin sesiones de servidor | Art. VI, VIII.4 |
| D-012 | 2026-10-28 | Alcance: núcleo completo sin IA ni página pública; código congelado 15-nov | Plazo 20-nov con colchón de 5 días | `ROADMAP.md` |

## 3. Modelos gratuitos asignados (OpenCode Zen)

Fuente de verdad: `opencode.json`. Verificados en septiembre de 2026; **los IDs gratuitos rotan**.

| Rol | Modelo | Agentes |
|---|---|---|
| Razonamiento / planeación | `opencode/nemotron-3-ultra-free` | `plan`, `spec-writer`, `modelador-mongo` |
| Revisión | `opencode/big-pickle` | `revisor-constitucion` (modelo por defecto) |
| Implementación | `opencode/nemotron-3-lightning-free` | `build`, `dev-go` |
| Pruebas / tareas rápidas | `opencode/ling-flash-free` | `qa-go`, `small_model` |
| Alternos | `opencode/mimo-free`, `opencode/muse-spark-free` | — |

**Cómo validar y cambiar:**
1. Ejecutar `opencode models` y confirmar que cada ID existe.
2. Editar solo `opencode.json` (sección `agent`, `model`, `small_model`).
3. Registrar el cambio en esta tabla y en la sección 2.

**Compatibilidad de versión:** los agentes usan el formato estable de OpenCode (`permission:` como mapa, carpeta `.opencode/agents/`). Si tu versión de OpenCode es la V2 y reporta campos obsoletos, cambia `permission:` por la lista `permissions:` (`action`, `resource`, `effect`) en cada agente.

## 4. Convenciones aprendidas

_(Los agentes agregan aquí hallazgos útiles y breves, uno por línea, con fecha.)_

- 2026-10-26 — Los modelos gratuitos rinden mejor con tareas de 1–3 archivos y criterios de aceptación explícitos.

## 5. Dependencias registradas

| Dependencia | Versión | Para qué | Spec |
|---|---|---|---|
| `github.com/go-chi/chi/v5` | última v5 | Router HTTP | 002 |
| `go.mongodb.org/mongo-driver/v2` | última v2 | Acceso a MongoDB | 002 |
| `github.com/go-playground/validator/v10` | última v10 | Validación de entrada | Primera spec funcional (no 002) |
| `github.com/stretchr/testify` | última | Aserciones en pruebas | 002 |
| `github.com/testcontainers/testcontainers-go` (+ módulo `mongodb`) | última | Pruebas de integración | 002 |

## 6. Pendientes y preguntas abiertas

- [ ] Aprobación humana de `specs/002-arquitectura-base/`.
- [ ] Definir el mecanismo de autenticación en la primera spec funcional.
- [x] Proveedor de MongoDB: contenedor en la VM de Azure (D-116).
- [ ] Política de conservación de chat y evidencias (según el documento de idea; se define en specs funcionales).

## 7. Riesgos conocidos

- Los modelos gratuitos pueden desaparecer o limitar el uso sin aviso → mantener alternos listados en la sección 3.
- Algunos modelos gratuitos pueden usar los datos enviados para mejorarse → **no pegar secretos ni datos personales reales** en las conversaciones con agentes.
- `make` no viene en Windows por defecto → usar Git Bash/WSL o los comandos equivalentes.

## 8. Decisiones de producto del 28 de oct (noche)

| # | Decisión | Efecto |
|---|---|---|
| D-101 | Políticas POL-01…17 **confirmadas**; POL-02 pasa a **máximo 7 días** | Specs 006, 007 (backend y móvil) actualizadas |
| D-102 | Categorías (POL-14): herramientas manuales, herramientas eléctricas, construcción y jardinería, movilidad (patines, patinetas, bicicletas), deportes, hogar, otros | Spec 005 |
| D-103 | Valor declarado **no** visible a terceros | Spec 005 |
| D-104 | **[REEMPLAZADA por D-116]** Hosting gratuito: Render (`vaiven-staging` y `vaiven-prod`) | Specs 001 y 013 |
| D-105 | Identificador de paquete `mx.vaiven.app` (permanente tras publicar) | Spec móvil 002 y 013 |
| D-106 | Meta original: primera versión pública en Google Play el 20 de nov (ver D-110). Solo Google Play; sin App Store | Specs 013; ROADMAP §1 |
| D-107 | Reportes de contenido y filtro de texto pasan a **P1 no recortables** (política de contenido de usuarios de Google Play) | Spec 011 |
| D-108 | Se agrega el proyecto `landing/` (sitio informativo estático, gratuito) con las páginas legales que exige Play | `vaiven-landing-page/`, ROADMAP |
| D-109 | La baja de cuenta **anonimiza** los datos personales (POL-17) | Spec backend 003 |
| D-110 | La cuenta de Google Play se compra el 29–30 oct: prueba cerrada 1–14 nov, solicitud 15 nov, publicación 1.0 (sin IA) meta 20 nov / estimada 22–25 nov. La IA va en la spec 014 como v1.1 posterior, detrás de un interruptor | ROADMAP §1 y §3; specs 013 |
| D-111 | **Entrega oficial: 1–4 dic con todos los specs completos.** Se agrega la spec 014 (IA: sugerir título y categoría por foto) como P1, v1.1, del 16 al 24 nov detrás del interruptor `AI_ENABLED`; proveedor gratuito y alcance académico por confirmar | ROADMAP §1 y §3; specs 014 |
| D-112 | ML = **recomendaciones** (spec 015): recomendador híbrido propio (coseno objeto-objeto + contenido) evaluado contra popularidad; IA = Gemini (spec 014). Ambas v1.1, 16–24 nov, con interruptores | ROADMAP §3; specs 014 y 015 |
| D-113 | **[REEMPLAZADA por D-115]** **Siete categorías de pruebas obligatorias** (unitarias, integración, contrato, base de datos, mutación, rendimiento, seguridad) y pipeline DevSecOps; constitución backend **v1.2.0** (Art. VII y VIII); spec 016. Equivalen a los proyectos de la solución de referencia `WebAPIDevSecOps` | Constitución; spec 016 |
| D-114 | Se separa el monorepo en repositorios independientes (`vaiven-backend`, `vaiven-frontend` y `vaiven-landing-page`): las rutas `../` pasan a ser locales o a apuntar al repositorio hermano; ROADMAP.md y docs/ se copian en cada repositorio; enmienda PATCH, sin cambios de principios; constitución backend **v1.2.1** | Constitución; AGENTS.md; specs |
| D-115 | Se elimina todo lo relativo a repositorio, ramas, PR y pipeline (incluida la spec 016 completa, que **reemplaza a D-113**: las siete categorías de pruebas y el pipeline DevSecOps quedan fuera hasta integrarlos de cero). Todo va por OpenCode y MCP: la spec **001 pasa a ser «Servicios por MCP»** (Atlas, Cloudinary, Render) y la arquitectura base es la 002; cada spec termina con una tarea «Despliegue» en staging (agente `devops-mcp`, único con permiso para los MCP) y la siguiente no empieza hasta que pase; constitución **v1.3.0** (Art. XIII). Render necesita un origen (repositorio o imagen): por definir | ROADMAP; constitution Art. XIII; specs 001 y 002; `.opencode/agents/devops-mcp.md` |
| D-116 | **Azure (o AWS) es obligatorio y el profesor mencionó Redis; el balanceo/nginx fue idea del equipo y es opcional** (corrige una atribución errónea previa). El backend (staging y producción) pasa a **una VM de Azure con cuenta gratuita** y Docker Compose: API → MongoDB (replica set de un nodo) y **Redis obligatorio** (límite de tasa y caché; nunca fuente de verdad); capa **opcional** `edge` (perfil de Compose) con nginx (TLS, `least_conn`) y `api2`, necesaria antes de la spec 013 por el HTTPS. **Reemplaza a D-104 y a lo de Atlas/Render de D-115.** Se mantienen Cloudinary y la landing en Render. Despliegue sin repositorio (`docker save \| ssh`; con `edge`, reinicio gradual `api1` → `api2`); agente `devops-mcp` con MCP de Azure (solo lectura) + `az` + Cloudinary; staging en 8443 y producción en 443 de la misma VM. Specs 001 (9 tareas), 002 (30), 013 reescritas. Constitución **v1.4.0** (Art. XIII y XIV). Pendiente: región, talla de VM gratuita (≈1 GB de RAM, margen justo) y capacidad de dos entornos | constitution Art. XIII y XIV; specs 001, 002 y 013; `.opencode/agents/devops-mcp.md` |
| D-117 | **Principio del profesor: la persona solo se registra y hace configuración mínima; todo lo demás lo hace OpenCode con los MCP.** El MCP de Azure se usa **con escritura** (sin `--read-only`, verificado que existe `compute_vm_create`) y cada llamada pide aprobación; `az` solo para lo que no cubra (NSG, IP estática, DNS) y SSH para dentro de la VM. Se reemplaza `cloud-init` por `deploy/bootstrap.sh` (el MCP no acepta `cloud-init`). Pasos humanos listados en la spec 001 (RF-00) y en las constituciones (backend XIII.2, móvil XIV.7, landing XII) | constitution XIII.2 y XIII.4; specs 001; `.opencode/agents/devops-mcp.md` |
| D-118 | **Se quita nginx; HTTPS sigue siendo forzoso.** La API sirve TLS directamente con un certificado de Let's Encrypt emitido por `certbot` en la VM y lo recarga sin reiniciarse; una sola réplica; sin balanceo. **Reemplaza la capa opcional `edge` de D-116.** Hallazgos del informe del 2 de nov: tallas gratuitas vigentes `Standard_B2ats_v2` (preferida), `Standard_B1s` (en retiro) y `Standard_B2pts_v2` (ARM64); no hay región garantizada (`NotAvailableForSubscription`; revisar `az vm list-skus`; sin zonas de disponibilidad); la **IP pública estándar no es gratuita** (≈ 3–4 USD/mes, excepción aceptada); sin disco temporal, swap de 2 GB obligatorio; créditos de CPU (línea base 20 %). Cuenta y licencia de Google Play ya compradas (2 nov); calendario recalculado en el ROADMAP §1. Spec 002 pasa a 31 tareas (nueva T028: servidor HTTPS con certificado recargable) | constitution Art. XIII y XIV; specs 001, 002 y 013 |
| D-119 | **Azure va en la cuenta principal del proyecto** (la de la licencia de Google Play), como exige el curso; **no** en una cuenta de estudiantes ni personal. Cuenta gratuita de Azure (tarjeta registrada, sin cobros mientras no se actualice a pago por uso; 200 USD durante 30 días desde el alta). Si la oferta se rechaza por uso previo, otra persona del equipo hace la verificación. Plan B sin VM (Azure Container Apps con su cupo gratuito) documentado como alternativa, sin aplicar | constitution XIII.1; spec 001; ROADMAP §10 y §11 |
| D-120 | Alta de Azure: se crea una cuenta Microsoft nueva con el correo de la cuenta principal del proyecto; el alta (tarjeta y teléfono) la hace un integrante que no haya usado la oferta gratuita (la mayoría del equipo no la ha reclamado). Responsable por definir; debe seguir en el equipo hasta el final. `az login` siempre con la cuenta del proyecto | spec 001 T001; constitution XIII.1 |
| D-121 | **Calendario fijado el 2 nov (el desarrollo arranca hoy):** 001–002 del 2 al 5 nov; primer AAB y día 1 de la prueba cerrada el 9 nov; día 14 el 22 nov; congelar la 1.0 el 22 nov y solicitar producción el 23 nov; publicación estimada 30 nov–7 dic (puede quedar después de la entrega); IA y ML (v1.1) del 23 nov al 1 dic; entrega 1–4 dic. Sustituye al plan del 28 oct | ROADMAP §1 y §3; specs 003–015 |
| D-122 | **Repositorio en GitHub con MCP de GitHub:** una rama y una PR por spec, `main` protegida; solo el agente `gestor-repo` usa el MCP (token `GITHUB_PERSONAL_ACCESS_TOKEN`) y nunca fusiona ni borra; una persona revisa y fusiona. Sin pipeline automatizado. Constitución con el Art. XV. Tarea nueva en la spec 001 | constitution Art. XV; spec 001 |

## Integracion DevSecOps — 2026-11-02

- [CI] Por solicitud expresa del responsable se prepara la integracion de ramas, PRs y seguridad que habia quedado fuera el 2026-11-01. Spec independiente: specs/017-devsecops/. No se modifica constitution.md ni se ejecutan specs funcionales.
- [CI] Herramientas exclusivas de CI: GitHub Actions fijadas por SHA, Gitleaks v8.30.1 (checksum verificado), CodeQL v4, golangci-lint v2.15.0, govulncheck v1.9.0. Sin dependencias nuevas de aplicacion.
- [CI] Revisores indicados: JesusTrz (escritura) y JosephYanez27 (lectura al preparar el cambio). El segundo necesita escritura para aprobaciones obligatorias y CODEOWNERS. Ruleset y ajustes administrativos quedan pendientes hasta que se apliquen y verifiquen.
- [CI] Estado inicial: documentacion sin aplicacion; CI informa las pruebas no aplicables y ejecuta politica/secretos. Despliegue Render por MCP y pruebas en Expo Go se conservan.

- [BE-B] 2026-11-02 — 001/T001: spec 001 aprobada por la persona responsable. Pasos humanos hechos (cuenta Azure del proyecto, az login, Cloudinary, CLOUDINARY_URL). Suscripción "Azure subscription 1" verificada como FreeTrial_2014-09-01 con spendingLimit On (sin cobros; el crédito vence 30 días después del alta). MCP conectados: azure, cloudinary-assets, cloudinary-config. MCP de Azure cubre: VM (create, delete, get, power-state, update), grupos de recursos (list), suscripciones (list) y precios (pricing get). NO cubre: IP pública estática con nombre DNS, reglas NSG, presupuesto/alerta de costo, cloud-init, SSH, certificados TLS.

- [BE-B] 2026-11-02 — Modelos de opencode.json actualizados porque los anteriores ya no existen en Zen: nemotron-3-ultra-free → nemotron-3.5-super-vl-free (plan, spec-writer, modelador-mongo, devops-mcp); nemotron-3-lightning-free → nemotron-3.5-lightning-free (build, dev-go, gestor-repo); ling-flash-free → glm-5.3-flash-free (qa-go, small_model). Verificado con `opencode models`.

- [BE-B] 2026-11-02 — 001/T001: CA-02 verificado (dev-go sin acceso a azure_* y su intento con `az` rechazado; devops-mcp ejecutó azure_subscription_list por MCP). T001 cerrada. Aprendizaje: con nemotron-3.5-super-vl-free, devops-mcp probó herramientas IoT irrelevantes en bucle (rechazadas); antes de crear recursos (T004) dar nombres exactos de herramientas y revisar el modelo de devops-mcp.

- [BE-B] 2026-11-03 — 001/T002: herramienta(s) MCP probadas: cloudinary-assets.search-folders (dos veces) y cloudinary-assets.get-usage-details (mismo lote); nube: NO verificada — error literal `Parameter 'cloud_name' must be a string or number` (repitió dos veces); cloud_name no llega al servidor MCP de Cloudinary aunque la persona declare CLOUDINARY_URL definida; vaiven/staging: no creada ni verificada (create-folder no se llamó por fallo previo de verificación); resultado: BLOQUEADO. Pendiente de que el MCP de Cloudinary reciba cloud_name (reexportar CLOUDINARY_URL y reiniciar OpenCode, luego reintentar T002: search-folders → create-folder vaiven → create-folder vaiven/staging → search-folders de comprobación).
- [BE-B] 2026-11-03 — 001/T002 (reintento tras reinicio del servicio): herramientas MCP exactas `cloudinary-assets["search-folders"]` y `cloudinary-assets["create-folder"]` (parámetro `folder`). La nube responde y muestra carpetas raíz (`samples/` con subcarpetas `landscapes`, `people`, `food`, `animals`, `ecommerce`; sin credenciales). Carpeta `vaiven/staging` creada en una llamada (ruta anidada soportada) y verificada: `search-folders` con `prefix: "vaiven"` devuelve `path: vaiven/staging` (`name: staging`) junto a su padre `vaiven`. Resultado: OK, RF-06 / CA-06 cumplida (`vaiven/staging` existe y se comprobó por MCP). No se escribió ni expuso `CLOUDINARY_URL`, API key ni API secret.

- [BE-B] 2026-11-03 — 001/T002: causa raíz del bloqueo anterior: el servicio en segundo plano de OpenCode (`opencode serve --service`) conserva el entorno con el que arrancó; definir `CLOUDINARY_URL` con `setx` y reabrir la ventana **no** basta, hace falta `opencode service restart` (reiniciar el servicio, no solo el cliente) para que los MCP reciban `{env:CLOUDINARY_URL}`. Síntoma inequívoco: `Parameter 'cloud_name' must be a string or number` en todas las herramientas de `cloudinary-assets` y `cloudinary-config`.

- [BE-B] 2026-11-03 — 001/T002: `opencode.json` migrado a la forma V2 de MCP (`mcp.servers`, sin `enabled`, `oauth: false` en `github`) según la documentación V2; `github` seguirá en `failed` hasta que exista `GITHUB_PERSONAL_ACCESS_TOKEN` (T009). Sin cambios en `permission` ni en `tools`.

- [BE-B] 2026-11-03 — 001/T003: archivos creados (bootstrap.sh, docker-compose.yml, tls.md), keyFile generado en la VM (uid 999, /opt/vaiven/secrets/mongo-keyfile) y límites: mongo 400 MiB con caché WiredTiger 0.25 GB, redis 128 MiB dentro de 192 MiB.

- [BE-B] 2026-11-03 — 001/T004: VM vm-vaiven creada en westus2, talla Standard_B2ats_v2 (x86-64, sin zona; en westus2 solo tenía restricción de zona, y eastus2 y B1s estaban restringidas), Ubuntu 24.04 LTS, disco Premium_LRS de 64 GB, usuario azureuser con la llave dedicada vaiven_azure. IP pública Standard estática; FQDN vaiven-staging.westus2.cloudapp.azure.com. NSG: 22 solo desde la IP de la administradora; 80, 443 y 8443 desde Internet; 27017 y 6379 cerrados (comprobado desde fuera). Presupuesto de 5 USD PENDIENTE: az consumption budget falla (API obsoleta); crearlo en el portal (Cost Management → Budgets) con avisos al 80% y al 100%; hoy el límite de gasto impide cobros.

- [BE-B] 2026-11-03 — Desviaciones de T004: compute_vm_create (MCP) se quedó colgado sin llegar a Azure, así que la VM se creó con `az vm create --nsg-rule NONE` y las reglas del NSG se agregaron después. OpenCode solo muestra 4 de las 73 herramientas del MCP de Azure; nemotron-3.5-super-vl-free elegía herramientas de IoT, así que devops-mcp pasa a opencode/big-pickle (Art. X.2). El crédito de prueba vence 30 días después del alta: decidir antes si se convierte a pago por uso.

- [BE-B] 2026-11-03 — 001/T005: bootstrap.sh ejecutado por SSH en vm-vaiven (dos veces, idempotente: una sola línea de swap en /etc/fstab). Docker 29.9.2, Compose v5.7.0, certbot 2.9.0 con certbot.timer activo, swap de 2G, 887 MiB de RAM (≈560 MiB disponibles) y 57G de disco libres, usuario deploy en el grupo docker, /opt/vaiven/secrets con 700. CA-03 cumplida: 27017 y 6379 no responden desde fuera.

- [BE-B] 2026-11-03 — 001/T005, desviaciones: bootstrap.sh no instalaba certbot (la sección se perdió al corregir T003); se agregó la sección 7 y se volvió a ejecutar. big-pickle agotó su cupo gratuito a mitad de la tarea, así que los últimos pasos (ejecución por SSH y verificación) se hicieron con Claude Code, fuera de OpenCode (Art. X). Pendiente: elegir otro modelo gratuito para devops-mcp (probados: deepseek-v4.1-flash-free y glm-5.3-flash-free).

- [BE-B] 2026-11-03 — 001/T006, corrección: mongo salía con código 1 («permissions on /etc/mongo/keyfile are too open»). Causa: bootstrap.sh creaba /opt/vaiven/secrets pero no el keyFile, y Docker montó en su lugar una carpeta vacía. bootstrap.sh ahora genera /opt/vaiven/secrets/mongo-keyfile (999:999, 400) antes del primer `up` y quita esa carpeta si existe; el compose base suma `restart: unless-stopped` a mongo y redis, y .gitattributes fuerza LF también en YAML. Aplicado en la VM (bootstrap ejecutado otra vez, contenedor mongo recreado sin tocar volúmenes): mongo y redis healthy. Desviación del Art. XIII.4: el diagnóstico y la corrección por SSH los hizo Claude Code porque OpenCode agotó su cupo gratuito. Pendiente de T006: rs.initiate, usuario vaiven_app, PONG autenticado, docker stats y créditos de CPU. Pendiente para la spec 002: fijar `name: vaiven-base` en el compose (hoy proyecto «base», red base_vaiven-net, volumen base_vaiven-mongo-data).

- [BE-B] 2026-11-03 — 001/T006 cerrada (CA-04): replica set rs0 iniciado con el miembro mongo:27017 en PRIMARY; usuario vaiven_app con readWrite solo sobre vaiven_staging (inicia sesión y escribe ahí, sin permiso en otras bases ni para rs.status); Redis responde PONG con contraseña y NOAUTH sin ella. Consumo: mongo 211 MiB / 400 MiB y redis 4 MiB / 192 MiB, ≈470 MiB disponibles en la VM; CPU de la VM 1–8 %, créditos de CPU ≈116 y subiendo. .env de la VM con 600 (dueño deploy); las contraseñas se pasaron por variables de entorno, nunca se imprimieron. Aprendizaje: en scripts por SSH con heredoc, `docker compose exec -T` consume la entrada estándar y corta el resto del script; usar `</dev/null`. Hecho con Claude Code por SSH (misma desviación del Art. XIII.4).

- [BE-B] 2026-11-04 — 001/T007 (en curso): certificado de Let's Encrypt emitido con `certbot certonly --standalone` para vaiven-staging.westus2.cloudapp.azure.com (ECDSA, emisor YE2, vence 2027-02-01). Rutas para el compose de la spec 002: /etc/letsencrypt/live/vaiven-staging.westus2.cloudapp.azure.com/fullchain.pem y privkey.pem (privkey solo legible por root). certbot.timer activo. CA-05: con `openssl s_server` temporal en 8443, `curl` sin -k = 200 y `openssl s_client` = Verify return code 0 con la cadena completa; s_server se detuvo solo. PENDIENTE: `certbot renew --dry-run` falla porque el servidor de pruebas de Let's Encrypt (acme-staging-v02) no responde ni desde la VM ni desde la máquina local (el de producción sí); repetir antes de cerrar T007. Aprendizajes: `s_server -cert fullchain.pem` solo envía el primer certificado (usar cert.pem + `-cert_chain chain.pem`; tls.md corregido); la IP pública de la administradora cambió y el SSH dejó de responder: se actualizó la regla Allow-SSH-Admin del NSG a la IP nueva (/32). Hecho con Claude Code (desviación del Art. XIII.4).
