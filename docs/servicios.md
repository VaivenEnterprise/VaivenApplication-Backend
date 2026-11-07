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
- Presupuesto **`vaiven-5usd`** (5 USD al mes, avisos al 80 % y al 100 % al rol Propietario): creado el 2026-11-04 con `az rest` contra `Microsoft.Consumption/budgets` (api-version 2023-05-01), porque `az consumption budget` falla (API obsoleta). No hizo falta el portal.
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
| `JWT_ACCESS_SECRET` | `/opt/vaiven/staging/.env` en la VM (600, dueño `deploy`) | `openssl rand -hex 32` en la VM (spec 003, T014; 64 caracteres) | Generar uno nuevo en la VM, reemplazar la línea y `scripts/deploy.sh staging`: todas las sesiones de acceso vencen (los refresh tokens siguen sirviendo) |
| Llave SSH `vaiven_azure` | `~/.ssh/vaiven_azure` de quien administra | `ssh-keygen` (T004) | Ver §4 |

Las contraseñas se pasan a los comandos como variables de entorno dentro de la VM; nunca se imprimen ni se pegan en conversaciones con modelos.

## 4. Recuperar el acceso SSH

- **Llave perdida:** generar una nueva con `ssh-keygen -t rsa -b 4096 -f ~/.ssh/vaiven_azure` y cargarla con `az vm user update -g rg-vaiven -n vm-vaiven -u azureuser --ssh-key-value ~/.ssh/vaiven_azure.pub`.
- **IP de la administradora cambiada:** obtener la IP con `curl -s https://api.ipify.org` y actualizar la regla del puerto 22 con `az network nsg rule update -g rg-vaiven --nsg-name vm-vaivenNSG -n Allow-SSH-Admin --source-address-prefixes <IP>/32` (con aprobación). Nunca abrir el 22 a Internet.
- **Comprobar sin SSH:** `az vm run-command invoke -g rg-vaiven -n vm-vaiven --command-id RunShellScript --scripts @script.sh` ejecuta un script como root por la API de Azure, sin abrir el NSG. Útil cuando la IP cambia seguido. Pasa el script como archivo (`@`): en Windows, los `|` escritos en línea los interpreta la terminal local.

## 5. Reconstruir el stack base desde cero

Orden para una VM nueva (o para comprobar que el repositorio basta). Todo en la VM; ningún valor secreto se imprime.

1. `sudo bash deploy/bootstrap.sh` — Docker, Compose, certbot, swap, usuario `deploy`, `/opt/vaiven` y el keyFile de MongoDB. Idempotente.
2. `sudo bash deploy/base/init-env.sh` — crea `/opt/vaiven/base/.env` (600) con contraseñas aleatorias y el enlace `/opt/vaiven/.env` que lee redis. No sobrescribe un `.env` existente.
3. Copiar `deploy/base/docker-compose.yml` a `/opt/vaiven/base/` y ejecutar `cd /opt/vaiven/base && sudo docker compose up -d`; esperar a que mongo y redis estén *healthy*.
4. Replica set (una vez): cargar las variables con `set -a; . ./.env; set +a` y ejecutar
   `docker compose exec -T -e MP="$MONGO_ROOT_PASSWORD" mongo mongosh --quiet --norc --eval 'db.getSiblingDB("admin").auth("root", process.env.MP); rs.initiate({_id:"rs0",members:[{_id:0,host:"mongo:27017"}]})' </dev/null`.
5. Usuario de aplicación: en la misma sesión,
   `docker compose exec -T -e MP="$MONGO_ROOT_PASSWORD" -e AP="$MONGO_APP_PASSWORD" mongo mongosh --quiet --norc --eval 'db.getSiblingDB("admin").auth("root", process.env.MP); db.getSiblingDB("vaiven_staging").createUser({user:"vaiven_app", pwd: process.env.AP, roles:[{role:"readWrite", db:"vaiven_staging"}]})' </dev/null`.
6. Certificado: seguir `deploy/base/tls.md` (`certbot certonly --standalone`).
7. Comprobar: `rs.status()` en PRIMARY, `redis-cli -a "$REDIS_PASSWORD" ping` = PONG y los puertos 27017 y 6379 cerrados desde fuera.

Las contraseñas se pasan por variables de entorno (`process.env`), no en la línea de comandos. `</dev/null` evita que `docker compose exec -T` consuma la entrada de un script.

## 6. Riesgos conocidos

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

## 7. API de staging (spec 002, T031)

- **URL para el móvil:** `https://vaiven-staging.westus2.cloudapp.azure.com:8443` (solo HTTPS, certificado de Let's Encrypt).
- Endpoints: `GET /healthz`, `GET /readyz` (`redis: ok|degradado`), `GET /api/v1/meta` y, desde la spec 003, `/api/v1/auth/{register,login,refresh,logout}`, `/api/v1/me`, `/api/v1/users/{id}` y, desde la spec 004, `POST /api/v1/uploads/signature`. Contrato: `api/openapi.yaml` y `api/paths/`.
- Contenedor `vaiven-staging-api-1` (proyecto de Compose `vaiven-staging`, `/opt/vaiven/staging/`), unido a la red `base_vaiven-net` del stack base; usuario no root, sistema de archivos de solo lectura, 160 MiB.
- Variables en `/opt/vaiven/staging/.env` (600, dueño `deploy`; plantilla en `deploy/.env.example`), generadas en la VM.
- Certificado para la API en `/opt/vaiven/certs` (uid 65532); lo renueva el hook `/etc/letsencrypt/renewal-hooks/deploy/vaiven-api.sh` (copia de `deploy/certbot-deploy-hook.sh`) y la API lo recarga sola.
- Desplegar: `scripts/deploy.sh staging` (con Docker o `ENGINE=podman`); volver atrás: `scripts/deploy.sh staging --rollback`.

## 8. Probar una subida real (spec 004)

El móvil sube cada imagen **directo a Cloudinary** con una firma de un solo uso que da la API; la imagen no pasa por nuestro servidor. El **API secret de Cloudinary nunca se usa del lado del cliente**: solo la API lo tiene y firma con él. Para probarlo a mano (Git Bash o WSL, con `jq`):

```bash
API=https://vaiven-staging.westus2.cloudapp.azure.com:8443/api/v1

# 1. Iniciar sesión con una cuenta de prueba (las credenciales van en variables, no en el historial).
TOKEN=$(curl -s "$API/auth/login" -H 'Content-Type: application/json' \
  -d "{\"email\":\"$VAIVEN_EMAIL\",\"password\":\"$VAIVEN_PASSWORD\"}" | jq -r .tokens.accessToken)

# 2. Pedir la firma (item_photo → pública; evidence → privada). Límite: 30 por hora.
SIG=$(curl -s "$API/uploads/signature" -H "Authorization: Bearer $TOKEN" \
  -H 'Content-Type: application/json' -d '{"purpose":"item_photo"}')

# 3. Subir a uploadUrl con los campos tal cual (jpg, jpeg, png o webp, máximo 5 MB).
curl -s "$(jq -r .uploadUrl <<<"$SIG")" \
  -F file=@foto.jpg \
  -F api_key="$(jq -r .apiKey <<<"$SIG")" \
  -F timestamp="$(jq -r .timestamp <<<"$SIG")" \
  -F signature="$(jq -r .signature <<<"$SIG")" \
  -F public_id="$(jq -r .publicId <<<"$SIG")" \
  -F type="$(jq -r .type <<<"$SIG")" \
  -F allowed_formats="$(jq -r .allowedFormats <<<"$SIG")" \
  -F overwrite="$(jq -r .overwrite <<<"$SIG")"
```

- Si se cambia cualquier campo firmado (`public_id`, `type`, `allowed_formats`, `overwrite`, `timestamp`), Cloudinary responde `401 Invalid Signature`. La firma vence a la hora.
- Al usarla en otra spec, el cliente manda solo la referencia (`{"photos":[{"publicId":"<publicId>"}]}`) y el servidor llama a `images.Service.Resolve`: comprueba que la imagen es de quien la envía y del propósito correcto, y pide a Cloudinary formato y tamaño. Si no cumple responde `400 validation_failed` con un detalle por imagen; si Cloudinary no responde, `503 service_unavailable`. Las URL (`url`, `thumbUrl`) se calculan al leer con `images.Service.URLs`; las evidencias dan una URL firmada que vence a los 10 minutos.
- Sin red ni credenciales, el mismo flujo se prueba con `go test -tags integration -run Integration ./internal/images/adapter/http/` (Cloudinary simulado).
