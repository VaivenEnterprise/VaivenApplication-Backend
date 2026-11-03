# Memoria del Backend — VAIVÉN

Memoria viva del proyecto. Los agentes la leen después de `constitution.md` y la **actualizan** al cerrar cada tarea relevante. No contradice la constitución: si hay conflicto, gana la constitución.

_Última actualización: 2026-10-28_

## 1. Estado actual

| Elemento | Estado |
|---|---|
| Constitución | v1.1.0 (enmendada 2026-10-28) |
| Spec 001 — Servicios con MCP | Redactada (reescrita el 2026-11-01), pendiente de aprobación humana |
| Spec 002 — Arquitectura base | Redactada, pendiente de aprobación humana |
| Specs 003–015 | Redactadas (spec, plan, tasks), pendientes de aprobación; ver `ROADMAP.md` |
| Tareas | **Sin ejecutar**; arranque planeado 2026-10-29 |
| Código | Ninguno todavía |

## 2. Decisiones (registro tipo ADR)

| # | Fecha | Decisión | Motivo | Constitución |
|---|---|---|---|---|
| D-001 | 2026-10-26 | Go + chi + Clean Architecture | Router ligero, cercano a `net/http`; capas claras para modelos con poco contexto | Art. III |
| D-002 | 2026-10-26 | MongoDB 7+ con driver oficial v2, replica set | Requisito del proyecto (NoSQL); replica set habilita transacciones futuras | Art. II |
| D-003 | 2026-10-26 | Dinero en centavos `int64`, sufijo `Cents` | Evitar errores de punto flotante; MXN único | Art. V |
| D-004 | 2026-10-26 | JSON y campos BSON en `camelCase`; colecciones `snake_case` plural | Un solo nombre por campo, sin mapeos intermedios | Art. IV, §6 |
| D-005 | 2026-10-26 | Errores con envoltorio único `{error:{code,message,details,requestId}}` | El móvil los normaliza en un solo lugar | Art. IV |
| D-006 | 2026-10-26 | Solo OpenCode + modelos gratuitos de Zen | Requisito del curso | Art. X |
| D-007 | 2026-10-26 | Documentos y prompts de agentes en español | Preferencia del equipo | Art. X |
| D-008 | 2026-10-26 | `task.md` se nombra `tasks.md` | Convención estándar de SDD | Art. I |
| D-009 | 2026-10-28 | Imágenes en Cloudinary (capa gratuita); MongoDB guarda solo referencias; sin SDK | Decisión del equipo: servicio gratuito y rápido | Art. II.4, VI.7 (enmienda 1.1.0) |
| D-010 | 2026-10-28 | Trabajo en paralelo: dueño por spec, contrato primero, módulos con una línea de registro, puertos compartidos | 4 personas con OpenCode propio sin pisarse | Art. XII (enmienda 1.1.0) |
| D-011 | 2026-10-28 | Autenticación con JWT de acceso + refresh rotativo, `argon2id` | Estándar para API móvil sin sesiones de servidor | Art. VI, VIII.4 |
| D-012 | 2026-10-28 | Alcance: núcleo completo sin IA ni página pública; código congelado 15-nov | Plazo 20-nov con colchón de 5 días | `ROADMAP.md` |

## 3. Modelos gratuitos asignados (OpenCode Zen)

Fuente de verdad: `opencode.json`. Verificados en septiembre de 2026; **los IDs gratuitos rotan**.

| Rol | Modelo | Agentes |
|---|---|---|
| Razonamiento / planeación | `opencode/nemotron-3-ultra-free` | `plan`, `spec-writer`, `modelador-mongo` |
| Revisión | `opencode/big-pickle` | `revisor-constitucion` (modelo por defecto) |
| Implementación | `opencode/nemotron-3-lightning-free` | `build`, `dev-go` |
| Pruebas / tareas rápidas | `opencode/ling-flash-free` | `qa-go`, `small_model` |
| Alternos | `opencode/mimo-free`, `opencode/muse-spark-free` | — |

**Cómo validar y cambiar:**
1. Ejecutar `opencode models` y confirmar que cada ID existe.
2. Editar solo `opencode.json` (sección `agent`, `model`, `small_model`).
3. Registrar el cambio en esta tabla y en la sección 2.

**Compatibilidad de versión:** los agentes usan el formato estable de OpenCode (`permission:` como mapa, carpeta `.opencode/agents/`). Si tu versión de OpenCode es la V2 y reporta campos obsoletos, cambia `permission:` por la lista `permissions:` (`action`, `resource`, `effect`) en cada agente.

## 4. Convenciones aprendidas

_(Los agentes agregan aquí hallazgos útiles y breves, uno por línea, con fecha.)_

- 2026-10-26 — Los modelos gratuitos rinden mejor con tareas de 1–3 archivos y criterios de aceptación explícitos.

## 5. Dependencias registradas

| Dependencia | Versión | Para qué | Spec |
|---|---|---|---|
| `github.com/go-chi/chi/v5` | última v5 | Router HTTP | 002 |
| `go.mongodb.org/mongo-driver/v2` | última v2 | Acceso a MongoDB | 002 |
| `github.com/go-playground/validator/v10` | última v10 | Validación de entrada | Primera spec funcional (no 002) |
| `github.com/stretchr/testify` | última | Aserciones en pruebas | 002 |
| `github.com/testcontainers/testcontainers-go` (+ módulo `mongodb`) | última | Pruebas de integración | 002 |

## 6. Pendientes y preguntas abiertas

- [ ] Aprobación humana de `specs/002-arquitectura-base/`.
- [ ] Definir el mecanismo de autenticación en la primera spec funcional.
- [ ] Definir proveedor de MongoDB para producción (p. ej. Atlas gratuito M0 o instancia propia) — **fuera de la spec 002**.
- [ ] Política de conservación de chat y evidencias (según el documento de idea; se define en specs funcionales).

## 7. Riesgos conocidos

- Los modelos gratuitos pueden desaparecer o limitar el uso sin aviso → mantener alternos listados en la sección 3.
- Algunos modelos gratuitos pueden usar los datos enviados para mejorarse → **no pegar secretos ni datos personales reales** en las conversaciones con agentes.
- `make` no viene en Windows por defecto → usar Git Bash/WSL o los comandos equivalentes.

## 8. Decisiones de producto del 28 de oct (noche)

| # | Decisión | Efecto |
|---|---|---|
| D-101 | Políticas POL-01…17 **confirmadas**; POL-02 pasa a **máximo 7 días** | Specs 006, 007 (backend y móvil) actualizadas |
| D-102 | Categorías (POL-14): herramientas manuales, herramientas eléctricas, construcción y jardinería, movilidad (patines, patinetas, bicicletas), deportes, hogar, otros | Spec 005 |
| D-103 | Valor declarado **no** visible a terceros | Spec 005 |
| D-104 | Hosting gratuito: **Render** (`vaiven-staging` y `vaiven-prod`) | Specs 001 y 013 |
| D-105 | Identificador de paquete `mx.vaiven.app` (permanente tras publicar) | Spec móvil 002 y 013 |
| D-106 | Meta original: primera versión pública en Google Play el 20 de nov (ver D-110). Solo Google Play; sin App Store | Specs 013; ROADMAP §1 |
| D-107 | Reportes de contenido y filtro de texto pasan a **P1 no recortables** (política de contenido de usuarios de Google Play) | Spec 011 |
| D-108 | Se agrega el proyecto `landing/` (sitio informativo estático, gratuito) con las páginas legales que exige Play | `vaiven-landing-page/`, ROADMAP |
| D-109 | La baja de cuenta **anonimiza** los datos personales (POL-17) | Spec backend 003 |
| D-110 | La cuenta de Google Play se compra el 29–30 oct: prueba cerrada 1–14 nov, solicitud 15 nov, publicación 1.0 (sin IA) meta 20 nov / estimada 22–25 nov. La IA va en la spec 014 como v1.1 posterior, detrás de un interruptor | ROADMAP §1 y §3; specs 013 |
| D-111 | **Entrega oficial: 1–4 dic con todos los specs completos.** Se agrega la spec 014 (IA: sugerir título y categoría por foto) como P1, v1.1, del 16 al 24 nov detrás del interruptor `AI_ENABLED`; proveedor gratuito y alcance académico por confirmar | ROADMAP §1 y §3; specs 014 |
| D-112 | ML = **recomendaciones** (spec 015): recomendador híbrido propio (coseno objeto-objeto + contenido) evaluado contra popularidad; IA = Gemini (spec 014). Ambas v1.1, 16–24 nov, con interruptores | ROADMAP §3; specs 014 y 015 |
| D-113 | **[REEMPLAZADA por D-115]** **Siete categorías de pruebas obligatorias** (unitarias, integración, contrato, base de datos, mutación, rendimiento, seguridad) y pipeline DevSecOps; constitución backend **v1.2.0** (Art. VII y VIII); spec 016. Equivalen a los proyectos de la solución de referencia `WebAPIDevSecOps` | Constitución; spec 016 |
| D-114 | Se separa el monorepo en repositorios independientes (`vaiven-backend`, `vaiven-frontend` y `vaiven-landing-page`): las rutas `../` pasan a ser locales o a apuntar al repositorio hermano; ROADMAP.md y docs/ se copian en cada repositorio; enmienda PATCH, sin cambios de principios; constitución backend **v1.2.1** | Constitución; AGENTS.md; specs |
| D-115 | Se elimina todo lo relativo a repositorio, ramas, PR y pipeline (incluida la spec 016 completa, que **reemplaza a D-113**: las siete categorías de pruebas y el pipeline DevSecOps quedan fuera hasta integrarlos de cero). Todo va por OpenCode y MCP: la spec **001 pasa a ser «Servicios por MCP»** (Atlas, Cloudinary, Render) y la arquitectura base es la 002; cada spec termina con una tarea «Despliegue» en staging (agente `devops-mcp`, único con permiso para los MCP) y la siguiente no empieza hasta que pase; constitución **v1.3.0** (Art. XIII). Render necesita un origen (repositorio o imagen): por definir | ROADMAP; constitution Art. XIII; specs 001 y 002; `.opencode/agents/devops-mcp.md` |


## Integracion DevSecOps — 2026-11-02

- [CI] Por solicitud expresa del responsable se prepara la integracion de ramas, PRs y seguridad que habia quedado fuera el 2026-11-01. Spec independiente: specs/017-devsecops/. No se modifica constitution.md ni se ejecutan specs funcionales.
- [CI] Herramientas exclusivas de CI: GitHub Actions fijadas por SHA, Gitleaks v8.30.1 (checksum verificado), CodeQL v4, golangci-lint v2.15.0, govulncheck v1.9.0. Sin dependencias nuevas de aplicacion.
- [CI] Revisores indicados: JesusTrz (escritura) y JosephYanez27 (lectura al preparar el cambio). El segundo necesita escritura para aprobaciones obligatorias y CODEOWNERS. Ruleset y ajustes administrativos quedan pendientes hasta que se apliquen y verifiquen.
- [CI] Estado inicial: documentacion sin aplicacion; CI informa las pruebas no aplicables y ejecuta politica/secretos. Despliegue Render por MCP y pruebas en Expo Go se conservan.
