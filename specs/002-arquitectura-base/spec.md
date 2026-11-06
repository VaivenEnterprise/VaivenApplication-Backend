# Spec 002 — Arquitectura base del backend

- **Estado:** Aprobada por la persona responsable (Joseph Yañez) el 2026-11-05. T001 y T002 se ejecutaron antes de esta aprobación (desviación del Art. I.1 registrada en `memoria.md`)
- **Proyecto:** `vaiven-backend/` (Go + MongoDB)
- **Constitución aplicable:** v1.0.0
- **Fecha:** 2026-10-26

> Esta spec describe **qué** debe existir y **por qué**. La tecnología y la estructura de archivos están en `plan.md`.
> **No incluye funcionalidad de negocio** de VAIVÉN (usuarios, objetos, solicitudes, chat, pagos, etc.). Eso se especifica en `002-*` y posteriores.

## 1. Resumen y objetivo

Dejar listo un esqueleto de backend ejecutable, probado y documentado sobre el cual se construirán las funcionalidades de VAIVÉN, de modo que cada spec funcional posterior solo agregue casos de uso, endpoints y colecciones sin rediseñar la base.

**Por qué:** una base común evita decisiones repetidas, hace consistentes los errores y las convenciones entre backend y app móvil, y permite que modelos gratuitos con poco contexto trabajen sobre una estructura predecible.

## 2. Historias técnicas

| # | Prioridad | Historia |
|---|---|---|
| H-01 | P1 | Como **persona desarrolladora**, quiero levantar la API y MongoDB con un solo comando, para empezar a trabajar sin configuración manual. |
| H-02 | P1 | Como **persona desarrolladora**, quiero un lugar único y validado para la configuración, para no depender de valores escondidos en el código. |
| H-03 | P1 | Como **cliente de la API (app móvil)**, quiero errores en un formato único y predecible, para manejarlos en un solo punto. |
| H-04 | P1 | Como **operación**, quiero comprobar si el servicio vive y si puede hablar con la base de datos, para detectar fallos rápido. |
| H-05 | P2 | Como **persona desarrolladora**, quiero registrar índices de cada colección desde código y que se creen al arrancar, para que el esquema sea reproducible. |
| H-06 | P2 | Como **persona desarrolladora**, quiero pruebas unitarias y de integración listas, para agregar funcionalidad con confianza. |
| H-07 | P2 | Como **persona desarrolladora**, quiero el contrato de la API descrito en OpenAPI, para que backend y móvil trabajen contra el mismo documento. |
| H-08 | P3 | Como **equipo**, quiero verificaciones automáticas (formato, lint, pruebas, compilación) en cada cambio, para mantener la calidad. |

## 3. Requisitos funcionales (técnicos)

| ID | Requisito |
|---|---|
| RF-01 | El servicio arranca leyendo su configuración **solo** de variables de entorno y falla al iniciar, con un mensaje claro, si falta una variable obligatoria o tiene un valor inválido. |
| RF-02 | Se incluye un entorno local reproducible con MongoDB 7+ como replica set de un nodo, levantable con un comando. |
| RF-03 | Al iniciar, el servicio se conecta a MongoDB y falla si no lo logra dentro del tiempo configurado. |
| RF-04 | `GET /healthz` responde 200 mientras el proceso vive, sin consultar la base de datos. |
| RF-05 | `GET /readyz` responde 200 si MongoDB responde a un *ping* y 503 (`service_unavailable`) si no. Incluye `redis: "ok"` o `"degradado"`; Redis caído **no** produce 503 (Art. XIV.3). |
| RF-06 | `GET /api/v1/meta` responde 200 con `name`, `version`, `environment` y `serverTime` (UTC). Sirve para que el móvil compruebe conectividad. |
| RF-07 | Toda respuesta incluye `X-Request-Id`; si la petición lo trae válido, se reutiliza; si no, se genera uno. |
| RF-08 | Todo error HTTP usa el envoltorio único `{ "error": { code, message, details, requestId } }` con los códigos de la constitución (Art. IV). |
| RF-09 | Una ruta inexistente responde 404 `not_found` y un método no permitido responde 405 con el mismo envoltorio. |
| RF-10 | Un fallo inesperado (*panic*) se captura, se registra y responde 500 `internal_error` sin exponer detalles internos. |
| RF-11 | Cada petición se registra en un log estructurado (método, ruta, estado, duración, `requestId`) sin datos personales ni cuerpos. |
| RF-12 | CORS se aplica con una lista explícita de orígenes configurable. |
| RF-13 | Existe un mecanismo de registro de índices: cada repositorio puede declararlos y el arranque los crea de forma idempotente. En esta spec el registro está vacío. |
| RF-14 | Ante `SIGINT`/`SIGTERM` el servicio deja de aceptar conexiones, espera las peticiones en curso hasta un máximo configurable y cierra la conexión a MongoDB. |
| RF-15 | `api/openapi.yaml` describe `/healthz`, `/readyz`, `/api/v1/meta` y el esquema `Error`. |

## 4. Requisitos no funcionales

| ID | Requisito |
|---|---|
| RNF-01 | Cobertura de pruebas ≥ 80 % en `domain` y `usecase` (Art. VII). En esta spec, `config`, middleware y respuestas HTTP también ≥ 80 %. |
| RNF-02 | `gofmt`, `go vet` y `golangci-lint` terminan sin advertencias. |
| RNF-03 | `/healthz` responde en menos de 50 ms en entorno local. |
| RNF-04 | La API arranca en menos de 5 s con MongoDB disponible (local). |
| RNF-05 | Ningún secreto ni dato personal aparece en el repositorio ni en los logs. |
| RNF-06 | Las dependencias se limitan a las registradas en `plan.md` y `memoria.md`. |
| RNF-07 | `domain` y `usecase` no importan paquetes de infraestructura (Art. III), comprobable automáticamente. |

## 5. Criterios de aceptación

| ID | Dado / Cuando / Entonces |
|---|---|
| CA-01 | **Dado** un equipo con Docker y Go, **cuando** ejecuto los comandos del README, **entonces** la API responde en `/healthz` con 200. |
| CA-02 | **Dado** que falta `MONGO_URI`, **cuando** arranco la API, **entonces** termina con código ≠ 0 y un mensaje que nombra la variable. |
| CA-03 | **Dado** MongoDB detenido, **cuando** pido `/readyz`, **entonces** recibo 503 con `error.code = "service_unavailable"` y `requestId`. |
| CA-04 | **Dado** MongoDB activo, **cuando** pido `/readyz`, **entonces** recibo 200. |
| CA-05 | **Dado** una ruta inexistente, **cuando** la solicito, **entonces** recibo 404 con el envoltorio de error y `X-Request-Id`. |
| CA-06 | **Dado** un handler que provoca *panic* (prueba), **cuando** se invoca, **entonces** recibo 500 `internal_error` y el servidor sigue atendiendo. |
| CA-07 | **Dado** `X-Request-Id: abc` en la petición, **cuando** recibo la respuesta, **entonces** trae el mismo valor. |
| CA-08 | **Dado** el servicio en marcha, **cuando** envío `SIGTERM`, **entonces** finaliza con código 0 tras cerrar conexiones. |
| CA-09 | **Dado** un repositorio de prueba que declara un índice, **cuando** arranca el registro dos veces, **entonces** el índice existe una sola vez y no hay error. |
| CA-10 | **Dado** el repositorio limpio, **cuando** ejecuto `make fmt lint test build`, **entonces** todo termina sin errores. |

## 6. Fuera de alcance

- Autenticación, autorización, usuarios y perfiles.
- Cualquier entidad o endpoint de negocio (objetos, solicitudes, reservas, chat, QR, incidencias, calificaciones, notificaciones).
- Despliegue a producción (spec 013) y pipeline CI/CD y repositorio (se integrarán aparte).
- Carga de archivos o imágenes.
- Observabilidad avanzada (métricas, trazas distribuidas).

## 7. Preguntas abiertas

- [NEEDS CLARIFICATION: ¿Se exige un nombre de módulo Go específico (p. ej. `github.com/<usuario>/vaiven-backend`)? Se propone `github.com/vaiven/backend` como valor provisional.]

## 8. Trazabilidad con la constitución

Art. I (esta spec), II (RF-02/03/13), III (RNF-07), IV (RF-06/08/09/15), V (alcance), VI (RF-01/12, RNF-05), VII (RNF-01/02), VIII (RNF-06), IX (RF-04/05/07/11/14), X (proceso).

## 7. Servicios, Redis, HTTPS y despliegue en Azure (Art. XIII y XIV)
| ID | Requisito |
|---|---|
| RF-S1 | La configuración se divide en subestructuras cargadas desde archivos separados (`Cloudinary` y las que agreguen specs posteriores); `Load()` las reúne y reporta **todos** los errores juntos. |
| RF-S2 | Variables nuevas: `CLOUDINARY_CLOUD_NAME`, `CLOUDINARY_API_KEY`, `CLOUDINARY_API_SECRET` (obligatorias salvo `APP_ENV=test`), `CLOUDINARY_BASE_FOLDER` (por defecto `vaiven/<APP_ENV>`); `APP_ENV=staging` permitido; `CORS_ALLOWED_ORIGINS` soportado. |
| RF-S3 | `go run ./cmd/admin check-services` verifica MongoDB (ping), Redis (`PING`) y Cloudinary (`GET /v1_1/<cloud>/ping`) e imprime `OK`/`FALLA` por servicio sin imprimir secretos; sale con código ≠ 0 si alguno falla. |
| RF-S4 | Esta spec termina **desplegada en la VM de Azure** (Art. XIII): `/healthz`, `/readyz` y `/api/v1/meta` responden por **HTTPS** en `https://<host>:8443`, conectados a MongoDB y Redis. |
| RF-S5 | **Redis:** variable `REDIS_URL`; adaptador `internal/platform/redis` (único que importa `go-redis`) con los puertos `ratelimit.Limiter` y `cache.Cache` y una versión **en memoria** de cada uno para pruebas y para el modo degradado. Prefijo de claves `vaiven:<APP_ENV>:`. |
| RF-S6 | **Sin estado y degradación:** un middleware de límite de tasa usa `ratelimit.Limiter` (por IP en rutas públicas y por usuario en rutas autenticadas, con límites configurables por política); si Redis falla, cae a un limitador local y lo registra una vez, sin devolver errores al cliente. |
| RF-S7 | **HTTPS forzoso (Art. XIV.4):** si `APP_ENV` es `staging` o `production`, la API exige `TLS_CERT_FILE` y `TLS_KEY_FILE`, sirve **solo HTTPS** en el puerto configurado y **recarga el certificado** cuando cambia el archivo, sin reiniciarse; en `development` y `test` puede servir HTTP. No hay puerto HTTP en staging ni en producción. |
| RF-S8 | **Artefactos de despliegue** (`deploy/`): `docker-compose.yml` con `api`, `mongo` (`rs0`) y `redis`; solo la API publica puerto (8443 en staging, 443 en producción) y monta de solo lectura el certificado de Let's Encrypt; `scripts/deploy.sh <entorno> [--rollback]` construye la imagen para la arquitectura de la VM (`amd64` o `arm64`), la envía a la VM, la reinicia y espera su `/healthz` por HTTPS. |

| ID | Criterio de aceptación |
|---|---|
| CA-S1 | Dado un `.env` válido, cuando ejecuto `check-services`, entonces veo `OK` en ambos servicios y código 0. |
| CA-S2 | Dado un `CLOUDINARY_API_SECRET` incorrecto, cuando ejecuto `check-services`, entonces Cloudinary muestra `FALLA` sin revelar el valor y el código es ≠ 0. |
| CA-S3 | Dado el staging desplegado, cuando consulto `https://<host>:8443/readyz` sin `-k`, entonces el certificado es válido y recibo 200. |
| CA-S4 | Dado el staging, cuando intento conectar por HTTP al puerto de la API, entonces no hay respuesta HTTP en claro (solo TLS). Y dado un certificado renovado en disco, cuando pasa el tiempo de recarga, entonces la API sirve el nuevo sin reiniciar. |
| CA-S5 | Dado Redis detenido, cuando pido un endpoint público, entonces responde con normalidad y `/readyz` informa `redis: "degradado"` con 200. |
| CA-S6 | Dado un límite de N peticiones, cuando se reciben más de N de la misma IP, entonces se aplica el límite en Redis (429 en el formato de error del Art. IV). |
