# Spec 013 — Entorno de producción (backend)

- **Dueño:** BE-B (con BE-A en revisión) · **Prioridad:** P0 · **Ventana:** fase 1: 30 oct–1 nov · fase 2: 12–15 nov · **Depende de:** 001, 002 (staging), 012
- **Constitución:** v1.3.0 · **Estado:** Borrador

## 1. Objetivo y por qué
Tener un **entorno de producción separado de staging**, gratuito y operable, desde el **1 de noviembre** (los testers de Google Play usarán la app contra producción), y dejarlo listo para el público el 20 de noviembre: secretos propios, base de datos propia, respaldo manual, monitoreo de disponibilidad y guía de operación.

## 2. Requisitos
| ID | Requisito |
|---|---|
| RF-01 | Servicio **Render** gratuito `vaiven-prod` (Docker, mismo `Dockerfile`), creado y desplegado por MCP (`create_web_service`, `trigger_deploy`), con variables de entorno propias; `/healthz` y `/readyz` responden 200. |
| RF-02 | **Base de datos propia en Atlas**: `vaiven_prod`, usuario con permisos solo sobre esa base, contraseña fuerte distinta de staging; los índices se crean con el comando `indexes` y `EnsureIndexes` al arranque. |
| RF-03 | **Cloudinary**: carpeta `vaiven/production/` (variable `CLOUDINARY_FOLDER`), separada de staging; el *API secret* solo en el backend. |
| RF-04 | **Secretos de producción distintos** de staging (`JWT_SECRET`, credenciales de Mongo y Cloudinary); guardados en Render; ninguno en git; procedimiento de rotación documentado. |
| RF-05 | **Sin datos de demostración** en producción: no se ejecuta `seed`. Solo existen las cuentas necesarias: un administrador (`create-admin`) y la **cuenta de revisión de Google Play** (creada a mano, datos ficticios). |
| RF-06 | **Monitor gratuito** (servicio de ping externo) que consulte `/healthz` cada 5–10 min para reducir el arranque en frío de Render; el arranque en frío (~1 min) queda documentado y la app lo tolera con estados de carga y reintento. |
| RF-07 | **Respaldo manual** con `mongodump` hacia un archivo cifrado fuera del repositorio, con restauración probada en staging; se ejecuta antes de cada despliegue importante y semanalmente. |
| RF-08 | **Prueba de humo** `go run ./cmd/admin smoke --base-url …`: registra una cuenta temporal, inicia sesión, publica y borra un objeto, y limpia lo creado; no deja datos. |
| RF-09 | **Guía de operación** `docs/produccion.md`: despliegue, reversión (volver al despliegue anterior en Render), lectura de logs, qué hacer si Atlas M0 se acerca a 512 MB, límites del plan gratuito de Render, contacto, rotación de secretos. |
| RF-10 | **Registros sin datos personales** (correos, contraseñas, tokens, textos de mensajes); revisión explícita antes de abrir al público. |
| RF-11 | Límite de tasa global y por ruta activos en producción (spec 012-T003). |
| RNF-01 | Producción responde p95 < 800 ms con el servicio despierto. |
| RNF-02 | Cero costo mensual (planes gratuitos de Render, Atlas M0 y Cloudinary). |

## 3. Criterios de aceptación
| ID | Dado / Cuando / Entonces |
|---|---|
| CA-01 | Dado `vaiven-prod`, entonces `/readyz` responde 200 y la app móvil de producción abre sesión contra él. |
| CA-02 | Dado staging y producción, entonces ninguna variable secreta es igual y las bases son distintas. |
| CA-03 | Dado `smoke`, entonces termina con código 0 y sin datos residuales. |
| CA-04 | Dado un respaldo, entonces se restaura en staging y las colecciones coinciden. |
| CA-05 | Dada una hora sin tráfico, entonces el monitor mantiene el servicio activo o el primer acceso tarda ≤ ~1 min. |
| CA-06 | Dado el repositorio y los registros, entonces no hay secretos ni datos personales. |

## 4. Fuera de alcance
Alta disponibilidad, escalado automático, respaldos automáticos continuos, dominio propio, CDN propio, alertas avanzadas (solo el monitor básico).

## 5. Preguntas abiertas
- [NEEDS CLARIFICATION: ¿El monitor de ping será un servicio gratuito externo? Provisional: sí; se anota cuál en `docs/produccion.md`.]
- [NEEDS CLARIFICATION: ¿Qué hacer cuando Atlas M0 se llene? Provisional: avisar y limpiar evidencia antigua manualmente; migrar a un plan de pago queda fuera de alcance.]
