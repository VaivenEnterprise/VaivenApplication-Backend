# Constitución del Backend — VAIVÉN

> **Versión 1.4.0** · Ratificada el 2026-10-26 · Enmendada el 2026-10-28 (Cloudinary para imágenes; Art. XII trabajo en paralelo), el 2026-10-29 (Art. VII), y el 2026-11-01 (repositorios separados; **MINOR:** se elimina todo lo de repositorio y pipeline —incluida la spec 016—, se agrega el Art. XIII de servicios y despliegue por MCP y se reordena el roadmap) · Proyecto: `vaiven-backend/` (Go + MongoDB)
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

### Art. XIII — Infraestructura en Azure y despliegue por MCP (enmienda 1.4.0)
1. La infraestructura del backend es **Azure, en la cuenta principal del proyecto** (la misma con la que se compró la licencia de Google Play; no en una cuenta personal ni de estudiantes), con la **cuenta gratuita de Azure** (crédito de 200 USD durante 30 días, 12 meses de servicios gratuitos y tarjeta registrada, pero **sin cobros mientras no se actualice a pago por uso**): una máquina virtual Linux de una talla elegible para la capa gratuita, con **Docker Compose**; Azure es obligatorio para el curso. Fuera de Azure solo quedan Cloudinary (imágenes) y la landing (Render). La talla preferida es **Standard_B2ats_v2** (x86-64, 2 vCPU, 1 GiB); alternativas: `Standard_B1s` y `Standard_B2pts_v2` (ARM64, que exige compilar la imagen para `linux/arm64`). Todo debe mantenerse en capas gratuitas, con una **excepción conocida**: la IP pública (SKU Estándar, obligatoria desde 2025) cuesta cerca de 3–4 USD al mes y sale del crédito; cualquier otro recurso con costo se detiene y se consulta.
2. **Trabajo humano mínimo; todo lo demás lo hace OpenCode.** La persona solo se **registra** (cuenta de Azure y de Cloudinary), inicia sesión (`az login`), copia `CLOUDINARY_URL` y **aprueba** las acciones que OpenCode le solicite. Crear la VM y los discos, abrir puertos, fijar la IP, instalar Docker, levantar MongoDB y Redis, emitir el certificado, desplegar y verificar lo hace `devops-mcp` desde OpenCode, usando **primero los MCP** (MCP de Azure —no en modo solo lectura— y MCP de Cloudinary) y, solo para lo que ningún MCP cubre, la CLI `az` o SSH a la VM, siempre ejecutados por OpenCode. Cualquier paso manual adicional se justifica en `docs/servicios.md`.
3. Los MCP se declaran **únicamente en `opencode.json`**; sus credenciales vienen de variables de entorno locales (`{env:...}`) o de la sesión de `az`; nunca en el repositorio, en `memoria.md` ni en specs.
4. Solo el agente `devops-mcp` puede usar los MCP, la CLI `az` y SSH a la VM. Como el MCP de Azure permite también borrar, **cada llamada a Azure pide aprobación** y el agente tiene prohibido borrar la VM, los discos, la IP, las reglas de red o el grupo de recursos, y ejecutar `docker compose down -v` o borrar volúmenes (excepción solo con aprobación humana explícita).
5. **Cada spec termina desplegada.** La última tarea de cada spec (tarea «Despliegue») se ejecuta solo cuando todas las anteriores están `[x]` y `make fmt lint test` pasa: `devops-mcp` ejecuta `scripts/deploy.sh staging`, que construye la imagen (para la arquitectura de la VM), la envía a la VM, reinicia la API (un corte de unos segundos) y espera a que `/healthz` responda; después verifica por **HTTPS** `/healthz`, `/readyz` y los endpoints de la spec, revisa los registros y anota el resultado en `memoria.md`. No se empieza la spec siguiente hasta que ese despliegue verifique.
6. **Entornos:** `staging` (specs 001–012, puerto 8443) y `production` (spec 013, puerto 443) son proyectos de Compose separados, con bases de datos, volúmenes y secretos distintos. Como la capa gratuita da una sola VM, comparten la máquina con límites de memoria por contenedor.
7. **El despliegue no depende del repositorio:** la imagen se construye en la máquina de quien despliega y viaja a la VM por SSH (`docker save | ssh … docker load`). No hay registro de imágenes (Azure Container Registry no es gratuito).

### Art. XIV — Redis obligatorio y HTTPS forzoso (enmienda 1.4.0)
1. La API es **sin estado**: nada que deba compartirse entre peticiones vive en la memoria del proceso, de modo que pueda escalar a más réplicas sin cambiar código. Hoy corre **una sola réplica** (`api`): la VM gratuita tiene ~1 GiB de RAM y **no hay nginx ni balanceador**; se acepta un corte de unos segundos al desplegar.
2. **Redis es obligatorio** (contenedor en la VM, sin puerto publicado, con contraseña y `maxmemory`). Se usa para el **límite de tasa** (POL-11 y los intentos de inicio de sesión) y para **caché de lecturas** con TTL corto (categorías y resultados de búsqueda). **Redis nunca es la fuente de verdad:** todo estado de negocio (QR de un solo uso, sesiones, tokens, candados de trabajos) vive en MongoDB. Si Redis cae, la API sigue funcionando: el límite de tasa baja a un límite local, la caché se omite y `/readyz` informa `redis: degradado` sin devolver 503.
3. Redis se accede por puertos (`ratelimit.Limiter`, `cache.Cache`) y solo el adaptador importa `github.com/redis/go-redis/v9` (Art. III). Las claves llevan el prefijo `vaiven:<entorno>:`.
4. **HTTPS forzoso, sin proxy.** En `staging` y `production` la API **solo** sirve HTTPS (no existe puerto HTTP de la API). El certificado es de **Let's Encrypt** para el nombre DNS de la IP pública, emitido y renovado con `certbot` en la VM (validación en el puerto 80) y montado de solo lectura en el contenedor; la API lee el certificado de archivos (`TLS_CERT_FILE`, `TLS_KEY_FILE`) y lo **recarga sin reiniciarse**. En desarrollo local puede usarse HTTP. MongoDB y Redis **nunca** publican puertos.
5. Cada contenedor tiene límite de memoria; la API usa `GOMEMLIMIT`; la VM lleva 2 GB de swap (las tallas gratuitas nuevas no traen disco temporal).

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
| Contenedores | Docker + Docker Compose (desarrollo y despliegue), Dockerfile multi‑etapa |
| Contrato | OpenAPI 3.1 en `api/openapi.yaml` + `api/paths/<modulo>.yaml` |
| Autenticación | JWT de acceso (`golang-jwt/jwt/v5`) + refresh rotativo; contraseñas con `argon2id` (`x/crypto`) |
| Imágenes | Cloudinary (capa gratuita), firmas emitidas por el backend, sin SDK |
| Notificaciones push | API HTTP de Expo Push, sin SDK |
| Nube | **Azure** (cuenta gratuita): una VM Linux con Docker Compose |
| TLS | **Let's Encrypt** con `certbot` en la VM; la API sirve HTTPS directamente (Art. XIV.4), sin nginx |
| Caché y límite de tasa (obligatorio) | **Redis 7**, cliente `github.com/redis/go-redis/v9` (Art. XIV) |
| Servicios y despliegue | **MCP de Azure** (`@azure/mcp`) + CLI `az` + MCP de Cloudinary, declarados en `opencode.json` |

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
├─ deploy/                    # bootstrap.sh de la VM, base/ (Mongo+Redis+TLS) y docker-compose.yml de la API (specs 001 y 002)
├─ scripts/                   # deploy.sh <env> [--rollback]: construye, envía por SSH y reinicia (spec 002)
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
