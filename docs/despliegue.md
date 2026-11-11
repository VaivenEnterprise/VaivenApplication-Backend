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

## 3. Utilidades de `cmd/admin` en staging (procedimiento propuesto para T009)

- **Por qué no se corren desde una laptop:** MongoDB y Redis no publican puertos (Art. XIV.4) y la imagen de la API no incluye `cmd/admin`. Hay que correrlas **dentro de la VM**, en la red de Docker del stack.
- **Quién y cuándo:** lo hace `@devops-mcp` por SSH, o una persona con acceso a la VM, después de un despliegue en verde.

1. Compilar el binario para Linux en tu equipo y copiarlo a la VM:
   ```bash
   CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -o /tmp/vaiven-admin ./cmd/admin
   scp -i ~/.ssh/vaiven_azure /tmp/vaiven-admin azureuser@vaiven-staging.westus2.cloudapp.azure.com:/tmp/
   ```
2. En la VM, correrlo en un contenedor de un solo uso, con las mismas variables de la API y en la red del stack:
   ```bash
   sudo docker run --rm --network base_vaiven-net \
     --env-file /opt/vaiven/staging/.env \
     -v /tmp/vaiven-admin:/admin:ro --entrypoint /admin \
     gcr.io/distroless/static-debian12 check-services
   # luego: ... indexes   (debe salir con código 0)
   ```
3. **Seed:** solo en staging (se niega con `APP_ENV=production`).
   1. Crea el archivo de credenciales **en la VM**, con permisos `600`, una contraseña distinta por cuenta y **sin pegarlas en ningún chat**.
   2. Móntalo en el contenedor y apunta `SEED_CREDENTIALS_FILE` a esa ruta. Por ejemplo, con el archivo en `/root/seed-creds.txt`, agrega `-v /root/seed-creds.txt:/creds.txt:ro -e SEED_CREDENTIALS_FILE=/creds.txt` y usa el subcomando `seed`.
   3. Al terminar, guarda el archivo **fuera de la VM**, en tu gestor de contraseñas, y bórralo de la VM.
   4. Antes del seed hay que haber rotado la llave de Cloudinary (P-1 de `docs/seguridad/remediacion-2026-11.md`): el seed sube fotos reales.
4. Borrar `/tmp/vaiven-admin` de la VM al terminar.
5. Anotar en `memoria.md` el resultado (códigos de salida y resumen del seed), sin valores secretos.

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
- **Incidente del 2026-11-11:** el deploy #18 se canceló a mitad. Desde la 01:33 UTC la VM quedó en `Failed (VMStartTimedOut)` y todos los deploys siguientes (#21 a #23) fallaron con `Conflict`. La versión en línea siguió funcionando (`101f255`). A las 05:07 UTC se lanzó `reapply`; **al escribir esto seguía en `Updating`**. Anotar el desenlace en `memoria.md` y aquí.

### `VAIVEN_DEPLOY_RESULT=rolled_back`

La versión nueva arrancó, pero no respondió `/healthz` o `/api/v1/meta` con su versión, y la VM volvió a la anterior. Revisa los registros de arranque de la API en la VM (`docker logs vaiven-staging-api-1`): suele ser una variable faltante en `/opt/vaiven/staging/.env` o un error al conectar a MongoDB o Redis.

### Conexión a Azure que se corta (`ConnectionResetError 10054`)

Es intermitente y viene de la red local. Repite el comando o usa Azure Cloud Shell, que no depende de tu conexión.
