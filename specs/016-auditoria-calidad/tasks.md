# Tareas 016 — Auditoría de calidad y consolidación (backend)

- **Dueño:** BE-B (coordina) con BE-A · **Estado:** sin ejecutar · `[P]` = paralelizable
- Empieza cuando la 015 termine su despliegue verificado (Art. XIII.5) y esta spec esté aprobada (Art. I).
- Cada tarea con código termina con `make fmt lint test` (y la integración del paquete) en verde.

### Fase 1 — Evaluar (sin cambiar código)

- [ ] **T001** Inventario de specs
  - Archivos: `docs/calidad/inventario.md`
  - Agente: `spec-writer` · Dueño: BE-B · Depende de: 015-T011
  - Aceptación: las 16 specs de backend con estado declarado, tareas hechas y abiertas (con dueño), despliegue verificado y veredicto de conformidad; diferencias con la línea base del 2026-11-17 explicadas
  - Refs: RF-01

- [ ] **T002** [P] Matriz de conformidad
  - Archivos: `docs/calidad/conformidad.md`
  - Agente: `revisor-constitucion` · Dueño: BE-B · Depende de: T001
  - Aceptación: spec × artículo con ✅/⚠️/❌ y evidencia en cada celda; las comprobaciones automáticas (arquitectura, lint, cobertura, gitleaks, govulncheck) citan su ejecución; cada ⚠️ y ❌ pasa al registro de deuda
  - Refs: RF-02, CA-01

- [ ] **T003** [P] Trazabilidad
  - Archivos: `scripts/trace.py`, `scripts/trace_test.py`, `docs/calidad/trazabilidad.md`, `.github/workflows/ci.yml` (ejecutar `trace_test.py`)
  - Agente: `qa-go` · Dueño: BE-B · Depende de: T001
  - Aceptación: cada RF, RNF y CA de las specs 003–015 con su prueba o «sin prueba»; el script no modifica nada y sus pruebas pasan
  - Refs: RF-03 · P-08

- [ ] **T004** Registro de deuda técnica
  - Archivos: `docs/calidad/deuda.md`
  - Agente: `spec-writer` · Dueño: BE-B · Depende de: T002, T003
  - Aceptación: deuda conocida (a–g de RF-04) más lo que salga de T002 y T003, cada una con ID, gravedad, dueño y tarea que la corrige; revisado por BE-A
  - Refs: RF-04

### Fase 2 — Corregir

- [ ] **T005** [P] `validator/v10` en los módulos de BE-A
  - Archivos: `internal/{bookings, handover, payments}/adapter/http/*` y pruebas
  - Agente: `dev-go` · Dueño: BE-A · Depende de: T004
  - Aceptación: CA-03 en cada ruta con cuerpo o parámetros; sin cambios de contrato; mismas reglas que el dominio
  - Refs: RF-06 · P-09

- [ ] **T006** [P] `validator/v10` en los módulos de BE-B
  - Archivos: `internal/{search, chat, ratings, incidents, sanctions, notifications, reports}/adapter/http/*` y pruebas
  - Agente: `dev-go` · Dueño: BE-B · Depende de: T004
  - Aceptación: CA-03 en cada ruta con cuerpo o parámetros; sin cambios de contrato; mismas reglas que el dominio
  - Refs: RF-06 · P-09

- [ ] **T007** Resto de la deuda P0 y P1
  - Archivos: los que indique cada `DT-NN` (por ejemplo `internal/assistant/adapter/search/finder.go` para DT-c; rutas de 009 para DT-b), `specs/*/spec.md` (línea de estado), `docs/politicas.md`
  - Agente: `dev-go` · Dueño: el de cada spec · Depende de: T004
  - Aceptación: CA-02; cada corrección con su prueba; POL-06 y POL-07 confirmadas por el equipo; foto huérfana borrada
  - Refs: RF-05, RF-07

### Fase 3 — Cerrar brechas de pruebas

- [ ] **T008** Pruebas de contrato
  - Archivos: `internal/adapter/http/contract/{contract.go, contract_test.go}`, pruebas de integración de los handlers y prueba de punta a punta (012) para engancharlo, `go.mod` si se aprueba P-03, `plan.md` y `memoria.md` (dependencia)
  - Agente: `qa-go` · Dueño: BE-B · Depende de: T004 (y la decisión de P-03)
  - Aceptación: CA-04; todas las rutas de `api/paths/*.yaml` ejercidas al menos una vez; la dependencia, si se usa, no aparece en el binario de la API (`go version -m bin/api`)
  - Refs: RF-08 · P-03, P-04

- [ ] **T009** [P] Mutación en todos los módulos
  - Archivos: `scripts/mutation.sh`, `docs/calidad/pruebas.md` (resultado)
  - Agente: `qa-go` · Dueño: BE-B · Depende de: T004
  - Aceptación: CA-05; corrida real desde Actions anotada con su enlace
  - Refs: RF-09 · P-05

- [ ] **T010** [P] Seguridad dinámica (ZAP baseline)
  - Archivos: `.github/workflows/zap.yml`, `docs/calidad/pruebas.md`
  - Agente: `qa-go` · Dueño: BE-B · Depende de: T004
  - Aceptación: CA-06; solo `workflow_dispatch` y solo staging; acciones fijadas por SHA; alertas Medias o Altas al registro de deuda
  - Refs: RF-10 · P-06

- [ ] **T011** [P] Escaneo de la imagen y SBOM
  - Archivos: `.github/workflows/ci.yml`, `docs/calidad/pruebas.md`
  - Agente: `qa-go` · Dueño: BE-B · Depende de: T004
  - Aceptación: CA-07; SBOM CycloneDX como artefacto de cada ejecución; falla con críticas o altas que tengan corrección
  - Refs: RF-11 · P-07

### Fase 4 — Demostrar y cerrar

- [ ] **T012** Staging de demostración
  - Archivos: `docs/calidad/demo.md`, `memoria.md`
  - Agente: `devops-mcp` · Dueño: BE-B · Depende de: T005–T007
  - Aceptación: CA-08; `seed` y `seed-interactions` cargados en staging (nunca en producción); guion recorrido completo con una cuenta de prueba que se borra al terminar
  - Refs: RF-13

- [ ] **T013** Informes y revisión de conformidad
  - Archivos: `docs/calidad/{pruebas.md, informe.md}`, `docs/calidad/conformidad.md` (actualizada), `memoria.md`
  - Agente: `spec-writer` y `revisor-constitucion` · Dueño: BE-B con firma de BE-A · Depende de: T005–T012
  - Aceptación: CA-09; siete categorías con resultado real, fecha y enlace; matriz actualizada tras las correcciones; veredicto **CONFORME**; firmado por BE-A y BE-B
  - Refs: RF-12, RF-14, DoD

- [ ] **T014** Despliegue de la spec en Azure (staging)
  - Archivos: `memoria.md` (solo anexar)
  - Agente: `devops-mcp` · Dueño: BE-B · Depende de: T013 (y todas las anteriores `[x]`)
  - Aceptación: `make fmt lint test` en verde; PR `spec/SPEC-16` → `develop` fusionado y `deploy-staging.yml` en verde (Art. XIII.5, enmienda 2.0.0); `/healthz` y `/readyz` 200 por HTTPS sin `-k`; una petición inválida por módulo corregido responde `validation_failed`; `scripts/check-logs.sh staging` con todas las cuentas en 0; resultado anotado en `memoria.md`
  - Refs: Art. XIII.5

| Total | 14 tareas |
|---|---|
