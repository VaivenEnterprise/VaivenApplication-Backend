# Plan 013 — Entorno de producción (backend)

- **Spec:** `spec.md` · **Dueño:** BE-B

## 1. Archivos
```
cmd/admin/smoke.go (+ prueba)
config/prod.example.env  (solo nombres de variables, sin valores)
docs/{produccion.md, respaldos.md}
scripts/backup.sh · scripts/restore.sh  (documentados; usan variables de entorno, no valores)
```
El resto es configuración en la VM de Azure y Cloudinary (documentada en `docs/produccion.md`).

## 2. Fases
| Fase | Fechas | Contenido |
|---|---|---|
| 1. Entorno | 6–8 nov | T001–T004: entorno en la VM, base, Cloudinary, monitor |
| 2. Operación | 19–22 nov | T005–T009: secretos, respaldo, humo, guía, revisión |

## 3. Decisiones
| # | Decisión | Art. |
|---|---|---|
| P-01 | Misma imagen y `deploy/docker-compose.yml` que staging (proyecto `vaiven-production`, API en 443); solo cambian variables (`APP_ENV=production`, URLs, secretos) | IX |
| P-02 | Mongo propio de producción en su proyecto de Compose, con volumen y usuario propios y sin puerto publicado; ya no hay lista de IP ni `0.0.0.0/0` | VI |
| P-03 | Una carpeta de Cloudinary por entorno (`vaiven/staging/`, `vaiven/production/`) en la misma cuenta | VI |
| P-04 | `smoke` usa solo la API pública (como lo haría la app) y borra lo creado con las rutas existentes (baja de cuenta anonimiza; el objeto se elimina) | III |
| P-05 | Respaldos con `mongodump --archive --gzip`; el archivo se cifra con una frase guardada por el administrador; nunca se sube a git | VI |
| P-06 | Registro estructurado ya definido en 002; esta spec solo **revisa** que no aparezcan datos personales | VI |
| P-07 | El monitor es un servicio externo gratuito (la VM no se duerme); no se agrega código al backend para él | VIII |

## 4. Pruebas
Prueba de `smoke` contra staging y producción; restauración de respaldo en staging; revisión de registros de una sesión real; `explain` de las consultas frecuentes ya cubierto en 012.

## 5. Verificación de constitución
| Art. | Cumple |
|---|---|
| II | Índices en código, verificados con `indexes` |
| VI | Secretos separados, sin PII en registros |
| VIII | Sin dependencias nuevas |
| IX | `/healthz`, `/readyz`, cierre ordenado, reversión documentada |
