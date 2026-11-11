# Plan 013 — Entorno de producción (backend)

- **Spec:** `spec.md` · **Dueño:** BE-B · **Ajustado:** 2026-11-11 (constitución v2.0.0, Art. XIII y XIV)

## 1. Archivos
```
deploy/.env.production.example        (solo nombres de variables, sin valores; junto a deploy/.env.example)
deploy/docker-compose.yml             (modificar: red configurable —hoy fija en base_vaiven-net, la de staging— y rotación de registros; lo usan staging y producción)
deploy/production/docker-compose.yml  (nuevo: MongoDB y Redis propios de producción, sin puertos publicados)
deploy/base/docker-compose.yml        (modificar: rotación de registros de MongoDB y Redis de staging)
cmd/admin/{smoke.go, smoke_test.go}   (recorrido que escribe datos; se usa contra staging)
scripts/{backup.sh, restore.sh}       (usan variables de entorno, nunca valores)
docs/{produccion.md, respaldos.md}
```
El resto es configuración en la VM, en Cloudinary y en Better Stack, documentada en `docs/produccion.md` sin valores secretos.

## 2. Fases
| Fase | Cuándo | Contenido |
|---|---|---|
| 0. Requisitos | antes de la fase 1 | P-1 (rotar Cloudinary) y P-2 (rotar el JWT de staging) del documento de remediación; spec 012 desplegada en staging |
| 1. Entorno | en cuanto se apruebe | T001–T004: entorno en la VM con su MongoDB y Redis, medición de capacidad, Cloudinary, monitor y rotación de registros |
| 2. Operación | 19–22 nov | T005–T009: secretos y cuentas, respaldo, humo, guía y lista final |

## 3. Decisiones
| # | Decisión | Art. |
|---|---|---|
| P-01 | Misma imagen que staging (la etiqueta ya probada en staging) y el mismo `deploy/docker-compose.yml` para la API, en el proyecto `vaiven-production`, puerto 443; solo cambian las variables (`APP_ENV=production`, secretos, `MONGO_URI`, `REDIS_URL`, `CLOUDINARY_BASE_FOLDER`). La red externa de la API pasa a ser una variable (`VAIVEN_NETWORK`, con `base_vaiven-net` por defecto para no cambiar staging): así la API de producción solo se conecta a la red de su MongoDB y su Redis | IX, XIII.6 |
| P-02 | **MongoDB y Redis propios de producción** en `deploy/production/docker-compose.yml` (red y volúmenes propios), con el mismo endurecimiento que el stack base (keyFile, contraseña, sin puertos). Ningún contenedor ni credencial se comparte con staging | VI, XIII.6, XIV.4 |
| P-03 | Una carpeta de Cloudinary por entorno en la misma cuenta; por eso P-1 (rotar la llave) es requisito previo | VI |
| P-04 | `admin smoke` usa solo la API pública y limpia lo que crea (la baja anonimiza, el objeto se elimina), pero corre contra **staging**; en producción solo `scripts/smoke.sh`, de lectura | III, VI |
| P-05 | Respaldos con `mongodump --archive --gzip` dentro del contenedor de MongoDB de producción; el archivo se cifra en la VM (`openssl enc -aes-256-cbc -pbkdf2`) con una frase que guarda el administrador fuera del repositorio, y se copia fuera de la VM; nunca va a git | VI |
| P-06 | El registro estructurado ya existe (002); esta spec solo **revisa** que no aparezcan datos personales | VI |
| P-07 | Monitor: Better Stack (gratuito, cada 3 min, aviso por correo). No se agrega código al backend | VIII |
| P-08 | Límites de memoria de producción fijados con la medición de T001. Punto de partida: API 160 MB (`GOMEMLIMIT=120MiB`), MongoDB 300 MB (caché de WiredTiger 0,15 GB), Redis 96 MB (`maxmemory 64mb`). Con los 2 GB de swap como colchón (Art. XIV.5) | XIII.6, XIV.5 |
| P-09 | Rotación de registros de Docker (`logging: json-file`, `max-size: 10m`, `max-file: 3`) en todos los contenedores de los dos entornos | IX |
| P-10 | Despliegue con `scripts/deploy.sh production` desde `origin/main`, ejecutado por `devops-mcp`; reversión con `--rollback` | XIII.4, XIII.5 |

## 4. Pruebas
- `admin smoke` con servidor simulado (unitarias) y contra staging (integración real).
- `scripts/smoke.sh` contra producción.
- Restauración de un respaldo de producción en staging (CA-04).
- Revisión de los registros de una sesión real (CA-06).
- `admin indexes` en producción, código 0.
- Reinicio de la VM con los dos entornos (CA-05).

## 5. Verificación de constitución
| Art. | Cumple |
|---|---|
| II | Índices en código, verificados con `admin indexes` |
| VI | Secretos y credenciales separados; sin datos personales en registros; respaldo cifrado |
| VIII | Sin dependencias nuevas |
| IX | `/healthz`, `/readyz`, cierre ordenado, reversión documentada, registros rotados |
| XIII | Misma VM gratuita, proyecto de Compose separado con bases, volúmenes y secretos distintos; despliegue por `devops-mcp`; sin costo adicional |
| XIV | HTTPS directo sin proxy; Redis obligatorio con prefijo `vaiven:production:`; límites de memoria por contenedor |
