# Plan 012 — Integración, endurecimiento y entrega (backend)

- **Spec:** `spec.md`

## 1. Archivos
```
test/e2e/{journey_test.go, helpers_test.go, fake_cloudinary_test.go}      # -tags e2e
cmd/admin/{seed.go, seed_data.go, indexes.go, assets/*.jpg}
internal/adapter/http/middleware/{ratelimit_global.go, security_headers.go} (+ tests)
Makefile (modificar: `make lint-api` y `make e2e`)
docs/{api.md, despliegue.md, politicas.md, demo.md, validacion.md, seguridad.md}
README.md (actualizar)
```

## 2. Decisiones
| # | Decisión | Art. |
|---|---|---|
| P-01 | E2E en proceso: `httptest.Server` con el router real, Mongo por testcontainers y Cloudinary falso (`httptest`) | VII |
| P-02 | El recorrido usa un **cliente HTTP de prueba** (helpers `register`, `login`, `do`) que reutiliza los DTO del contrato | VII |
| P-03 | Reloj inyectable en jobs (expiración, retrasos) para **adelantar el tiempo** en E2E | VII |
| P-04 | `seed` usa los casos de uso y repositorios reales (no inserta documentos a mano), excepto fechas del pasado de reservas de demostración | III |
| P-05 | `indexes` recorre el registro de `EnsureIndexes` y llama `listIndexes` por colección | II |
| P-06 | Validación OpenAPI en local con `npx @redocly/cli lint api/openapi.yaml` (no agrega dependencia al repositorio) | VIII |
| P-07 | Límite global en memoria por IP; se documenta que es para una instancia | VIII |
| P-08 | `docs/seguridad.md` es una lista de comprobación firmada por quien revisa, con la fecha | VI |

## 3. Orden de trabajo (4 días)
| Día | Foco |
|---|---|
| 12 nov | E2E del flujo principal (RF-01), `indexes`, límites y cabeceras |
| 13 nov | `seed`, rendimiento y `explain`, OpenAPI local |
| 14 nov | Despliegue final en staging, `seed`, revisión de seguridad |
| 15 nov | Documentación, `demo.md`, revisión de conformidad, congelar código |

## 4. Pruebas
La propia E2E; pruebas unitarias de middleware nuevo; `seed` probado dos veces; `indexes` probado con una colección sin índice.

## 5. Verificación de constitución
| Art. | Cumple |
|---|---|
| VI | Revisión de seguridad documentada |
| VII | E2E + cobertura global |
| IX | Despliegue verificado con `/readyz` |
| XII | Sin cambiar contratos; cualquier hallazgo se corrige en el módulo dueño |
