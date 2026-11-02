---
name: mcp-deploy
description: Procedimiento para aprovisionar Azure (VM, red, Docker Compose con Redis y MongoDB, HTTPS con Let's Encrypt) y Cloudinary desde OpenCode (spec 001) y para desplegar cada spec del backend en staging y verificarla (tarea final «Despliegue»). Úsala en cualquier tarea de infraestructura o despliegue.
---

# Skill: infraestructura en Azure y despliegue (backend)

## Arquitectura (Art. XIII y XIV)
```
Internet ─ HTTPS 8443 (staging) / 443 (producción) ─► api (Go, sirve TLS) ─► mongo (rs0) · redis   (red interna de Compose)
                 puerto 80 abierto solo para certbot (Let's Encrypt)
```
Una VM Linux gratuita de Azure; proyectos de Compose `vaiven-staging` y `vaiven-production`. Solo la API publica puerto; Mongo y Redis nunca. No hay nginx ni balanceador: una réplica.

## Antes de empezar (siempre)
- `az account show` muestra la suscripción gratuita correcta; `opencode mcp list` muestra `azure`, `cloudinary-assets` y `cloudinary-config` conectados. Si no, detente y pide a la persona `az login` o revisar `CLOUDINARY_URL`.
- Lista las herramientas del MCP de Azure con `learn=true` (p. ej. `compute_vm_create`, `compute_vm_get`) y úsalas **primero**; recurre a `az` solo para lo que no cubran (reglas del NSG, IP estática y nombre DNS) y a SSH para lo que ocurre dentro de la VM. Todo lo ejecutas tú desde OpenCode.
- Revisa que no haya recursos ya creados antes de crear otros.

## A. Aprovisionamiento (spec 001)
1. **Elegir región y talla.** No hay región que garantice VM gratuita (`NotAvailableForSubscription`). Para cada región candidata y cada talla en este orden —`Standard_B2ats_v2` (x86-64, AMD), `Standard_B1s` (x86-64, en retiro), `Standard_B2pts_v2` (ARM64)— ejecuta `az vm list-skus --location <región> --resource-type virtualMachines --size Standard_B --all --output table` y quédate con la primera combinación **sin `Restrictions`**. Si la suscripción es «Azure for Students», limita las regiones a las de `az policy assignment list` (política `sys:regionrestriction`). **Nunca** pidas zonas de disponibilidad. Anota la elección.
2. **Llave SSH:** `ssh-keygen` (con aprobación) para una llave dedicada de VAIVÉN; la privada nunca se imprime ni se sube.
3. **VM (por MCP):** `compute_vm_create` en el grupo `rg-vaiven` con la talla elegida **pasada explícitamente** (el MCP trae `Standard_DS1_v2` por defecto, que **no** es gratuita), Ubuntu 24.04 LTS, la llave pública y un disco de SO Premium de hasta 64 GB. El MCP crea también la red, el NSG y la IP pública.
4. **Red (por `az`, porque el MCP no lo cubre):** IP pública **estándar estática** con etiqueta DNS (`<etiqueta>.<region>.cloudapp.azure.com`; cuesta ≈ 3–4 USD/mes y es la única excepción de costo aceptada); NSG con 80 (certbot), 443 y 8443 abiertos y 22 solo desde la IP pública de la persona (detectada con `curl`). Crea una alerta de presupuesto.
5. **Preparar la VM (por SSH, `deploy/bootstrap.sh`):** Docker, plugin de Compose, `certbot`, swap de 2 GB (las tallas nuevas no traen disco temporal), usuario `deploy`, `/opt/vaiven` y actualizaciones de seguridad. Verifica versiones, swap, memoria y disco.
6. **Stack base** (`deploy/base/`): MongoDB como replica set `rs0` de un nodo y Redis. Verifica `rs.status()` en `PRIMARY` y `redis-cli ping` = `PONG`; anota `docker stats` y los créditos de CPU.
7. **HTTPS:** `certbot --standalone` para el nombre DNS; `certbot renew --dry-run` bien; comprobación temporal con `openssl s_server` en 8443 validada con `curl` sin `-k`; anota las rutas de los archivos.
8. **Cloudinary (por MCP):** confirma que la nube responde y crea la carpeta `vaiven/staging`.
9. Las contraseñas y la `MONGO_URI` interna se generan en la VM y viven solo en su `.env` (permisos 600); nunca en el repo ni en la conversación.

## B. Despliegue de una spec (tarea final de cada spec)
1. Confirma que **todas** las tareas anteriores de la spec están `[x]` en `tasks.md`.
2. `make fmt lint test`; si falla, detente.
3. `scripts/deploy.sh staging`: compila para la arquitectura de la VM (`amd64`, o `arm64` si es `B2pts_v2`), envía la imagen con `docker save | ssh … docker load`, actualiza el proyecto Compose, reinicia la API y espera su `/healthz` por HTTPS (hay un corte de unos segundos).
4. Verifica contra `https://<host>:8443` **sin `-k`** (reintenta hasta 60 s): `/healthz` y `/readyz` → 200; `/api/v1/meta` → 200.
5. Siempre: el límite de tasa responde 429 al pasarse; Redis detenido no tumba la API (`/readyz` informa `degradado`); no hay respuesta HTTP en claro en el puerto de la API.
6. Por cada endpoint de `api/paths/<modulo>.yaml` de la spec: una petición válida y una inválida (formato de error del Art. IV, con `X-Request-Id`). Endpoints protegidos: usa una cuenta de prueba (spec 003) y limpia lo que crees.
7. Registros: `docker compose logs --since 10m api` sin secretos, tokens, correos ni cuerpos de petición.
8. Si falla: revierte (`scripts/deploy.sh staging --rollback`), reporta y no marques la tarea.
9. Anota en `memoria.md` (solo anexar): fecha, spec, versión, resultado de cada verificación. Sin secretos.
10. La spec queda **cerrada** solo si todo pasó; mientras tanto no se empieza la siguiente.

## Prohibido
Borrar el grupo de recursos, la VM, discos, IP o reglas de red; `docker compose down -v`; borrar volúmenes; publicar los puertos de Mongo o Redis; crear recursos de pago (salvo la IP pública); pedir zonas de disponibilidad.

## Notas
- **Créditos de CPU:** en la serie B la línea base es 20 % (10 % en B1s); si se agotan, la VM se estrangula. Vigílalo y repórtalo.
- Con 1 GiB de RAM el margen es estrecho: límites de memoria por contenedor, `GOMEMLIMIT` y swap. Si no alcanza, reporta; no subas de talla sin preguntar.
- Una sola VM de 750 h/mes (12 meses) es gratuita; no hay segunda.
- El crédito de prueba de Azure caduca a los 30 días; la IP pública sigue facturándose. Vigila *Cost Management*.
