# Plan 016 — Auditoría de calidad y consolidación (backend)

- **Spec:** `spec.md` · **Dueño:** BE-B (coordina) con BE-A · **Constitución:** v2.0.0

## 1. Archivos
```
docs/calidad/{inventario.md, conformidad.md, trazabilidad.md, deuda.md, pruebas.md, demo.md, informe.md}
scripts/trace.py, scripts/trace_test.py                     trazabilidad RF/CA → pruebas (solo lectura del repositorio)
internal/{bookings, handover, payments}/adapter/http/*      validator/v10 en los DTO (BE-A: specs 007 y 009)
internal/{search, chat, ratings, incidents, sanctions, notifications, reports}/adapter/http/*   (BE-B: 006, 008, 010 y 011)
internal/adapter/http/contract/{contract.go, contract_test.go}   verificación de respuestas contra api/openapi.yaml (solo pruebas)
scripts/mutation.sh                                         todos los internal/*/domain e internal/*/usecase
.github/workflows/zap.yml                                   ZAP baseline contra staging, solo a mano
.github/workflows/ci.yml                                    escaneo de la imagen y SBOM
specs/*/spec.md (solo la línea de estado), docs/politicas.md (POL-06 y POL-07), memoria.md
```

## 2. Decisiones
| # | Decisión | Art. |
|---|---|---|
| P-01 | **Evaluar antes de corregir:** las tareas de auditoría (T001–T004) no cambian código; producen la línea base y el registro de deuda que ordena las correcciones | I |
| P-02 | **Cada dueño corrige lo suyo** (`ROADMAP.md` §2): BE-A las specs 003, 005, 007, 009 y 014; BE-B las 001, 004, 006, 008, 010, 011 y 015. Si alguien corrige un módulo ajeno, el dueño lo aprueba en el PR | XII.1 |
| P-03 | **Pruebas de contrato con una dependencia solo de pruebas** que valide respuestas contra OpenAPI **3.1** (**aprobada por BE-B el 2026-11-17**: `github.com/pb33f/libopenapi-validator`, porque el contrato es 3.1 y otras bibliotecas solo cubren bien la 3.0). Se usa únicamente en archivos `_test.go` y en el paquete `contract`, que solo importan las pruebas; nunca entra al binario de la API. Al instalarla (T008) se fija una versión publicada hace al menos dos semanas, se revisa con `govulncheck` y se registra en `memoria.md` | VIII.1 |
| P-04 | La verificación de contrato se engancha como *middleware* de prueba al router real en las pruebas de integración de los handlers y en la prueba de punta a punta de la 012; así no hay que reescribir pruebas | VII |
| P-05 | Mutación: se amplía la lista por defecto de `mutation.sh`; los disparadores siguen siendo semanal y a mano porque una corrida completa es lenta | VII |
| P-06 | ZAP *baseline* (solo pasivo) con `zaproxy/action-baseline`, solo `workflow_dispatch` y solo contra la URL de staging; nunca producción | VI, XIII.1 |
| P-07 | Escaneo de la imagen y SBOM CycloneDX con Trivy (`aquasecurity/trivy-action`) en la CI; acciones fijadas por SHA como el resto de la 017; falla con críticas o altas que tengan corrección | VI, XV.7 |
| P-08 | La trazabilidad se genera con `scripts/trace.py` (Python sin dependencias, como `scan-archives.py`): busca `RF-NN`, `RNF-NN` y `CA-NN` en nombres y comentarios de pruebas y los cruza con las tablas de cada `spec.md` | VIII |
| P-09 | `validator/v10` en los DTO replica las reglas que ya aplica el dominio; el dominio sigue validando (doble validación, Art. VI.2). Los mensajes y `details` usan la traducción que ya existe en los módulos que lo usan | VI.2, IV.3 |
| P-10 | Ningún documento de `docs/calidad/` lleva secretos, correos ni datos de personas; los resultados citan la ejecución (enlace de Actions o comando) | VI |

## 3. Pruebas
- `validator/v10`: por módulo, una prueba de tabla con un caso válido y uno inválido por campo (CA-03).
- Contrato: prueba del propio verificador (una respuesta válida pasa; un campo faltante, un tipo incorrecto o un error fuera del formato del Art. IV fallan con ruta y campo, CA-04).
- `trace.py`: pruebas con un repositorio de ejemplo pequeño.
- Mutación, ZAP y Trivy: sus resultados reales se anotan en `docs/calidad/pruebas.md`.

## 4. Verificación de constitución
| Art. | Cumple |
|---|---|
| I | Spec, plan y tareas antes de corregir; la auditoría ordena el trabajo |
| IV | Sin cambios incompatibles; el contrato se verifica con respuestas reales |
| VI | Validación en la frontera en todos los módulos; DAST y escaneo de imagen; documentos sin datos personales |
| VII | Las siete categorías de pruebas con resultados reales |
| VIII | Una sola dependencia, solo de pruebas, justificada y aprobada (P-03) |
| XI | Si hace falta cambiar la constitución, se propone por escrito; esta spec no la toca |
| XII | Cada dueño corrige o aprueba lo suyo |
| XIII.5 | Termina con el despliegue verificado en staging (T014) |
