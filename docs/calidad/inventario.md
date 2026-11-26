# Inventario de specs del backend (spec 016, T001)

- **Fecha:** 2026-11-17 · **Base:** `develop` en `fa686a5` · **Autor:** BE-B (con Claude Code)
- **Método:** se leyó la línea de estado de cada `spec.md`, se contaron las tareas `[x]` y `[ ]` de cada `tasks.md`, se revisó la tarea de despliegue y el último veredicto de conformidad, y se cruzó con `memoria.md`.

## 1. Specs de backend (001–015 y 017)

| Spec | Dueño | Estado declarado | Tareas hechas / abiertas | Despliegue verificado | Conformidad |
|---|---|---|---|---|---|
| 001 Servicios MCP | BE-B | Implementada y desplegada; T010 pendiente de decisiones humanas | 9 / 1 | No aplica (no hay API) | Condicionada a dos decisiones humanas |
| 002 Arquitectura base | BE-B (de hecho) · **sin dueño en el encabezado** | Aprobada (2026-11-05) | 31 / 0 | Sí (T031) | CONFORME |
| 003 Cuenta y autenticación | BE-A | Desplegada; ajustes del plan §9 esperan a BE-A | 14 / 0 | Sí (T014) | CONFORME |
| 004 Imágenes | BE-B | Aprobada e implementada | 11 / 0 | Sí (T011) | CONFORME |
| 005 Objetos | BE-A | Implementada; **pendiente de visto bueno de BE-A** | 10 / 0 | Sí (T010) | CONFORME |
| 006 Búsqueda | BE-B | Implementada (desviación del Art. I.1 registrada) | 10 / 0 | Sí (T010) | CONFORME |
| 007 Solicitudes y reservas | BE-A | Implementada; **pendiente de visto bueno de BE-A** | 13 / 0 | Sí (T013) | CONFORME |
| 008 Chat | BE-B | Implementada y verificada en staging | 10 / 0 | Sí (T010) | CONFORME |
| 009 Entrega, devolución y pagos | BE-A | **«Borrador»** (desactualizado) | 15 / 0 | Sí (T015) | CONFORME CON OBSERVACIONES |
| 010 Calificaciones e incidencias | BE-B | **«Borrador»** (desactualizado) | 12 / 0 | Sí (T012) | CONFORME |
| 011 Notificaciones y moderación | BE-B | **«Borrador»** (desactualizado) | 14 / 0 | Sí (T014) | CONFORME |
| 012 Integración y entrega | Todo el equipo (BE-A integra, BE-B despliega) | Aprobada (2026-11-09) | **4 / 8** | **No** (T012 abierta) | Pendiente (T011 abierta) |
| 013 Producción | BE-B (BE-A revisa) | Aprobada; producción desplegada y verificada | **8 / 2** | Producción desplegada (`deploy-production.sh`) | CONFORME (parcial; falta T009) |
| 014 IA generativa | BE-A (BE-B revisa) | Implementada, desplegada y verificada | 17 / 0 | Sí (T015) | CONFORME |
| 015 Recomendaciones con ML | BE-B (BE-A revisa) | Implementada, desplegada y verificada | 11 / 0 | Sí (T011) | CONFORME |
| 017 DevSecOps | BE-B · **sin encabezado de spec** | **Sin línea de estado** | **6 / 4** (formato sin IDs) | No aplica (pipeline) | **Sin veredicto** |

**Total backend:** 195 tareas hechas y 15 abiertas en las 16 specs.

## 2. Tareas abiertas, con dueño

| Spec | Tarea | Dueño | Motivo |
|---|---|---|---|
| 001 | T010 Revisión de conformidad | BE-B | Esperaba decisiones humanas (desviaciones y la enmienda 1.6.0) |
| 012 | T001 y T002 (E2E), T003 (cabeceras y límite de cuerpo), T008 (revisión de seguridad), T011 (revisión final) | **BE-A** | Sin ejecutar |
| 012 | T009 y T012 (despliegue), T010 (documentación) | BE-B | Sin ejecutar; T012 depende de las anteriores |
| 013 | T008 Guía de operación y reversión | BE-B | Avance registrado; la reversión ya la ejecutó otra persona en staging (`memoria.md`), pero la casilla no se marcó |
| 013 | T009 Lista de producción y conformidad | BE-B | Depende de 012-T011 (BE-A) |
| 017 | Revisión humana y fusión; habilitar al segundo revisor; activar las reglas de ramas; T009 (formato de ramas con SCRUM) | Administración del repositorio | Tareas de administración en GitHub |

## 3. Specs fuera de las 16 de backend

| Spec | Estado |
|---|---|
| 016 Auditoría de calidad | Aprobada por BE-B (2026-11-17); en curso |
| 018 Hardening de seguridad | **Propuesta** publicada en `develop` (PR #103); aprobación formal pendiente |

## 4. Diferencias con la línea base del 2026-11-17 (registrada en la 016)

| Línea base | Ahora | Explicación |
|---|---|---|
| 012 con 8 abiertas | Igual | Nadie ha trabajado la 012 desde entonces |
| 017 con 4 abiertas | Igual | Son tareas de administración del repositorio |
| 013 con 2 abiertas | Igual, pero T008 tiene evidencia | Falta marcarla (deuda DT-d) |
| 001 con 1 abierta | Igual | — |
| 009, 010 y 011 en «Borrador» | Igual | Estado desactualizado (deuda DT-d) |
| 014 en 16 de 17 | **17 / 17** | La T015 se cerró después (PR #93) |
| 015 sin implementar | **11 / 11**, desplegada | La spec se completó el mismo día |
| — | **018** nueva (propuesta) | Se agregó al final del día |

## 5. Hallazgos que pasan al registro de deuda (T004)

1. Estados desactualizados: 009, 010 y 011 dicen «Borrador»; 005 y 007 esperan el visto bueno de BE-A; 003 tiene ajustes sin visto bueno.
2. 002 no tiene dueño en el encabezado y 017 no tiene encabezado ni línea de estado (Art. I y XII.1).
3. 013-T008 tiene evidencia y no está marcada.
4. 012 está casi entera abierta, incluida la T003 (cabeceras), que es requisito de seguridad.
5. 017 usa casillas sin IDs de tarea, a diferencia del formato de `sdd-workflow`.
