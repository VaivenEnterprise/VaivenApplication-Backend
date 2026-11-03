# Plan 001 — Infraestructura en Azure y servicios (backend)

- **Spec:** `spec.md` · **Dueño:** BE-B

## 1. Archivos
```
opencode.json                         # bloque "mcp" (azure, cloudinary) y permisos (ya declarado)
.opencode/agents/devops-mcp.md        # único agente con permiso para MCP y az
.opencode/skills/mcp-deploy/SKILL.md  # procedimiento de aprovisionamiento y despliegue
deploy/bootstrap.sh                   # Docker, Compose, swap, usuario, actualizaciones (se ejecuta por SSH)
deploy/base/docker-compose.yml        # mongo (rs0) y redis
deploy/base/tls.md                    # procedimiento de certbot y comprobación (sin servidor web permanente)
docs/servicios.md
.env.example
```

## 2. Decisiones
| # | Decisión | Art. |
|---|---|---|
| P-01 | **Azure:** MCP local oficial `@azure/mcp` (`npx -y @azure/mcp@latest server start`), con la sesión de `az login`, **con escritura** y aprobación en cada llamada. Se usa primero (`compute_vm_create`, `compute_vm_get`…); la CLI `az` cubre lo que el MCP no (NSG, IP estática y DNS) y SSH lo que ocurre dentro de la VM | XIII.2 |
| P-02 | **Cloudinary:** MCP locales oficiales `@cloudinary/asset-management-mcp` y `@cloudinary/environment-config-mcp`, con `CLOUDINARY_URL` | XIII |
| P-03 | **VM única** de la capa gratuita con Docker Compose; red: 80 (solo certbot), 443 y 8443 (API) abiertos; SSH limitado a la IP de la administradora | XIII, XIV |
| P-04 | **MongoDB** en contenedor como replica set `rs0` de un nodo (transacciones, Art. II.2), con autenticación y volumen con nombre; sin puerto publicado | II, VI |
| P-05 | **Redis 7** en contenedor, sin publicar puerto, con contraseña y límite de memoria (`maxmemory` + política `allkeys-lru`) | XIV | Redis obligatorio, puertos internos y HTTPS forzoso desde el primer día |
| P-06 | **HTTPS sin proxy:** `certbot --standalone` en el puerto 80 de la VM emite el certificado de Let's Encrypt para el nombre DNS; la renovación es automática (temporizador de `certbot`) y se prueba con `--dry-run`; la API (spec 002) monta los archivos y los recarga. No hay nginx | XIV.4 |
| P-07 | Permisos: MCP y `az` denegados por defecto y habilitados solo para `devops-mcp`; destructivos denegados incluso para él | XIII.4 |
| P-08 | Credenciales: `CLOUDINARY_URL` como variable de entorno local; contraseñas y cadenas de conexión se generan en la VM y viven solo en su `.env` (600) | VI, XIII.3 |

## 3. Variables (nombres, sin valores)
| Variable | Dónde vive | Para qué |
|---|---|---|
| `CLOUDINARY_URL` | Entorno local de quien administra | MCP de Cloudinary |
| `MONGO_ROOT_PASSWORD`, `MONGO_APP_PASSWORD` | `.env` de la VM | Usuario raíz y usuario de aplicación de MongoDB |
| `REDIS_PASSWORD` | `.env` de la VM | Autenticación de Redis |
| `AZURE_HOST` | `.env` local de despliegue | Nombre DNS de la VM (no es secreto) |

## 4. Pruebas
No hay código de la API. Verificación por criterios CA-01…CA-07, con evidencia anotada en `memoria.md` sin valores secretos.

## 5. Verificación de constitución
| Art. | Cumple |
|---|---|
| II | MongoDB como único almacén de negocio, replica set |
| VI | Secretos solo en el `.env` de la VM y variables locales |
| X | Todo por OpenCode y modelos gratuitos |
| XIII | MCP en `opencode.json`, solo `devops-mcp`, sin acciones destructivas |
| XIV | Redis obligatorio, puertos internos y HTTPS forzoso desde el primer día |

## 6. Riesgos
| Riesgo | Mitigación |
|---|---|
| Memoria insuficiente (~1 GB) | Límites por contenedor, swap de 2 GB; si no alcanza, reportar y consultar antes de cambiar de talla |
| Un recurso genera costo | `devops-mcp` lista antes de crear, usa solo tallas gratuitas y revisa *Cost Management* |
| Un secreto llega al modelo por la salida de un comando | Secretos solo en la VM; rotar tras el aprovisionamiento |
| La IP de SSH de la administradora cambia | Actualizar la regla del NSG con `az` (aprobación) |
| Let's Encrypt no emite o no renueva | Verificar con `--dry-run`; documentar el procedimiento manual; el puerto 80 debe estar abierto en el NSG |
| La región o la talla no están disponibles (`NotAvailableForSubscription`) | Revisar `az vm list-skus` antes de crear; probar otra región o talla; sin zonas de disponibilidad |
| La IP pública estándar genera cargo | Aceptado (≈ 3–4 USD/mes); alerta de presupuesto |
