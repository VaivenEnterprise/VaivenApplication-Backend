# VAIVÉN — Backend — Spec Driven Development con OpenCode

Plataforma móvil de préstamo y renta de objetos entre personas (México, MXN).
Contexto completo: [`docs/VAIVEN_Idea_y_Objetivo.pdf`](docs/VAIVEN_Idea_y_Objetivo.pdf). Calendario, reparto entre personas y políticas confirmadas: [`ROADMAP.md`](ROADMAP.md).

**Stack:** Go + chi + MongoDB (NoSQL) · Redis · HTTPS con Let's Encrypt · Docker Compose en una VM de **Azure** (cuenta gratuita) · Cloudinary

Este repositorio es **independiente**. Forma parte de VAIVÉN junto con [`vaiven-frontend`](../vaiven-frontend) (app móvil) y [`vaiven-landing-page`](../vaiven-landing-page) (sitio informativo). Se asume que los tres repositorios están clonados lado a lado, porque algunos documentos del SDD apuntan al repositorio hermano con rutas `../vaiven-*`.

## Qué contiene

| Archivo / carpeta | Para qué sirve |
|---|---|
| `constitution.md` | Reglas técnicas no negociables. Máxima autoridad. |
| `AGENTS.md` | Instrucciones que OpenCode carga siempre: orden de lectura, flujo, agentes, comandos, reglas. |
| `memoria.md` | Decisiones, estado, modelos asignados, dependencias y aprendizajes. |
| `opencode.json` | Modelos gratuitos por agente, instrucciones cargadas y permisos. |
| `.opencode/agents/` y `.opencode/skills/` | Subagentes propios y skills con plantillas y listas de comprobación. |
| `specs/001…015/` | `spec.md`, `plan.md` y `tasks.md` de cada fase: **001 infraestructura en Azure y servicios**, 002 arquitectura base (incluye Redis, HTTPS y `deploy/`), cuenta, imágenes, objetos, búsqueda, reservas, chat, entrega y pagos, calificaciones, notificaciones, integración, producción, IA y ML. Cada una termina con su tarea «Despliegue». |
| `ROADMAP.md`, `docs/` | Copia del plan maestro y del documento de idea del proyecto. |

Contrato de API: `api/openapi.yaml` (Art. IV). El móvil (`../vaiven-frontend`) desarrolla contra `api/paths/<modulo>.yaml`.

## Infraestructura (Azure)

```
Internet ─ HTTPS ─► api (Go) ─► MongoDB (rs0) · Redis      ← una VM de Azure, Docker Compose
```
Staging en el puerto 8443 y producción en el 443 (proyectos de Compose separados). La API **solo** sirve HTTPS, con un certificado de Let's Encrypt emitido con `certbot` en la VM (sin nginx). **Redis es obligatorio** (límite de tasa y caché; nunca fuente de verdad, Art. XIV). Cloudinary guarda las fotos. Una sola réplica: la VM gratuita tiene ~1 GiB de RAM.

## Flujo: OpenCode + MCP, spec por spec

1. Inicia sesión con `az login` y define `CLOUDINARY_URL`; reinicia OpenCode y comprueba con `opencode mcp list` que `azure` y los dos MCP de Cloudinary estén conectados.
2. Spec **001**: `@devops-mcp` elige región y talla gratuitas, crea la VM, la red y el stack base (MongoDB y Redis) y emite el certificado HTTPS.
3. Desde la **002**: se implementan las tareas con `@dev-go`; la última tarea de cada spec es «Despliegue» (`@devops-mcp` ejecuta `scripts/deploy.sh staging` y verifica). **La spec siguiente no empieza hasta que esa tarea pase.**
4. El despliegue no depende del repositorio (la imagen viaja por SSH). Repositorio en GitHub: una rama y una PR por spec con `main` protegida, por el MCP de GitHub y `@gestor-repo` (Art. XV); sin pipeline automatizado.

## Cómo se usa con OpenCode

```bash
opencode          # desde la raíz de este repositorio
```
1. Ejecuta `opencode models` y confirma que los modelos de `opencode.json` existen (los gratuitos rotan); si cambias alguno, anótalo en `memoria.md`.
2. Agente `plan`: revisa y ajusta `spec.md`, `plan.md` y `tasks.md`.
3. Con la spec aprobada por una persona, cambia al agente `build` y pide @dev-go la tarea que toque.
4. Al cerrar una fase, pide `@revisor-constitucion revisa los cambios`.

## Coherencia entre repositorios

- Los Artículos **IV** (contrato de API) y **V** (dinero, fechas, idioma) deben mantenerse idénticos en las constituciones de backend y móvil. Si cambias uno, cámbialo en ambos repositorios y registra la decisión en ambos `memoria.md`.
- `ROADMAP.md` y `docs/VAIVEN_Idea_y_Objetivo.pdf` son copias en cada repositorio; si se actualiza una, hay que actualizar las demás.

## Estado

Todavía no hay código ni se han ejecutado tareas: están definidos los archivos de gobierno, el SDD por spec y la configuración de OpenCode con MCP. El repositorio y el pipeline se integrarán de cero.
