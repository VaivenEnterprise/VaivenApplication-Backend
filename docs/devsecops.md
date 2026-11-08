# DevOps y DevSecOps

Solicitud del responsable: integrar CI, seguridad y validacion de ramas y PRs en los tres repositorios (2026-11-02). Esta integracion es independiente de las specs funcionales y no cambia la constitucion.

## Ramas y revisores

- Cambios en tipo/SPEC-01-T001 (feat, feature, fix, bugfix, hotfix, chore, ci, docs, refactor, test, perf, security, release, build o revert) hacia develop.
- Publicaciones de develop hacia main, directas o mediante una rama intermedia release/SPEC-01-T001 creada desde develop; urgencias en hotfix/SPEC-01-T001 hacia main. Despues de un release o hotfix, sincronizacion de main hacia develop.
- Titulos libres; completar Resumen, Validación y Seguridad en el PR.
- Revisores solicitados: @JesusTrz y @JosephYanez27. JesusTrz tiene escritura; JosephYanez27 solo lectura al preparar esta configuracion y su aprobacion aun no cuenta como obligatoria. Un administrador debe concederle escritura antes de agregarlo a CODEOWNERS.
- Una aprobacion de un code owner distinto al autor, aprobacion del ultimo cambio, conversaciones resueltas y rama actualizada. Cambios nuevos invalidan aprobaciones anteriores.

## Controles

CI corre en PRs, push a main/develop/spec/**, manualmente y semanalmente. Required checks agrega Policy, Quality, Secrets y CodeQL; falla si un control requerido falla o se cancela. CodeQL solo se omite cuando no hay aplicacion inicializada.

Policy verifica nombres, destino, secciones del PR y archivos sensibles. Gitleaks examina el historial alcanzable completo y los archivos actuales, con salida redactada, binario versionado y SHA-256 verificado. Las acciones se fijan por commit y Dependabot propone actualizaciones hacia develop. El token es de lectura salvo la carga de resultados de CodeQL. No se usan secretos de despliegue en PRs.

Al existir go.mod: gofmt, go vet, golangci-lint v2.15.0, pruebas con race detector, cobertura minima del 80% en domain/usecase, pruebas de integracion con etiqueta integration sobre Docker del runner, go build y govulncheck versionado. Las pruebas testcontainers deben crear su replica set MongoDB; no se usa la base de produccion.

Mientras solo exista documentacion, Quality informa que no aplican las pruebas de aplicacion; los controles de politica y secretos siguen siendo reales. Agregar codigo sin su manifiesto/verificador hace fallar CI. Dependabot para gomod empezara a trabajar cuando exista su manifiesto.

## Activacion administrativa pendiente

Los archivos de este PR no activan por si solos proteccion de ramas ni ajustes de seguridad. La conexion usada no expone administracion de repositorios.

1. Revisar y fusionar este PR en develop con sus comprobaciones en verde; promover los mismos archivos a main mediante PR develop -> main.
2. Conceder escritura a JosephYanez27 si debe ser revisor obligatorio y agregarlo a la linea de CODEOWNERS. Si el autor es JesusTrz, necesitara a JosephYanez27 o al propietario VaivenEnterprise como code owner habilitado para aprobar.
3. En Settings > Rules > Rulesets > New ruleset > Import a ruleset, importar .github/rulesets/protected-branches.json. Revisar que ambas ramas tienen el workflow antes de activarlo. Requiere Required checks proveniente de GitHub Actions, una aprobacion de code owner, ultimo cambio aprobado y conversaciones resueltas; bloquea eliminaciones, force push y push directo, sin excepciones.
4. En Settings > Code security, habilitar Dependabot alerts/security updates, secret scanning, push protection y private vulnerability reporting cuando esten disponibles. No activar CodeQL default setup porque este PR ya define advanced setup.
5. En Actions > General: token de lectura, no permitir que Actions apruebe PRs, y requerir aprobacion de workflows para contribuciones externas. Revisar ejecuciones antes de aprobar codigo externo.

## Despliegue y recuperacion

Backend y landing conservan el despliegue por MCP a Render; movil conserva Expo Go y la publicacion de la spec 013. Este cambio no crea servicios ni publica produccion. Antes de desplegar se exige CI en verde, aprobacion humana y las verificaciones de cada spec. Render no debe publicar automaticamente cada push sin estos controles. La cuenta administradora debe revisar esa configuracion cuando conecte Render al origen.

Para volver a una version anterior: PR de revert, mismos controles y despliegue por MCP del commit aprobado. No usar force push. No hay credenciales nuevas ni un workflow con acceso a produccion.

CodeQL exporta SARIF y el workflow bloquea hallazgos con security-severity >= 7 o nivel error. No basta con que la carga de resultados termine correctamente.

## Nombres de ramas con trazabilidad

Formato para ramas de tareas: `tipo/SPEC-NN-TNNN`. SPEC requiere al menos dos digitos; la tarea al menos tres; se admiten numeros mayores. Ejemplo: `chore/SPEC-01-T001`. No se permiten sufijos ni segmentos adicionales. La validacion comprueba el formato, no la existencia del ticket en Jira.

Se conservan las promociones develop -> main, la sincronizacion main -> develop y las ramas automaticas dependabot/* solo para PRs del bot autentico hacia develop. La regla se ejecuta en Policy; para bloquear la fusion se debe exigir Required checks en la proteccion de ramas. No impide crear una rama en GitHub.


## Trabajo por spec completa

Crear `spec/SPEC-02` desde `develop`. No lleva sufijo de tarea. SCRUM y SPEC requieren al menos dos digitos, sin texto adicional.

Implementar las tareas en esa rama, una a la vez, o crear ramas de tarea desde ella y abrir sus PRs hacia la rama de spec. Por ejemplo, `feat/SPEC-02-T001` -> `spec/SPEC-02`. El segmento opcional SCRUM-NN- puede variar por tarea; el numero SPEC debe coincidir (02 y 002 son equivalentes). Los titulos de PR son libres; Resumen, Validación y Seguridad siguen siendo obligatorios.

Al completar la spec: PR de la rama de spec hacia `develop`. Para actualizarla: PR `develop` -> rama de spec. No se permite spec -> main ni integrar tareas de otra SPEC. Se conserva el flujo tarea -> develop y la promocion develop -> main.

CI se ejecuta en push y PR hacia `spec/**`, ademas de main/develop. Primero integrar esta configuracion en develop y despues crear las ramas de spec. Para una spec ya creada, incorporar estos archivos antes de usar el nuevo flujo.

Las protecciones administrativas no se activan con estos archivos. Si se quiere exigir revision y Required checks tambien en specs, un administrador debe incluir las ramas `spec/**` en un ruleset activo. Sin esa configuracion, los checks se ejecutan pero no impiden por si solos una fusion.

## Ticket SCRUM opcional

Formatos: `spec/SPEC-02` para una spec completa y `feat/SPEC-02-T001` para una tarea (con los tipos existentes). Tambien se aceptan `spec/SCRUM-01-SPEC-02` y `feat/SCRUM-15-SPEC-02-T001`. Si se incluye SCRUM, conserva mayusculas y al menos dos digitos. SPEC sigue siendo obligatorio con al menos dos digitos; T sigue siendo obligatorio en ramas de tarea con al menos tres. Los destinos permitidos, las revisiones y los controles de seguridad no cambian.

## Ramas por fase

Tambien se permite `tipo/SPEC-NN-Fase-N`, por ejemplo `feat/SPEC-02-Fase-3`, con los mismos tipos de cambio que las tareas. `SCRUM-NN-` sigue siendo opcional: `feat/SCRUM-01-SPEC-02-Fase-3`. Se escribe `Fase` exactamente asi y su numero lleva uno o mas digitos (incluido 0 si el plan tiene fase cero). SPEC mantiene al menos dos digitos.

Crear la rama de fase desde la rama de spec y abrir su PR hacia esa misma SPEC; tambien se permite fase -> develop. No se permite fase -> main, ni fase -> otra SPEC. Las ramas de fase no son destinos de PR de tareas: implementar las tareas de la fase en ella y conservar el formato por tarea para los flujos existentes. Los titulos de PR siguen siendo libres y se mantienen las secciones de evidencia y los controles de seguridad.

Las ramas de spec existentes deben incorporar esta actualizacion desde develop para validar PRs de fase con la nueva politica.

## Despliegue continuo a staging (spec 017)

> **Apagado hasta aprobar la enmienda** `docs/enmienda-2.0.0-propuesta.md` (choca con los Art. VII.4, XIII.2, XIII.4, XIII.5, XIII.7 y XV.7). Fusionar el workflow no despliega nada: el disparo automático exige la variable de repositorio `STAGING_AUTODEPLOY=true`. Esta sección reemplaza, para el backend en staging, lo dicho arriba en «Despliegue y recuperacion».

Workflow `.github/workflows/deploy-staging.yml` y script de VM `deploy/remote-deploy.sh`:

| Job | Qué hace | Permisos |
|---|---|---|
| Prepare | Corre cuando `CI` termina en verde para un push a `develop` de este repositorio (`workflow_run`), o a mano solo sobre `develop` (comprueba que el commit tiene CI en verde). Omite un commit si `develop` ya avanzó (lo cubre el despliegue del más nuevo) | `actions: read`, `contents: read` |
| Build image | `docker build --platform linux/amd64 --build-arg APP_VERSION=<commit corto>` con el `Dockerfile` sin cambios; push a `ghcr.io/vaivenenterprise/vaiven-api:<commit corto>`. Si la etiqueta ya existe, no recompila | `packages: write` |
| Deploy to VM | Entorno `staging`; `azure/login` con OIDC; `az vm run-command invoke … --scripts @archivo` ejecuta `deploy/remote-deploy.sh` como root en la VM, sin SSH | `id-token: write`, `packages: read` |
| Smoke (HTTPS) | Desde el runner: `/healthz` (`ok`), `/readyz` (`ready`) y `/api/v1/meta` (`staging` y versión = commit) | ninguno |

`concurrency: deploy-staging` sin cancelación: un despliegue a la vez y nunca se corta uno a medias.

**Por qué `workflow_run` y no `needs`:** `needs` obligaría a meter el despliegue dentro de `ci.yml` (y a darle a toda la CI permisos de Azure y de paquetes). Con `workflow_run` el despliegue vive aparte, espera el resultado completo de CI (incluido *Required checks*), corre con el workflow de la rama por defecto y solo para pushes del propio repositorio (nunca para PR de forks).

**En la VM** (`deploy/remote-deploy.sh`, mismos pasos que `scripts/deploy.sh`): descarga la imagen con el token del job (`docker login --password-stdin` en un `DOCKER_CONFIG` temporal), la etiqueta `vaiven-api:<tag>`, instala `deploy/docker-compose.yml` y el hook de certbot, rota `current-tag`/`previous-tag` en `/opt/vaiven/staging` (sin pisar `previous-tag` al repetir el mismo commit), `docker compose up -d --remove-orphans api`, espera hasta 60 s `/healthz` y la versión en `/api/v1/meta` por HTTPS **sin `-k`** (`curl --resolve` a 127.0.0.1 valida el certificado con el nombre real) y, si falla, vuelve a `previous-tag` y restaura las etiquetas. Solo conserva la imagen actual y la anterior. Run Command no propaga el código de salida: el script termina siempre con `VAIVEN_DEPLOY_RESULT=deployed|rolled_back|failed` y el workflow falla salvo con `deployed` (o `rolled_back` en modo rollback).

**Credencial para descargar la imagen.** Se usa el `GITHUB_TOKEN` del job *Deploy to VM* (`packages: read`), que caduca al terminar el job. Alternativa descartada: un token de solo lectura guardado en la VM (vive meses, un PAT clásico con `read:packages` lee todos los paquetes de la cuenta y hay que rotarlo a mano). Riesgo residual del token del job: viaja dentro del script (archivo 600 en el runner, `--scripts @archivo`, nunca en argumentos ni en el log) y queda una copia del script en `/var/lib/waagent/run-command/` de la VM (solo root) con el token ya caducado. Como el repositorio es público, el paquete puede hacerse público y entonces no haría falta ningún token.

**Volver atrás a mano:** *Actions → Deploy staging → Run workflow* (rama `develop`, `action: rollback`) o `scripts/deploy.sh staging --rollback`. No ejecutar `scripts/deploy.sh` mientras corre el workflow (comparten `current-tag`/`previous-tag`).

### Pasos humanos (una sola vez, tras aprobar la enmienda)

1. **Aprobar la enmienda 2.0.0** y registrar la aprobación (ver la propuesta).
2. **Azure (OIDC + rol mínimo).** Con `az login` en la cuenta del proyecto, en Git Bash:

   ```bash
   SUB=$(az account show --query id -o tsv)
   VM_ID=$(az vm show -g rg-vaiven -n vm-vaiven --query id -o tsv)
   APP_ID=$(az ad app create --display-name gh-vaiven-deploy-staging --query appId -o tsv)
   SP_OID=$(az ad sp create --id "$APP_ID" --query id -o tsv)
   printf '%s' '{"name":"github-env-staging","issuer":"https://token.actions.githubusercontent.com","subject":"repo:VaivenEnterprise/VaivenApplication-Backend:environment:staging","audiences":["api://AzureADTokenExchange"]}' > fic.json
   az ad app federated-credential create --id "$APP_ID" --parameters @fic.json
   printf '{"Name":"Vaiven VM Run Command","Description":"Leer la VM y ejecutar Run Command (despliegue a staging desde GitHub Actions)","Actions":["Microsoft.Compute/virtualMachines/read","Microsoft.Compute/virtualMachines/runCommand/action"],"AssignableScopes":["/subscriptions/%s/resourceGroups/rg-vaiven"]}' "$SUB" > role.json
   az role definition create --role-definition @role.json
   az role assignment create --assignee-object-id "$SP_OID" --assignee-principal-type ServicePrincipal --role "Vaiven VM Run Command" --scope "$VM_ID"
   rm -f fic.json role.json
   echo "AZURE_CLIENT_ID=$APP_ID"; az account show --query "{AZURE_TENANT_ID:tenantId, AZURE_SUBSCRIPTION_ID:id}" -o yaml
   ```

   El rol se asigna **solo a la VM** (un rol personalizado no admite una VM como ámbito asignable; por eso declara el grupo y se asigna a la VM). Sin secreto de cliente: no hay nada que rotar. Alternativa más amplia, si no se pueden crear roles: `--role "Virtual Machine Contributor" --scope "$VM_ID"` (también permite apagar, cambiar o borrar la VM). La asignación tarda unos minutos en propagarse.

   **Hecho el 2026-11-08** (aplicación `gh-vaiven-deploy-staging`, credencial `github-env-staging`, rol `Vaiven VM Run Command` asignado solo a `vm-vaiven`, 0 secretos de cliente). En Git Bash de Windows: exportar `MSYS_NO_PATHCONV=1` (si no, convierte `/subscriptions/...` en rutas de Windows) y, si `az role definition create` corta la conexión, crear el rol con `az rest --method put` contra `.../providers/Microsoft.Authorization/roleDefinitions/<guid>?api-version=2022-04-01` con el mismo contenido.
3. **GitHub, entorno `staging`** (*Settings → Environments → New environment*): *Deployment branches and tags* → *Selected branches* → `develop`; *Environment variables* `AZURE_CLIENT_ID`, `AZURE_TENANT_ID` y `AZURE_SUBSCRIPTION_ID` (no son secretos). Opcionales a nivel de repositorio: `AZURE_RESOURCE_GROUP` y `AZURE_VM_NAME` (por defecto `rg-vaiven` y `vm-vaiven`).
4. **Primera ejecución de prueba** (el botón *Run workflow* aparece cuando el workflow ya está en `develop`): *Actions → Deploy staging → Run workflow* (rama `develop`, `action: deploy`). Esperado: imagen en GHCR, `Resultado en la VM: deployed`, humo OK con la versión del commit. Luego `action: rollback` (vuelve a la imagen anterior) y otra vez `action: deploy`. Si el push a GHCR falla con `permission_denied`, revisar en la organización *Settings → Packages* que los miembros puedan crear paquetes de contenedor; tras el primer push, en el paquete `vaiven-api` comprobar que el repositorio tiene acceso (*Manage Actions access*).
5. **Encender el despliegue automático:** variable de repositorio `STAGING_AUTODEPLOY=true` (*Settings → Secrets and variables → Actions → Variables*). Para apagarlo, borrarla o cambiar su valor.

### Producción (diseño, no implementado)

`main` no se despliega solo. Diseño propuesto para la spec 013: workflow `deploy-production.yml` solo con `workflow_dispatch` desde `main`; entorno `production` con *Required reviewers* (aprobación humana antes de cada ejecución) y rama `main`; credencial federada aparte (`…:environment:production`) con el mismo rol mínimo; y **promoción sin recompilar** de la imagen ya verificada en staging (misma etiqueta y digest) con `deploy/remote-deploy.sh` en `ENV_NAME=production` y puerto 443 (hoy el script rechaza todo lo que no sea staging, a propósito). Mientras tanto, producción se despliega con `scripts/deploy.sh production`.
