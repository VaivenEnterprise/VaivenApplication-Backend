# Matriz de conformidad con la constitución v2.0.0 (spec 016, T002)

- **Fecha:** 2026-11-17 · **Base:** `develop` en `fa686a5` · Protocolo de `revisor-constitucion`, aplicado por Claude Code a pedido de BE-B
- **Actualizada:** 2026-11-17, tras las correcciones de T005–T008 (sección 4). Las celdas que cambiaron están en la sección 4; las observaciones originales se conservan abajo para que se vea de dónde se partió.
- **Leyenda:**
  - ✅ cumple, con evidencia;
  - ⚠️ cumple con una observación;
  - ❌ no cumple;
  - — no aplica.
- **Regla:** ninguna celda es ✅ sin evidencia. Cada ⚠️ y cada ❌ pasa al registro de deuda (`deuda.md`) con su ID.

## 1. Comprobaciones automáticas (evidencia común)

| Control | Resultado | Evidencia |
|---|---|---|
| Arquitectura (Art. III.2) | ✅ `domain` y `usecase` sin infraestructura | `go test ./internal/ -run Infrastructure`: `TestInnerLayersHaveNoInfrastructureDeps` en verde (2026-11-17) |
| Lint (Art. VII.3) | ✅ 0 avisos | `golangci-lint run ./...` (gosec, revive y staticcheck) |
| Cobertura (Art. VII.1) | ✅ ≥ 80 % en `domain` y `usecase` | Puerta de la CI `.github/scripts/coverage.mjs`, en verde en `fa686a5` |
| Pruebas (Art. VII.2) | ✅ 760 unitarias y 135 de integración | `go test -json` del 2026-11-17. Una prueba de integración falla **solo en Windows** por los finales de línea CRLF (DT-l); en la CI pasa |
| Vulnerabilidades (Art. VI) | ✅ 0 | `govulncheck ./...`: «No vulnerabilities found» (2026-11-17) |
| Secretos (Art. VI.1, XV.4) | ✅ 0 | Gitleaks de la CI sobre todo el historial, en verde |
| Contrato (Art. IV.6) | ✅ válido | `redocly lint`: válido, con 2 advertencias que ya existían (licencia y servidor de ejemplo) |
| Dependencias (Art. VIII) | ✅ solo las aprobadas | `go.mod` sin cambios no registrados en `memoria.md` |

## 2. Matriz por spec

Columnas:
- **I:** spec primero, sin desviaciones;
- **III:** arquitectura;
- **IV:** contrato;
- **V:** dinero, fechas e idioma;
- **VI:** seguridad (VI.2 = validación con `validator/v10` en la frontera);
- **VII:** pruebas;
- **VIII:** dependencias;
- **X:** desarrollo con OpenCode;
- **XII:** dueño y módulo;
- **XIII.5:** termina desplegada;
- **§6:** datos en MongoDB.

| Spec | I | III | IV | V | VI | VII | VIII | X | XII | XIII.5 | §6 |
|---|---|---|---|---|---|---|---|---|---|---|---|
| 001 | ✅ | — | — | — | ✅ | — | ✅ | ⚠️¹ | ✅ | — | — |
| 002 | ⚠️² | ✅ | ✅ | ✅ | ✅ | ✅ | ✅ | ⚠️¹ | ✅ | ✅ | ✅ |
| 003 | ✅ | ✅ | ✅ | ✅ | ✅ | ✅ | ✅ | ✅ | ⚠️⁴ | ✅ | ✅ |
| 004 | ✅ | ✅ | ✅ | ✅ | ✅ | ✅ | ✅ | ✅ | ✅ | ✅ | ✅ |
| 005 | ✅ | ✅ | ✅ | ✅ | ✅ | ✅ | ✅ | ⚠️¹ | ⚠️⁵ | ✅ | ✅ |
| 006 | ⚠️² | ✅ | ✅ | ✅ | ✅ | ✅ | ✅ | ⚠️¹ | ✅ | ✅ | ✅ |
| 007 | ⚠️² | ✅ | ✅ | ✅ | ✅ | ✅ | ✅ | ⚠️¹ | ⚠️⁵ | ✅ | ✅ |
| 008 | ⚠️² | ✅ | ✅ | ✅ | ✅ | ✅ | ✅ | ⚠️¹ | ✅ | ✅ | ✅ |
| 009 | ⚠️² | ✅ | ✅ | ✅ | ⚠️⁸ | ✅ | ✅ | ⚠️¹ | ✅ | ✅ | ✅ |
| 010 | ⚠️² | ✅ | ✅ | ✅ | ✅ | ✅ | ✅ | ⚠️¹ | ✅ | ✅ | ✅ |
| 011 | ⚠️² | ✅ | ✅ | ✅ | ✅ | ✅ | ✅ | ⚠️¹ | ✅ | ✅ | ✅ |
| 012 | ✅ | ✅ | ✅ | ✅ | ❌⁹ | ❌¹⁰ | ✅ | — | ✅ | ❌¹¹ | ✅ |
| 013 | ✅ | — | — | — | ✅ | ⚠️¹² | ✅ | ⚠️¹ | ✅ | ✅ | ✅ |
| 014 | ✅ | ✅ | ✅ | ✅ | ✅ | ✅ | ✅ | ⚠️¹ | ⚠️⁵ | ✅ | ✅ |
| 015 | ✅ | ✅ | ✅ | ✅ | ✅ | ✅ | ✅ | ⚠️¹ | ✅ | ✅ | ✅ |
| 017 | ❌¹⁴ | — | — | — | ✅ | ✅ | ✅ | ⚠️¹ | ⚠️³ | — | — |

### Observaciones (cada una con su deuda)

1. **Art. X:** la spec se desarrolló (en parte) con Claude Code en lugar de OpenCode con modelos gratuitos. Está registrado en `memoria.md` como desviación aceptada por el responsable (27 menciones). → **DT-h**
2. **Art. I.1:** se implementó antes de aprobar la spec; desviación registrada en `memoria.md` o en el propio encabezado. → **DT-i**
3. **Art. XII.1 / Art. I:** 002 no tiene «Dueño» en su encabezado; 017 no tiene encabezado de spec ni línea de estado. → **DT-j**
4. **Art. XII.1:** los ajustes del plan §9 de la 003 esperan el visto bueno de BE-A. → **DT-d**
5. **Art. XII.1:** la implementó BE-B a pedido del responsable; el estado dice «pendiente de visto bueno de BE-A». → **DT-d**
6. **Art. VI.2:** los DTO de entrada no usan `validator/v10`; la validación vive solo en el dominio. Módulos afectados: `search` (006), `bookings` (007), `chat` (008), `handover` y `payments` (009), `ratings`, `incidents` y `sanctions` (010), `notifications` y `reports` (011). → **DT-a**
7. **Art. I / X.5:** el estado sigue en «Borrador» aunque la spec está desplegada. → **DT-d**
8. **009, revisión propia:** las rutas no montan `RequireActive`. → **DT-b**
9. **Art. VI (RF-04 de la 012):** la cabecera `nosniff` en todas las respuestas no está implementada (012-T003 abierta). → **DT-k**
10. **Art. VII:** la prueba de punta a punta (012-T001 y T002) no existe. → **DT-k**
11. **Art. XIII.5:** la 012 no tiene despliegue de cierre (T012 abierta). → **DT-k**
12. **013:** T008 tiene evidencia sin marcar; T009 depende de 012-T011. → **DT-d**
13. **014:** la prueba de claves da un falso positivo en Windows (CRLF). → **DT-l**
14. **017:** sin spec formal en el formato `sdd-workflow` (sin RF, CA ni IDs de tarea). → **DT-j**

## 3. Veredicto

**CONFORME CON OBSERVACIONES** en las specs desplegadas, salvo la **012**, que **no cumple** (seguridad, pruebas de punta a punta y despliegue abiertos), y la **017**, que no tiene formato de spec. Las ❌ y ⚠️ quedan en `deuda.md`. Esta matriz se actualiza en la T013 tras las correcciones.

## 4. Actualización tras las correcciones (T005–T013)

| Cambio en la matriz | Motivo | Evidencia |
|---|---|---|
| VI de 006, 007, 008, 010 y 011: ⚠️⁶ → ✅; 009: ⚠️⁶ ⁸ → ⚠️⁸ | Los 9 módulos con cuerpo de entrada validan en la frontera con `validator/v10` y el helper común `internal/adapter/http/validate`. `search` (006) no recibe cuerpo: sus parámetros los valida `domain.Parse`, con detalle por campo | `validation_test.go` en 7 módulos; pruebas de handlers e integración en verde (DT-a) |
| I de 009, 010 y 011: ⚠️⁷ → ⚠️² | Su estado ya dice «implementada y desplegada en staging». Sigue la observación del Art. I.1: se implementaron antes de aprobarse | `spec.md` y `tasks.md` de 009 y 011; `spec.md` de 010 (DT-d) |
| XII de 002: ⚠️³ → ✅ | El encabezado ya tiene «Dueño» (repartida entre BE-A y BE-B según ROADMAP §5) | `specs/002-arquitectura-base/spec.md` (DT-j) |
| VII de 014: ⚠️¹³ → ✅ | La prueba de claves ya no da falso positivo con CRLF y sigue detectando una clave real | Corrida en Windows: pasa sin clave y falla con un archivo de prueba que tiene una (DT-l) |
| Comprobaciones automáticas | Unitarias 779; unitarias más integración 914; contrato 69/69 operaciones y 0 incumplimientos; `govulncheck` 0; Gitleaks 0; lint 0 | `docs/calidad/pruebas.md` |

**Sin cambio:**

| Celda | Por qué sigue igual |
|---|---|
| 012 (❌ VI, VII y XIII.5) | Son tareas de la 012 con su dueño (DT-k) |
| 017 (❌ I) | Requiere reescribirla como spec formal, fuera del alcance de la 016 (DT-j) |
| X (⚠️¹) | Desviación aceptada (DT-h) |
| XII de 003, 005 y 007 (⚠️⁴ ⁵) | Esperan el visto bueno de BE-A (DT-d2) |
| VI de 009 (⚠️⁸) | `RequireActive` no se monta por decisión documentada (DT-b) |

**Veredicto actualizado:** **CONFORME CON OBSERVACIONES** en las specs desplegadas, salvo la 012 y la 017.

El veredicto **CONFORME** que pide la T013 todavía no se alcanza. Faltan:
- la 012 (BE-A);
- las corridas de mutación, ZAP y Trivy (sección 5 de `pruebas.md`);
- el `seed` de staging (`demo.md`);
- la firma de BE-A.
