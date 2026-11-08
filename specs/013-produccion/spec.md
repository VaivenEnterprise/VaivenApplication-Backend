# Spec 013 — Entorno de producción (backend)

- **Dueño:** BE-B (con BE-A en revisión) · **Prioridad:** P0 · **Ventana:** fase 1: 6–8 nov · fase 2: 19–22 nov · **Depende de:** 001, 002 (staging), 012
- **Constitución:** v2.0.0 · **Estado:** Borrador

## 1. Objetivo y por qué
Tener un **entorno de producción separado de staging**, gratuito (en Azure) y operable, desde el **8 de noviembre** (el primer AAB sube el 9 y los testers de Google Play usarán la app contra producción), y dejarlo listo para el público el 23 de noviembre (solicitud de producción): secretos propios, base de datos propia, respaldo manual, monitoreo de disponibilidad y guía de operación.

## 2. Requisitos
| ID | Requisito |
|---|---|
| RF-01 | Entorno `production` en la **misma VM de Azure** que staging: proyecto de Compose `vaiven-production` (API en el puerto 443, MongoDB y Redis propios), desplegado con `scripts/deploy.sh production` por `devops-mcp`; `/healthz` y `/readyz` responden 200 por HTTPS. |
| RF-01b | **HTTPS forzoso** en producción (Android bloquea HTTP en las compilaciones publicadas): la API sirve solo HTTPS con el certificado de Let's Encrypt de la VM (Art. XIV.4), con renovación automática comprobada. |
| RF-02 | **Base de datos propia:** `vaiven_prod` en el MongoDB de producción (volumen y usuario propios, permisos solo sobre esa base, contraseña fuerte distinta de staging); Mongo y Redis sin puertos publicados; los índices se crean con el comando `indexes` y `EnsureIndexes` al arranque. |
| RF-03 | **Cloudinary**: carpeta `vaiven/production/` (variable `CLOUDINARY_BASE_FOLDER`), separada de staging; el *API secret* solo en el backend. |
| RF-04 | **Secretos de producción distintos** de staging (`JWT_SECRET`, credenciales de Mongo, Redis y Cloudinary); guardados solo en el `.env` de producción de la VM (600); ninguno en git; procedimiento de rotación documentado. |
| RF-05 | **Sin datos de demostración** en producción: no se ejecuta `seed`. Solo existen las cuentas necesarias: un administrador (`create-admin`) y la **cuenta de revisión de Google Play** (creada a mano, datos ficticios). |
| RF-06 | **Monitor gratuito** (servicio de ping externo) que consulte `/healthz` cada 5 min y avise si la VM o la API dejan de responder; reinicio automático de contenedores (`restart: unless-stopped`) y arranque de Docker con la VM. |
| RF-07 | **Respaldo manual** con `mongodump` ejecutado en la VM y copiado **fuera de la VM**, cifrado, con restauración probada en staging; se ejecuta antes de cada despliegue importante y semanalmente. |
| RF-08 | **Prueba de humo** `go run ./cmd/admin smoke --base-url …`: registra una cuenta temporal, inicia sesión, publica y borra un objeto, y limpia lo creado; no deja datos. |
| RF-09 | **Guía de operación** `docs/produccion.md`: despliegue, reversión (`deploy.sh --rollback`), lectura de logs, memoria y disco de la VM, cómo reiniciar un contenedor o la VM, rotación de secretos, qué hacer si el crédito de Azure vence o la VM se detiene, contacto. |
| RF-10 | **Registros sin datos personales** (correos, contraseñas, tokens, textos de mensajes); revisión explícita antes de abrir al público. |
| RF-11 | Límite de tasa en Redis activo en producción (spec 012-T003). |
| RF-12 | **Capacidad:** staging y producción caben en la VM con límites de memoria; si no caben, se documenta y se consulta antes de cambiar de talla. |
| RNF-01 | Producción responde p95 < 800 ms. |
| RNF-02 | Cero costo mensual (capa gratuita de Azure y Cloudinary); revisión de *Cost Management*. |

## 3. Criterios de aceptación
| ID | Dado / Cuando / Entonces |
|---|---|
| CA-01 | Dado `vaiven-production`, entonces `https://<host>/readyz` responde 200 y la app móvil de producción abre sesión contra él. |
| CA-02 | Dado staging y producción, entonces ninguna variable secreta es igual y las bases y volúmenes son distintos. |
| CA-03 | Dado `smoke`, entonces termina con código 0 y sin datos residuales. |
| CA-04 | Dado un respaldo, entonces se restaura en staging y las colecciones coinciden. |
| CA-05 | Dada la VM reiniciada, entonces ambos entornos vuelven solos y responden en pocos minutos. |
| CA-06 | Dado el repositorio y los registros, entonces no hay secretos ni datos personales. |
| CA-07 | Dado el certificado, entonces `https://<host>/readyz` valida sin `-k` y `certbot renew --dry-run` termina bien. |

## 4. Fuera de alcance
Alta disponibilidad real (una sola VM), escalado automático, respaldos automáticos continuos, dominio propio, CDN propio, alertas avanzadas (solo el monitor básico).

## 5. Preguntas abiertas
- [NEEDS CLARIFICATION: ¿Cabe producción junto a staging en la VM gratuita (~1 GB)? Provisional: staging se reduce o se detiene cuando producción esté en uso; si aun así no cabe, se consulta antes de cambiar de talla.]
- [NEEDS CLARIFICATION: ¿Qué monitor externo gratuito? Provisional: uno de ping HTTP; se anota cuál en `docs/produccion.md`.]
- [NEEDS CLARIFICATION: ¿Qué hacer con el disco de la VM si se llena? Provisional: avisar y limpiar evidencia antigua manualmente.]
