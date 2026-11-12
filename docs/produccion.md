# Producción — guía de operación (spec 013)

> **Estado (2026-11-11):** borrador de T001 y T004. Los archivos ya están en el repositorio; **el entorno todavía no existe en la VM**. Requisito previo: P-1 (rotar la llave de Cloudinary) y P-2 (rotar el JWT de staging), de `docs/seguridad/remediacion-2026-11.md`. Las secciones de base de datos, imágenes, secretos, respaldo y operación se completan en T002, T003 y T005 a T008. Solo `devops-mcp`, o una persona con acceso, opera la VM (Art. XIII.4).

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
| Despliegue | automático al fusionar en `develop` | `scripts/deploy.sh production` desde `origin/main`, por `devops-mcp` |

Nada se comparte entre entornos: ni proceso, ni volumen, ni red, ni credenciales. Todo vive en la misma VM gratuita, con un límite de memoria por contenedor. El certificado de Let's Encrypt es el mismo, porque es el del único nombre DNS de la IP.

## 2. Crear el entorno (T001) — una sola vez, en la VM

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

   Anotar los valores aquí y en `memoria.md`. Si la memoria libre más el swap no da margen, ajustar los límites de producción o reducir o detener staging. **No** cambiar de talla sin consultar, porque tiene costo (Art. XIII.1).

## 3. Rotación de registros (T004, RF-13)

Todos los contenedores (API, MongoDB y Redis de los dos entornos) usan `json-file` con `max-size: 10m` y `max-file: 3`: como máximo unos 30 MB por contenedor. **Se aplica al recrear el contenedor:**
- La API de staging, en su siguiente despliegue.
- MongoDB y Redis de staging, con `sudo docker compose --env-file /opt/vaiven/.env -f deploy/base/docker-compose.yml up -d`. Es un corte de segundos y no borra datos (el volumen se conserva). **Nunca** usar `down -v`.
- Producción ya nace con la rotación.

Revisión semanal del disco (P6, 64 GiB): `df -h /` y `docker system df`.

## 4. Monitor (T004, RF-06)

Better Stack, plan gratuito. Lo da de alta una persona:
- Monitor HTTP a `https://vaiven-staging.westus2.cloudapp.azure.com/healthz`, cada 3 minutos, con aviso por correo.
- Anotar aquí la fecha y quién recibe los avisos, sin contraseñas.

**Registro de alta:**

| Fecha | Monitor | Intervalo | Aviso | Alta por |
|---|---|---|---|---|
| 2026-11-11 | Staging: `https://vaiven-staging.westus2.cloudapp.azure.com:8443/healthz` | 3 min | correo de la cuenta del equipo | BE-B |
| — | Producción: `https://vaiven-staging.westus2.cloudapp.azure.com/healthz` | 3 min | correo | pendiente: al crear el entorno (§2) |

Plan gratuito, sin tarjeta ni teléfono (Art. XIII.1). El enlace de invitación de la cuenta no se comparte en el repositorio ni en chats: a los demás integrantes se les invita por correo desde *Team members*.

## 5. Pendiente de completar

- Base de datos e índices (T002).
- Imágenes (T003).
- Secretos, administrador y cuenta de revisión (T005).
- Respaldo y restauración (T006).
- Prueba de humo (T007).
- Operación diaria, reversión y registros (T008).
- Lista final firmada (T009).
