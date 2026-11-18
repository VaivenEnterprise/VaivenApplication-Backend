# Informe de las siete categorías de pruebas (spec 016, RF-12)

- **Fecha:** 2026-11-17 · **Base:** rama `docs/SPEC-16-T001` (desde `spec/SPEC-16`) · **Dueño:** BE-B
- **Regla (RF-12, CA-09):** nunca se reporta un resultado que no se ejecutó. Cada fila dice dónde se ejecutó. Las corridas locales no tienen enlace; la CI de la PR las repite y su enlace se anota al abrirla. Lo que todavía no se ejecuta dice **pendiente**.
- **Entorno local:** Windows 11, Go 1.26.11, Podman (Testcontainers con `mongo:7`). Sin gcc, así que localmente no se usa `-race`; la CI sí lo usa (`ubuntu-24.04`).

## Resumen

| # | Categoría | Último resultado | Dónde | Fecha | Enlace |
|---|---|---|---|---|---|
| 1 | Unitarias | **779** en verde, 0 fallas, 2 omitidas. Los 33 paquetes `domain` y `usecase` tienen ≥ 91,2 % de cobertura (umbral: 80 %) | Local, `go test ./... -cover` | 2026-11-17 | Local; CI de la PR |
| 2 | Integración | **914** en verde (unitarias más integración), 0 fallas, 2 omitidas | Local, `go test -tags=integration ./...` | 2026-11-17 | Local; CI de la PR |
| 3 | Contrato | **69 de 69** operaciones de `openapi.yaml` ejercidas con respuestas reales; **0** incumplimientos | Local, dentro de la integración (T008) | 2026-11-17 | Local; CI de la PR |
| 4 | Base de datos | Repositorios de todos los módulos contra MongoDB 7 real (índices, transacciones, concurrencia), dentro de la integración: en verde | Local (Testcontainers) | 2026-11-17 | Local; CI de la PR |
| 5 | Mutación | **Pendiente:** el alcance se amplió a los 32 `domain` y `usecase` de módulo (T009), pero la corrida completa todavía no se lanza | GitHub Actions, `mutation.yml` (manual o semanal) | — | — |
| 6 | Rendimiento | p95 de búsqueda **13,2 ms** y de «mis reservas» **6,7 ms** con `seed --scale 100` (umbral: 500 ms), 0 errores. Recomendaciones en staging: p95 **205 ms** con conexión reutilizada (spec 015) | Local con k6 (`docs/rendimiento.md`) y staging (`memoria.md`, 015) | 2026-11-11 y 2026-11-17 | `docs/rendimiento.md` |
| 7 | Seguridad | `govulncheck`: 0 vulnerabilidades (incluida la dependencia de pruebas). Gitleaks: 0 hallazgos. `golangci-lint` (con gosec): 0 avisos. CodeQL: en la CI. ZAP: **pendiente**. Trivy y SBOM: **pendiente** de la primera corrida en la CI | Local, más la CI de la PR | 2026-11-17 | Local; CI de la PR |

## 1. Unitarias

- **Qué se prueba:** reglas de dominio puras, casos de uso con dobles en memoria y handlers HTTP con `httptest`.
- **Dónde:** `internal/**/*_test.go`, sin etiqueta de compilación.
- **Cómo se ejecuta:** `make test` (`go test ./... -race -cover`). La puerta de cobertura de la CI exige ≥ 80 % en `domain` y `usecase` (Art. VII.1).
- **Nuevo en la 016:**
  - `internal/adapter/http/validate` (100 %);
  - `validation_test.go` en 7 módulos, que comprueba las etiquetas de `validator/v10` de cada DTO con el detalle exacto por campo (T005, T006).
- **Cobertura de los adaptadores HTTP tocados:** bookings 98,2 %, handover 98,9 %, payments 97,7 %, chat 97,4 %, ratings 98,8 %, incidents 99,3 %, notifications 100 %, reports 98,9 %, sanctions 93,9 %.

## 2. Integración

- **Qué se prueba:** flujos HTTP completos de cada módulo con MongoDB real y, donde aplica, Redis y servicios externos simulados (Cloudinary, proveedor de IA).
- **Dónde:** `*_integration_test.go` (etiqueta `integration`).
- **Cómo se ejecuta:** `make test-integration`. Necesita Docker o Podman.
- **Nuevo en la 016:**
  - `bookings` ejerce cambiar el punto de encuentro y rechazar;
  - `auth` ejerce `PATCH /me`;
  - `items` ejerce `GET /categories`;
  - la prueba de claves de la 014 ya no da falso positivo en Windows (DT-l).

## 3. Contrato (T008, plan P-03 y P-04)

- **Qué se prueba:** que cada respuesta real (código, cabecera `Content-Type` y cuerpo) cumpla `openapi.yaml` 3.1, incluidos los errores 4xx.
- **Dónde:** `internal/adapter/http/contract` envuelve el router de las 14 pruebas de integración que levantan la API: auth, images, items, search (dentro de bookings y reports), bookings, chat, handover, payments, ratings, incidents, sanctions, notifications, reports, assistant, suggest, reco y readyz.
- **Cómo se ejecuta:**
  - va dentro de `make test-integration`;
  - para medir la cobertura de rutas: `CONTRACT_COVERAGE=<archivo>` en la corrida y luego `CONTRACT_COVERAGE_REPORT=<archivo> go test -run TestCoverageReport -v ./internal/adapter/http/contract/`.
- **Dependencia:** `github.com/pb33f/libopenapi-validator` **v0.16.2**, publicada el 2026-10-30 (P-03 pide al menos dos semanas; la v0.16.3 es del 2026-11-11 y no cumplía).
  - Solo la importan las pruebas;
  - `go version -m` del binario de la API no la lista (0 coincidencias);
  - `govulncheck` sin hallazgos.
- **Resultado:** 69 de 69 operaciones, 0 incumplimientos.

## 4. Base de datos

- **Qué se prueba:** repositorios de MongoDB (índices únicos y parciales, `WithTransaction`, escrituras condicionadas por versión, carreras de aceptación y confirmación) contra `mongo:7` real.
- **Dónde:** `internal/*/adapter/mongo/*_integration_test.go`, `cmd/admin/indexes_integration_test.go` y las pruebas de punta a punta de cada módulo.
- **Cómo se ejecuta:** dentro de `make test-integration`.

## 5. Mutación (T009)

- **Qué se prueba:** que las pruebas detecten cambios pequeños en el código (gremlins). Umbrales: eficacia ≥ 75 % y cobertura de mutantes ≥ 70 %.
- **Dónde:**
  - `scripts/mutation.sh`: por omisión, además de `internal/domain`, `internal/usecase` y `internal/platform/security`, recorre los 16 `internal/*/domain` y los 16 `internal/*/usecase` (CA-05);
  - `.github/workflows/mutation.yml`, con tiempo límite subido a 180 minutos.
- **Cómo se ejecuta:** *Actions → Mutation → Run workflow* (o el domingo, por calendario).
- **Resultado:** **pendiente.** La corrida es lenta y se lanza desde Actions después de fusionar. Los paquetes que no alcancen el umbral pasan a `deuda.md` (DT-o).

## 6. Rendimiento

- **Qué se prueba:** p95 de búsqueda y «mis reservas» con k6 (umbral: 500 ms) y uso de índices (profiler de MongoDB).
- **Dónde:** `tests/load/catalog.js` y `.github/workflows/load.yml`.
- **Resultado:**
  - local, 2026-11-11: búsqueda 13,2 ms y reservas 6,7 ms de p95, sin `COLLSCAN` (`docs/rendimiento.md`);
  - recomendaciones en staging, 2026-11-17: p95 de 205 ms (spec 015).
- **Sin cambios en la 016:** no hay funcionalidad nueva (RNF-01).

## 7. Seguridad

| Control | Resultado | Fecha | Dónde |
|---|---|---|---|
| `govulncheck ./...` | No vulnerabilities found | 2026-11-17 | Local y CI |
| `govulncheck -test` del paquete `contract` | No vulnerabilities found | 2026-11-17 | Local |
| Gitleaks (árbol de trabajo) | No leaks found | 2026-11-17 | Local. La CI también recorre el historial |
| `golangci-lint` (gosec, revive, staticcheck) | 0 avisos | 2026-11-17 | Local y CI |
| CodeQL | Corre en la CI y bloquea hallazgos altos y críticos | — | CI de la PR |
| Validación en la frontera (`validator/v10`, Art. VI.2) | 9 de 9 módulos con cuerpo de entrada; `search` no tiene cuerpo | 2026-11-17 | T005 y T006 |
| ZAP baseline (T010) | **Pendiente.** `.github/workflows/zap.yml`: solo manual, solo staging (pide escribir `staging` y rechaza otro destino), solo pasivo, acciones fijadas por SHA. No se ejecutó: necesita autorización del responsable | — | Actions |
| Trivy y SBOM CycloneDX (T011) | **Pendiente** de la primera corrida: job `image` de la CI. Falla con críticas o altas que tengan corrección y sube `sbom-cyclonedx` como artefacto. Es requisito de la puerta «Required checks» | — | CI de la PR |
