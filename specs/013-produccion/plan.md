# Plan 013 — Entorno de producción (backend)

- **Spec:** `spec.md` · **Dueño:** BE-B

## 1. Archivos
```
cmd/admin/smoke.go (+ prueba)
config/prod.example.env  (solo nombres de variables, sin valores)
docs/{produccion.md, respaldos.md}
scripts/backup.sh · scripts/restore.sh  (documentados; usan variables de entorno, no valores)
```
El resto es configuración en Render, Atlas y Cloudinary (trámites manuales documentados en `docs/produccion.md`).

## 2. Fases
| Fase | Fechas | Contenido |
|---|---|---|
| 1. Entorno | 30 oct–1 nov | T001–T004: servicio, base, Cloudinary, monitor |
| 2. Operación | 12–15 nov | T005–T009: secretos, respaldo, humo, guía, revisión |

## 3. Decisiones
| # | Decisión | Art. |
|---|---|---|
| P-01 | Mismo código y `Dockerfile` que staging; solo cambian variables (`APP_ENV=production`, URLs, secretos) | IX |
| P-02 | Atlas M0 exige lista de IP permitidas; Render gratuito no tiene IP fija, por lo que se permite `0.0.0.0/0` **con** usuario de permisos mínimos, TLS obligatorio y contraseña fuerte; el riesgo queda anotado en `docs/produccion.md` | VI |
| P-03 | Una carpeta de Cloudinary por entorno (`vaiven/staging/`, `vaiven/production/`) en la misma cuenta | VI |
| P-04 | `smoke` usa solo la API pública (como lo haría la app) y borra lo creado con las rutas existentes (baja de cuenta anonimiza; el objeto se elimina) | III |
| P-05 | Respaldos con `mongodump --archive --gzip`; el archivo se cifra con una frase guardada por el administrador; nunca se sube a git | VI |
| P-06 | Registro estructurado ya definido en 002; esta spec solo **revisa** que no aparezcan datos personales | VI |
| P-07 | El monitor es un servicio externo gratuito; no se agrega código al backend para él | VIII |

## 4. Pruebas
Prueba de `smoke` contra staging y producción; restauración de respaldo en staging; revisión de registros de una sesión real; `explain` de las consultas frecuentes ya cubierto en 012.

## 5. Verificación de constitución
| Art. | Cumple |
|---|---|
| II | Índices en código, verificados con `indexes` |
| VI | Secretos separados, sin PII en registros |
| VIII | Sin dependencias nuevas |
| IX | `/healthz`, `/readyz`, cierre ordenado, reversión documentada |
