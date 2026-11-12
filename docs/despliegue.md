# Despliegue de la API en staging

> Spec 012 (RF-07, RF-09; tareas T009 y T010). Guía de operación del despliegue. Para no duplicar, el detalle de la infraestructura está en `docs/servicios.md` §7 y las reglas de ramas, revisores y CI, en `docs/devsecops.md`.
>
> **Estado (2026-11-11):** las secciones 1, 2 y 5 describen lo que ya funciona. La sección 3 es un **procedimiento propuesto** que se valida al ejecutar T009. Solo `@devops-mcp` (o una persona con `az login`) opera la VM (Art. XIII).

## 1. Cómo llega una versión a staging

1. Se fusiona un PR en `develop`.
2. La CI corre sobre `develop` (Policy, Quality, Secrets, CodeQL).
3. Si la CI pasa, `deploy-staging.yml` (disparado por `workflow_run`):
   1. Publica la imagen `ghcr.io/vaivenenterprise/vaiven-api:<commit corto>`. Si ya existe, no la recompila.
   2. Inicia sesión en Azure por **OIDC**, sin secretos guardados, con la identidad `gh-vaiven-deploy-staging`. Su rol solo permite `virtualMachines/read` y `runCommand/action` sobre `vm-vaiven`.
   3. Ejecuta `deploy/remote-deploy.sh` **dentro de la VM** con `az vm run-command invoke`, sin SSH.
   4. Corre las pruebas de humo por HTTPS desde el runner.
4. En la VM, `remote-deploy.sh` descarga la imagen, arranca la versión nueva y comprueba `/healthz` y `/api/v1/meta`. Si fallan, **vuelve sola a la versión anterior**. Su última línea siempre es `VAIVEN_DEPLOY_RESULT=deployed | rolled_back | failed`, y el workflow la lee porque Run Command no propaga el código de salida.
5. Los despliegues se ponen en fila (grupo `deploy-staging`, sin cancelar el que corre).

**Despliegue manual** (por ejemplo, la tarea «Despliegue» de una spec antes de fusionar): `scripts/deploy.sh staging`, o `scripts/deploy.sh staging --allow-unmerged` con la rama publicada. Ver `docs/servicios.md` §7.

**Volver a la versión anterior:**
- Desde *Actions* → *Deploy staging* → *Run workflow* con `rollback`.
- O con `scripts/deploy.sh staging --rollback`.

## 2. Verificar un despliegue

```bash
curl -s https://vaiven-staging.westus2.cloudapp.azure.com:8443/healthz    # {"status":"ok"}
curl -s https://vaiven-staging.westus2.cloudapp.azure.com:8443/readyz     # {"status":"ready","redis":"ok"}
curl -s https://vaiven-staging.westus2.cloudapp.azure.com:8443/api/v1/meta # "version": <commit corto esperado>
bash scripts/smoke.sh https://vaiven-staging.westus2.cloudapp.azure.com:8443
```

`version` en `/api/v1/meta` debe ser el commit desplegado. Si sigue el anterior, el despliegue no se aplicó o volvió atrás: revisar el resumen del run («Resultado en la VM»).

## 3. Utilidades de `cmd/admin` en staging (T009)

- **Por qué en la VM:** MongoDB y Redis no publican puertos (Art. XIV.4), así que no se alcanzan desde una laptop.
- **Cómo:** la imagen de la API trae también `/admin` (ver `Dockerfile`). Se corre en un contenedor de un solo uso con `--entrypoint /admin`, la misma imagen que corre la API, las mismas variables y la red del stack. **No hace falta SSH ni copiar binarios:** basta con `az vm run-command` desde Cloud Shell.
- **Quién y cuándo:** `@devops-mcp` o una persona con acceso a Azure, después de un despliegue en verde de una versión que ya incluya `/admin`.
- `APP_ENV`, `TLS_CERT_FILE` y `TLS_KEY_FILE` no están en el `.env` (los pone `deploy/docker-compose.yml`), así que se pasan con `-e`. La configuración solo exige que existan; `/admin` no abre los certificados.

### 3.1 `check-services` e `indexes`

En Cloud Shell (Bash):

```bash
az vm run-command invoke -g rg-vaiven -n vm-vaiven --command-id RunShellScript --query "value[0].message" -o tsv --scripts '
IMG=$(docker inspect -f "{{.Config.Image}}" vaiven-staging-api-1)
adm() { docker run --rm --network base_vaiven-net --env-file /opt/vaiven/staging/.env   -e APP_ENV=staging -e TLS_CERT_FILE=/certs/fullchain.pem -e TLS_KEY_FILE=/certs/privkey.pem   --entrypoint /admin "$IMG" "$@"; }
echo "imagen: $IMG"
adm check-services; echo "check-services exit=$?"
adm indexes; echo "indexes exit=$?"'
```

Esperado: MongoDB, Redis y Cloudinary en `OK` (`exit=0`); `indexes exit=0`.

### 3.2 `seed` (solo staging; se niega con `APP_ENV=production`)

Las contraseñas de las 7 cuentas (`admin.demo@` y `ana`…`fer.demo@vaiven.example`) viven solo en el archivo de quien presenta, **nunca en el repositorio ni en chats**.

1. **Generar el archivo en Cloud Shell** (es el almacenamiento propio de quien opera). Ejecútalo una sola vez; si ya existe, reutilízalo para no cambiar contraseñas:
   ```bash
   umask 077; F=~/vaiven-seed-creds.txt
   [ -e "$F" ] || for e in admin ana beto carla diego elena fer; do
     printf '%s.demo@vaiven.example=%s
' "$e" "$(openssl rand -base64 18)"; done > "$F"
   wc -l "$F"   # 7
   ```
2. **Correr el seed dos veces** (la segunda no debe crear nada, CA-02). El archivo viaja como parámetro protegido (Azure lo guarda cifrado y no lo devuelve), se escribe con permisos 600, se monta de solo lectura y se borra al terminar:
   ```bash
   az vm run-command create -g rg-vaiven --vm-name vm-vaiven --location westus2      --name vaiven-seed --async-execution false --timeout-in-seconds 1500      --protected-parameters SEED_B64="$(base64 -w0 ~/vaiven-seed-creds.txt)"      --script '
   set -u
   C=$(mktemp); chmod 644 "$C"; printf "%s" "$SEED_B64" | base64 -d > "$C"; chmod 444 "$C"
   IMG=$(docker inspect -f "{{.Config.Image}}" vaiven-staging-api-1)
   adm() { docker run --rm --network base_vaiven-net --env-file /opt/vaiven/staging/.env      -e APP_ENV=staging -e TLS_CERT_FILE=/certs/fullchain.pem -e TLS_KEY_FILE=/certs/privkey.pem      -v "$C":/creds.txt:ro -e SEED_CREDENTIALS_FILE=/creds.txt --entrypoint /admin "$IMG" "$@"; }
   adm seed; echo "seed 1 exit=$?"
   adm seed; echo "seed 2 exit=$?"
   rm -f "$C"; echo "credenciales borradas de la VM"'
   az vm run-command show -g rg-vaiven --vm-name vm-vaiven --name vaiven-seed --instance-view      --query "instanceView.{estado:executionState,salida:output,error:error}"
   az vm run-command delete -g rg-vaiven --vm-name vm-vaiven --name vaiven-seed --yes
   ```
   El archivo temporal es legible por el usuario `nonroot` del contenedor (444) solo mientras dura el comando; vive en `/tmp` de la VM y se borra al final.
3. **Guardar las contraseñas** de `~/vaiven-seed-creds.txt` en el gestor de contraseñas de quien presenta (descárgalo desde Cloud Shell: *Manage files → Download*). Se puede dejar en Cloud Shell, que es privado, pero nunca subirlo al repositorio.
4. Anotar en `memoria.md` los códigos de salida y el resumen del seed, sin valores.

## 4. Lista de comprobación de T009 (RF-07)

- [ ] La CI y el despliegue de `develop` en verde; `/api/v1/meta` con el commit esperado.
- [ ] `/readyz` con `ready` y `redis: ok`.
- [ ] `check-services`: MongoDB, Redis y Cloudinary en `OK`.
- [ ] `indexes`: código 0 (34 índices correctos).
- [ ] `seed`: ejecutado una vez, y una segunda ejecución sin cambios (CA-02).
- [ ] `scripts/smoke.sh` en verde.
- [ ] Resultado anotado en `memoria.md`.

## 5. Problemas conocidos y qué hacer

### `ERROR: (Conflict) Run command extension execution is in progress`

Azure solo ejecuta un Run Command a la vez por VM, y no lo acepta si la VM tiene una operación de plataforma pendiente.

- **No canceles un deploy mientras está en «Deploy to VM».** Cancelar el workflow no detiene el comando en la VM, que sigue ocupada.
- **No relances en bucle:** cada intento falla igual en segundos.
- Revisa el estado de la VM (es de solo lectura):
  ```bash
  az vm show -g rg-vaiven -n vm-vaiven --query provisioningState -o tsv
  az vm get-instance-view -g rg-vaiven -n vm-vaiven --query "instanceView.statuses" -o json
  ```
  - `Succeeded`: espera a que termine el comando anterior y relanza **una** vez.
  - `Failed` (por ejemplo `VMStartTimedOut`): ejecuta `az vm reapply -g rg-vaiven -n vm-vaiven`, mejor desde Azure Cloud Shell, y espera a `Succeeded`. Si sigue en `Updating` o `Failed` mucho tiempo, reinicia la VM con `az vm restart -g rg-vaiven -n vm-vaiven`. Staging queda caído 1–3 minutos; los contenedores vuelven solos porque tienen `restart: unless-stopped`.
  - Último recurso, porque apaga la VM más tiempo y la mueve de servidor: **Redeploy** desde el portal.
- **Incidente del 2026-11-11:** el deploy #18 se canceló a mitad. Desde la 01:33 UTC la VM quedó en `Failed (VMStartTimedOut)` y todos los deploys siguientes (#21 a #23) fallaron con `Conflict`. La versión en línea siguió funcionando (`101f255`). El `reapply` (05:07 UTC) y el `restart` (05:48 UTC) también terminaron en `VMStartTimedOut`, lo que apunta a un problema del servidor físico de Azure. La VM se recuperó sola a las 06:31 UTC (`Succeeded`, `running`), sin llegar a usar Redeploy, y el deploy #24 (`a56c642`) salió bien. **Lección:** esperar a que cada operación de Azure termine antes de lanzar la siguiente.

### La CI de `develop` falla en `govulncheck` sin que el código haya cambiado

La base de vulnerabilidades de Go se actualiza sola. Si aparece una vulnerabilidad nueva en la biblioteca estándar, la CI de `develop` falla, el despliegue se salta (`skipped`) y staging se queda en la versión anterior. Remedio: subir la versión en la línea `toolchain` de `go.mod` y en `ARG GO_VERSION` del `Dockerfile` (deben coincidir). Así pasó el 2026-11-11 con GO-2026-6859 (Go 1.26.10 → 1.26.11).

### `VAIVEN_DEPLOY_RESULT=rolled_back`

La versión nueva arrancó, pero no respondió `/healthz` o `/api/v1/meta` con su versión, y la VM volvió a la anterior. Revisa los registros de arranque de la API en la VM (`docker logs vaiven-staging-api-1`): suele ser una variable faltante en `/opt/vaiven/staging/.env` o un error al conectar a MongoDB o Redis.

### Conexión a Azure que se corta (`ConnectionResetError 10054`)

Es intermitente y viene de la red local. Repite el comando o usa Azure Cloud Shell, que no depende de tu conexión.
