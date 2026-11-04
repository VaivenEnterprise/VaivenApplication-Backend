# Servicios del backend — VAIVÉN (staging)

Inventario de la infraestructura creada en la spec 001 (`specs/001-servicios-mcp/`). **Sin secretos:** aquí solo hay nombres, rutas y procedimientos; los valores viven en la VM o en el entorno local de quien administra.

## 1. Servicios

| Servicio | Dónde | Detalle | Costo |
|---|---|---|---|
| VM `vm-vaiven` | Azure, grupo `rg-vaiven`, región `westus2` | `Standard_B2ats_v2` (x86-64, 2 vCPU, ~1 GiB), Ubuntu 24.04 LTS, disco de SO Premium_LRS de 64 GB, sin zona de disponibilidad | Capa gratuita (750 h/mes) |
| IP pública | Azure | Standard, estática; DNS `vaiven-staging.westus2.cloudapp.azure.com` (`AZURE_HOST`) | ≈ 3–4 USD/mes (única excepción aceptada) |
| NSG | Azure | 80 (certbot), 443 y 8443 (API) abiertos; 22 solo desde la IP de la persona administradora; 27017 y 6379 cerrados | Gratis |
| MongoDB 7 | Contenedor `base-mongo-1` en la VM | Replica set `rs0` de un nodo, autenticación con keyFile, base `vaiven_staging`, usuario de aplicación `vaiven_app` (readWrite solo sobre esa base); sin puerto publicado; 400 MiB | — |
| Redis 7 | Contenedor `base-redis-1` en la VM | Contraseña, `maxmemory 128mb`, `allkeys-lru`; sin puerto publicado; 192 MiB | — |
| Certificado TLS | Let's Encrypt, en la VM | `/etc/letsencrypt/live/<AZURE_HOST>/fullchain.pem` y `privkey.pem` (ECDSA, vence 2027-02-01); renovación automática con `certbot.timer` | Gratis |
| Cloudinary | Nube de Cloudinary | Carpeta base `vaiven/staging` | Capa gratuita |

Stack base: `deploy/base/docker-compose.yml` (proyecto Compose `base`, red `base_vaiven-net`, volumen `base_vaiven-mongo-data`). Preparación de la VM: `deploy/bootstrap.sh`. Certificado: `deploy/base/tls.md`.

## 2. Pasos humanos (RF-00, Art. XIII.2)

Son los únicos pasos manuales; todo lo demás lo hace `devops-mcp` desde OpenCode.

1. Crear la cuenta de Azure con el correo de la cuenta principal del proyecto (cuenta gratuita, suscripción `FreeTrial` con límite de gasto activo).
2. Crear la cuenta de Cloudinary.
3. Ejecutar `az login` con la cuenta del proyecto.
4. Definir `CLOUDINARY_URL` como variable de entorno del sistema y reiniciar OpenCode.
5. Aprobar las acciones que OpenCode pida.

Pasos manuales adicionales, justificados:
- Crear el **presupuesto de 5 USD** en el portal (Cost Management → Budgets, avisos al 80 % y al 100 %), porque `az consumption budget` falla (API obsoleta). Pendiente.
- Crear el token `GITHUB_PERSONAL_ACCESS_TOKEN` para el MCP de GitHub (T009, Art. XV.6).

## 3. Variables y credenciales

| Variable | Dónde vive | Cómo se obtuvo | Cómo se rota |
|---|---|---|---|
| `CLOUDINARY_URL` | Entorno local de quien administra (`setx`) | Panel de Cloudinary → API Keys | Generar una API key nueva en Cloudinary, actualizar la variable, reiniciar OpenCode y desactivar la anterior |
| `MONGO_ROOT_PASSWORD` | `/opt/vaiven/base/.env` en la VM (600, dueño `deploy`) | `openssl rand -hex 24` en la VM | En la VM: `db.getSiblingDB("admin").changeUserPassword("root", <nueva>)` y actualizar `.env`. El usuario raíz se creó al primer arranque; cambiar solo la variable no lo cambia |
| `MONGO_APP_PASSWORD` | `/opt/vaiven/base/.env` en la VM | `openssl rand -hex 24` en la VM | `db.getSiblingDB("vaiven_staging").changeUserPassword("vaiven_app", <nueva>)`, actualizar `.env` y reiniciar la API |
| `REDIS_PASSWORD` | `/opt/vaiven/base/.env` en la VM | `openssl rand -hex 24` en la VM | Actualizar `.env` y `docker compose up -d redis` (Redis no guarda datos persistentes) |
| keyFile de MongoDB | `/opt/vaiven/secrets/mongo-keyfile` (999:999, 400) | `deploy/bootstrap.sh` | Detener mongo, regenerar el archivo con los mismos permisos y arrancar |
| `AZURE_HOST` | `.env` local de despliegue | Etiqueta DNS de la IP pública | No es secreto; no se rota |
| Llave SSH `vaiven_azure` | `~/.ssh/vaiven_azure` de quien administra | `ssh-keygen` (T004) | Ver §4 |

Las contraseñas se pasan a los comandos como variables de entorno dentro de la VM; nunca se imprimen ni se pegan en conversaciones con modelos.

## 4. Recuperar el acceso SSH

- **Llave perdida:** generar una nueva con `ssh-keygen -t rsa -b 4096 -f ~/.ssh/vaiven_azure` y cargarla con `az vm user update -g rg-vaiven -n vm-vaiven -u azureuser --ssh-key-value ~/.ssh/vaiven_azure.pub`.
- **IP de la administradora cambiada:** obtener la IP con `curl -s https://api.ipify.org` y actualizar la regla del puerto 22 con `az network nsg rule update -g rg-vaiven --nsg-name vm-vaivenNSG -n Allow-SSH-Admin --source-address-prefixes <IP>/32` (con aprobación). Nunca abrir el 22 a Internet.

## 5. Riesgos conocidos

| Riesgo | Mitigación |
|---|---|
| Memoria justa (~887 MiB en la VM; mongo ≈211 MiB y redis ≈4 MiB en reposo) | Límites por contenedor, swap de 2 GB, `wiredTigerCacheSizeGB 0.25`; si no alcanza, consultar antes de cambiar de talla |
| Créditos de CPU de la serie B (línea base 20 %) | Vigilar la métrica «CPU Credits Remaining» |
| El crédito de 200 USD vence 30 días después del alta | Con el límite de gasto activo, los recursos se desactivan; decidir antes si se pasa a pago por uso |
| La IP pública estándar tiene costo | Aceptado; presupuesto con avisos |
| Una sola VM, sin alta disponibilidad ni copias automáticas | `restart: unless-stopped`; `mongodump` manual cifrado antes de cambios grandes |
| Secretos en el contexto del modelo | Los secretos se generan en la VM; nunca se imprimen ni se pegan en chats; rotar si alguno se expone |
| La renovación del certificado necesita el puerto 80 | Mantener abierto el 80 en el NSG; `certbot renew --dry-run` pendiente de repetir (el servidor de pruebas de Let's Encrypt no respondía el 2026-11-04) |
| La IP pública de la administradora cambia | El SSH deja de responder: actualizar la regla `Allow-SSH-Admin` (§4) |
