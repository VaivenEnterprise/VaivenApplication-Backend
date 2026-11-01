---
name: sdd-workflow
description: Flujo Spec Driven Development del backend de VAIVÉN. Úsala al crear o ejecutar una spec, un plan o una lista de tareas, o antes de empezar cualquier implementación.
---

# Skill: Spec Driven Development (backend)

## Principio
La **spec es la fuente de verdad**; el código es su consecuencia. Orden fijo:

`constitution.md → spec.md → plan.md → tasks.md → implementación → revisión`

## Estructura de una spec
Carpeta `specs/NNN-nombre-corto/` con tres archivos.

### spec.md — QUÉ y POR QUÉ (sin tecnología)
1. Resumen y objetivo.
2. Historias (`Como <rol>, quiero <acción>, para <beneficio>`) con prioridad P1/P2/P3.
3. Requisitos funcionales `RF-01…` y no funcionales `RNF-01…` (medibles).
4. Criterios de aceptación (dado / cuando / entonces).
5. Fuera de alcance (explícito).
6. Preguntas abiertas: `[NEEDS CLARIFICATION: …]`.

### plan.md — CÓMO
1. Enfoque y arquitectura (capas, paquetes).
2. Estructura de archivos a crear.
3. Contratos (endpoints, errores) y datos (colecciones, índices) si aplica.
4. Dependencias nuevas con justificación.
5. Estrategia de pruebas.
6. **Verificación de constitución**: tabla artículo → cumple / cómo.
7. Riesgos.

### tasks.md — TAREAS EJECUTABLES
Cada tarea:
```
- [ ] **T001** [P] Título imperativo corto
  - Archivos: `ruta/a.go`, `ruta/a_test.go`
  - Agente: `dev-go`
  - Depende de: T000
  - Aceptación: criterio verificable 1; criterio 2
  - Refs: plan §x · Art. N
```
Reglas: 1–3 archivos por tarea; criterios verificables con un comando; `[P]` solo si no comparte archivos con otra tarea paralela.

## Ejecución
1. Elige la primera tarea no marcada cuyas dependencias estén hechas.
2. Léela entera; no avances si algo es ambiguo (pregunta).
3. Implementa, prueba, ejecuta `make fmt lint test`.
4. Marca `[x]` y actualiza `memoria.md` si procede.
5. Repite con la siguiente.

## Lista de comprobación antes de aprobar una spec
- [ ] ¿Cada requisito es verificable?
- [ ] ¿No hay tecnología en `spec.md`?
- [ ] ¿Todo lo dudoso está como `[NEEDS CLARIFICATION]`?
- [ ] ¿El plan cita artículos de la constitución?
- [ ] ¿Las tareas son lo bastante pequeñas para un modelo gratuito?
