# Spec 012 — Integración, endurecimiento y entrega (backend)

- **Dueño:** todo el equipo (BE-A lidera integración; BE-B lidera despliegue) · **Prioridad:** P0 · **Ventana:** 19–22 nov · **Depende de:** 001–011
- **Constitución:** v1.4.0 · **Estado:** Borrador

## 1. Objetivo y por qué
Convertir el conjunto de módulos en un backend **entregable**: recorrido de punta a punta automatizado, datos de demostración, endurecimiento básico de seguridad y rendimiento, despliegue verificado y documentación para la presentación. No agrega funcionalidad.

## 2. Requisitos
| ID | Requisito |
|---|---|
| RF-01 | Una prueba **E2E** (`-tags e2e`) recorre el flujo completo con Mongo real y Cloudinary simulado: registrar A y B → A publica → B busca y cotiza → solicita → A acepta → chat → evidencia y QR de entrega → pago reportado y confirmado → devolución con QR → ambas partes califican → incidencia abierta y cerrada por admin. |
| RF-02 | `go run ./cmd/admin seed` crea datos de demostración de forma **idempotente**: 6 personas, 15 objetos con fotos de ejemplo, 3 reservas en distintos estados y 1 administrador; las fotos se suben a Cloudinary con imágenes incluidas en el repositorio (≤ 100 KB cada una). |
| RF-03 | `go run ./cmd/admin indexes` compara los índices esperados (de `EnsureIndexes`) con los existentes y reporta faltantes o sobrantes. |
| RF-04 | Límite global por IP (120 peticiones/min) además de los de auth, chat e imágenes; cabeceras de seguridad (`X-Content-Type-Options`, `Cache-Control: no-store` en respuestas autenticadas); tamaño máximo de cuerpo revisado. |
| RF-05 | Revisión de seguridad documentada: ninguna ruta de datos sensibles sin autenticación, `RequireRole` en `/admin/*`, ningún secreto en logs, CORS acotado, evidencia solo con URL firmada. |
| RF-06 | `api/openapi.yaml` (con todos los `paths/*.yaml`) valida sin errores con `npx @redocly/cli lint` en local. |
| RF-07 | Staging actualizado con la versión final, conectado a MongoDB y Redis de la VM de Azure y a Cloudinary, con `check-services`, `indexes` y `seed` ejecutados. |
| RF-08 | Rendimiento: `GET /items` y `GET /bookings` con p95 < 500 ms con los datos de `seed` ×100; sin consultas sin índice (revisar con `explain` las 10 consultas más frecuentes). |
| RF-09 | Documentación: `README` final, `docs/api.md` (resumen de endpoints por módulo), `docs/despliegue.md`, `docs/politicas.md` (POL-xx vigentes) y `docs/demo.md` (guion de demostración con las cuentas del seed). |
| RF-10 | `docs/validacion.md`: plan breve de validación con personas usuarias (qué observar: objetos publicados, solicitudes aceptadas, devoluciones, tiempos de respuesta e incidencias), sin presentar expectativas como resultados comprobados. |
| RNF-01 | `make fmt lint test` y `go test -tags integration ./...` pasan; cobertura global ≥ 75 %. |
| RNF-02 | La revisión final de `revisor-constitucion` sobre todo el repositorio es **CONFORME**. |

## 3. Criterios de aceptación
| ID | Dado / Cuando / Entonces |
|---|---|
| CA-01 | Dado el repositorio limpio, cuando ejecuto la prueba E2E, entonces el recorrido completo termina sin errores. |
| CA-02 | Dado staging recién desplegado, cuando ejecuto `seed` dos veces, entonces no hay duplicados. |
| CA-03 | Dado `indexes`, entonces no hay índices faltantes. |
| CA-04 | Dada una persona que sigue `docs/demo.md`, entonces puede recorrer la demostración sin ayuda. |

## 4. Fuera de alcance
Funcionalidad nueva, migración a otro hosting, monitoreo avanzado, copias de seguridad automatizadas.

## 5. Preguntas abiertas
- [NEEDS CLARIFICATION: ¿Qué debe mostrarse en la presentación final y quién presenta? Define el guion de `docs/demo.md`.]
