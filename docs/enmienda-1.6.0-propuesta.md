# Propuesta de enmienda 1.6.0 (MINOR) — constitución del backend

- **Estado:** propuesta, pendiente de aprobación humana (Art. XI.1). `constitution.md` **no** se ha modificado.
- **Actualización 2026-11-08:** la enmienda 2.0.0 (aprobada) ya reescribió los Art. VII.4 y XV.7. Subsiste solo el cambio del Art. XV.2 (rama y PR por spec → por tarea o fase). La versión vigente es 2.0.0, así que esta propuesta, si se aprueba, sería 2.1.0 (MINOR) y no 1.6.0.
- **Fecha:** 2026-11-04 · **Origen:** revisión T010 de la spec 001.
- **Tipo:** MINOR (Art. XI.2): se reconoce un proceso que ya existe (CI de la spec 017 y ramas por tarea); no se elimina ningún principio.

## Motivo

La constitución v1.5.0 contradice lo que el repositorio ya hace y exige:

1. **Ramas y PR.** El Art. XV.2 pide una rama y una PR **por spec** (`spec/NNN-nombre`). `AGENTS.md` y la CI (`.github/scripts/policy.mjs`, spec 017) exigen una rama y una PR **por tarea** con el formato `tipo/SCRUM-NN-SPEC-NN-TNNN`, y todas las PR de la spec 001 (#3 a #14) siguieron ese formato.
2. **CI.** Los Art. VII.4 y XV.7 dicen que no hay pipeline automatizado, pero la spec 017 instaló uno (`.github/workflows/ci.yml`: Policy, Quality, Secrets, CodeQL y Required checks) y `main`/`develop` lo exigen como check obligatorio.

Mientras dure la contradicción, gana la constitución, y el proceso real queda fuera de ella.

## Cambios propuestos (texto exacto)

### Art. VII.4

- **Actual:** «Las pruebas se ejecutan **en local** con `make test`. El repositorio y las PR siguen el Art. XV; no hay pipeline automatizado. Antes de cada despliegue (Art. XIII) la verificación la hace el agente `devops-mcp`.»
- **Propuesto:** «Las pruebas se ejecutan **en local** con `make test` y, en cada PR, en la CI del Art. XV.7. Antes de cada despliegue (Art. XIII) la verificación la hace el agente `devops-mcp`.»

### Art. XV.2

- **Actual:** «Una rama por spec (`spec/NNN-nombre`) y una PR por spec, que se abre cuando su tarea de cierre pasa. Otra persona la revisa y la fusiona; la spec siguiente puede partir de la rama anterior mientras tanto.»
- **Propuesto:** «Una rama y una PR **por tarea**, con el formato `tipo/SCRUM-NN-SPEC-NN-TNNN` (tipo en minúsculas: `feat`, `fix`, `chore`, `docs`, `ci`, `test`, `refactor`…), hacia `develop`. El título sigue Conventional Commits y la descripción completa las secciones Resumen, Validación y Seguridad. Otra persona la revisa y la fusiona. Si las ramas se apilan, se fusionan en orden.»

### Art. XV.7

- **Actual:** «No hay pipeline automatizado (CI) definido: las verificaciones son locales y el despliegue lo hace `devops-mcp` o `probador-expo` según el proyecto.»
- **Propuesto:** «La CI de GitHub Actions (spec 017) valida cada PR: política de rama, título y descripción, calidad (`gofmt`, `go vet`, pruebas, cobertura, `golangci-lint`), secretos (gitleaks) y CodeQL. Sus checks son obligatorios para fusionar en `main` y `develop`. La CI no despliega: el despliegue lo hace `devops-mcp` o `probador-expo` según el proyecto.»

### Encabezado

Versión **1.6.0** y nota: «Enmienda 1.6.0 (2026-11-XX, MINOR): ramas y PR por tarea; se reconoce la CI de la spec 017 (Art. VII.4, XV.2 y XV.7).»

## Revisión de coherencia (Art. XI.1)

| Archivo | Cambio necesario si se aprueba |
|---|---|
| `constitution.md` | Los tres artículos y el encabezado |
| `memoria.md` | Nueva decisión en la sección 2 (solo anexar, con la etiqueta de quien la aplique) |
| `AGENTS.md` §Repositorio | Ya dice «una rama y PR por tarea»: sin cambios |
| `.opencode/agents/gestor-repo.md` | Ya usa «una rama por tarea: `tipo/SCRUM-NN-SPEC-NN-TNNN`»: sin cambios |
| `ROADMAP.md` §7.3 | Dice «una rama y una PR por spec» y «sin pipeline automatizado»: cambiar a «por tarea» y citar la CI |
| `.github/scripts/policy.mjs` | Sin cambios (ya aplica el formato) |

## Cómo aprobarla

Una persona del equipo responde «apruebo la enmienda 1.6.0» (en la PR o en el grupo). Después se aplican los cambios de la tabla en una sola PR `docs/SCRUM-NN-SPEC-01-TNNN` y se registra la decisión en `memoria.md`.
