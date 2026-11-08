# Propuesta de enmienda 2.0.0 (MAJOR) — despliegue continuo a staging

- **Estado:** propuesta, pendiente de aprobación del equipo (Art. XI.1). `constitution.md` **no** se ha modificado.
- **Fecha:** 2026-11-08 · **Origen:** spec 017 (DevSecOps), workflow `.github/workflows/deploy-staging.yml`.
- **Tipo:** MAJOR (Art. XI.2): cambia dos principios vigentes —quién despliega staging (Art. XIII.2, XIII.4 y XIII.5) y «no hay registro de imágenes» (Art. XIII.7)— además de la frase «no hay pipeline automatizado» (Art. VII.4 y XV.7). Si el equipo lo considera una excepción acotada y no un cambio de principio, puede registrarse como MINOR (1.7.0, o dentro de la 1.6.0 si esta sigue sin aprobarse).
- **Relación con la 1.6.0** (`docs/enmienda-1.6.0-propuesta.md`, también pendiente): son independientes. Si se aprueban juntas, el texto de Art. VII.4 y XV.7 de abajo reemplaza al de la 1.6.0.

**Mientras no se apruebe, el workflow queda apagado**: fusionar la PR no despliega nada, porque el disparo automático exige la variable de repositorio `STAGING_AUTODEPLOY=true` (sin ella el workflow se omite, sin compilar ni publicar), y sin las variables `AZURE_*` del entorno `staging` el job *Deploy to VM* no puede iniciar sesión en Azure. Ver «Cómo aprobarla».

## Motivo

Hoy cada fusión en `develop` exige que alguien ejecute `ENGINE=podman scripts/deploy.sh staging` desde su PC, con la llave SSH y el puerto 22 abierto a su IP (que cambia seguido). El resultado: staging queda atrás de `develop`, y el despliegue depende de una sola persona y de una sola máquina. El workflow propuesto despliega solo, después de que la CI pasa, sin SSH y sin secretos guardados en GitHub.

## Qué hace el workflow (resumen)

1. Corre cuando la CI (`ci.yml`, con «Required checks») termina **en verde** para un push a `develop` (o a mano, solo sobre `develop`, comprobando que ese commit tiene CI en verde).
2. Compila la imagen `linux/amd64` con el `Dockerfile` del repositorio (sin cambiarlo) y la publica en GHCR como `ghcr.io/vaivenenterprise/vaiven-api:<commit corto>`.
3. Inicia sesión en Azure con **OIDC** (credencial federada, sin secreto de cliente) con una identidad que **solo** puede ejecutar Run Command sobre `vm-vaiven`.
4. Ejecuta `deploy/remote-deploy.sh` en la VM con `az vm run-command invoke`: descarga la imagen con el token del job (caduca al terminar), mantiene `current-tag`/`previous-tag`, `docker compose up -d`, espera `/healthz` y la versión en `/api/v1/meta` por HTTPS **sin `-k`** y, si falla, **vuelve sola** a la imagen anterior.
5. Prueba de humo desde el runner: `/healthz`, `/readyz` y `/api/v1/meta` con la versión del commit.

Producción (`main`) **no** cambia: sigue siendo manual (`scripts/deploy.sh production`, Art. XIII).

## Cambios propuestos (texto exacto)

### Art. VII.4

- **Actual:** «Las pruebas se ejecutan **en local** con `make test`. El repositorio y las PR siguen el Art. XV; no hay pipeline automatizado. Antes de cada despliegue (Art. XIII) la verificación la hace el agente `devops-mcp`.»
- **Propuesto:** «Las pruebas se ejecutan **en local** con `make test` y, en cada PR y en cada push a `develop`, en la CI del Art. XV.7. El despliegue a staging solo ocurre si esa CI termina en verde (Art. XIII.5); antes de un despliegue manual la verificación la hace el agente `devops-mcp`.»

### Art. XIII.4 (se agrega un párrafo al final)

- **Propuesto (agregar):** «Excepción: el workflow `deploy-staging.yml` de GitHub Actions usa la CLI `az` con una identidad federada (OIDC, sin secretos) cuyo único permiso es ejecutar Run Command sobre la VM; solo despliega a staging, solo desde `develop` y solo después de la CI en verde. No puede crear, borrar ni reconfigurar recursos. Las demás operaciones siguen siendo exclusivas de `devops-mcp`.»

### Art. XIII.5

- **Actual (primera oración tras los dos puntos):** «`devops-mcp` ejecuta `scripts/deploy.sh staging`, que construye la imagen …»
- **Propuesto:** «Al fusionar en `develop`, el workflow `deploy-staging.yml` despliega staging solo (imagen del commit, reinicio de la API con un corte de unos segundos, `/healthz` y vuelta automática a la imagen anterior si falla). La tarea «Despliegue» de cada spec verifica esa ejecución; si hay que desplegar antes de fusionar, `devops-mcp` usa `scripts/deploy.sh staging --allow-unmerged`. En ambos casos `devops-mcp` verifica por **HTTPS** `/healthz`, `/readyz` y los endpoints de la spec, revisa los registros y anota el resultado en `memoria.md`.» (El resto del artículo no cambia.)

### Art. XIII.7

- **Actual:** «**El despliegue no depende del repositorio:** la imagen se construye en la máquina de quien despliega y viaja a la VM por SSH (`docker save | ssh … docker load`). No hay registro de imágenes (Azure Container Registry no es gratuito).»
- **Propuesto:** «**Imágenes:** el despliegue automático a staging publica la imagen en GitHub Container Registry (gratuito para este repositorio público; ACR no es gratuito) con la etiqueta del commit corto. El despliegue manual (`scripts/deploy.sh`) sigue sin depender del registro: construye la imagen en la máquina de quien despliega y la envía por SSH. La VM conserva solo la imagen actual y la anterior.»

### Art. XV.7

- **Actual:** «No hay pipeline automatizado (CI) definido: las verificaciones son locales y el despliegue lo hace `devops-mcp` o `probador-expo` según el proyecto.»
- **Propuesto:** «La CI de GitHub Actions (spec 017) valida cada PR y cada push a `develop` y `main`. En el backend, un push a `develop` con la CI en verde despliega staging (Art. XIII.5). Producción no se despliega automáticamente: la despliega `devops-mcp` desde `main`, con aprobación humana.»

### Encabezado

Versión **2.0.0** y nota: «Enmienda 2.0.0 (2026-11-XX, MAJOR): despliegue continuo a staging con GitHub Actions, OIDC y Run Command; imágenes en GHCR (Art. VII.4, XIII.4, XIII.5, XIII.7 y XV.7).»

## Riesgos y mitigaciones

| Riesgo | Mitigación |
|---|---|
| Un commit roto llega a staging | Solo tras la CI completa en verde; `/healthz` y versión verificadas en la VM; vuelta automática a `previous-tag`; humo desde fuera |
| Credenciales de Azure en GitHub | No hay: OIDC con sujeto `repo:VaivenEnterprise/VaivenApplication-Backend:environment:staging`; las variables `AZURE_CLIENT_ID/TENANT_ID/SUBSCRIPTION_ID` no son secretas |
| La identidad de GitHub toma el control de Azure | Rol personalizado con solo `virtualMachines/read` y `virtualMachines/runCommand/action`, asignado a la VM (no al grupo ni a la suscripción). Run Command corre como root en la VM: quien pueda modificar el workflow en `develop` puede ejecutar comandos en la VM, por eso `develop` exige PR revisada y el entorno `staging` solo acepta `develop` |
| Token para descargar la imagen | `GITHUB_TOKEN` del job con `packages: read`: caduca al terminar el job; viaja dentro del script (archivo 600 con `@archivo`, no en argumentos) y llega a `docker login` por stdin; la sesión vive en un `DOCKER_CONFIG` temporal que se borra. Queda una copia del script en `/var/lib/waagent/run-command/` (solo root) con el token ya caducado |
| Dos despliegues a la vez | `concurrency: deploy-staging` sin cancelación; un commit viejo no se despliega encima de uno más nuevo |
| Cuota de GHCR | Repositorio público: almacenamiento y descarga sin costo. Si se vuelve privado, revisar la cuota de GitHub Packages y borrar versiones viejas |

## Revisión de coherencia (Art. XI.1)

| Archivo | Cambio necesario si se aprueba |
|---|---|
| `constitution.md` | Los cinco puntos de arriba y el encabezado |
| `memoria.md` | Pasar D-123 de «propuesta» a «aprobada» (solo anexar) |
| `AGENTS.md` §5b | «Cada spec termina con su tarea Despliegue (`@devops-mcp` ejecuta `scripts/deploy.sh staging`…)» → citar el workflow y `--allow-unmerged` |
| `.opencode/agents/devops-mcp.md` y skill `mcp-deploy` | Verificar la ejecución del workflow en lugar de desplegar tras cada fusión |
| `ROADMAP.md` §7.3 | Quitar «sin pipeline automatizado» |
| `docs/servicios.md` y `docs/devsecops.md` | Ya describen el workflow (esta PR) |

## Cómo aprobarla

1. Una persona del equipo responde «apruebo la enmienda 2.0.0» (en la PR o en el grupo).
2. Se aplica la tabla anterior en una PR `docs/SPEC-17-TNNN` y se registra la aprobación en `memoria.md`.
3. Solo entonces se hacen los pasos de Azure y GitHub de `docs/devsecops.md` §«Despliegue continuo a staging», la primera ejecución manual de prueba y, si pasa, se crea `STAGING_AUTODEPLOY=true`.
