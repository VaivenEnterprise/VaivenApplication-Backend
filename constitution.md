# Constitución del Backend — VAIVÉN

> **Versión 1.3.0** · Ratificada el 2026-10-26 · Enmendada el 2026-10-28 (Cloudinary para imágenes; Art. XII trabajo en paralelo), el 2026-10-29 (Art. VII), y el 2026-11-01 (repositorios separados; **MINOR:** se elimina todo lo de repositorio y pipeline —incluida la spec 016—, se agrega el Art. XIII de servicios y despliegue por MCP y se reordena el roadmap) · Proyecto: `vaiven-backend/` (Go + MongoDB)
>
> Esta es la máxima autoridad técnica del backend. Si `AGENTS.md`, `memoria.md`, `spec.md`, `plan.md`, `tasks.md`, un agente o una skill la contradicen, **gana la constitución** y el otro archivo debe corregirse.
> La constitución del móvil (`../vaiven-frontend/constitution.md`) comparte los Artículos IV y V; ambas deben mantenerse idénticas en esos puntos.

---

## 1. Propósito y alcance

VAIVÉN es una plataforma móvil de préstamo y renta de objetos entre personas, que opera en México y expresa todos los importes en pesos mexicanos (MXN). Contexto completo en `docs/VAIVEN_Idea_y_Objetivo.pdf`.

El backend es una **API REST en Go** con persistencia en **MongoDB**. Es la única fuente de verdad de los datos: la app móvil nunca accede a la base de datos.

**Alcance de esta constitución:** arquitectura, reglas técnicas y forma de trabajo. **No** define funcionalidad de negocio (usuarios, objetos, solicitudes, chat, etc.); eso se especifica en specs posteriores (`specs/003-*` en adelante) sin contradecir lo aquí escrito.

---

## 2. Principios no negociables

### Art. I — Spec primero (Spec Driven Development)
1. Ningún código se escribe sin una `spec.md` aprobada, un `plan.md` y un `tasks.md` de la misma carpeta `specs/NNN-nombre/`.
2. El orden es siempre: **constitución → spec → plan → tasks → implementación → revisión**.
3. Si durante la implementación se descubre que la spec es incorrecta, se **corrige la spec primero** y luego el código.
4. Cada tarea cambia un conjunto pequeño y verificable de archivos y se marca como hecha solo si cumple sus criterios de aceptación.

### Art. II — MongoDB es la única base de datos (NoSQL)
1. La persistencia es **exclusivamente MongoDB** (versión 7 o superior) mediante el driver oficial `go.mongodb.org/mongo-driver/v2`. Prohibido introducir bases relacionales, ORMs o capas SQL.
2. MongoDB se ejecuta como **replica set** (de un nodo en desarrollo) para poder usar transacciones cuando una spec las requiera.
3. El diseño de datos sigue los patrones de documentos de la sección 6 (incrustar vs. referenciar, instantáneas, índices declarados en código).
4. **Imágenes (enmienda 1.1.0):** los archivos binarios (fotos de objetos y evidencias) se almacenan en **Cloudinary** (capa gratuita). MongoDB guarda solo referencias (`publicId`, ancho, alto, formato, bytes, tipo de entrega), nunca el binario. Los datos de negocio siguen siendo exclusivamente de MongoDB.

### Art. III — Clean Architecture y regla de dependencia
1. Capas: `domain` → `usecase` → `adapter` (http, mongo) → `cmd`. **Las dependencias solo apuntan hacia adentro.**
2. `domain` y `usecase` **no importan** `net/http`, `chi`, `bson`, `mongo` ni ningún paquete de infraestructura.
3. Los puertos (interfaces) los declara la capa que los consume (`usecase`); los adaptadores los implementan.
4. Los handlers solo traducen HTTP ⇄ casos de uso; la lógica de negocio vive en `usecase`/`domain`.

### Art. IV — Contrato de API estable y compartido con el móvil
1. API REST JSON bajo el prefijo `/api/v1`. Endpoints de infraestructura (`/healthz`, `/readyz`) fuera del prefijo.
2. Campos JSON en `camelCase`; fechas en ISO-8601 UTC (salvo periodos de reserva, Art. V.3) (`2026-10-26T15:00:00Z`); identificadores como cadenas.
3. Errores siempre con este formato único:
   ```json
   { "error": { "code": "validation_failed", "message": "texto legible en español",
                "details": [{ "field": "email", "issue": "formato inválido" }],
                "requestId": "01J..." } }
   ```
   Códigos permitidos: `validation_failed`, `unauthorized`, `forbidden`, `not_found`, `conflict`, `rate_limited`, `internal_error`, `service_unavailable`.
4. Listas paginadas por cursor: `{ "items": [...], "nextCursor": "..." | null }`.
5. Cada respuesta incluye `X-Request-Id`. Si la petición trae uno válido (≤ 64 caracteres alfanuméricos y guiones) se reutiliza; si no, se genera. Los códigos `network_error` y `timeout` son propios del cliente móvil y el servidor nunca los emite.
6. Todo endpoint se documenta en `api/openapi.yaml` **antes** de implementarse. Un cambio incompatible exige nueva versión de ruta.

### Art. V — Dinero, fechas e idioma
1. Moneda única: **MXN**. No existe selección de moneda ni conversión.
2. Los importes se guardan y transmiten como **enteros en centavos** (`int64`, campo con sufijo `Cents`, p. ej. `amountCents`). Prohibido `float` para dinero.
3. Todas las fechas se almacenan y transmiten en **UTC**; la presentación en zona horaria local es responsabilidad del cliente.
   - **Excepción (v1.1.0):** los periodos de reserva (`startDate`, `endDate`) son **fechas de calendario** `YYYY-MM-DD`, inclusivas, interpretadas en `America/Mexico_City` (requiere `time/tzdata`). No se convierten a UTC ni se muestran en otra zona.
4. Mensajes de error y textos visibles al usuario en **español (es-MX)**; identificadores de código, logs y nombres técnicos en inglés.
5. VAIVÉN **no ejecuta cobros ni verifica pagos**: el backend solo registra lo acordado y lo reportado.

### Art. VI — Seguridad y privacidad por defecto
1. Secretos solo por variables de entorno; nunca en el repositorio. Se mantiene un `.env.example` sin valores reales.
2. Toda entrada se valida en la frontera HTTP (`validator/v10`) y se vuelve a validar en el dominio cuando proteja invariantes.
3. Contraseñas con `argon2id` (o `bcrypt` con costo ≥ 12 si no hay alternativa); jamás en logs ni respuestas.
4. Datos sensibles (ubicación exacta, conversaciones, evidencias) solo son accesibles a las personas autorizadas; las respuestas públicas exponen únicamente el mínimo necesario.
5. Los logs **no contienen** datos personales, tokens ni cuerpos de petición completos.
6. CORS con lista explícita de orígenes; límites de tamaño de cuerpo; tiempos de espera en servidor y cliente HTTP.
7. El `API secret` de Cloudinary vive solo en el backend. La app sube archivos con **firmas de un solo uso** emitidas por el backend. Las evidencias (entrega, devolución, incidencias) se suben como `authenticated` y solo se entregan mediante URLs firmadas de corta duración a las personas autorizadas.

### Art. VII — Pruebas y calidad
1. Toda tarea con lógica incluye pruebas. Mínimo **80 %** de cobertura en `domain` y `usecase`.
2. Pruebas unitarias sin infraestructura; pruebas de integración con MongoDB real (testcontainers) para repositorios e índices.
3. `gofmt`, `go vet` y `golangci-lint` deben pasar sin advertencias antes de dar una tarea por hecha.
4. Las pruebas se ejecutan **en local** con `make test`. El proyecto no define todavía repositorio, ramas ni pipeline automatizado: se integrarán aparte. Antes de cada despliegue (Art. XIII) la verificación la hace el agente `devops-mcp`.

### Art. VIII — Simplicidad y dependencias mínimas
1. Se prefiere la biblioteca estándar. Una dependencia nueva exige justificación en `plan.md` y un registro en `memoria.md`.
2. Prohibido construir abstracciones "por si acaso": solo lo que una spec vigente necesita.
3. Funciones cortas, un propósito por paquete, nombres explícitos.
4. **Dependencias previstas** (ya aprobadas, se registran en `memoria.md` al instalarse): `go-playground/validator/v10`, `golang.org/x/crypto` (argon2), `github.com/golang-jwt/jwt/v5`. Cloudinary y Expo Push se usan con `net/http` y la biblioteca estándar, **sin SDK**.

### Art. IX — Observabilidad
1. Logs estructurados con `log/slog` (JSON en producción) y un `requestId` por petición, devuelto en el encabezado `X-Request-Id`.
2. `/healthz` indica que el proceso vive; `/readyz` verifica la conexión a MongoDB.
3. Cierre ordenado (*graceful shutdown*) ante `SIGINT`/`SIGTERM`.

### Art. X — Desarrollo asistido con OpenCode y modelos gratuitos
1. Este proyecto se desarrolla **100 % con OpenCode** y **modelos gratuitos**, y los servicios externos se operan mediante **servidores MCP** (Art. XIII). Ninguna herramienta o modelo de pago es requisito.
2. Los modelos se asignan en `opencode.json` (un solo lugar). Los identificadores gratuitos rotan: al cambiarlos se registra en `memoria.md`.
3. Como los modelos gratuitos tienen menos capacidad, **las tareas se escriben pequeñas y explícitas**: archivos exactos, criterios de aceptación verificables y contexto mínimo.
4. Los agentes operan con **permisos mínimos** (ver `.opencode/agents/`). Un agente nunca ejecuta comandos destructivos ni modifica `constitution.md` sin aprobación humana.
5. Una persona revisa y aprueba cada spec, plan y conjunto de tareas antes de implementar.

### Art. XI — Gobernanza y enmiendas
1. Cambiar la constitución requiere: propuesta escrita, registro en `memoria.md` (sección *Decisiones*), incremento de versión y revisión de coherencia de todos los archivos.
2. Versionado: **MAJOR** cambia o elimina un principio; **MINOR** agrega un principio o sección; **PATCH** aclara redacción.

### Art. XII — Trabajo en paralelo (4 personas, cada una con su OpenCode)
1. Cada spec tiene **una persona dueña** (ver `ROADMAP.md`). Solo el dueño edita el `tasks.md` y el código de su spec; si necesita tocar algo de otra persona, lo avisa antes.
2. **Contrato primero:** la primera tarea de cada spec publica sus endpoints en `api/paths/<modulo>.yaml` (referenciado desde `api/openapi.yaml`) para que el móvil trabaje contra el contrato sin esperar al código.
3. **Módulos:** cada funcionalidad es un módulo (`internal/<modulo>/` con sus capas) que se registra con **una sola línea** en `cmd/api/modules.go`. Prohibido editar rutas de otro módulo.
4. **Puertos compartidos** (`notify.Notifier`, `moderation.Checker`, `chat.Opener`, `images.Service`) se definen una vez con implementación vacía y las specs posteriores los reemplazan sin cambiar a quien los consume.
5. **`memoria.md` es solo-anexar:** cada persona agrega líneas nuevas al final de su sección con su etiqueta (`[BE-A]`, `[BE-B]`); ante un conflicto de fusión se conservan ambas.
6. Mientras una dependencia entre specs no esté lista, se usa el puerto vacío; nunca se duplica ni se copia código de otro módulo.

### Art. XIII — Servicios y despliegue por MCP (enmienda 1.3.0)
1. Todo servicio externo (MongoDB Atlas, Cloudinary, Render) se aprovisiona, configura y verifica con **servidores MCP** invocados desde OpenCode (spec 001), no con paneles manuales. Solo se hace a mano lo que ningún MCP cubre (alta de cuenta, creación de las credenciales de API), y se anota en `memoria.md` sin valores secretos.
2. Los MCP se declaran **únicamente en `opencode.json`** y sus credenciales vienen de variables de entorno locales (`{env:...}`); nunca en el repositorio, en `memoria.md` ni en specs.
3. Solo el agente `devops-mcp` tiene permiso para usar los MCP; los demás agentes no. Las herramientas destructivas (`drop-database`, `drop-collection`, `delete-*`, borrar recursos) están denegadas por permiso; una excepción exige aprobación humana.
4. **Cada spec termina desplegada.** La última tarea de cada spec (tarea «Despliegue») se ejecuta solo cuando todas las anteriores están `[x]` y `make fmt lint test` pasa: `devops-mcp` lanza el despliegue de `vaiven-staging` en Render (`trigger_deploy`), espera a que termine, verifica `/healthz`, `/readyz` y los endpoints de la spec contra staging, y anota el resultado en `memoria.md`. No se empieza la spec siguiente hasta que ese despliegue verifique.
5. `vaiven-prod` solo se toca en la spec 013.
6. El origen del despliegue (repositorio o imagen que Render consume) queda **por definir** cuando se integre el repositorio; mientras no exista, la tarea «Despliegue» no puede cerrarse.

---

## 3. Stack tecnológico fijo

| Área | Decisión |
|---|---|
| Lenguaje | Go 1.24 o superior |
| Router HTTP | `github.com/go-chi/chi/v5` |
| Base de datos | MongoDB 7+ (replica set) |
| Driver | `go.mongodb.org/mongo-driver/v2` |
| Validación | `github.com/go-playground/validator/v10` |
| Logging | `log/slog` (biblioteca estándar) |
| Configuración | Variables de entorno (biblioteca estándar) |
| Pruebas | `testing` + `testify` + `testcontainers-go` (módulo mongodb) |
| Calidad | `gofmt`, `go vet`, `golangci-lint` |
| Contenedores | Docker + Docker Compose (desarrollo), Dockerfile multi‑etapa |
| Contrato | OpenAPI 3.1 en `api/openapi.yaml` + `api/paths/<modulo>.yaml` |
| Autenticación | JWT de acceso (`golang-jwt/jwt/v5`) + refresh rotativo; contraseñas con `argon2id` (`x/crypto`) |
| Imágenes | Cloudinary (capa gratuita), firmas emitidas por el backend, sin SDK |
| Notificaciones push | API HTTP de Expo Push, sin SDK |
| Servicios y despliegue | Servidores **MCP** declarados en `opencode.json`: MongoDB (Atlas), Cloudinary y Render |

## 4. Estructura del repositorio

```
vaiven-backend/             # raíz del repositorio
├─ cmd/api/main.go            # composición e inicio
├─ cmd/api/modules.go         # lista de módulos registrados (una línea por módulo)
├─ cmd/admin/                 # utilidades de administración (crear admin, sembrar datos)
├─ internal/
│  ├─ config/                 # lectura y validación de entorno
│  ├─ domain/                 # entidades, objetos de valor, errores de dominio
│  ├─ usecase/                # casos de uso y puertos (interfaces)
│  ├─ adapter/
│  │  ├─ http/                # router, handlers, middleware, DTO, respuestas
│  │  └─ mongo/               # repositorios, mapeo BSON, índices
│  └─ platform/               # logger, cliente Mongo, reloj, generador de IDs
├─ api/openapi.yaml
├─ test/e2e/                  # recorrido de punta a punta (spec 012); las unitarias van junto al código
├─ specs/NNN-nombre/{spec.md, plan.md, tasks.md}
├─ .opencode/{agents, skills}
├─ constitution.md · AGENTS.md · memoria.md · opencode.json
└─ docker-compose.yml · Dockerfile · Makefile · .env.example
```

## 5. Convenciones de código

- Paquetes en minúscula, sin guiones bajos; archivos en `snake_case.go`.
- Errores: se devuelven, no se ignoran; se envuelven con `fmt.Errorf("contexto: %w", err)`. Los errores de dominio son tipos propios que el adaptador HTTP traduce a los códigos del Art. IV.
- `context.Context` es el primer parámetro de toda operación que toque I/O.
- Sin variables globales mutables; las dependencias se inyectan por constructor.
- Comentarios de paquete y de símbolos exportados en español, concisos.

## 6. Convenciones de datos en MongoDB

1. **Colecciones:** `snake_case`, plural (`loan_requests`). **Campos:** `camelCase` (idénticos en BSON y JSON). `_id` es `ObjectID` y se expone como `id` (cadena).
2. **Incrustar** cuando los datos se leen juntos y tienen cardinalidad acotada; **referenciar** (guardando el `ObjectID`) cuando crecen sin límite o se consultan por separado. Prohibidos los arreglos de crecimiento ilimitado dentro de un documento.
3. **Instantáneas:** cuando un acuerdo debe conservar lo que se aceptó (precio, condiciones), se copia el dato al documento del acuerdo; no se referencia al original mutable.
4. **Todo documento** incluye `createdAt`, `updatedAt` (UTC) y, si admite ediciones concurrentes, un entero `version` para control optimista.
5. **Índices** declarados en código (`EnsureIndexes` por repositorio) y creados al arrancar de forma idempotente; toda consulta frecuente tiene índice. Se usan índices únicos para invariantes de unicidad.
6. **Validación de esquema** con `$jsonSchema` en colecciones críticas, además de la validación en la aplicación.
7. **Transacciones** solo cuando una operación deba ser atómica entre documentos; se prefiere diseñar para atomicidad de un solo documento.
8. Borrado lógico (`deletedAt`) o TTL según lo defina cada spec; nunca borrado físico de evidencia o historial sin política escrita.
9. Dinero: `…Cents` como `int64`; nunca `Decimal128` ni `double`.
10. Imágenes: objeto `{publicId, width, height, format, bytes, delivery}` con `delivery` = `public` o `authenticated`; máximo 5 por documento. Antes de guardar una referencia se valida que `publicId` pertenezca a la carpeta de la persona que la envía (`vaiven/<env>/<proposito>/<userId>/…`).
11. Ubicación: los listados públicos solo exponen zona y un punto **aproximado** (coordenadas redondeadas a 2 decimales ≈ 1 km); la ubicación exacta solo se comparte entre las partes de una reserva.

## 7. Definición de Hecho (DoD) de una tarea

- [ ] Cumple los criterios de aceptación de la tarea en `tasks.md`.
- [ ] Respeta todos los artículos de esta constitución (revisado por `revisor-constitucion`).
- [ ] `gofmt`, `go vet`, `golangci-lint` y `go test ./...` sin fallos.
- [ ] `api/openapi.yaml` actualizado si cambió el contrato.
- [ ] `memoria.md` actualizado si hubo decisión, dependencia nueva o aprendizaje relevante.
