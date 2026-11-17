# Tareas 014 — IA generativa: sugerencias y asistente de proyectos (backend)

- **Dueño:** BE-A · **Estado:** completa (17/17), desplegada y verificada en staging el 2026-11-17 · `[P]` = paralelizable
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

- [x] **T012** Handler, rutas y registro (B)
  - Archivos: `internal/assistant/adapter/http/handler.go` (DTO y rutas en el mismo archivo), `cmd/api/modules.go` (una línea), `cmd/api/main.go` (composición y caché), `internal/search/adapter/http/handler.go` (exporta `CardResponse`/`ToCard` para no copiar la tarjeta, Art. XII.6)
  - Agente: `dev-go` · Depende de: T001, T011
  - Aceptación: cumple el contrato; autenticada; 10 mensajes por minuto por ruta; `DELETE` de la conversación propia; registros sin el texto de mensajes ni respuestas (RF-18)
  - Refs: RF-09, RF-13, RF-15, RF-18
  - Hecho (2026-11-16, BE-B): `POST /api/v1/ai/assistant/messages` y `DELETE /api/v1/ai/assistant/conversations/{id}` con sesión y 10 por minuto por persona; `validator/v10` (mensaje de 2 a 500 runas, `conversationId` de 24 hexadecimales, `lat`/`lng` en rango) y campos desconocidos rechazados; detalles de validación por campo; las coincidencias salen con la misma `ItemCard` de la búsqueda (`searchhttp.ToCard`); listas vacías como `[]`; límite diario → 429 con `Retry-After` hasta la medianoche de México. Los registros no llevan mensaje, respuesta ni ubicación. La caché usa Redis solo si está configurado (sin Redis, `nil`: nada en la memoria del proceso, Art. XIV.1). Con `ASSISTANT_ENABLED=false` no se crea el cliente de Gemini. Handler al 97 %; todas las pruebas y el lint en verde (también con `integration`).

- [x] **T013** Prueba de integración (B)
  - Archivos: `internal/assistant/integration_test.go`
  - Agente: `qa-go` · Depende de: T012
  - Aceptación: CA-07 a CA-14 con proveedor falso, objetos reales en MongoDB y Redis real; inyección de instrucciones; sin red; limpia sus datos
  - Refs: RNF-01
  - Hecho (2026-11-16, BE-B): `internal/assistant/integration_test.go` recorre el endpoint real (JWT real, búsqueda real de la 006 sobre cuentas y objetos sembrados en MongoDB 7, conversaciones y contador reales, caché en Redis 7 con Testcontainers y proveedor falso, sin red). CA-07 (solo objetos activos de otras personas con cuenta activa, dentro del radio y por distancia: quedan fuera el propio, el pausado, el de cuenta suspendida y el de CDMX; tarjetas con ubicación aproximada y distancia), CA-08 (`[]` y nota en el resumen), CA-09, CA-10, CA-11 (la ubicación no llega al proveedor ni a la conversación), CA-12 (503 y nada guardado), CA-13 (historial al proveedor, 404 para otra persona, DELETE 204 y luego 404) y CA-14 (el mismo primer mensaje de otra persona sale de Redis, y sus coincidencias se recalculan: no ve sus objetos). Cada caso usa su propia base y la borra. Además se corrigió `catalog_integration_test.go` (spec 012): su siembra genérica no traía `feature` ni `day` y chocaba con el índice único de `ai_usage` desde T004.

## Cierre

- [x] **T014** Variables en producción y revisión
  - Archivos: `docs/produccion.md` (sección IA), `memoria.md` (solo anexar)
  - Agente: `revisor-constitucion` · Depende de: T007, T013, 013-T008
  - Aceptación: pasos para definir `AI_API_KEY`, `AI_ENABLED` y `ASSISTANT_ENABLED` en el `.env` de la VM, sin valores; procedimiento de apagado; privacidad revisada (RF-07, RF-18); **CONFORME**
  - Refs: RF-06, RF-16, CA-06
  - Hecho (2026-11-17, BE-B): `docs/produccion.md` §11 (clave por entorno, encendido, apagado de emergencia, privacidad y producción como v1.1); `scripts/set-ai-key.sh` (valida la clave con Google antes de tocar la VM, la envía como parámetro protegido, se niega si coincide con la del otro entorno y borra el run command; probado con archivos de prueba); variables de IA en `deploy/.env.example` y `deploy/production/.env.example` (apagadas, sin valores).
  - **Revisión de conformidad (revisor-constitucion, 2026-11-17): CONFORME**, con evidencia: I (spec, plan y tasks aprobados por BE-B; revisados contra la constitución el 16 nov) · III (puertos declarados por cada caso de uso; prueba de arquitectura en verde) · IV (contrato `ai.yaml` antes del código; las 3 rutas registradas coinciden con el contrato; envoltorio de errores) · V (textos en es-MX; centavos en la tarjeta) · VI (`validator/v10`; clave como `Secret`, en cabecera; gitleaks sin fugas en la rama; los registros solo llevan la causa del error, nunca fotos, mensajes, respuestas ni ubicación) · VII (dominio y casos de uso: 100 %, 96,5 %, 97,3 % y 96,8 %; integración con MongoDB y Redis reales; 90 paquetes en verde) · VIII (`go.mod` sin cambios; sin SDK) · XII (una línea por módulo; la tarjeta de la 006 se reutiliza, no se copia) · XIII.1 (Gemini en capa gratuita) · XIV (Redis solo para caché y límites; sin Redis, sin caché en el proceso).
  - Observaciones (no bloquean): (1) BE-A, dueño de la 014, debe confirmar en el PR (Art. XII.1); (2) revisar *Cost Management* después de encender la IA en staging; (3) Ollama (P2) no se hizo, es opcional; (4) el aviso de privacidad y «Seguridad de datos» de Play son de la app (mobile 014-T006); (5) depende de 013-T008, que solo espera la reversión hecha por otra persona.

- [x] **T015** Despliegue de la spec en Azure (staging)
  - Archivos: `memoria.md` (solo anexar)
  - Agente: `devops-mcp` · Depende de: T014, T016, T017 (y todas las anteriores `[x]`)
  - Aceptación: `make fmt lint test` en verde; al fusionar en `develop`, `deploy-staging.yml` despliega staging; `/healthz` y `/readyz` devuelven 200 por HTTPS sin `-k`; cada endpoint nuevo de `api/paths/ai.yaml` responde con una petición correcta y una inválida en el formato de error del Art. IV; una conversación real con el proveedor del asistente (Groq desde T016) en staging devuelve necesidades, coincidencias y pasos; registros sin secretos ni datos personales; resultado anotado en `memoria.md`. **La spec siguiente no empieza hasta que esta tarea pase.**
  - Refs: Art. XIII.5
  - **Hecho (2026-11-17, BE-B):** PR #88 (`spec/SPEC-14` → `develop`); CI, Smoke y `deploy-staging.yml` en verde para `b288cf5`. `/healthz` y `/readyz` 200 por HTTPS sin `-k` (Redis `ok`); `/meta` con `aiSuggestions` y `projectAssistant` en `true`. Asistente: 401 sin sesión, 400 con mensaje inválido, conversación real 200 (8 necesidades y 8 pasos, ~3 s), segundo turno 200, borrar 204 y luego 404; coincidencias verificadas («Cable HDMI» → PlayStation5, tarjeta pública con `approxLocation`). Sugerencias: 400 sin imagen, 404 con foto ajena, 200 con foto real (~5,6 s, JSON válido). Registros (`scripts/check-logs.sh staging 2h`): 71 líneas, 18 peticiones a `/api/v1/ai/`, 0 apariciones de contraseñas, tokens, claves, correos de prueba, textos de los mensajes o la foto. Cuentas de prueba dadas de baja. Observaciones en `memoria.md`.

- [x] **T016** Adaptador compatible con OpenAI (Groq en staging, Ollama en local)
  - Archivos: `internal/ai/adapter/openai/{client.go, client_test.go}`, `internal/ai/adapter/imagefetch/{fetch.go, fetch_test.go}` (y `gemini/client.go` pasa a usarlo), `internal/config/ai.go` (+ test), `cmd/api/main.go` (elegir adaptador), `scripts/set-ai-key.sh`, `.env.example`, `deploy/*.env.example`, `docs/produccion.md` §11
  - Agente: `dev-go` · Depende de: T003, T014
  - Aceptación: `POST {AI_BASE_URL}/chat/completions` con `Authorization: Bearer`; instrucciones fijas como mensaje `system` con el esquema JSON; turnos `user`/`assistant`; la miniatura como `image_url` en base64 (descarga acotada, solo https); JSON pedido con `response_format` cuando no hay imagen y extraído del texto siempre (sin cercos de código); un reintento ante 429, 5xx o red; `content_filter` → `ErrBlocked`; errores sin el contenido de la persona; `AI_PROVIDER` acepta `gemini`, `openai` y `ollama` (este solo en development); pruebas con `httptest`, sin red; `set-ai-key.sh` valida la clave con una generación mínima en Groq; sin dependencias nuevas
  - Refs: RF-03, RF-07, RF-17, RF-18 · P-01, P-05, P-06
  - Hecho (2026-11-17, BE-B): `internal/ai/adapter/openai` (90,2 %) y `internal/ai/adapter/imagefetch` (91,7 %; Gemini pasó a usarlo, sin duplicar código). `AI_PROVIDER` acepta `openai` (por defecto: Groq, modelo de texto e imagen), `gemini` (alternativa) y `ollama` (solo development); en `cmd/api/main.go`, `newAIProvider` elige el adaptador. `set-ai-key.sh` valida con Groq (lista de modelos y generación mínima con el modelo real) antes de tocar la VM y escribe proveedor, URL, modelo y clave; probado con archivos de prueba. Guía §11 y `.env.example` al día. Unitarias e integración de la IA en verde, lint en 0 y `go.mod` sin cambios.

- [x] **T017** Proveedor propio para las sugerencias por foto (OpenRouter, modelo gratuito con visión)
  - Archivos: `internal/config/ai.go` (+ test), `cmd/api/main.go`, `scripts/set-ai-key.sh`, `.env.example`, `deploy/*.env.example`, `docs/produccion.md` §11
  - Agente: `dev-go` · Depende de: T016
  - Aceptación: variables `SUGGEST_PROVIDER`, `SUGGEST_BASE_URL`, `SUGGEST_MODEL` y `SUGGEST_API_KEY` (clave como `Secret`) que, si faltan, toman las `AI_*`; la parte A crea su propio cliente y la B sigue igual; `set-ai-key.sh` con `PROVIDER=openrouter` valida la clave y hace una prueba **con una imagen** antes de tocar la VM, escribe las `SUGGEST_*` y enciende o apaga solo `AI_ENABLED`; sin dependencias nuevas; pruebas de configuración
  - Refs: RF-03, RF-07 · P-01
  - Hecho (2026-11-17, BE-B): `config.AI.Suggest` y `config.AI.Assistant()` (`config.Provider`) con herencia de `AI_*`; cada proveedor exige su clave solo si su parte está encendida, y los errores nombran la variable que de verdad falta. `main.go` crea un cliente por parte. `set-ai-key.sh` con `PROVIDER=openrouter` valida la clave (`/key`) y hace una prueba **con una imagen** antes de tocar la VM; escribe solo las `SUGGEST_*` y `--enable`/`--disable` tocan solo `AI_ENABLED` (probado: la clave de Groq queda intacta). Modelo elegido de la lista pública de OpenRouter (8 gratuitos con imagen): `google/gemma-4-31b-it:free` daba 429 (proveedor saturado), así que queda `nvidia/nemotron-3.5-super-vl:free` (prueba con imagen OK, sin pago; clave y encendido aplicados en staging el 2026-11-17), con respaldo `google/gemma-4-26b-a4b-it:free`. Configuración al 100 %.

## Trazabilidad
| Requisito | Tareas |
|---|---|
| RF-01, RF-02, RF-04, RF-05, RF-06, RF-07 | T001, T005, T006, T007 |
| RF-03, RF-17 | T002, T003, T008, T016, T017 |
| RF-08 | T001 |
| RF-09 a RF-16, RF-18 | T008 a T013 |
| RNF-01 | T007, T010, T013 |

| Total | 17 tareas |
|---|---|
