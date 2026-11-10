# Plan 012 — Integración, endurecimiento y entrega (backend)

- **Spec:** `spec.md` · **Revisión arquitectónica:** 2026-11-09 (sección 6)

## 1. Archivos
```
test/e2e/{journey_test.go, helpers_test.go, fake_cloudinary_test.go}      # -tags e2e, solo local
cmd/admin/{seed.go, seed_data.go, seed_upload.go, indexes.go, assets/*.jpg} (+ tests)
cmd/admin/main.go (modificar: subcomandos `seed` e `indexes`)
internal/adapter/http/middleware/{security_headers.go, security_headers_test.go}
internal/adapter/http/router.go (modificar: montar las cabeceras)
internal/<módulo>/adapter/http/{dto.go|handler.go} (modificar solo donde falte distinguir *http.MaxBytesError; lo corrige el dueño del módulo)
internal/auth/adapter/http/middleware.go (modificar: RequireRole consulta el rol en la base; AUD-017)
scripts/scan-archives.py, scripts/scan_archives_test.py (modificar: AUD-015)
tests/load/ (extender el k6 existente con GET /items y GET /me/bookings)
Makefile (modificar: `make lint-api` y `make e2e`)
docs/{api.md, despliegue.md, politicas.md, demo.md, validacion.md, seguridad.md, rendimiento.md}
docs/seguridad/remediacion-2026-11.md (modificar: AUD-018)
README.md (actualizar)
```
No se crea `ratelimit_global.go`: el límite global ya existe (decisión P-07).

## 2. Decisiones
| # | Decisión | Art. |
|---|---|---|
| P-01 | E2E en proceso: `httptest.Server` con el router real, Mongo por Testcontainers (Docker o Podman) y Cloudinary falso (`httptest`). No se conecta a ningún servicio remoto | VII |
| P-02 | El recorrido usa un **cliente HTTP de prueba** (helpers `register`, `login`, `do`) que reutiliza los DTO del contrato | VII |
| P-03 | Reloj inyectable en jobs (expiración, retrasos) para **adelantar el tiempo** en E2E; se reutiliza `internal/platform/clock` | VII |
| P-04 | `seed` usa los casos de uso y repositorios reales (no inserta documentos a mano), excepto fechas del pasado de reservas de demostración | III |
| P-05 | `indexes` recorre el registro de `EnsureAll` (002-T014) y llama `listIndexes` por colección; solo lee | II |
| P-06 | Validación OpenAPI en local con `npx @redocly/cli lint api/openapi.yaml` (no agrega dependencia al repositorio) | VIII |
| P-07 | **Se conserva el limitador existente** (`internal/adapter/http/middleware/ratelimit.go`, puerto `ratelimit.Limiter`): 120 peticiones/min por IP en Redis; si Redis falla, pasa a un límite local por proceso y lo registra una vez (Art. XIV.2). La IP sale de `RemoteAddr` y **no se confía en `X-Forwarded-For`**, porque la API recibe el tráfico directo, sin proxy (Art. XIV.4); si algún día hay un proxy, esto debe revisarse. No se crea un segundo limitador | XIV |
| P-08 | `docs/seguridad.md` es una lista de comprobación firmada por quien revisa, con la fecha | VI |
| P-09 | Cabeceras en un único middleware global: `nosniff` en todo; `no-store` cuando la petición lleva identidad autenticada. No se duplica el `no-store` que ya pone `handover`, ni las cabeceras de `/docs` | VI |
| P-10 | El límite de 1 MiB ya existe (`limitBody` en `router.go`) y el cuerpo nunca se lee más allá. T003 verifica que **cada** módulo que decodifica JSON distinga `*http.MaxBytesError` y responda 400 `validation_failed` «El cuerpo de la petición es demasiado grande.», como ya hacen auth, bookings, handover e items; chat, ratings, incidents, reports, sanctions, notifications e images hoy responden un genérico «no es un JSON válido». No se usa 413: el contrato no lo declara | IV |
| P-11 | `RequireRole` reutiliza el `UserLookup` que ya usa `RequireActive`: compara el rol del token con el de la base y responde 403 si no coinciden. Un solo punto de autorización; los módulos no cambian | VI |
| P-12 | Credenciales del `seed`: archivo `correo=contraseña` fuera del repositorio, indicado por `SEED_CREDENTIALS_FILE`; 7 entradas distintas (6 personas y el admin), contraseñas no repetidas y de largo mínimo de la política de auth. Sin archivo válido, `seed` termina con código ≠ 0 sin escribir nada | VI |
| P-13 | Idempotencia del `seed` por búsqueda, no por sobrescritura: persona por correo; objeto por dueña y título; reserva por objeto, solicitante y fechas. Si ya existe, se omite y se cuenta como «existente» | II |
| P-14 | Fotos del `seed`: `images.Service` solo firma (no sube desde el servidor). `seed_upload.go` pide la firma, sube el archivo a Cloudinary con esa firma y luego llama `Resolve`, igual que la app. No se agrega un método al puerto. Las fotos se leen del binario o del repositorio en memoria (sin archivos temporales); si algo falla después de subir, `seed_upload.go` borra la foto con la API firmada `destroy` de Cloudinary y lo informa sin mostrar firma ni secreto (CA-09) | III, XII |
| P-15 | Rendimiento con el k6 existente (`tests/load`, spec 017) contra la API local con `seed --scale 100` en MongoDB local; nunca contra staging en esta tarea | VII |

## 3. Orden de trabajo (4 días)
| Día | Foco |
|---|---|
| 19 nov | E2E del flujo principal (RF-01), `indexes`, cabeceras y cuerpo demasiado grande |
| 20 nov | `seed`, rendimiento y `explain`, OpenAPI local |
| 21 nov | Despliegue final en staging, `seed`, revisión de seguridad |
| 22 nov | Documentación, `demo.md`, revisión de conformidad, congelar código |

## 4. Pruebas
- La propia E2E (local).
- Pruebas unitarias del middleware de cabeceras y de `RequireRole` con un rol distinto en la base (CA-06).
- `seed` dos veces sobre Mongo de Testcontainers (CA-02), con `APP_ENV=production` y con un archivo de credenciales inválido.
- `indexes` con una colección a la que le falta un índice (CA-03).
- Escáner: casos «comprimido renombrado en el historial» y «formato no soportado» (AUD-015).
- Cobertura: no se agregan pruebas solo para subir el porcentaje.

## 5. Verificación de constitución
| Art. | Cumple |
|---|---|
| II | `indexes` solo lee; `seed` idempotente por búsqueda |
| III | `seed` usa casos de uso; sin `mongo`/`bson` en `domain` ni `usecase` |
| IV | 400 y 403 en el formato de error estándar; sin códigos nuevos fuera del contrato |
| VI | Revisión de seguridad documentada; rol de admin contra la base; sin contraseñas en el repositorio |
| VII | E2E + cobertura global |
| IX | Despliegue verificado con `/readyz` |
| XII | Sin cambiar contratos; cualquier hallazgo se corrige en el módulo dueño |
| XIV | Un solo limitador, en Redis, con respaldo local documentado |

## 6. Revisión arquitectónica (2026-11-09)
Comprobado en el código:
- Límite global de 120/min por IP en Redis con respaldo local: `cmd/api/main.go:84` y `router.go:72`. No confía en `X-Forwarded-For` (`ratelimit.go:72`).
- Límite de cuerpo de 1 MiB global (`router.go:19` y `limitBody`); 7 módulos no distinguen el cuerpo demasiado grande en su mensaje de error (P-10). Ningún módulo usa 413 y el contrato no lo declara.
- `Cache-Control: no-store` solo en `handover`; `nosniff` solo en `/docs`.
- `RequireRole` confía solo en el claim (`internal/auth/adapter/http/middleware.go:54`); ya existe `UserLookup` para reutilizar.
- `images.Service` expone `Signature`, `Resolve` y `URLs`, no una subida desde el servidor (P-14).
- 002-T014 (`EnsureAll`) está cerrada y basta para `indexes` y para que `seed` cree colecciones e índices.
- Servidor con `ReadTimeout`, `ReadHeaderTimeout` y `WriteTimeout` (`cmd/api/main.go:415`); `/healthz` y `/readyz` existen; la configuración se valida al inicio (`internal/config`).
- La prueba de arquitectura (`architecture_test.go`) vigila las dependencias entre capas.

No comprobado aquí (queda para T008 y T011): lógica de negocio en handlers, uso de contexto y cancelación en cada repositorio, y la validación de entradas módulo por módulo.
