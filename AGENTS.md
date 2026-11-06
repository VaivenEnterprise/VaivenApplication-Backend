# AGENTS.md — Backend VAIVÉN (Go + MongoDB)

Instrucciones para cualquier agente de OpenCode que trabaje en `vaiven-backend/`. Este archivo se carga automáticamente. **Subordinado a `constitution.md`.**

## 1. Orden de lectura obligatorio

1. `constitution.md` — reglas no negociables (artículos I a XII). Para saber qué spec te toca, quién es el dueño y en qué orden, lee `ROADMAP.md`.
2. `memoria.md` — decisiones, estado actual y aprendizajes.
3. `specs/<spec-activa>/spec.md` → `plan.md` → `tasks.md`.
4. Solo entonces, el código.

Si algo no está definido en esos archivos, **no lo inventes**: pregunta a la persona o anótalo como `[NEEDS CLARIFICATION]` en la spec.

## 2. Proyecto en una línea

API REST en Go (chi) con MongoDB para VAIVÉN, una plataforma móvil de préstamo y renta de objetos en México (MXN). Arquitectura Clean: `domain → usecase → adapter → cmd`.

## 3. Flujo SDD (cómo trabajar)

| Fase | Quién | Resultado |
|---|---|---|
| Especificar | `plan` + subagente `spec-writer` | `spec.md` (qué y por qué, sin tecnología) |
| Planificar | `plan` + `spec-writer` (+ `modelador-mongo` si hay datos) | `plan.md` (cómo) |
| Dividir | `plan` + `spec-writer` | `tasks.md` (tareas pequeñas) |
| Implementar | `build` + `dev-go`, `modelador-mongo` | código + pruebas, **una tarea por vez** |
| Probar | `qa-go` | pruebas y cobertura |
| Revisar | `revisor-constitucion` | informe de conformidad |

Detalle en la skill `sdd-workflow`.

## 4. Agentes disponibles

Agentes nativos de OpenCode (se reutilizan tal cual):

| Agente | Modo | Uso en este proyecto |
|---|---|---|
| `plan` | primario | Especificar, planificar y dividir tareas. Solo lectura de código. |
| `build` | primario | Implementar tareas aprobadas. |
| `explore` | subagente | Buscar en el código sin modificarlo. |
| `general` | subagente | Investigación de varios pasos. |

Subagentes propios (`.opencode/agents/`):

| Agente | Responsabilidad | Edita código |
|---|---|---|
| `spec-writer` | Redacta `spec.md`, `plan.md`, `tasks.md` y actualiza `memoria.md` | No (solo `specs/` y `memoria.md`) |
| `dev-go` | Implementa tareas en Go siguiendo Clean Architecture | Sí |
| `modelador-mongo` | Diseña colecciones, índices, repositorios y validación de esquema | Sí (`adapter/mongo`) |
| `qa-go` | Escribe y ejecuta pruebas, mide cobertura | Solo `*_test.go` |
| `gestor-repo` | Crea ramas, commits y PR en GitHub por MCP; único agente con permiso para el MCP de GitHub; nunca fusiona ni borra | No |
| `devops-mcp` | Aprovisiona Azure y despliega cada spec en staging con el **MCP de Azure**, `az` y los MCP de Cloudinary; único agente con permiso para ellos | No (solo `memoria.md` y docs) |
| `revisor-constitucion` | Verifica conformidad con la constitución | No |

Invocación: `@dev-go implementa la tarea T005 de specs/002-arquitectura-base/tasks.md`.

## 5. Skills disponibles (`.opencode/skills/`)

| Skill | Cuándo cargarla |
|---|---|
| `sdd-workflow` | Al crear o ejecutar spec, plan o tareas |
| `go-clean-architecture` | Al crear o modificar paquetes de Go |
| `mongodb-go-patterns` | Al tocar colecciones, índices o repositorios |
| `api-rest-conventions` | Al crear o modificar endpoints, errores o `openapi.yaml` |
| `mcp-deploy` | Al aprovisionar Azure o desplegar una spec (solo `devops-mcp`) |

## 5b. Azure, MCP y despliegue (Art. XIII y XIV)

- Infraestructura: una VM de Azure (cuenta gratuita) con Docker Compose: la API (una réplica, **sirve HTTPS forzoso** con certificado de Let's Encrypt), **MongoDB** y **Redis** (obligatorios). No hay nginx. Cloudinary sigue para imágenes. Los MCP (`azure`, `cloudinary-assets`, `cloudinary-config`) están en `opencode.json`; sus credenciales son `az login` y `CLOUDINARY_URL`, nunca archivos del repositorio.
- **Trabajo humano mínimo:** la persona solo se registra (Azure, Cloudinary), hace `az login`, define `CLOUDINARY_URL` y aprueba acciones. **Todo lo demás lo hace `devops-mcp` desde OpenCode**, primero con los MCP y, si no cubren algo, con `az` o SSH.
- **Solo `devops-mcp` usa los MCP, `az` y SSH a la VM.** Si tu tarea necesita infraestructura, pide `@devops-mcp ...`.
- La API debe ser **sin estado** (Art. XIV): nada compartido en memoria; límite de tasa y caché en Redis (obligatorio) por los puertos `ratelimit.Limiter` y `cache.Cache`; Redis nunca es fuente de verdad.
- **Orden de trabajo:** spec 001 (Azure y servicios) → 002 (base) → 003 … Cada spec termina con su tarea «Despliegue» (`@devops-mcp` ejecuta `scripts/deploy.sh staging` y verifica por HTTPS). No empieces la siguiente spec hasta que pase.

## 6. Comandos

```bash
make run          # levanta la API (requiere MongoDB en marcha)
make up           # docker compose up -d (MongoDB replica set)
make down         # docker compose down
make test         # go test ./... -race -cover
make lint         # golangci-lint run
make fmt          # gofmt -s -w .
make build        # compila bin/api
```

Si `make` no está disponible en Windows, usar Git Bash o WSL, o ejecutar el comando equivalente que aparece en el `Makefile`.

## 7. Reglas de oro

1. **Una tarea a la vez.** Termina y verifica antes de pasar a la siguiente.
2. **Toca solo los archivos listados** en la tarea. Si necesitas otro, avisa y justifica.
3. **Sin dependencias nuevas** sin registrarlas en `plan.md` y `memoria.md`.
4. **Nunca** uses `float` para dinero, SQL, ni importes `mongo`/`bson` en `domain` o `usecase`.
5. **Nunca** escribas secretos, datos personales ni tokens en código ni en logs.
6. **Nunca** modifiques `constitution.md`; propón la enmienda por escrito.
7. Al terminar una tarea: ejecuta `make fmt lint test`, marca `[x]` en `tasks.md` y actualiza `memoria.md` si aprendiste algo.
8. Si un comando falla dos veces con el mismo error, **detente** y explica el problema en lugar de probar variaciones al azar.
9. Responde en español; el código y los identificadores, en inglés.

## 8. Trabajo en paralelo (4 personas, Art. XII)

- Trabaja **solo en la spec asignada** a tu persona en `ROADMAP.md` y solo si las specs de las que depende ya están **desplegadas** (Art. XIII).
- Tu primera tarea de cada spec publica el contrato en `api/paths/<modulo>.yaml`.
- Registra tu módulo con **una línea** en `cmd/api/modules.go`. No edites rutas ni archivos de otros módulos.
- Los puertos compartidos (`notify.Notifier`, `moderation.Checker`, `chat.Opener`, `images.Service`) ya existen con implementación vacía: úsalos, no los reimplementes.
- `memoria.md` es solo-anexar: escribe al final de tu sección con tu etiqueta (`[BE-A]` o `[BE-B]`).

## 9. Alcance

Specs `001` a `015` en `specs/`, en el orden y con los dueños de `ROADMAP.md`. Las tareas aún **no se ejecutan** hasta la fecha de arranque del calendario y la aprobación de cada spec.

## Repositorio y PR (Art. XV)
- Una rama de integracion por spec (`spec/SCRUM-NN-SPEC-NN`), creada desde `develop`: implementar sus tareas ahi o en ramas `tipo/SCRUM-NN-SPEC-NN-TNNN` con PR hacia la misma SPEC. Al completar la spec, PR hacia `develop`; publicar con PR `develop` -> `main`. Se conserva tarea -> `develop`. Titulos de PR libres; completar Resumen, Validación y Seguridad. Solo `@gestor-repo` usa el MCP de GitHub; nunca fusiona ni borra: una persona revisa y fusiona. Nunca se suben secretos.

