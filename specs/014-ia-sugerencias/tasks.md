# Tareas 014 — IA generativa: sugerencias y asistente de proyectos (backend)

- **Dueño:** BE-A · **Estado:** sin ejecutar · `[P]` = paralelizable
- Se ejecuta **después** del congelamiento de la 1.0 (22 nov); no toca el AAB en revisión.
- Parte B (asistente, T008 a T013) agregada el 2026-11-16; pendiente de aprobación de BE-A. Si el calendario no alcanza, la parte B se entrega apagada (`ASSISTANT_ENABLED=false`) y se demuestra en staging.

## Comunes y parte A — Sugerencias al publicar

- [ ] **T001** Contrato OpenAPI (A y B)
  - Archivos: `api/paths/ai.yaml`, `api/openapi.yaml` (referencias), `/meta` con `features.aiSuggestions` y `features.projectAssistant`
  - Agente: `spec-writer` · Depende de: 005-T001, 006-T001
  - Aceptación: rutas `listing-suggestions`, `assistant/messages` y `assistant/conversations/{id}`; cuerpos; tarjeta de coincidencia reutilizando el esquema de la búsqueda; errores (`validation_failed`, `forbidden`, `not_found`, `rate_limited`, `service_unavailable`); MO-A lo consume el mismo día
  - Refs: RF-01, RF-06, RF-08, RF-09, RF-13, RF-16

- [ ] **T002** [P] Puerto común, validación de salida y configuración
  - Archivos: `internal/ai/{port.go, schema.go, schema_test.go}`, `internal/config/…` (variables `AI_*` y `ASSISTANT_*`), `.env.example`
  - Agente: `dev-go` · Depende de: T001
  - Aceptación: interfaz `Provider` (`DescribeImage`, `GenerateJSON`); validación de esquemas de A y B (categoría POL-14, longitudes, conteos, campos extra fuera); valores por defecto seguros (`AI_ENABLED=false`, `ASSISTANT_ENABLED=false`); sin secretos
  - Refs: RF-03, RF-04, RF-10, RF-17 · P-01, P-03

- [ ] **T003** [P] Adaptadores Gemini y Ollama
  - Archivos: `internal/ai/adapter/gemini/{client.go, client_test.go}`, `internal/ai/adapter/ollama/{client.go, client_test.go}`
  - Agente: `dev-go` · Depende de: T002
  - Aceptación: `net/http` con tiempo de espera y un reintento; JSON con esquema; maneja errores y respuestas inválidas; pruebas con `httptest`; la clave nunca se registra; Ollama solo se acepta con `APP_ENV=development`
  - Refs: RF-03, RF-07 · P-05, P-06

- [ ] **T004** [P] Contador de uso común en MongoDB
  - Archivos: `internal/ai/adapter/mongo/usage.go`, `internal/ai/adapter/mongo/usage_test.go`, índice en `Indexes()` (spec 012)
  - Agente: `modelador-mongo` · Depende de: T002
  - Aceptación: incremento atómico por persona, función y día; índice único `{userId, feature, day}` y TTL; prueba de concurrencia
  - Refs: RF-05, RF-15 · P-04

- [ ] **T005** Caso de uso de sugerencias (A)
  - Archivos: `internal/suggest/{domain.go, usecase.go, usecase_test.go}`
  - Agente: `dev-go` · Depende de: T003, T004, 004-T007, 011 (filtro)
  - Aceptación: verifica que la imagen es `item_photo` de esa persona (CA-02); construye la URL; aplica el límite (CA-05) y el filtro (CA-03); apagado o caída → `service_unavailable` (CA-04)
  - Refs: RF-02, RF-04, RF-05, RF-06

- [ ] **T006** Handler, rutas y registro (A)
  - Archivos: `internal/suggest/adapter/http/{handler.go, dto.go, routes.go}`, `cmd/api/modules.go` (una línea)
  - Agente: `dev-go` · Depende de: T001, T005
  - Aceptación: cumple el contrato; autenticada; límite de tasa por ruta; registros sin imagen ni texto sugerido (RF-07)
  - Refs: RF-01, RF-07

- [ ] **T007** Prueba de integración (A)
  - Archivos: `internal/suggest/integration_test.go`
  - Agente: `qa-go` · Depende de: T006
  - Aceptación: CA-01 a CA-06 con proveedor falso; sin red; limpia sus datos
  - Refs: RNF-01

## Parte B — Asistente de proyectos

- [ ] **T008** [P] Instrucciones fijas y reglas del asistente
  - Archivos: `internal/assistant/{domain.go, prompt.go}`, `internal/assistant/prompt_test.go`
  - Agente: `dev-go` · Depende de: T002
  - Aceptación: instrucciones del sistema fijas con alcance y negativas (RF-14); el mensaje y los turnos van como bloque de datos delimitado (P-08); la lista de categorías de POL-14 sale del código, no se escribe a mano; pruebas de que ninguna entrada cambia las instrucciones
  - Refs: RF-10, RF-14, RF-17 · P-08

- [ ] **T009** [P] Conversaciones en MongoDB
  - Archivos: `internal/assistant/adapter/mongo/{conversations.go, conversations_test.go}`, índices en `Indexes()`
  - Agente: `modelador-mongo` · Depende de: T002
  - Aceptación: `ai_conversations` con dueño, máximo 6 turnos (`$slice`), TTL de 24 h sobre `updatedAt` y borrado por dueño; otra persona → no encontrada; pruebas con MongoDB real
  - Refs: RF-13 · P-09

- [ ] **T010** [P] Buscador de coincidencias sobre la spec 006
  - Archivos: `internal/assistant/adapter/search/{finder.go, finder_test.go}`
  - Agente: `dev-go` · Depende de: T002, 006 (caso de uso de búsqueda)
  - Aceptación: por necesidad llama a la búsqueda con `q`, `category` y ubicación o municipio; hasta 5 resultados con la tarjeta pública (`approxLocation`, `distanceKm`); máximo 4 búsquedas simultáneas con tiempo de espera; hereda los filtros de visibilidad de la 006 (sin propios, pausados ni cuentas suspendidas)
  - Refs: RF-11, RNF-01 · P-07, P-10

- [ ] **T011** Caso de uso del asistente
  - Archivos: `internal/assistant/{usecase.go, usecase_test.go}`
  - Agente: `dev-go` · Depende de: T003, T004, T008, T009, T010, 011 (filtro)
  - Aceptación: arma el contexto (máximo 6 turnos); llama al proveedor **sin ubicación** (CA-11); valida y recorta la salida (CA-12); filtro de contenido en entrada y salida; caché en Redis del primer mensaje normalizado sin cachear coincidencias (CA-14); límites diarios y por minuto (CA-05); negativas (CA-10); necesidades sin coincidencias (CA-08); apagado o caída → `service_unavailable`
  - Refs: RF-09 a RF-18

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
