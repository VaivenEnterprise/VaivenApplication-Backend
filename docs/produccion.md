# Producción — guía de operación (spec 013)

> **Estado (2026-11-15):** datos de producción creados (T001). La guía cubre la API (T010), base de datos (T002), imágenes (T003), secretos (T005), respaldos (T006, `docs/respaldos.md`), humo (T007) y operación (T008). Todo corre **sin SSH**, desde Azure Cloud Shell, con `scripts/cloudshell-run.sh` y `az vm run-command`. Solo `devops-mcp`, o una persona con acceso, opera la VM (Art. XIII.4).

## 1. Cómo está armado (Art. XIII.6)

| | Staging | Producción |
|---|---|---|
| URL | `https://vaiven-staging.westus2.cloudapp.azure.com:8443` | `https://vaiven-staging.westus2.cloudapp.azure.com` (443) |
| API (Compose) | `vaiven-staging`, en `/opt/vaiven/staging/` | `vaiven-production`, en `/opt/vaiven/production/` |
| MongoDB y Redis | stack `base` (`deploy/base/docker-compose.yml`) | stack `vaiven-production-data` (`deploy/production/docker-compose.yml`) |
| Red | `base_vaiven-net` | `vaiven-production-net` |
| Volumen de MongoDB | `base_vaiven-mongo-data` | `vaiven-production-mongo-data` |
| Base de datos | `vaiven_staging` | `vaiven_prod` |
| Secretos | `/opt/vaiven/base/.env` y `/opt/vaiven/staging/.env` | `/opt/vaiven/production/data.env` y `/opt/vaiven/production/.env` |
| Carpeta de Cloudinary | `vaiven/staging/` | `vaiven/production/` |
| Despliegue | automático al fusionar en `develop` | `scripts/deploy-production.sh <commit>` desde Cloud Shell; el commit debe estar en `origin/main` (§5) |

Nada se comparte entre entornos: ni proceso, ni volumen, ni red, ni credenciales. Todo vive en la misma VM gratuita, con un límite de memoria por contenedor. El certificado de Let's Encrypt es el mismo, porque es el del único nombre DNS de la IP.

## 2. Crear el entorno (T001) — una sola vez, en la VM

### 2.0 Sin SSH: pasos 1 a 5 con `az vm run-command` (camino usado)

La VM no tiene SSH, así que los pasos 1 a 5 los hace `deploy/production/provision-data.sh` en una sola ejecución de Run Command: contraseñas y keyFile, stack de datos, replica set, usuario `vaiven_app` y `.env` de la API. Este último parte del de staging, pero con MongoDB, Redis y JWT nuevos, la carpeta `vaiven/production` y `DOCS_ENABLED=false`. El script es idempotente y no imprime valores. Desde Cloud Shell (Bash), con `C` = el commit (completo) de `develop` que contiene los tres archivos:

```bash
C=<commit completo>
B=https://raw.githubusercontent.com/VaivenEnterprise/VaivenApplication-Backend/$C/deploy/production
mkdir -p ~/vaiven-prod && cd ~/vaiven-prod
for f in docker-compose.yml init-env.sh provision-data.sh; do curl -fsSL "$B/$f" -o "$f"; done
sha256sum docker-compose.yml init-env.sh provision-data.sh   # comparar con las huellas del PR
{ echo "COMPOSE_B64=$(base64 -w0 docker-compose.yml)"; echo "INIT_B64=$(base64 -w0 init-env.sh)"; cat provision-data.sh; } > payload.sh
az vm run-command invoke -g rg-vaiven -n vm-vaiven --command-id RunShellScript   --scripts @payload.sh --query "value[0].message" -o tsv
```

La salida debe mostrar `mongo: healthy`, `primario: true`, el usuario creado, el `.env` creado y la lista de variables copiadas de staging **solo por nombre**. Si alguna de esas copiadas es un secreto, se cambia por uno propio de producción (CA-02). Luego, seguir con el paso 6.

**Llave de Cloudinary propia (CA-02):** el script copia de staging `CLOUDINARY_API_KEY` y `CLOUDINARY_API_SECRET`. Se reemplazan por una llave propia de producción, creada en la misma cuenta (rol Master Admin), con un *Managed Run Command* y `--protected-parameters`, igual que P-1 en `docs/seguridad/remediacion-2026-11.md` §3. El script se niega si la llave es la de staging y solo escribe si el `ping` a Cloudinary da 200. Hecho el 2026-11-11 con la llave `vaiven-production-2026-11`.

### 2.1 Pasos manuales (referencia, con acceso a la VM)

1. **Contraseñas y keyFile propios:** `sudo deploy/production/init-env.sh`. Crea `/opt/vaiven/production/data.env` y el keyFile; no imprime valores ni toca los de staging.
2. **Levantar MongoDB y Redis de producción:**
   ```bash
   sudo docker compose --env-file /opt/vaiven/production/data.env \
     -f deploy/production/docker-compose.yml up -d
   ```
3. **Replica set**, una vez. `-p` pide la contraseña raíz por teclado, así no queda en el historial:
   ```bash
   sudo docker compose --env-file /opt/vaiven/production/data.env \
     -f deploy/production/docker-compose.yml exec mongo \
     mongosh -u root -p --authenticationDatabase admin \
     --eval 'rs.initiate({_id:"rs0",members:[{_id:0,host:"mongo:27017"}]})'
   ```
4. **Usuario de la aplicación con permisos solo sobre `vaiven_prod`** (RF-02). `passwordPrompt()` pide la contraseña: pega `MONGO_APP_PASSWORD` de `data.env` sin mostrarla:
   ```bash
   sudo docker compose --env-file /opt/vaiven/production/data.env \
     -f deploy/production/docker-compose.yml exec mongo \
     mongosh -u root -p --authenticationDatabase admin --eval \
     'db.getSiblingDB("vaiven_prod").createUser({user:"vaiven_app",pwd:passwordPrompt(),roles:[{role:"readWrite",db:"vaiven_prod"}]})'
   ```
5. **`.env` de la API:** crear `/opt/vaiven/production/.env` (600, dueño `deploy`) a partir de `deploy/production/.env.example`, con secretos **nuevos** (`openssl rand -hex 32` para el JWT, generado en la VM). Debe incluir `VAIVEN_NETWORK=vaiven-production-net`.
6. **Desplegar la API:** `scripts/deploy.sh production`, desde un commit ya fusionado en `main`.
7. **Verificar** (CA-01, CA-07):
   ```bash
   curl -s https://vaiven-staging.westus2.cloudapp.azure.com/readyz      # ready, redis: ok
   curl -s https://vaiven-staging.westus2.cloudapp.azure.com/api/v1/meta # environment: production
   bash scripts/smoke.sh https://vaiven-staging.westus2.cloudapp.azure.com
   ```
8. **Medir la capacidad** (RF-12, CA-08) con los dos entornos encendidos:
   ```bash
   free -m
   docker stats --no-stream --format '{{.Name}} {{.MemUsage}} {{.CPUPerc}}'
   ```
   **Línea base (2026-11-11, solo staging encendido):**

   | | Valor |
   |---|---|
   | RAM total / usada / disponible | 887 / 302 / 584 MB |
   | Swap (`/swapfile`) | 2 GB, 173 MB en uso |
   | `base-mongo-1` | 177 MB de 400 MB |
   | `base-redis-1` | 7 MB de 192 MB |
   | `vaiven-staging-api-1` | 8 MB de 160 MB |
   | Disco `/` | 7,4 GB de 61 GB (13 %); imágenes 1,8 GB, 444 MB recuperables |

   Estimado para producción en reposo: unos 200 MB (similar a staging), así que quedarían unos 380 MB disponibles más el swap. Los límites suman 1,3 GB contra 887 MB de RAM: es una sobreasignación que se apoya en el swap y en que ningún contenedor llega a su límite en reposo. Si la medición con los dos entornos muestra swap sostenido alto u OOM, primero se bajan los límites de staging.

   **Con los datos de producción encendidos (2026-11-11, sin la API de producción):**

   | | Valor |
   |---|---|
   | RAM disponible | 592 MB de 887 MB |
   | Swap | 302 MB de 2 GB en uso |
   | `vaiven-production-data-mongo-1` | 25 MB de 400 MB (recién creado, base vacía) |
   | `vaiven-production-data-redis-1` | 4 MB de 96 MB |
   | `base-mongo-1` / `base-redis-1` / `vaiven-staging-api-1` | 183 / 5 / 10 MB |

   Falta repetir la medición con la API de producción (paso 6) y con datos (CA-08).

   Anotar los valores aquí y en `memoria.md`. Si la memoria libre más el swap no da margen, ajustar los límites de producción o reducir o detener staging. **No** cambiar de talla sin consultar, porque tiene costo (Art. XIII.1).

## 3. Rotación de registros (T004, RF-13)

Todos los contenedores (API, MongoDB y Redis de los dos entornos) usan `json-file` con `max-size: 10m` y `max-file: 3`: como máximo unos 30 MB por contenedor. **Se aplica al recrear el contenedor:**
- La API de staging, en su siguiente despliegue.
- MongoDB y Redis de staging, con `sudo docker compose --env-file /opt/vaiven/.env -f deploy/base/docker-compose.yml up -d`. Es un corte de segundos y no borra datos (el volumen se conserva). **Nunca** usar `down -v`.
- Producción ya nace con la rotación.

**Registro:** 2026-11-16, rotación aplicada a `base-mongo-1` y `base-redis-1` con `bash cloudshell-run.sh <commit> scripts/update-base-stack.sh <commit>` (copia del compose anterior junto al de `/opt/vaiven/base/`). La API de staging la tiene desde su despliegue automático y producción nace con ella.

Revisión semanal del disco (P6, 64 GiB): `df -h /` y `docker system df`.

## 4. Monitor (T004, RF-06)

Better Stack, plan gratuito. Lo da de alta una persona:
- Monitor HTTP a `https://vaiven-staging.westus2.cloudapp.azure.com/healthz`, cada 3 minutos, con aviso por correo.
- Anotar aquí la fecha y quién recibe los avisos, sin contraseñas.

**Registro de alta:**

| Fecha | Monitor | Intervalo | Aviso | Alta por |
|---|---|---|---|---|
| 2026-11-11 | Staging: `https://vaiven-staging.westus2.cloudapp.azure.com:8443/healthz` | 3 min | correo de la cuenta del equipo | BE-B |
| — | Producción: `https://vaiven-staging.westus2.cloudapp.azure.com/healthz` | 3 min | correo | pendiente: después del despliegue (§5) |

Plan gratuito, sin tarjeta ni teléfono (Art. XIII.1). El enlace de invitación de la cuenta no se comparte en el repositorio ni en chats: a los demás integrantes se les invita por correo desde *Team members*.

## 5. API de producción: despliegue y reversión (T010)

Producción no tiene workflow. Se despliega desde Cloud Shell con `scripts/deploy-production.sh`, que:
1. comprueba con la API pública de GitHub que el commit ya está en `origin/main` (si no, se niega);
2. usa la imagen `vaiven-api:<commit corto>` ya cargada en la VM, la misma que se probó en staging (no compila otra);
3. ejecuta `deploy/remote-deploy.sh` de ese commit con `ENV_NAME=production` y `API_PORT=443`;
4. espera `/healthz` y `/api/v1/meta` con esa versión por HTTPS sin `-k`; si falla, vuelve a la imagen anterior.

**Antes de desplegar:** respaldo de producción (`docs/respaldos.md` §2) y el puerto 443 abierto en el NSG de la VM (si falta, una regla de entrada TCP 443, sin costo).

```bash
C=<commit de develop que corre en staging y ya está en main>
curl -fsSLO "https://raw.githubusercontent.com/VaivenEnterprise/VaivenApplication-Backend/$C/scripts/deploy-production.sh"
bash deploy-production.sh "$C"            # termina con «Resultado: deployed»
bash deploy-production.sh --rollback      # reversión: vuelve a previous-tag
```

**Verificación (CA-01, CA-07):**

```bash
curl -s https://vaiven-staging.westus2.cloudapp.azure.com/readyz        # ready, redis: ok
curl -s https://vaiven-staging.westus2.cloudapp.azure.com/api/v1/meta   # environment: production
curl -fsS https://vaiven-staging.westus2.cloudapp.azure.com:8443/readyz # staging sigue bien
SMOKE_EXPECT_ENV=production bash scripts/smoke.sh https://vaiven-staging.westus2.cloudapp.azure.com
```

**URL para el móvil (mobile 013-T001):** `https://vaiven-staging.westus2.cloudapp.azure.com` (puerto 443). Staging sigue en `:8443`.

**Imágenes en la VM:** la limpieza de cada despliegue conserva la imagen actual y la anterior **de cada entorno** (`/opt/vaiven/*/current-tag` y `previous-tag`): desplegar staging no borra la reversión de producción.

## 6. Base de datos de producción (T002)

`vaiven_prod`, con el usuario `vaiven_app` (`readWrite` solo en esa base) y contraseña propia (CA-02, T001). La API crea los índices al arrancar. Comprobación:

```bash
bash cloudshell-run.sh <commit> scripts/vm-admin.sh production indexes    # código 0
```

## 7. Imágenes de producción (T003)

`CLOUDINARY_BASE_FOLDER=vaiven/production` y llave propia `vaiven-production-2026-11`, con el secreto solo en `/opt/vaiven/production/.env`. Comprobación:

```bash
bash cloudshell-run.sh <commit> scripts/vm-admin.sh production check-services   # MongoDB, Redis y Cloudinary en OK
bash cloudshell-run.sh <commit> scripts/vm-admin.sh production upload-test      # sube con firma real, verifica y borra
```

`upload-test` no escribe en MongoDB; por eso se permite en producción.

## 8. Secretos, administrador y cuenta de revisión (T005)

| Secreto | Dónde vive | Cómo se rota |
|---|---|---|
| `MONGO_ROOT_PASSWORD`, `MONGO_APP_PASSWORD`, `REDIS_PASSWORD` | `/opt/vaiven/production/data.env` (600) | Como en `docs/servicios.md` para staging, contra `vaiven-production-data-mongo-1` y `vaiven_prod` |
| `JWT_ACCESS_SECRET` | `/opt/vaiven/production/.env` (600) | Igual que P-2 (`docs/seguridad/remediacion-2026-11.md` §4, **probado en staging el 2026-11-11**): secreto nuevo generado en la VM con `openssl rand -hex 32` y la API recreada con `docker compose up -d --force-recreate api` en `/opt/vaiven/production` (`VAIVEN_ENV=production API_PORT=443 API_IMAGE_TAG=$(cat current-tag)`) |
| `CLOUDINARY_API_KEY` y `CLOUDINARY_API_SECRET` | `/opt/vaiven/production/.env` | Igual que P-1: llave nueva aplicada con `--protected-parameters` y ping 200 antes de escribir; luego se revoca la vieja |
| Frase de respaldos | `/opt/vaiven/backup/backup.key` y gestor de contraseñas | `docs/respaldos.md` §1 (los respaldos viejos necesitan la frase vieja) |

Ninguno es igual a los de staging (CA-02). **Sin `seed` en producción** (RF-05): `cmd/admin seed` se niega con `APP_ENV=production`.

**Cuenta de administrador.** `create-admin` lee correo y contraseña por la entrada estándar; desde Cloud Shell viajan como parámetros protegidos (procedimiento en el mensaje de operación del 2026-11-15 en `memoria.md`): `read -s` en Cloud Shell, `az vm run-command create --protected-parameters`, `docker run -i … --entrypoint /admin vaiven-api:<current-tag> create-admin`, y luego se borra el run command.

**Cuenta de revisión de Google Play.** Se registra por la API pública de producción con datos ficticios (nombre «Revisión Google Play», un correo del equipo dedicado a eso); su contraseña va solo a Play Console y al gestor de contraseñas. No publica objetos reales.

## 9. Operación diaria, registros y reversión (T008)

- **Despliegue:** §5. **Reversión:** `bash deploy-production.sh --rollback`; termina con `Resultado: rolled_back` y `/api/v1/meta` vuelve a la versión anterior. Luego se vuelve a desplegar la buena.
- **Registros sin datos personales (CA-06):** tras una sesión completa (registro, publicar, buscar, chat), contar en `docker logs --since 2h vaiven-production-api-1` las apariciones de `password`, `Bearer `, `refreshToken`, `api_secret` y correos: todas deben ser 0.
- **Límite de tasa en Redis (RF-11):** 130 peticiones seguidas a `/api/v1/meta` desde una IP empiezan a dar 429 pasadas las 120 por minuto, y `/readyz` sigue con `redis: ok`.
- **Memoria y disco:** `free -m`, `docker stats --no-stream` y `df -h /` cada semana; límites en §2.8.
- **Reinicio de la VM:** `restart: unless-stopped` en todos los contenedores y Docker habilitado al arranque; tras `az vm restart`, los dos entornos vuelven solos (CA-05).
- **Crédito de Azure:** la cuenta gratuita vence; revisar *Cost Management* cada mes (RNF-02).

## 10. Lista de comprobación de producción (T009)

| Punto | Evidencia | Estado |
|---|---|---|
| Secretos propios (CA-02) | §8 y T001 | |
| Índices (T002) | `vm-admin.sh production indexes` = 0 | |
| Sin seed (RF-05) | `seed` se niega en producción | |
| Monitor (RF-06) | §4 | |
| Respaldo y restauración (CA-04) | `docs/respaldos.md` §5 | |
| Humo (CA-03) | §5 y T007 | |
| Registros (CA-06) | §9 | |
| HTTPS 443 y 8443 (CA-07) | §5 | |
| Capacidad (CA-08) | §2.8 | |
| Disco | §3 | |
| p95 < 800 ms (RNF-01) | `tests/load` contra producción | |
| Costo cero (RNF-02) | *Cost Management* | |

## 11. IA generativa (spec 014)

Dos funciones, las dos **apagadas por defecto**: sugerencias al publicar (`AI_ENABLED`, parte A) y asistente de proyectos (`ASSISTANT_ENABLED`, parte B). `GET /api/v1/meta` dice cuáles están encendidas (`features`).

**Proveedor (RF-03, T016).** **Groq**, con su capa gratuita y sin tarjeta (costo cero, Art. XIII.1), mediante el adaptador compatible con OpenAI (`AI_PROVIDER=openai`, `AI_BASE_URL=https://api.groq.com/openai/v1`, `AI_MODEL=openai/gpt-oss-120b`). **Groq no ofrece hoy (17 nov) un modelo con visión** para la clave del proyecto, así que en staging se enciende **solo el asistente** (`--assistant-only`); las sugerencias por foto quedan apagadas hasta que haya uno, sin tocar código. Verificado con una petición real: JSON en español, `finish_reason: stop`, ~1 060 tokens con el razonamiento (por eso el límite de salida es 4 096). Gemini queda como alternativa (`AI_PROVIDER=gemini`): el 17 nov su nivel gratuito no estaba disponible para la cuenta del proyecto y pagar no está permitido. En local se puede usar Ollama con el mismo adaptador (`AI_PROVIDER=ollama`). Cambiar de proveedor es cambiar variables.

**Clave.** Se crea en **console.groq.com** (*API Keys → Create API Key*) con la cuenta del proyecto, **una por entorno**: la de producción es distinta de la de staging (CA-02). Se guarda en el gestor de contraseñas del equipo y nunca en el repositorio ni en chats. Para ponerla en la VM, desde Cloud Shell (Bash):

```bash
C=<commit que tenga el script>
curl -fsSLO "https://raw.githubusercontent.com/VaivenEnterprise/VaivenApplication-Backend/$C/scripts/set-ai-key.sh"
bash set-ai-key.sh staging --assistant-only  # valida con Groq, guarda la clave y enciende solo B
bash set-ai-key.sh staging --enable   # enciende A y B (cuando haya un modelo con visión: AI_MODEL=<modelo>)
bash set-ai-key.sh staging --disable  # apaga las dos (la clave se conserva)
PROVIDER=gemini bash set-ai-key.sh staging --enable   # alternativa, si hay nivel gratuito
```

El script valida la clave y hace una generación mínima con el modelo real **antes** de tocar la VM; si el proveedor pide pago (429/403) o el modelo no existe (404), se detiene. La envía como parámetro protegido, se niega si coincide con la del otro entorno y borra el run command. El cambio se aplica **al recrear la API**: en staging, con el siguiente despliegue (o «Run workflow → deploy»); en producción, con `scripts/deploy-production.sh`.

**Apagado de emergencia.** `bash set-ai-key.sh <entorno> --disable` y recrear la API. La app sigue sin IA (`features` en `false`; las rutas responden 503).

**Privacidad (RF-07 y RF-18).**
- Parte A: al proveedor solo viaja la **miniatura** de la foto, cuando la persona pide la sugerencia.
- Parte B: solo viaja el **texto** de la conversación (máximo 6 turnos). La **ubicación nunca sale del backend**.
- No se guardan fotos ni respuestas del modelo; la conversación dura 24 h y se puede borrar. Los registros no llevan fotos, mensajes ni respuestas.
- El aviso de privacidad (landing) y «Seguridad de datos» de Play deben nombrar al proveedor (Groq) y lo que hace con los datos según sus términos vigentes (mobile 014-T006). La app pide no escribir datos personales en el chat.

**Producción.** Según el ROADMAP, la IA sale como v1.1: se enciende en producción solo cuando el equipo lo decida y la versión de la app con IA esté publicada.
