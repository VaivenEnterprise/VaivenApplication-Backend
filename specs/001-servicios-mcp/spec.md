# Spec 001 — Infraestructura en Azure y servicios (backend)

- **Dueño:** BE-B · **Prioridad:** P0 · **Depende de:** — (es la primera spec; no requiere código de la API)
- **Constitución:** v1.4.0 (Art. XIII y XIV) · **Estado:** Borrador, pendiente de aprobación

## 1. Objetivo y por qué
Dejar lista, **antes de escribir código de la API**, la infraestructura gratuita donde correrá todo el backend: una **VM de Azure** con Docker Compose que ejecuta **Redis** y **MongoDB**, con **HTTPS forzoso** (certificado de Let's Encrypt para el nombre DNS de la IP), más la carpeta de imágenes en **Cloudinary**; todo aprovisionado y verificado desde OpenCode con el **MCP de Azure**, la CLI `az` y los MCP de Cloudinary, no con paneles manuales. Así, desde la spec 002 cada spec termina desplegada sin trámites a mano.

## 2. Historias técnicas
| # | Prio. | Historia |
|---|---|---|
| H-01 | P0 | Como **equipo**, quiero que OpenCode opere Azure y Cloudinary, para crear y verificar la infraestructura sin salir del flujo SDD. |
| H-02 | P0 | Como **administrador**, quiero que solo `devops-mcp` pueda usar esas herramientas y que las acciones destructivas estén bloqueadas, para no perder recursos ni datos. |
| H-03 | P0 | Como **desarrollador**, quiero una VM con MongoDB (replica set) y Redis ya funcionando, para programar y desplegar contra la infraestructura real. |
| H-04 | P0 | Como **equipo**, quiero un certificado HTTPS válido y renovable ya emitido para la VM, para que la API y la app publicada solo hablen por HTTPS. |
| H-05 | P1 | Como **equipo**, quiero documentación de cada servicio sin secretos, para reproducir el entorno. |

## 3. Requisitos
| ID | Requisito |
|---|---|
| RF-01 | El MCP `azure` (con permiso de escritura, cada llamada con aprobación) y los MCP `cloudinary-assets` y `cloudinary-config` están declarados en `opencode.json` y conectan desde OpenCode (`opencode mcp list`); `az` tiene sesión iniciada en la suscripción de la cuenta gratuita. |
| RF-00 | **Trabajo humano mínimo (Art. XIII.2):** la persona solo (1) crea la cuenta de Azure con el correo de la **cuenta principal del proyecto** (la misma de Google Play), (2) crea la cuenta de Cloudinary, (3) ejecuta `az login`, (4) define `CLOUDINARY_URL` y (5) aprueba las acciones que OpenCode pida. Todo lo demás —VM, red, Docker, MongoDB, Redis, certificado y verificaciones— lo hace `devops-mcp` desde OpenCode, primero con los MCP y, para lo que no cubran, con `az` o SSH. `docs/servicios.md` lista esos pasos humanos y justifica cualquier otro. |
| RF-02 | Solo el agente `devops-mcp` puede usar MCP y `az`; los demás son rechazados. Las acciones destructivas están denegadas. |
| RF-03 | **Azure:** grupo de recursos, red virtual, **IP pública estándar estática** con nombre DNS (no es gratuita: ≈ 3–4 USD/mes, ver RNF-02), reglas de red (80 para el certificado, 443 y 8443 para la API; 22 solo desde la IP de la persona administradora) y una VM Linux (Ubuntu 24.04 LTS) de una talla gratuita —`Standard_B2ats_v2` preferida; `Standard_B1s` o `Standard_B2pts_v2` (ARM64) como alternativas— **sin zonas de disponibilidad**, con disco de SO Premium de **hasta 64 GB** (cubierto por la capa gratuita), creada con el MCP de Azure y preparada por SSH con `deploy/bootstrap.sh` (Docker, Compose, swap de 2 GB). |
| RF-04 | **Stack base** (`deploy/base/`): MongoDB 7 como replica set `rs0` de un nodo y Redis 7. Ninguno publica puertos al exterior. **No hay nginx ni balanceador.** |
| RF-05 | **HTTPS forzoso:** certificado Let's Encrypt para el nombre DNS de la IP pública, emitido con `certbot` (validación en el puerto 80) y con renovación automática verificada (`--dry-run`); los archivos quedan en la VM listos para que la API los monte (Art. XIV.4). |
| RF-06 | **Cloudinary:** la nube responde por MCP y existe la carpeta base `vaiven/staging`. El *API secret* solo existe en el entorno local de quien administra y, después, en el `.env` de la VM. |
| RF-07 | `docs/servicios.md` y `.env.example` listan servicios, variables (sin valores), región, talla de VM, cómo se obtuvo cada credencial, cómo rotarla y cómo recuperar acceso. |
| RNF-01 | Ningún secreto en el repositorio, en `memoria.md`, en specs ni en registros. |
| RNF-02 | Costo mensual casi cero: solo recursos de la capa gratuita de Azure (750 h/mes de **una** VM durante 12 meses —tras actualizar a pago por uso—, dos discos P6, 100 GB de salida; durante los primeros 30 días todo sale del crédito). **Única excepción aceptada:** la IP pública estándar (≈ 0,004–0,005 USD/h). Cualquier otro recurso con costo se detiene y se consulta. |

## 4. Criterios de aceptación
| ID | Dado / Cuando / Entonces |
|---|---|
| CA-01 | Dadas las variables y la sesión de `az`, cuando ejecuto `opencode mcp list`, entonces `azure`, `cloudinary-assets` y `cloudinary-config` aparecen conectados. |
| CA-02 | Dado `dev-go`, cuando intenta una herramienta MCP o `az`, entonces se rechaza; con `devops-mcp` funciona. |
| CA-03 | Dada la VM, cuando `devops-mcp` entra por SSH, entonces Docker y Compose responden, hay 2 GB de swap activos y los puertos de Mongo (27017) y Redis (6379) **no** son alcanzables desde fuera. |
| CA-04 | Dado el stack base, cuando consulto `rs.status()`, entonces el nodo es `PRIMARY`; y `redis-cli ping` devuelve `PONG`. |
| CA-05 | Dado el certificado, cuando una prueba temporal sirve HTTPS en el puerto 8443 (`openssl s_server`), entonces `openssl s_client` y `curl` validan la cadena sin `-k` para el nombre DNS; y `certbot renew --dry-run` termina bien. |
| CA-06 | Dado Cloudinary, cuando `devops-mcp` consulta la carpeta `vaiven/staging`, entonces existe. |
| CA-07 | Dado el repositorio, cuando busco cadenas sospechosas (URI con contraseña, `API_SECRET`, claves, llaves SSH), entonces no hay ninguna. |

## 5. Fuera de alcance
Código Go de la API, `scripts/deploy.sh` y el Compose definitivo (spec 002), la configuración HTTPS de la API (spec 002), producción (spec 013), dominio propio, monitoreo avanzado, alta disponibilidad real (una sola VM), copias de seguridad automáticas.

## 6. Notas y preguntas abiertas
- **Disponibilidad regional (NotAvailableForSubscription):** no existe una región que garantice una VM gratuita; la capacidad cambia día a día y las cuentas de prueba tienen cuotas bajas (≈ 4–6 vCPU). Antes de crear nada, `devops-mcp` revisa `az vm list-skus --location <región> --resource-type virtualMachines --size Standard_B --all` y solo usa una región donde la talla elegida no tenga `Restrictions`; si falla, prueba la siguiente región y luego la siguiente talla (B2ats_v2 → B1s → B2pts_v2). **No** pide zonas de disponibilidad.
- **Cuenta:** la cuenta de Azure es la **cuenta principal del proyecto** (requisito del curso), no la de estudiantes. La política de regiones permitidas (`sys:regionrestriction`) es propia de «Azure for Students»; en la cuenta gratuita normal aplican cuotas bajas de vCPU (≈ 4–6 núcleos por región). Si el alta es rechazada porque la persona ya usó la oferta gratuita, otra persona del equipo puede completar la verificación (tarjeta y teléfono) para la misma cuenta del proyecto. Plan acordado: se crea una cuenta Microsoft nueva con el correo del proyecto y el alta la hace un integrante que no haya usado la oferta (responsable por definir).
- **Rendimiento en ráfaga:** las tallas B funcionan con créditos de CPU (línea base 20 % en B2ats_v2/B2pts_v2, 10 % en B1s); si se agotan, la VM se estrangula. Para la API y MongoDB de este proyecto basta, pero debe vigilarse (`docker stats`, métrica de créditos).
- **ARM64:** si hubiera que usar `Standard_B2pts_v2`, la imagen de la API se compila con `GOARCH=arm64` y MongoDB y Redis usan sus imágenes oficiales `arm64`.
- **Una sola VM de 750 h/mes:** alcanza para una VM encendida todo el mes; no hay segunda VM gratuita.
- **Crédito y cobros:** el crédito de 200 USD dura **30 días desde el alta** (por eso la cuenta se crea justo antes de empezar la spec 001). Mientras la suscripción **no se actualice a pago por uso**, Azure no cobra: al acabarse el crédito o los 30 días, los recursos se desactivan. Si hace falta mantener el backend más allá (la prueba cerrada y la revisión de Google pueden pasar de principios de diciembre), la decisión de actualizar la toma una persona, sabiendo que lo único con costo sería la IP pública (≈ 0,12 USD al día) y que se borra todo al terminar. Alerta de presupuesto desde el primer día.
- Producción comparte la misma VM (spec 013) salvo que se decida otra cosa.
- La IP desde la que se permite SSH la detecta el agente (la de la persona administradora); cambia si su proveedor la rota y se actualiza con aprobación.
- El MCP de Azure (verificado en `compute`) no tiene parámetro para `cloud-init` ni vi herramientas de red; qué más cubre se confirma con `learn=true` en T004. Lo que no cubra se hace con `az` por OpenCode.
