# Spec 013 — Entorno de producción (backend)

- **Dueño:** BE-B (con BE-A en revisión) · **Prioridad:** P0 · **Ventana:** fase 1: en cuanto se apruebe (la original, 6–8 nov, ya venció) · fase 2: 19–22 nov · **Depende de:** 001, 002 (staging) para empezar; 012 para cerrar
- **Constitución:** v2.0.0 (Art. XIII y XIV) · **Estado:** Aprobada por BE-B (2026-11-11), tras ajustarla a la constitución y a la VM real; BE-A revisa

## 1. Objetivo y por qué
Tener un **entorno de producción separado de staging**, en la **misma VM gratuita de Azure** (Art. XIII.6), operable y listo para el público antes de pedir el acceso a producción en Google Play (23 nov): proyecto de Compose propio con MongoDB, Redis, volúmenes y secretos propios; respaldo manual; monitoreo de disponibilidad y guía de operación.

## 2. Requisitos
| ID | Requisito |
|---|---|
| RF-01 | Entorno `production` en la **misma VM de Azure** que staging (Art. XIII.6): proyecto de Compose `vaiven-production` con **su propia API, su propio MongoDB y su propio Redis**, cada uno con volumen y límite de memoria propios; API en el puerto **443**; desplegado por `devops-mcp` con `scripts/deploy.sh production` desde `origin/main` (Art. XIII.4: el despliegue automático solo existe para staging); `/healthz` y `/readyz` responden 200 por HTTPS. |
| RF-01b | **HTTPS forzoso, sin proxy** (Art. XIV.4): la API sirve solo HTTPS con el certificado de Let's Encrypt del nombre DNS de la IP pública (`vaiven-staging.westus2.cloudapp.azure.com`, el mismo de staging; producción en 443 y staging en 8443) y lo recarga sola; renovación automática comprobada. |
| RF-02 | **Base de datos propia:** `vaiven_prod` en el MongoDB de producción, con volumen propio, usuario de permisos mínimos sobre esa base y contraseña fuerte distinta de staging. MongoDB y Redis sin puertos publicados. Los índices se crean al arrancar (`EnsureIndexes`) y se verifican con `admin indexes` (código 0). |
| RF-03 | **Cloudinary:** carpeta `vaiven/production/` (`CLOUDINARY_BASE_FOLDER`), separada de staging, en la misma cuenta; el *API secret* solo en el backend. **Requisito previo:** rotar la llave filtrada (P-1 de `docs/seguridad/remediacion-2026-11.md`), porque la cuenta es la misma. |
| RF-04 | **Secretos de producción nuevos y distintos** de staging (`JWT_ACCESS_SECRET`, MongoDB, Redis); guardados solo en `/opt/vaiven/production/.env` de la VM (600); ninguno en git; procedimiento de rotación escrito. |
| RF-05 | **Sin datos de demostración** en producción: `seed` se niega con `APP_ENV=production` (T005 de la 012). Solo existen un administrador (`create-admin`) y la **cuenta de revisión de Google Play** (datos ficticios; su contraseña se entrega en Play Console, nunca en el repositorio). |
| RF-06 | **Monitor externo gratuito: Better Stack** (plan gratuito, revisión de `/healthz` de producción cada 3 min y aviso por correo); reinicio automático de contenedores (`restart: unless-stopped`) y Docker habilitado al arranque. |
| RF-07 | **Respaldo manual** con `mongodump --archive --gzip` en la VM, **cifrado** y copiado fuera de la VM; restauración probada en staging; antes de cada despliegue a producción y semanalmente. |
| RF-08 | **Prueba de humo:** en producción solo de lectura (`scripts/smoke.sh`: `/healthz`, `/readyz`, `/api/v1/meta` y un 404 con el formato de error); el recorrido que escribe datos (`go run ./cmd/admin smoke`: registrar, iniciar sesión, publicar y borrar) corre en **staging**. Así producción nunca acumula cuentas anonimizadas de prueba (POL-17). |
| RF-09 | **Guía de operación** `docs/produccion.md`: despliegue, reversión (`deploy.sh production --rollback`), registros, memoria y disco de la VM, reinicio de un contenedor o de la VM, rotación de secretos, respaldo, qué hacer si el crédito de Azure vence o la VM se detiene, y contacto. |
| RF-10 | **Registros sin datos personales** (correos, contraseñas, tokens, textos de mensajes); revisión explícita de una sesión completa antes de abrir al público. |
| RF-11 | Límite de tasa en Redis activo en producción (`/readyz` con `redis: ok`); el limitador ya existe (Art. XIV.2). |
| RF-12 | **Capacidad medida, no supuesta:** con staging y producción encendidos se mide la memoria disponible de la VM y el uso de cada contenedor; los límites de memoria de producción se fijan con esa medición. Si no caben, staging se reduce o se detiene fuera de las ventanas de prueba; cambiar de talla tiene costo y se consulta antes (Art. XIII.1). |
| RF-13 | **Disco:** los registros de Docker se rotan (tamaño y número máximos por contenedor) en ambos entornos; la VM conserva solo la imagen actual y la anterior de cada entorno (Art. XIII.7); la guía incluye la revisión semanal del uso de disco. |
| RNF-01 | Producción responde con p95 < 800 ms. |
| RNF-02 | Cero costo mensual adicional: capa gratuita de Azure y Cloudinary; la IP pública sigue siendo la única excepción (Art. XIII.1). Revisión de *Cost Management*. |

## 3. Criterios de aceptación
| ID | Dado / Cuando / Entonces |
|---|---|
| CA-01 | Dado `vaiven-production`, entonces `https://vaiven-staging.westus2.cloudapp.azure.com/readyz` (puerto 443) responde 200 con `redis: ok`, `/api/v1/meta` dice `environment: production`, y la app móvil de producción inicia sesión contra él. |
| CA-02 | Dado staging y producción, entonces ningún secreto es igual, y las bases, volúmenes, contenedores de MongoDB y de Redis son distintos. |
| CA-03 | Dado `scripts/smoke.sh` contra producción, entonces todas sus comprobaciones pasan; dado `admin smoke` contra staging, termina con código 0 y limpia lo que creó. |
| CA-04 | Dado un respaldo cifrado de producción, entonces se restaura en staging y las colecciones y conteos coinciden. |
| CA-05 | Dada la VM reiniciada, entonces ambos entornos vuelven solos y responden en pocos minutos. |
| CA-06 | Dado el repositorio y los registros de una sesión completa, entonces no hay secretos ni datos personales. |
| CA-07 | Dado el certificado, entonces `https://…/readyz` valida sin `-k` en los puertos 443 y 8443, y `certbot renew --dry-run` termina bien. |
| CA-08 | Dada la medición de RF-12, entonces queda anotado el uso de memoria por contenedor y la memoria libre de la VM con los dos entornos encendidos. |

## 4. Fuera de alcance
Alta disponibilidad real (una sola VM), escalado automático, servicios administrados (Atlas, Cosmos DB, Azure Cache for Redis), nginx o cualquier proxy (Art. XIV.4), dominio propio, segunda IP pública, respaldos automáticos continuos, CDN propio, alertas avanzadas (solo el monitor básico) y un workflow de despliegue automático a producción (Art. XIII.4). Cualquiera de estos exige primero una enmienda de la constitución.

## 5. Decisiones del 2026-11-11 (antes, preguntas abiertas)
- **Capacidad (RF-12):** se mide con los dos entornos encendidos (T001). Referencia: con 1515 objetos, la API usó ~28 MiB y MongoDB ~187 MiB (`docs/rendimiento.md`); los límites de staging son topes, no consumo. Si no cabe, se reduce o se detiene staging; no se cambia de talla sin consultar.
- **Monitor (RF-06):** Better Stack, plan gratuito. Lo da de alta una persona; se anota en `docs/produccion.md`.
- **Disco (RF-13):** disco P6 de 64 GiB; las fotos y la evidencia están en Cloudinary, no en el disco. Lo que crece es MongoDB, las imágenes de Docker y los registros de los contenedores, que hoy no se rotan: se agrega la rotación y una revisión semanal.
- **Nombre DNS:** producción usa el mismo nombre de la IP pública (contiene «staging»), porque la constitución fija un solo nombre y un certificado (Art. XIV.4). Un nombre propio para producción exigiría una enmienda (dominio, proxy o segunda IP con costo). Se verificó que `vaiven.com` y `vaiven.app` ya están registrados por terceros.
- **Despliegue:** `scripts/deploy.sh production` por `devops-mcp` (Art. XIII.4). El diseño de `deploy-production.yml` de `docs/devsecops.md` queda para una futura enmienda.
