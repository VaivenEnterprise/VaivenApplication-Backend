# Tareas 020 — Bitácora de auditoría inmutable y pruebas de seguridad avanzadas (backend)

- **Dueño propuesto:** BE-B (con BE-A para las rutas de cuenta y de pagos de la Fase 2) · **Estado:** aprobada el 2026-11-26 (Art. I) · `[P]` = paralelizable
- La **Fase 1** se implementa ya. La **Fase 2** está **BLOQUEADA** hasta que la spec 021 se fusione en `develop` (toca los mismos repositorios, esquemas, `user_repo.go` y `cmd/admin/main.go`). La **Fase 3** (C-10) depende de decisiones y acciones de personas.
- Cada tarea con código termina con `gofmt`, `go vet` y las pruebas en verde (y `golangci-lint` al cerrar la fase).

## Fase 1 — pruebas de seguridad

- [x] **T001** [P] Fuzzing nativo de Go: cursores, DTO, JWT, QR e IP del cliente
  - Archivos: `fuzz_test.go` en `internal/{bookings,items,notifications,chat,incidents,auth}/adapter/http`, `internal/platform/security`, `internal/handover/domain` e `internal/adapter/http/middleware`; `scripts/fuzz.sh`; corpus de regresión en `testdata/fuzz/`
  - Agente: `qa-go` · Dueño: BE-B · Depende de: —
  - Aceptación: RF-10, RF-11, RF-13, CA-07 (corpus) y CA-08 (regresión); 12 objetivos con semillas; cada uno pasa `go test -run=^$ -fuzz=<Objetivo> -fuzztime=20s`; los casos mínimos de un fallo quedan en `testdata/fuzz` y corren en `go test`
  - **Hecho (2026-11-26):** 12 objetivos; 20 s por objetivo en local. El fuzzing encontró un fallo real en `middleware.ClientIP` (dirección IPv6 con zona, p. ej. `::%[`): corregido quitando la zona y dejado como regresión (`testdata/fuzz/FuzzClientIP` y `TestClientIPIgnoresIPv6Zone`). Ver `docs/seguridad/pentest-interno.md` (H1).
  - Refs: RF-10 a RF-13

- [x] **T002** Fuzzing nocturno en CI
  - Archivos: `.github/workflows/fuzz-nightly.yml`
  - Agente: `dev-go` · Dueño: BE-B · Depende de: T001
  - Aceptación: RF-12; cron diario y `workflow_dispatch`; 12 objetivos × 2 min = 24 min (≥ 10 min); acciones fijadas por SHA; sin servicios de pago; un crash deja el trabajo en rojo y sube el caso como artefacto; antes de fuzzear corre el corpus como pruebas normales
  - **Hecho (2026-11-26):** el flujo cumple `policy.mjs`. Falta su primera ejecución en Actions (se valida al fusionar).
  - Refs: RF-12

- [x] **T003** Tabla de clasificación y pruebas de 401/403 por ruta, generadas desde el contrato
  - Archivos: `cmd/api/routes_security_test.go`
  - Agente: `qa-go` · Dueño: BE-B · Depende de: —
  - Aceptación: CA-09, CA-10; las 69 operaciones clasificadas (pública, sesión, dueño, administración) y coherentes con el contrato; 401 con el sobre estándar sin sesión y con 8 variantes de sesión inválida en las 62 operaciones protegidas; 403 de una sesión de usuario en las 6 de administración; `reject` y `cancel` con prueba propia de 401; el router es el real con dobles vacíos de casos de uso
  - **Hecho (2026-11-26):** 566 casos verdes; cada respuesta se valida además contra el contrato. Corre en cada PR sin Testcontainers.
  - Refs: RF-14, RF-15, RF-16

- [x] **T004** [P] Pruebas de IDOR en bookings, items, chat y notifications
  - Archivos: `idor_integration_test.go` en `internal/{bookings,items,chat,notifications}/adapter/http` (integración, MongoDB real)
  - Agente: `qa-go` · Dueño: BE-B · Depende de: —
  - Aceptación: RF-15, CA-10; una persona ajena (o con otro rol) no lee ni modifica el recurso de otra y este queda intacto; respuesta 403 o 404 según el contrato
  - **Hecho (2026-11-26):** 4 pruebas de integración. Las operaciones de dueño de entrega, devolución, pagos, calificaciones, incidencias y asistente quedan como pendientes manuales en `docs/seguridad/pentest-interno.md`.
  - Refs: RF-15

- [x] **T005** [P] Pruebas de las 3 rutas de Swagger UI por entorno
  - Archivos: `cmd/api/docs_routes_test.go`
  - Agente: `qa-go` · Dueño: BE-B · Depende de: —
  - Aceptación: RF-17, CA-11; `/docs`, `/docs/init.js` y `/docs/openapi.yaml` dan 200 en development y staging y 404 estándar en production; production se niega a arrancar con `DOCS_ENABLED=true`
  - **Hecho (2026-11-26):** usa `config.Load`, `docsHandler` y el router real; no requiere contenedores.
  - Refs: RF-17

- [x] **T006** ZAP activo autenticado con persona sintética
  - Archivos: `.github/workflows/zap-isolated.yml`
  - Agente: `dev-go` · Dueño: BE-B · Depende de: —
  - Aceptación: RF-18, CA-12; registro de una persona sintética en la API efímera, token enmascarado y entregado a ZAP como `Authorization`; la puerta sigue en `High` con excepciones con vencimiento; un paso comprueba que el escaneo alcanzó rutas protegidas; sin secretos reales ni salida a internet
  - **Hecho (2026-11-26) — sin validar en local:** necesita Docker con red interna y la imagen de ZAP; **hay que validarlo en la primera ejecución de Actions**. Se excluyen del escaneo `DELETE /me`, `POST /auth/logout` y `DELETE /me/push-tokens` (destruyen la sesión sintética); el rol admin queda como mejora.
  - Refs: RF-18

- [x] **T007** Checklist de pentest interno (OWASP WSTG / ASVS L2)
  - Archivos: `docs/seguridad/pentest-interno.md`
  - Agente: `spec-writer` · Dueño: BE-B · Depende de: T001, T003, T004, T005, T006
  - Aceptación: categorías WSTG y capítulos ASVS L2 mapeados a las 69 operaciones, con lo que cubre cada prueba automática, lo que queda manual, el registro de evidencias y los hallazgos; sin datos personales ni secretos
  - **Hecho (2026-11-26):** incluye el hallazgo H1 del fuzzing.
  - Refs: RF-14 a RF-18, RNF-06

## Fase 2 — bitácora inmutable (BLOQUEADA hasta fusionar la spec 021)

> Condición de desbloqueo: la spec 021 fusionada en `develop`, para partir de sus repositorios y esquemas. No empezar antes.

- [ ] **T008** Dominio y puerto de auditoría con cadena de hash
  - Archivos: `internal/audit/` (`entry.go`, `chain.go`, `recorder.go` y pruebas)
  - Agente: `dev-go` · Dueño: BE-B · Depende de: spec 021 fusionada
  - Aceptación: RF-02, RF-04, RNF-03, RNF-05; catálogo cerrado de acciones (RF-03); campos sin datos personales ni secretos; hash SHA-256 y génesis conocidos; cobertura ≥ 80 %
  - Refs: RF-02 a RF-04

- [ ] **T009** Repositorio MongoDB de `audit_log` (solo insertar) y permisos
  - Archivos: `internal/audit/adapter/mongo/` (repositorio, índices únicos en `seq` y `prevHash`, esquema), `docs/servicios.md` (rol de solo insertar y leer)
  - Agente: `modelador-mongo` · Dueño: BE-B · Depende de: T008
  - Aceptación: RF-01, CA-05, CA-06; inserción con reintento por clave duplicada, íntegra con dos réplicas a la vez; el usuario de la API no puede actualizar ni borrar (prueba de integración)
  - Refs: RF-01, RF-02

- [ ] **T010** Instrumentar las acciones auditadas (fallo cerrado)
  - Archivos: casos de uso de administración, sanciones, incidencias, pagos y cuenta (con BE-A), sin cambiar el contrato
  - Agente: `dev-go` · Dueño: BE-B con BE-A · Depende de: T009
  - Aceptación: RF-03, RF-05, CA-01; cada acción del catálogo deja su registro; si no se puede escribir, la operación falla con 503 sin efectos; evento de seguridad al fallar; medición de latencia (≤ 10 ms orientativo)
  - Refs: RF-03, RF-05, RNF-04

- [ ] **T011** `cmd/admin audit-verify`
  - Archivos: `cmd/admin/` (subcomando y pruebas)
  - Agente: `dev-go` · Dueño: BE-B · Depende de: T009
  - Aceptación: RF-06, RF-08, CA-02 a CA-04; código 0 con cadena íntegra (total y hash de cabeza); código ≠ 0 con registro modificado, eliminado, insertado a mano o cadena cortada, indicando posición e identificador; `--export-head` y `--head` detectan un borrado de la cola
  - Refs: RF-06, RF-08

- [ ] **T012** Retención y verificación periódica
  - Archivos: `docs/seguridad/` (política de 12 meses), flujo de CI de verificación contra una base de pruebas
  - Agente: `spec-writer` · Dueño: BE-B · Depende de: T011
  - Aceptación: RF-07; retención de 12 meses documentada; la CI ejecuta la verificación y falla si hay manipulación
  - Refs: RF-07

## Fase 3 — cierre de C-10 (tareas humanas)

- [ ] **T013** Suplente de guardia con nombre y escalamiento
  - Archivos: `docs/seguridad/respuesta-incidentes.md`, variable `ALERT_ASSIGNEES`
  - Agente: persona (responsable) · Dueño: responsable · Depende de: **decisión humana: nombre y cuenta de GitHub del suplente** (`[NEEDS CLARIFICATION]`)
  - Aceptación: RF-20, CA-13; titular y suplente en `ALERT_ASSIGNEES`, recepción confirmada por el suplente y regla de escalamiento a los 15 min
  - Refs: RF-20

- [ ] **T014** Evidencias de entrega A3 a A6, simulacro y cadencia medida
  - Archivos: `docs/seguridad/respuesta-incidentes.md`, informe de C-10
  - Agente: persona (BE-B) · Dueño: BE-B · Depende de: T013
  - Aceptación: RF-19, RF-21, RF-22, CA-13 a CA-15; recepción confirmada de A3 a A6 (solo identificador, severidad, entorno y hora); acta del simulacro con fecha, escenario, tiempos y acciones, y la siguiente fecha trimestral; cadencia real de las últimas 20 ejecuciones (meta realista ≤ 6 h con el cron de GitHub; mejora futura documentada)
  - Refs: RF-19, RF-21, RF-22

## Cierre

- [ ] **T015** Despliegue
  - Archivos: ninguno de código nuevo; se verifica `deploy-staging` y la corrida de `fuzz-nightly.yml` y de `zap-isolated.yml` autenticado
  - Agente: `devops-mcp` · Dueño: BE-B · Depende de: Fase 1 fusionada (y, para la bitácora, las fases 2 y 3)
  - Aceptación: al fusionar en `develop`, `deploy-staging.yml` despliega staging; humo por HTTPS en verde; primera ejecución de Actions del fuzzing nocturno y del ZAP autenticado en verde (o con hallazgos registrados); CA-16 (sin diferencias en `openapi.yaml` ni `api/paths/`); `memoria.md` actualizada con la etiqueta `[BE-B]`
  - Refs: Art. XIII.5, RNF-01
