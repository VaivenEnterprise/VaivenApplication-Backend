# Tareas 004 — Imágenes con Cloudinary (backend)

- **Dueño:** BE-B · **Estado:** sin ejecutar · `[P]` = paralelizable
- **T001 se integra el primer día**; MO-B y las demás specs leen el contrato y `images.Service`.

- [x] **T001** Contrato OpenAPI de subida
  - Archivos: `api/paths/uploads.yaml`, `api/openapi.yaml` (agregar `$ref`)
  - Agente: `spec-writer` · Depende de: 003-T001
  - Aceptación: describe `POST /api/v1/uploads/signature`, esquemas `UploadSignature`, `ImageInput`, `Image`; ejemplos
  - Refs: Art. IV, XII

- [ ] **T002** Firmador de Cloudinary
  - Archivos: `internal/images/adapter/cloudinary/signer.go`, `internal/images/adapter/cloudinary/signer_test.go`
  - Agente: `dev-go` · Depende de: 002-T024
  - Aceptación: función `Sign(params, secret)` con parámetros ordenados, SHA-1 hex; prueba con vector conocido de la documentación de Cloudinary; no registra el secreto; **una subida de prueba real documentada en `memoria.md`** confirma el algoritmo
  - Refs: RF-03 · P-01

- [x] **T003** [P] Dominio de imágenes
  - Archivos: `internal/images/domain/image.go`, `internal/images/domain/image_test.go`
  - Agente: `dev-go` · Depende de: 002-T006
  - Aceptación: tipos `Image`, `Purpose`, `Delivery`; constructor del `publicId` `<base>/<purpose>/<userId>/<uuid>`; validación de prefijo propio; solo stdlib
  - Refs: RF-03, RF-06 · Art. III

- [x] **T004** [P] Cliente de administración de Cloudinary
  - Archivos: `internal/images/adapter/cloudinary/admin_client.go`, `internal/images/adapter/cloudinary/admin_client_test.go`
  - Agente: `dev-go` · Depende de: 002-T025
  - Aceptación: `GetResource(ctx, type, publicId)` devuelve `width,height,format,bytes`; 404 → error de dominio; 5xx → `service_unavailable`; timeout 5 s; pruebas con `httptest`
  - Refs: RF-05, RF-08 · P-02

- [x] **T005** [P] Constructor de URLs
  - Archivos: `internal/images/adapter/cloudinary/urlbuilder.go`, `internal/images/adapter/cloudinary/urlbuilder_test.go`, `internal/platform/clock/clock.go`
  - Agente: `dev-go` · Depende de: T002, T003
  - Aceptación: URL pública con transformaciones del RF-07; URL de descarga privada con `expires_at = ahora + 10 min` firmada; pruebas con reloj falso
  - Refs: RF-07 · P-03, P-06

- [x] **T006** Casos de uso: firma y resolución
  - Archivos: `internal/images/usecase/ports.go`, `signature.go`, `resolve.go`, `urls.go`, `usecase_test.go`
  - Agente: `dev-go` · Depende de: T003, T004, T005
  - Aceptación: casos CA-01…CA-04 con dobles; límites POL-13 (1–5 imágenes, formato, ≤ 5 MB); carpeta ajena → `validation_failed` con `details`; ≥ 80 %
  - Refs: RF-01, RF-02, RF-05 · POL-13

- [x] **T007** Fachada `images.Service`
  - Archivos: `internal/images/service.go`, `internal/images/service_test.go`
  - Agente: `dev-go` · Depende de: T006
  - Aceptación: expone `Signature`, `Resolve`, `URLs`; es lo único que importan otras specs; documentación del paquete con ejemplo de uso
  - Refs: P-04 · Art. XII

- [ ] **T008** Handler, límite de tasa y registro del módulo
  - Archivos: `internal/images/adapter/http/{handler.go, dto.go, routes.go, handler_test.go}`, `cmd/api/modules.go` (una línea)
  - Agente: `dev-go` · Depende de: T001, T007, 003-T010
  - Aceptación: 401 sin sesión; 429 tras 30 firmas/hora; respuesta cumple el contrato; módulo registrado con una línea
  - Refs: RF-01, RF-04 · CA-05

- [ ] **T009** Prueba de integración con Cloudinary simulado y guía de prueba real
  - Archivos: `internal/images/adapter/http/images_integration_test.go`, `docs/servicios.md` (sección "Probar subida real")
  - Agente: `qa-go` · Depende de: T008
  - Aceptación: flujo firma→`Resolve` con servidor simulado pasa; la guía explica cómo subir una imagen real con `curl`/`.http`
  - Refs: CA-01…CA-05

- [ ] **T010** Memoria y revisión
  - Archivos: `memoria.md` (solo anexar)
  - Agente: `revisor-constitucion` · Depende de: T009
  - Aceptación: veredicto **CONFORME**; sin secretos; hito H1 (backend) marcado
  - Refs: DoD

- [ ] **T011** Despliegue de la spec en Azure (staging)
  - Archivos: `memoria.md` (solo anexar)
  - Agente: `devops-mcp` · Depende de: T010 (y todas las anteriores `[x]`)
  - Aceptación: `make fmt lint test` en verde; `scripts/deploy.sh staging` (construye, envía a la VM y reinicia la API); `/healthz` y `/readyz` devuelven 200 por HTTPS sin `-k`; cada endpoint nuevo de esta spec (`api/paths/<modulo>.yaml`) responde con una petición correcta y una inválida en el formato de error del Art. IV; registros sin secretos ni datos personales; resultado anotado en `memoria.md`. **La spec siguiente no empieza hasta que esta tarea pase.**
  - Refs: Art. XIII.5

| Total | 11 tareas |
|---|---|
