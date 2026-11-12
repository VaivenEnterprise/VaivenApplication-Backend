# Rendimiento — spec 012, T006 (RF-08, CA-07)

> Medición local del 2026-11-11. **No** es una medición de staging. Todo lo de abajo se puede repetir con los comandos de la sección 2.

## 1. Resultado

| Ruta | n | p50 | p95 | p99 | máx | Umbral p95 |
|---|---|---|---|---|---|---|
| `GET /api/v1/items` (búsqueda, 6 variantes) | 356 | 5,7 ms | **13,2 ms** | 23,5 ms | 36,7 ms | < 500 ms ✅ |
| `GET /api/v1/me/bookings` (solicitante y dueña) | 163 | 3,2 ms | **6,7 ms** | 11,8 ms | 387,8 ms | < 500 ms ✅ |

- Peticiones medidas: 519 (~104 por minuto), con 0 errores y 0 respuestas 429; todos los umbrales de k6 se cumplieron.
- El máximo de 387,8 ms en «mis reservas» es un único valor aislado (p99 = 11,8 ms); no se repitió y no afecta al p95.
- Una corrida previa de 5 minutos con el mismo escenario dio resultados equivalentes (p95 de 9,7 ms en búsqueda y 6,2 ms en reservas).
- **Ninguna consulta usó `COLLSCAN`** (sección 4).
- Recursos durante la corrida completa (setup incluido, 365 s; muestreo de `/proc` cada 5 s):

| Proceso | CPU promedio | CPU máx | RSS promedio | RSS máx |
|---|---|---|---|---|
| API | 0,5 % de un núcleo | 3,6 % | 28 MiB | 56 MiB |
| MongoDB | 1,4 % de un núcleo | 3,1 % | 187 MiB | 189 MiB |

No hubo optimizaciones: los resultados están muy por debajo del umbral, así que no hay cifras de «antes y después». Sí queda un riesgo de escalabilidad documentado en la sección 5.

## 2. Cómo se midió (reproducible)

**Datos.** `seed --scale 100` sobre una base vacía, con `APP_ENV=development`: **607 personas, 1515 objetos y 303 reservas** (más 20 solicitudes que crea el setup de k6). El seed tardó 53 s.

```bash
# 1. MongoDB 7 con replica set (Podman o Docker)
podman run -d --name vaiven-perf-mongo -p 27017:27017 docker.io/library/mongo:7 --replSet rs0 --bind_ip_all --wiredTigerCacheSizeGB 0.25
podman exec vaiven-perf-mongo mongosh --eval 'rs.initiate({_id:"rs0",members:[{_id:0,host:"localhost:27017"}]})'

# 2. Datos (credenciales en un archivo FUERA del repo; ver T005)
APP_ENV=development MONGO_URI='mongodb://localhost:27017/?directConnection=true' MONGO_DB=vaiven_perf \
  JWT_ACCESS_SECRET=<64 hex> CLOUDINARY_CLOUD_NAME=perf-local CLOUDINARY_API_KEY=perf-local \
  CLOUDINARY_API_SECRET=perf-local CLOUDINARY_BASE_FOLDER=vaiven/perf \
  SEED_CREDENTIALS_FILE=/ruta/fuera/del/repo/creds.txt \
  go run ./cmd/admin seed --scale 100

# 3. API (mismas variables, HTTP_ADDR=:8080). Los valores de Cloudinary son ficticios:
#    las lecturas medidas no lo contactan.
go run ./cmd/api

# 4. Carga: k6 2.3.0 (la versión fija del CI), 5 minutos
k6 run -e BASE_URL=http://localhost:8080 \
  -e REQUESTER_EMAIL=beto.demo@vaiven.example -e REQUESTER_PASSWORD=<del archivo> \
  -e OWNER_EMAIL=ana.demo@vaiven.example -e OWNER_PASSWORD=<del archivo> \
  tests/load/catalog.js
```

**Escenario (`tests/load/catalog.js`).**
- El `setup` inicia sesión con dos cuentas del seed y crea 20 solicitudes para la solicitante, para que `GET /me/bookings` tenga varias reservas.
- Después espera 65 s, para vaciar la ventana del límite de tasa.
- Mide durante 5 minutos una mezcla con pesos:
  - **Búsqueda (70 %):**
    - reciente, y reciente con segunda página por cursor;
    - por categoría;
    - con filtros combinados de categoría, modalidad y municipio;
    - por texto;
    - por fechas de disponibilidad.
  - **Mis reservas (30 %):** como solicitante y como dueña.
- **Ritmo:** 8 iteraciones cada 5 s, unas 106 peticiones por minuto contando la segunda página. Queda por debajo del límite de 120/min por IP (Art. XIV.2), así que se miden respuestas 200 y no 429.
- **Ajuste durante la medición:** una primera versión con 9 iteraciones cada 5 s llegaba a ~122/min y produjo 6 respuestas 429 en 2 minutos; por eso se bajó el ritmo.
- **Seguridad:** el script se niega a correr si `BASE_URL` no es local, porque crea datos.

**Entorno.**

| | |
|---|---|
| Equipo | Intel Core i7-12700H (14 núcleos, 20 hilos), 16 GB de RAM, Windows 11 Home |
| Contenedores | Podman 6.0.2 en WSL2 (máquina de 10 CPU y 2 GiB) |
| MongoDB | 7.0.45, replica set de un nodo, caché de WiredTiger de 0,25 GB (como staging) |
| API | Binario Linux del commit `ce0d149` (Go 1.26.10), en contenedor con red del host |
| Carga | k6 2.3.0 (`grafana/k6:2.3.0`), en contenedor con red del host |
| Redis | Sin Redis: `/readyz` informa `redis: degradado` y el límite de tasa usa el respaldo local |

**Diferencias con staging (limitaciones de esta medición).**
- **Recursos:** staging limita la API a 160 MB y 0,5 CPU (`GOMEMLIMIT=120MiB`) y MongoDB a 400 MB y 0,5 CPU. Aquí **esos límites no se aplicaron**: en Podman sobre WSL los contenedores no crean cgroup (`podman stats` y `--memory`/`--cpus` no funcionan). La API y MongoDB tuvieron más CPU disponible que en la VM.
- **Red:** sin TLS ni red de por medio. Las latencias son del servidor, no de un cliente en internet.
- **Redis:** sin Redis, cada petición evita la ida y vuelta a Redis del limitador.
- **Conclusión:** el margen es enorme (p95 de ~10 ms frente a 500 ms), así que es muy probable que staging también cumpla. Confirmarlo con la VM es parte del despliegue final (T009).

## 3. Cómo leer los resultados

- p50, p95 y p99 salen de `summaryTrendStats` de k6, separados por la etiqueta `kind` (`search` o `my_bookings`).
- El JSON completo de k6 se guarda donde indique `SUMMARY_FILE` (por defecto `catalog-summary.json` en el directorio actual: no lo subas al repositorio, guárdalo fuera o bórralo).
- CPU: ticks de `utime` + `stime` de `/proc/1/stat` (100 por segundo) entre muestras de 5 s, expresados en % de un núcleo. RSS: `VmRSS` de `/proc/1/status`.

## 4. Las 10 consultas más frecuentes y su plan

Se obtuvieron en una corrida aparte de 2 minutos **con el profiler de MongoDB** (`setProfilingLevel(2)`), para no alterar las latencias de la medición principal. Se agrupó `system.profile` por colección, comando, forma de la consulta y plan. Hubo 315 operaciones en 11 formas distintas.

| # | Colección | Operación (origen) | Plan | Veces | Prom. | Claves / docs examinados → devueltos |
|---|---|---|---|---|---|---|
| 1 | items | aggregate: búsqueda reciente | `IXSCAN {status, createdAt:-1, _id:-1}` (`search_recent`) | 86 | 4,7 ms | 1540 / 1536 → 21 |
| 2 | users | find por `_id` (perfiles públicos) | `IXSCAN {_id}` | 67 | 0 ms | 9 / 8 → 8 |
| 3 | bookings | find: mis reservas como solicitante | `IXSCAN {requesterId, status, createdAt:-1}` | 45 | 0 ms | 21 / 21 → 21 |
| 4 | items | aggregate: búsqueda con fechas (`$or`) | `IXSCAN {status, createdAt:-1, _id:-1}` (`search_recent`) | 28 | 3,5 ms | 1536 / 1536 → 21 |
| 5 | items | aggregate: filtros combinados | `IXSCAN {status, category, modality, zone.municipality, createdAt:-1}` (`search_filters`) | 24 | 1,3 ms | 141 / 35 → 5 |
| 6 | bookings | find: mis reservas como dueña | `IXSCAN {ownerId, status, createdAt:-1}` | 22 | 0 ms | 2 / 2 → 2 |
| 7 | items | aggregate: por categoría | `IXSCAN {status, category, createdAt:-1}` | 21 | 3,0 ms | 237 / 237 → 21 |
| 8 | items | aggregate: búsqueda de texto | `IXSCAN {_fts, _ftsx}` (`search_text`) | 14 | 2,6 ms | 122 / 223 → 21 |
| 9 | bookings | find: job que vence solicitudes | `IXSCAN {status, expiresAt}` | 3 | 0 ms | 0 / 0 → 0 |
| 10 | users | find por correo (inicio de sesión) | `IXSCAN {email}` (`email_unique`) | 2 | 0 ms | 1 / 1 → 1 |

**`COLLSCAN`: ninguno** en las 11 formas observadas.

## 5. Riesgo: la búsqueda reciente recorre todos los objetos activos

- **Qué se ve:** las consultas 1 y 4 usan el índice correcto, pero examinan **todos** los objetos activos (~1536) para devolver 21. Hoy cuesta 3,5–4,7 ms; crecerá en forma lineal con el catálogo.
- **Causa:** en `internal/search/adapter/mongo/pipelines.go`, `BuildPipeline` aplica `ownerStages` (un `$lookup` a `users` y un `$match` que descarta dueños dados de baja o suspendidos) **antes** del `$limit`. Como el filtro depende del documento unido, MongoDB no detiene el recorrido del índice al llegar a 21 resultados.
- **Decisión:** no se optimiza en T006. RF-08 se cumple con mucho margen y la spec pide no optimizar sin necesidad medida. Queda como observación para el dueño de la spec 006 (BE-B).
- **Medida sugerida si el catálogo crece:** repetir la medición con `--scale 1000` (~15 000 objetos) y, si p95 se acerca al umbral, evaluar:
  - desnormalizar en `items` el estado del dueño, actualizándolo al suspender o dar de baja la cuenta, para filtrar antes del `$limit`;
  - o hacer un `$limit` previo con sobrepedido y completar la página.
