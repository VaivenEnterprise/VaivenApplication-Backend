# Tareas 014 — IA generativa: sugerencias y asistente de proyectos (backend)

- **Dueño:** BE-A · **Estado:** sin ejecutar · `[P]` = paralelizable
- Se ejecuta **después** del congelamiento de la 1.0 (22 nov); no toca el AAB en revisión.
- Parte B (asistente, T008 a T013) agregada el 2026-11-16; spec aprobada por BE-B y revisada contra la constitución el mismo día. BE-A (dueño) confirma en el PR quién implementa (Art. XII.1). Si el calendario no alcanza, la parte B se entrega apagada (`ASSISTANT_ENABLED=false`) y se demuestra en staging.

## Comunes y parte A — Sugerencias al publicar

- [x] **T001** Contrato OpenAPI (A y B)
  - Archivos: `api/paths/ai.yaml`, `api/openapi.yaml` (referencias), `/meta` con `features.aiSuggestions` y `features.projectAssistant`
  - Agente: `spec-writer` · Depende de: 005-T001, 006-T001
  - Aceptación: rutas `listing-suggestions`, `assistant/messages` y `assistant/conversations/{id}`; cuerpos; tarjeta de coincidencia reutilizando el esquema de la búsqueda; errores (`validation_failed`, `forbidden`, `not_found`, `rate_limited`, `service_unavailable`); MO-A lo consume el mismo día
  - Refs: RF-01, RF-06, RF-08, RF-09, RF-13, RF-16
  - Hecho (2026-11-16, BE-B): `api/paths/ai.yaml` con `listing-suggestions`, `assistant/messages` y `assistant/conversations/{id}` (DELETE); esquemas en `api/openapi.yaml` (`ItemCard` reutilizada para las coincidencias); `Meta.features` opcional (compatible, Art. IV.6); etiqueta `ia`; contrato empaquetado para `/docs`. Redocly válido (solo los 2 avisos que ya existían). **Avisar a MO-A** para que lo consuma.

- [x] **T002** [P] Validación de salida y configuración
  - Archivos: `internal/ai/domain/{schema.go, schema_test.go}`, `internal/config/…` (variables `AI_*` y `ASSISTANT_*`), `.env.example`
  - Agente: `dev-go` · Depende de: T001
  - Aceptación: validación de esquemas de A y B, sin infraestructura (Art. III) (categoría POL-14, longitudes, conteos, campos extra fuera); valores por defecto seguros (`AI_ENABLED=false`, `ASSISTANT_ENABLED=false`); sin secretos
  - Refs: RF-03, RF-04, RF-10, RF-17 · P-01, P-03
  - Hecho (2026-11-16, BE-B): `internal/ai/domain` (ParseSuggestion/ValidateSuggestion, ParsePlan/ValidatePlan: categorías de POL-14 desde `internal/policy`, recortes por runas, máximo 8 necesidades, 10 pasos y 3 avisos, campos fuera del esquema descartados, listas vacías como `[]`, negativa válida, ErrNothingValid y ErrMalformed); `internal/config/ai.go` (AI_* y ASSISTANT_*, apagadas por defecto, clave como `Secret`, Ollama solo en development, HTTPS en staging y production). Cobertura del 100 % en los dos paquetes; prueba de arquitectura y lint en verde. La ruta pasó a `internal/ai/domain` para que la prueba de arquitectura la vigile (Art. III).

- [x] **T003** [P] Adaptador Gemini (y Ollama opcional, P2)
  - Archivos: `internal/ai/adapter/gemini/{client.go, client_test.go}`, `internal/ai/adapter/ollama/{client.go, client_test.go}`
  - Agente: `dev-go` · Depende de: T002
  - Aceptación: implementa `suggest.Describer` y `assistant.Planner` (P-01); `net/http` con tiempo de espera y un reintento; JSON con esquema; maneja errores y respuestas inválidas; pruebas con `httptest`; la clave nunca se registra. Ollama es opcional: solo con `APP_ENV=development` y se omite si el calendario no alcanza (Art. VIII.2)
  - Refs: RF-03, RF-07 · P-05, P-06
  - Hecho (2026-11-16, BE-B): `internal/ai/adapter/gemini` implementa `Generate` (P-01) con `net/http`, sin SDK: clave en la cabecera `x-goog-api-key` y nunca en la URL; instrucciones fijas en `systemInstruction`; turnos con rol; salida `application/json` con `responseSchema`; miniatura descargada solo por https, como máximo 5 MB, enviada en `inlineData`; un reintento ante 429, 5xx o red; 4xx sin reintento; bloqueos de seguridad → `ErrBlocked`; errores sin el cuerpo de la petición ni de la respuesta. Tipos de petición y esquemas en `internal/ai/domain/request.go` (categorías de POL-14 tomadas del código; el plan no tiene campos de objetos ni personas). Pruebas con `httptest` y sin red: adaptador 83 %, dominio 100 %; lint 0. **Ollama (P2) no se hizo:** queda opcional (Art. VIII.2).

- [x] **T004** [P] Contador de uso común en MongoDB
  - Archivos: `internal/ai/adapter/mongo/usage.go`, `internal/ai/adapter/mongo/usage_test.go`, índice en `Indexes()` (spec 012)
  - Agente: `modelador-mongo` · Depende de: T002
  - Aceptación: incremento atómico por persona, función y día; índice único `{userId, feature, day}` y TTL; prueba de concurrencia
  - Refs: RF-05, RF-15 · P-04
  - Hecho (2026-11-16, BE-B): `internal/ai/adapter/mongo/usage.go` (`ai_usage`: `{userId, feature, day, count, createdAt, updatedAt}`). `Take` es atómico con un solo FindOneAndUpdate con upsert y filtro `count < limit`; la clave duplicada del índice único significa «sin cupo». Índices `user_feature_day_unique` y `created_ttl` (3 días) en el catálogo (36 índices) y creados al arrancar la API (`cmd/api/main.go`), para que `admin indexes` coincida aunque la IA esté apagada. Integración con MongoDB 7 real: 50 peticiones simultáneas con límite 20 → exactamente 20 pasan. Se actualizaron las pruebas de la 012 que fijaban 34 índices y el fixture (solo se agregaron las 2 entradas de `ai_usage`).

- [x] **T005** Caso de uso de sugerencias (A)
  - Archivos: `internal/suggest/usecase/{suggest.go, suggest_test.go}`
  - Agente: `dev-go` · Depende de: T003, T004, 004-T007, 011 (filtro)
  - Aceptación: declara el puerto `Describer` (Art. III.3); verifica que la imagen es `item_photo` de esa persona (CA-02); construye la URL; aplica el límite (CA-05) y el filtro (CA-03); apagado o caída → `service_unavailable` (CA-04); cobertura ≥ 80 % (Art. VII.1)
  - Refs: RF-02, RF-04, RF-05, RF-06
  - Hecho (2026-11-16, BE-B): el caso de uso declara sus puertos `Describer`, `Images` y `Usage` (Art. III.3). Orden: apagado → validación del `publicId` → `OwnedBy` (foto ajena, evidencia, otra carpeta o una URL → 404 sin llamar al proveedor ni gastar cupo, CA-02) → `Resolve` (Cloudinary caído → 503) → cupo diario por día de **Ciudad de México** (CA-05; `ErrDailyLimit`, que la capa HTTP traduce a 429) → miniatura → Gemini con tiempo de espera de 10 s → validación de la salida → filtro de contenido (título rechazado → no hay sugerencia, CA-03; descripción rechazada → se quita). Caída, bloqueo o salida inválida → 503 con causa interna (CA-04). Cobertura 96,8 %; arquitectura y lint en verde.

- [x] **T006** Handler, rutas y registro (A)
  - Archivos: `internal/suggest/adapter/http/handler.go` (DTO y rutas en el mismo archivo, como el módulo de imágenes), `cmd/api/modules.go` (una línea), `cmd/api/main.go` (composición), `internal/adapter/http/handler/meta.go` (`features`)
  - Agente: `dev-go` · Depende de: T001, T005
  - Aceptación: cumple el contrato; autenticada; límite de tasa por ruta; registros sin imagen ni texto sugerido (RF-07)
  - Refs: RF-01, RF-07
  - Hecho (2026-11-16, BE-B): `POST /api/v1/ai/listing-suggestions` con sesión, `validator/v10`, campos desconocidos rechazados (`additionalProperties: false`) y 10 peticiones por minuto por persona (Redis con respaldo local). `ErrDailyLimit` → 429 `rate_limited` con `Retry-After` hasta la medianoche de Ciudad de México; los errores de dominio usan el envoltorio del Art. IV. Los registros no llevan la foto, el publicId ni la sugerencia; solo la causa interna cuando el proveedor cae. `GET /meta` expone `features.aiSuggestions` y `features.projectAssistant`. Una línea en `modules.go`. Con `AI_ENABLED=false` no se crea el cliente de Gemini. Handler al 95 %; todas las pruebas unitarias del repositorio y el lint en verde.

- [x] **T007** Prueba de integración (A)
  - Archivos: `internal/suggest/integration_test.go`
  - Agente: `qa-go` · Depende de: T006
  - Aceptación: CA-01 a CA-06 con proveedor falso; sin red; limpia sus datos
  - Refs: RNF-01
  - Hecho (2026-11-16, BE-B): `internal/suggest/integration_test.go` recorre el endpoint real (router, JWT real, `images.Service` contra un Cloudinary simulado, contador en MongoDB 7 real con Testcontainers y proveedor de IA falso, sin red): CA-01 (la miniatura la arma el servidor con https://res.cloudinary.com), CA-02 (foto ajena, evidencia o inexistente → 404 sin llamar al proveedor), CA-03, CA-04 (apagada, proveedor caído y Cloudinary caído → 503), CA-05 (con límite 3: la cuarta → 429; otra persona tiene su propio cupo) y CA-06 (`git grep --untracked` no encuentra un valor de `AI_API_KEY`; verificado que falla si se siembra una clave). Cada caso usa su propia base. Corre en la CI con `-tags=integration`.

## Parte B — Asistente de proyectos

- [x] **T008** [P] Instrucciones fijas y reglas del asistente
  - Archivos: `internal/assistant/domain/{prompt.go, prompt_test.go}`
  - Agente: `dev-go` · Depende de: T002
  - Aceptación: instrucciones del sistema fijas con alcance y negativas (RF-14); el mensaje y los turnos van como bloque de datos delimitado (P-08); la lista de categorías de POL-14 sale del código, no se escribe a mano; pruebas de que ninguna entrada cambia las instrucciones
  - Refs: RF-10, RF-14, RF-17 · P-08
  - Hecho (2026-11-16, BE-B): `SystemPrompt` fijo, en español de México, con alcance, negativas (RF-14), «nunca inventes objetos ni personas» y las categorías tomadas de `policy.POL14Categories`. El mensaje de la persona viaja en `<mensaje_de_la_persona>…</mensaje_de_la_persona>`: `Sanitize` neutraliza cualquier variante del delimitador (con espacios, mayúsculas o barra), quita caracteres de control y saltos, y recorta a 500. `BuildRequest` conserva como máximo 6 turnos contando el nuevo y no modifica el historial recibido; `ModelTurn` resume la respuesta previa sin coincidencias; `Normalize` y `CacheKey` (SHA-256, sin el texto) sirven para la caché. Pruebas de inyección (CA-09): ninguna entrada cambia las instrucciones ni rompe el bloque. Cobertura 96,5 %; sin dependencias nuevas (los acentos se quitan con la biblioteca estándar, Art. VIII.1); sin estado global mutable (§5).

- [x] **T009** [P] Conversaciones en MongoDB
  - Archivos: `internal/assistant/adapter/mongo/{conversations.go, conversations_test.go}`, índices en `Indexes()`
  - Agente: `modelador-mongo` · Depende de: T002
  - Aceptación: `ai_conversations` con dueño, `createdAt` y `updatedAt` (§6.4), máximo 6 turnos de hasta 2 000 caracteres (`$slice`, §6.2), TTL de 24 h sobre `updatedAt` y borrado por dueño; otra persona → no encontrada; pruebas con MongoDB real
  - Refs: RF-13 · P-09
  - Hecho (2026-11-16, BE-B): `internal/assistant/adapter/mongo/conversations.go` (`ai_conversations`: `{_id, userId, turns[{role,text}], createdAt, updatedAt}`) con Create, Get, Append (`$push` + `$each` + `$slice: -6`) y Delete, todos filtrando por `_id` y `userId`: otra persona, un id inválido o una conversación caducada → `ErrConversationNotFound` (404, CA-13). Textos recortados a 2 000 runas; nunca guarda la ubicación. TTL `updated_ttl` de 24 h en el catálogo (37 índices), en la API y en el fixture (solo una entrada nueva). Integración con MongoDB 7 real en verde.

- [x] **T010** [P] Buscador de coincidencias sobre la spec 006
  - Archivos: `internal/assistant/adapter/search/{finder.go, finder_test.go}`
  - Agente: `dev-go` · Depende de: T002, 006 (caso de uso de búsqueda)
  - Aceptación: por necesidad llama a la búsqueda con `q`, `category` y ubicación o municipio; hasta 5 resultados con la tarjeta pública (`approxLocation`, `distanceKm`); máximo 4 búsquedas simultáneas con tiempo de espera; hereda los filtros de visibilidad de la 006 (sin propios, pausados ni cuentas suspendidas)
  - Refs: RF-11, RNF-01 · P-07, P-10
  - Hecho (2026-11-16, BE-B): puertos `Finder`, `FindRequest` y `NeedResult` en `internal/assistant/usecase/ports.go` (reutilizan `searchusecase.CardView` de la 006, sin copiarla, Art. XII.6). `internal/assistant/adapter/search`: con ubicación busca término por término (la búsqueda por cercanía exige todas las palabras) y junta resultados únicos hasta 5; sin ubicación hace una sola búsqueda de texto o filtra por municipio; como máximo 4 búsquedas simultáneas entre todas las necesidades; 3 s por búsqueda; una falla deja esa necesidad en `[]` y solo si fallan todas se devuelve error; conserva el orden y nunca devuelve `null` (CA-08, error real encontrado por la prueba). Cobertura 98,5 %; lint y arquitectura en verde.

- [x] **T011** Caso de uso del asistente
  - Archivos: `internal/assistant/usecase/{ports.go, assistant.go, assistant_test.go}`
  - Agente: `dev-go` · Depende de: T003, T004, T008, T009, T010, 011 (filtro)
  - Aceptación: declara los puertos `Planner` y `Finder` (Art. III.3); arma el contexto (máximo 6 turnos); llama al proveedor **sin ubicación** (CA-11); valida y recorta la salida (CA-12); filtro de contenido en entrada y salida; caché en Redis del primer mensaje normalizado sin cachear coincidencias (CA-14); límites diarios y por minuto (CA-05); negativas (CA-10); necesidades sin coincidencias (CA-08); apagado o caída → `service_unavailable`; la ubicación no se guarda; textos en es-MX; cobertura ≥ 80 % (Art. VII.1)
  - Refs: RF-09 a RF-19
  - Hecho (2026-11-16, BE-B): `Send` sigue este orden: apagado → longitud medida **antes** de recortar (501 caracteres = 400; error real que encontró la prueba) → filtro de contenido de la entrada (sin gastar cupo ni llamar al proveedor) → conversación propia (CA-13) → cupo diario en hora de México → plan (caché de 1 h solo en el primer mensaje, CA-14; el proveedor nunca recibe la ubicación, CA-11; un bloqueo del proveedor da una negativa amable, CA-10; una salida inválida o una caída da 503, CA-12) → filtro de la salida (resumen bloqueado → 503; necesidades, pasos y avisos bloqueados se quitan) → búsqueda → guarda los turnos (usuario en bloque de datos y resumen del modelo) → resumen con «Por ahora nadie presta cerca de ti: …» escrito por el backend (CA-08). `Delete` para la conversación propia. Cobertura 97,3 %; arquitectura y lint en verde.

- [ ] **T012** Handler, rutas y registro (B)
  - Archivos: `internal/assistant/adapter/http/{handler.go, dto.go, routes.go}`, `cmd/api/modules.go` (una línea)
  - Agente: `dev-go` · Depende de: T001, T011
  - Aceptación: cumple el contrato; autenticada; 10 mensajes por minuto por ruta; `DELETE` de la conversación propia; registros sin el texto de mensajes ni respuestas (RF-18)
  - Refs: RF-09, RF-13, RF-15, RF-18

- [ ] **T013** Prueba de integración (B)
  - Archivos: `internal/assistant/integration_test.go`
  - Agente: `qa-go` · Depende de: T012
  - Aceptación: CA-07 a CA-14 con proveedor falso, objetos reales en MongoDB y Redis real; inyección de instrucciones; sin red; limpia sus datos
  - Refs: RNF-01

## Cierre

- [ ] **T014** Variables en producción y revisión
  - Archivos: `docs/produccion.md` (sección IA), `memoria.md` (solo anexar)
  - Agente: `revisor-constitucion` · Depende de: T007, T013, 013-T008
  - Aceptación: pasos para definir `AI_API_KEY`, `AI_ENABLED` y `ASSISTANT_ENABLED` en el `.env` de la VM, sin valores; procedimiento de apagado; privacidad revisada (RF-07, RF-18); **CONFORME**
  - Refs: RF-06, RF-16, CA-06

- [ ] **T015** Despliegue de la spec en Azure (staging)
  - Archivos: `memoria.md` (solo anexar)
  - Agente: `devops-mcp` · Depende de: T014 (y todas las anteriores `[x]`)
  - Aceptación: `make fmt lint test` en verde; al fusionar en `develop`, `deploy-staging.yml` despliega staging; `/healthz` y `/readyz` devuelven 200 por HTTPS sin `-k`; cada endpoint nuevo de `api/paths/ai.yaml` responde con una petición correcta y una inválida en el formato de error del Art. IV; una conversación real con Gemini en staging devuelve necesidades, coincidencias y pasos; registros sin secretos ni datos personales; resultado anotado en `memoria.md`. **La spec siguiente no empieza hasta que esta tarea pase.**
  - Refs: Art. XIII.5

## Trazabilidad
| Requisito | Tareas |
|---|---|
| RF-01, RF-02, RF-04, RF-05, RF-06, RF-07 | T001, T005, T006, T007 |
| RF-03, RF-17 | T002, T003, T008 |
| RF-08 | T001 |
| RF-09 a RF-16, RF-18 | T008 a T013 |
| RNF-01 | T007, T010, T013 |

| Total | 15 tareas |
|---|---|
