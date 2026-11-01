---
name: mcp-deploy
description: Procedimiento para aprovisionar MongoDB Atlas, Cloudinary y Render por MCP (spec 001) y para desplegar cada spec del backend en staging y verificarla (tarea final «Despliegue»). Úsala en cualquier tarea de servicios o despliegue.
---

# Skill: servicios y despliegue por MCP (backend)

## Antes de empezar (siempre)
- `opencode mcp list` debe mostrar `mongodb`, `cloudinary-assets`, `cloudinary-config` y `render` conectados. Si no, detente y pide a la persona que revise sus variables de entorno (`MDB_MCP_API_CLIENT_ID`, `MDB_MCP_API_CLIENT_SECRET`, `CLOUDINARY_URL`, `RENDER_API_KEY`).
- Lista las herramientas de cada MCP y usa solo las que existan.

## A. Aprovisionamiento (spec 001)
1. **Atlas:** lista proyectos y clústeres; si ya existe un clúster gratuito, **no crees otro**. Si no, crea el clúster gratuito `vaiven`; crea el usuario `vaiven_staging` con `readWrite` solo sobre `vaiven_staging` y una contraseña fuerte; agrega la lista de acceso `0.0.0.0/0`. Entrega la contraseña/URI a la persona **solo por el canal local** para que la guarde en `MONGO_URI`; no la escribas en ningún archivo.
2. **Verificar MongoDB:** conecta y lista bases de datos. No insertes ni borres datos de negocio.
3. **Cloudinary:** confirma que la nube responde y crea la carpeta `vaiven/staging`.
4. **Render:** selecciona el espacio de trabajo de VAIVÉN y lista servicios. No crees `vaiven-staging` hasta la tarea de despliegue de la spec 002.

## B. Despliegue de una spec (tarea final de cada spec)
1. Confirma que **todas** las tareas anteriores de la spec están `[x]` en `tasks.md`.
2. Ejecuta `make fmt lint test`; si falla, detente (el despliegue no se intenta).
3. Render: `list_services`. Si `vaiven-staging` no existe (solo en 002-T027), créalo (`create_web_service`, Docker, plan gratuito) y define sus variables con `update_environment_variables` desde el entorno local: `APP_ENV=staging`, `MONGO_URI`, `CLOUDINARY_*`, `CORS_ALLOWED_ORIGINS`, y las que la spec agregue (nombres en `.env.example`).
4. `trigger_deploy` y consulta `list_deploys` cada 30 s hasta `live` o fallo (máximo 10 min). Si falla, `list_logs` y reporta.
5. Verifica con `curl` (con reintentos hasta 90 s por el arranque en frío del plan gratuito): `/healthz` y `/readyz` → 200; `/api/v1/meta` → 200.
6. Por cada endpoint de `api/paths/<modulo>.yaml` de la spec: una petición válida (éxito esperado) y una inválida (formato de error del Art. IV, con `X-Request-Id`). Endpoints protegidos: usa una cuenta de prueba creada por la spec 003 y limpia lo que crees.
7. `list_logs` de los últimos minutos: sin secretos, tokens, correos ni cuerpos de petición.
8. Anota en `memoria.md` (solo anexar): fecha, spec, ID del despliegue, resultado de cada verificación. Sin secretos.
9. La spec queda **cerrada** solo si todo pasó. Si algo falló, no se empieza la spec siguiente.

## Notas
- Render gratuito se duerme tras inactividad: el primer acceso tarda ~1 min.
- Render avisa que no garantiza que los secretos no lleguen al modelo: usa un espacio de trabajo solo para VAIVÉN y rota claves si una salida las mostró.
- Producción (`vaiven-prod`) solo en la spec 013.
