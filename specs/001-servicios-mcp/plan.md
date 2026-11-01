# Plan 001 — Servicios externos con MCP (backend)

- **Spec:** `spec.md` · **Dueño:** BE-B

## 1. Archivos
```
opencode.json                         # bloque "mcp" y permisos (ya declarado)
.opencode/agents/devops-mcp.md        # único agente con permiso para los MCP
.opencode/skills/mcp-deploy/SKILL.md  # procedimiento de aprovisionamiento y despliegue
docs/servicios.md
.env.example
```

## 2. Decisiones
| # | Decisión | Art. |
|---|---|---|
| P-01 | **MongoDB:** MCP local oficial `mongodb-mcp-server` (`npx -y mongodb-mcp-server`), con credenciales de cuenta de servicio de Atlas (`MDB_MCP_API_CLIENT_ID`, `MDB_MCP_API_CLIENT_SECRET`). Herramientas Atlas usadas: crear clúster gratuito, crear usuario de base y crear lista de acceso | XIII |
| P-02 | **Cloudinary:** MCP locales oficiales `@cloudinary/asset-management-mcp` (carpetas y activos) y `@cloudinary/environment-config-mcp` (configuración), con `CLOUDINARY_URL` | XIII |
| P-03 | **Render:** MCP remoto oficial `https://mcp.render.com/mcp` con `Authorization: Bearer {env:RENDER_API_KEY}`. Herramientas permitidas: listar y seleccionar espacio de trabajo, crear servicio web, variables de entorno, desplegar, ver despliegues, eventos y registros | XIII |
| P-04 | Permisos: todas las herramientas MCP denegadas por defecto y habilitadas solo para `devops-mcp`; las destructivas denegadas incluso para él | XIII.3 |
| P-05 | Las credenciales viven en **variables de entorno del sistema** (OpenCode no lee `.env`); `opencode.json` solo las referencia con `{env:...}` | VI.1, XIII.2 |
| P-06 | Atlas M0: usuario con rol `readWrite` solo sobre `vaiven_staging`; lista de acceso `0.0.0.0/0` con contraseña fuerte (se documenta el riesgo) | VI |

## 3. Variables locales (nombres, sin valores)
| Variable | Para qué |
|---|---|
| `MDB_MCP_API_CLIENT_ID`, `MDB_MCP_API_CLIENT_SECRET` | Cuenta de servicio de la API de Atlas (MCP de MongoDB) |
| `CLOUDINARY_URL` | `cloudinary://<api_key>:<api_secret>@<cloud_name>` (MCP de Cloudinary) |
| `RENDER_API_KEY` | API key de Render (MCP de Render) |
| `MONGO_URI` | Cadena de conexión del usuario `vaiven_staging` (se usa en la spec 002) |

## 4. Pruebas
No hay código. La verificación es por criterios CA-01…CA-05, con evidencia anotada en `memoria.md` sin valores secretos.

## 5. Verificación de constitución
| Art. | Cumple |
|---|---|
| II.4, VI | Atlas y Cloudinary como únicos servicios de datos; secretos solo por entorno |
| X | Todo por OpenCode y modelos gratuitos |
| XIII | MCP en `opencode.json`, solo `devops-mcp`, sin acciones destructivas |

## 6. Riesgos
| Riesgo | Mitigación |
|---|---|
| Un secreto llega al modelo por una respuesta del MCP | Espacio de trabajo de Render solo para VAIVÉN; rotar claves tras cada fase de aprovisionamiento |
| El MCP crea más de un clúster gratuito (Atlas M0 permite uno por proyecto) | `devops-mcp` lista antes de crear y se detiene si ya existe |
| Cambio de nombres de herramientas del MCP | La skill `mcp-deploy` pide listar las herramientas disponibles antes de usarlas |
