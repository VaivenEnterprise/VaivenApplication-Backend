# Spec 001 — Servicios externos con MCP (backend)

- **Dueño:** BE-B · **Prioridad:** P0 · **Depende de:** — (es la primera spec; no requiere código)
- **Constitución:** v1.3.0 (Art. XIII) · **Estado:** Borrador, pendiente de aprobación

## 1. Objetivo y por qué
Dejar listos, **antes de escribir código**, los servicios gratuitos que usará todo el backend (MongoDB Atlas, Cloudinary y Render), aprovisionados y verificados mediante **servidores MCP** desde OpenCode y no con paneles manuales. Así, desde la spec 002 cada spec puede terminar desplegada sin trámites a mano.

## 2. Historias técnicas
| # | Prio. | Historia |
|---|---|---|
| H-01 | P0 | Como **equipo**, quiero que OpenCode se conecte a Atlas, Cloudinary y Render por MCP, para operar los servicios sin salir del flujo SDD. |
| H-02 | P0 | Como **administrador**, quiero que solo un agente (`devops-mcp`) pueda usar esos MCP y que las acciones destructivas estén bloqueadas, para no perder datos ni recursos por error. |
| H-03 | P0 | Como **desarrollador**, quiero una base de datos de staging y una carpeta de imágenes de staging ya creadas, para empezar a programar contra servicios reales. |
| H-04 | P1 | Como **equipo**, quiero documentación de cada servicio sin secretos, para reproducir el entorno. |

## 3. Requisitos
| ID | Requisito |
|---|---|
| RF-01 | Los MCP `mongodb`, `cloudinary` y `render` están declarados en `opencode.json`, con credenciales tomadas de variables de entorno locales, y conectan desde OpenCode (`opencode mcp list`). |
| RF-02 | Solo el agente `devops-mcp` puede usar herramientas MCP; cualquier otro agente que intente usarlas es rechazado. Las herramientas destructivas (`drop-*`, `delete-*`) están denegadas. |
| RF-03 | **Atlas:** clúster gratuito M0 `vaiven`, usuario `vaiven_staging` con rol `readWrite` solo sobre la base `vaiven_staging` y lista de acceso `0.0.0.0/0` (necesaria con hosting gratuito sin IP fija; riesgo documentado), todo creado con las herramientas Atlas del MCP. |
| RF-04 | La conexión a `vaiven_staging` se verifica por MCP (listar bases de datos) sin escribir datos de negocio. |
| RF-05 | **Cloudinary:** la nube responde por MCP y existe la carpeta base `vaiven/staging`. El *API secret* solo existe en el entorno local de quien administra y, después, en las variables de Render. |
| RF-06 | **Render:** espacio de trabajo seleccionado y verificado. La creación del servicio `vaiven-staging` ocurre en la tarea de despliegue de la spec 002, porque necesita código y `Dockerfile`. |
| RF-07 | `docs/servicios.md` y `.env.example` listan servicios, variables (sin valores), cómo se obtuvo cada credencial y cómo rotarla. |
| RNF-01 | Ningún secreto en el repositorio, en `memoria.md`, en specs ni en registros. |
| RNF-02 | Costo mensual cero (Atlas M0, Cloudinary gratuito, Render gratuito). |

## 4. Criterios de aceptación
| ID | Dado / Cuando / Entonces |
|---|---|
| CA-01 | Dadas las variables locales definidas, cuando ejecuto `opencode mcp list`, entonces `mongodb`, `cloudinary` y `render` aparecen conectados. |
| CA-02 | Dado el agente `dev-go`, cuando intenta una herramienta MCP, entonces se rechaza; con `devops-mcp` funciona. |
| CA-03 | Dado Atlas, cuando `devops-mcp` lista las bases por MCP, entonces la conexión con el usuario `vaiven_staging` funciona y nada más es accesible. |
| CA-04 | Dado Cloudinary, cuando `devops-mcp` consulta la carpeta `vaiven/staging`, entonces existe. |
| CA-05 | Dado el repositorio, cuando busco cadenas sospechosas (URI con contraseña, `API_SECRET`, claves), entonces no hay ninguna. |

## 5. Fuera de alcance
Código Go, creación del servicio de Render (spec 002), producción (spec 013), dominios propios, monitoreo avanzado, copias de seguridad.

## 6. Preguntas abiertas
- [NEEDS CLARIFICATION: ¿Cuál es el proyecto (`projectId`) de Atlas y qué cuenta de servicio de la API de Atlas se usará? Se crean en la consola de Atlas (trámite manual, ningún MCP crea cuentas).]
- [NEEDS CLARIFICATION: ¿El MCP de Render usa una API key de la cuenta o de un espacio de trabajo dedicado? Render avisa que no garantiza que los secretos no lleguen al modelo; se recomienda un espacio de trabajo solo para VAIVÉN.]
- Hosting: **Render, plan gratuito** (se duerme tras inactividad; ~1 min de arranque en frío). Dos servicios: `vaiven-staging` (spec 002) y `vaiven-prod` (spec 013).
