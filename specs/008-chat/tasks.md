# Tareas 008 — Chat privado (backend)

- **Dueño:** BE-B · **Estado:** sin ejecutar · `[P]` = paralelizable
- Puede empezar con el contrato y el dominio antes de que 007 esté integrada.

- [ ] **T001** Contrato OpenAPI del chat
  - Archivos: `api/paths/chat.yaml`, `api/openapi.yaml` (agregar `$ref`)
  - Agente: `spec-writer` · Depende de: 007-T001
  - Aceptación: las 5 rutas del plan §2, esquemas `Conversation` y `Message`, ejemplos de `before` y `after`
  - Refs: Art. IV, XII

- [ ] **T002** [P] Dominio del chat
  - Archivos: `internal/chat/domain/{conversation.go, message.go, errors.go}`, `internal/chat/domain/domain_test.go`
  - Agente: `dev-go` · Depende de: 002-T006
  - Aceptación: `NewMessage` valida cuerpo (1–1000 runas, no solo espacios); `Conversation.CanWrite(bookingStatus, counterpartActive)`; solo stdlib; ≥ 80 %
  - Refs: RF-04, RF-06, RF-07

- [ ] **T003** Repositorios de conversaciones y mensajes
  - Archivos: `internal/chat/adapter/mongo/{docs.go, conversation_repo.go, message_repo.go, booking_reader.go, repo_integration_test.go}`
  - Agente: `modelador-mongo` · Depende de: T002, 002-T014
  - Aceptación: colecciones e índices del plan §3 (incluye el único parcial); `upsert` por `bookingId`; `before`/`after` sin repetidos; `$inc`/`$set` de no leídos; `booking_reader.go` lee **solo** `status`, `ownerId`, `requesterId` y datos de objeto de `bookings` (solo lectura); prueba de integración
  - Refs: plan §3 · Art. II

- [ ] **T004** Casos de uso: abrir y listar
  - Archivos: `internal/chat/usecase/{ports.go, open.go, list.go, unread.go, usecase_read_test.go}`
  - Agente: `dev-go` · Depende de: T002, T003
  - Aceptación: `Open` idempotente (CA-01); lista con contraparte, objeto y `canWrite` (usa `BookingReader` y repositorio de usuarios con dobles); conteo de no leídos
  - Refs: RF-01, RF-02, RF-09

- [ ] **T005** Casos de uso: mensajes, envío y lectura
  - Archivos: `internal/chat/usecase/{messages.go, send.go, read.go, usecase_write_test.go}`
  - Agente: `dev-go` · Depende de: T004, 003-T002
  - Aceptación: ajenos → no encontrado (CA-02); `clientId` idempotente (CA-03); estado cerrado → conflicto (CA-04); `Checker` de moderación; notificación solo en el primer no leído (RF-12); contadores (CA-06); ≥ 80 %
  - Refs: RF-03…RF-08, RF-10, RF-12

- [ ] **T006** Handlers, límite de tasa y rutas
  - Archivos: `internal/chat/adapter/http/{handler.go, dto.go, routes.go, handler_test.go}`, `cmd/api/modules.go` (una línea)
  - Agente: `dev-go` · Depende de: T001, T005, 003-T009, 003-T010
  - Aceptación: 5 rutas cumplen el contrato; 429 tras 30 mensajes/min; 400 para cuerpo de 1001 caracteres (CA-05); la ruta estática `unread-count` antes de `{id}`
  - Refs: RF-11

- [ ] **T007** Reemplazar `chat.Opener` vacío
  - Archivos: `internal/chat/opener.go`, `cmd/api/modules.go` (cambio de una línea, acordado con BE-A)
  - Agente: `dev-go` · Depende de: T004, 007-T009
  - Aceptación: al crear una solicitud existe su conversación; si falla, la solicitud no se crea (probado con el flujo de 007)
  - Refs: RF-01 · P-07

- [ ] **T008** Prueba de integración y consulta periódica
  - Archivos: `internal/chat/adapter/http/chat_integration_test.go`
  - Agente: `qa-go` · Depende de: T006, T007
  - Aceptación: flujo de dos personas (enviar, `after`, `read`), CA-01…CA-06, y `after` responde < 100 ms con 5,000 mensajes sembrados
  - Refs: RNF-01 · CA-01…CA-06

- [ ] **T009** Memoria y revisión
  - Archivos: `memoria.md` (solo anexar)
  - Agente: `revisor-constitucion` · Depende de: T008
  - Aceptación: veredicto **CONFORME** (revisar que no se expone lectura de la otra parte); hito H3 (backend) marcado
  - Refs: DoD

- [ ] **T010** Despliegue de la spec en Azure (staging)
  - Archivos: `memoria.md` (solo anexar)
  - Agente: `devops-mcp` · Depende de: T009 (y todas las anteriores `[x]`)
  - Aceptación: `make fmt lint test` en verde; `scripts/deploy.sh staging` (construye, envía a la VM y reinicia la API); `/healthz` y `/readyz` devuelven 200 por HTTPS sin `-k`; cada endpoint nuevo de esta spec (`api/paths/<modulo>.yaml`) responde con una petición correcta y una inválida en el formato de error del Art. IV; registros sin secretos ni datos personales; resultado anotado en `memoria.md`. **La spec siguiente no empieza hasta que esta tarea pase.**
  - Refs: Art. XIII.5

| Total | 10 tareas |
|---|---|
