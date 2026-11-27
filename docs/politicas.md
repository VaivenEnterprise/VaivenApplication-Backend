# Políticas vigentes (POL-01 a POL-17)

> Spec 012, T010 (RF-09). Valores **confirmados el 28 de octubre** por el responsable del producto (`ROADMAP.md` §8). En el backend viven en una sola constante (`internal/policy/policy.go`); en la app móvil, en `src/lib/policy.ts`. Si este documento y el código no coinciden, **manda el código**: hay que corregir este documento.
>
> Dinero siempre en **centavos de MXN** (Art. V): `1_000` = $10. Fechas en `America/Mexico_City`.

| ID | Política | Valor vigente | Dónde se aplica |
|---|---|---|---|
| POL-01 | Cobro | Tarifa por día. Fechas inclusivas: `días = (fin − inicio) + 1`; un día iniciado cuenta completo. Total = días × tarifa. Préstamo = **$0**; renta = al menos **$10/día** (`POL01MinRentPriceCents = 1_000`) y nunca más que el valor declarado | `policy.POL01*`, `items/domain`, `bookings/domain/quote.go` |
| POL-02 | Duración | De **1 a 7 días** de calendario (`POL02MinDays`, `POL02MaxDays`) | `bookings/domain`, búsqueda por fechas |
| POL-03 | Vencimiento de solicitud | **48 h** o al llegar la fecha de inicio, lo que ocurra primero (`POL03RequestExpiryHours`) | `bookings` (job `ExpireDue`) |
| POL-04 | Precio sugerido | **3 %** del valor declarado por día, mínimo **$10**, redondeado hacia arriba a múltiplos de **$5**. Valor declarado entre **$50 y $100,000** | `policy.POL04*`, `items/domain/pricing.go` |
| POL-05 | Modalidades de pago | `full_on_delivery` (100 % en la entrega) o `split_50_50` (entrega = `ceil(total/2)`, devolución = `floor(total/2)`). El préstamo usa `none` | `bookings/domain/quote.go`, `payments` |
| POL-06 | QR | Vigencia **15 min**, un solo uso, ligado a la reserva y la etapa; regenerar invalida el anterior (`POL06QRValidityMinutes`) | `handover` |
| POL-07 | Devolución tardía | Tolerancia de **2 h** tras el fin del último día. Ajuste = días de retraso (redondeo hacia arriba) × tarifa diaria; **solo se registra, no se cobra**. El préstamo no tiene ajuste (`POL07LateToleranceHours`) | `payments/domain`, job de retrasos |
| POL-08 | Reporte tras devolver | Una incidencia se puede abrir hasta **72 h** después de la devolución; como máximo **3 abiertas** por persona (`POL08IncidentWindowHours`, `POL08MaxOpenIncidents`) | `incidents` |
| POL-09 | Cancelación tardía y ausencia | Sin monto automático. Se informa antes de solicitar; la consecuencia puede ser una incidencia y una advertencia o suspensión manual | `policy.POL09*` (textos), `bookings`, `sanctions` |
| POL-10 | Calificaciones | Puntaje de **1 a 5**, comentario de hasta **300** caracteres, hasta **14 días** después de completar la reserva | `ratings` |
| POL-11 | Chat | Texto de hasta **1000** caracteres y **30 mensajes por minuto** por persona; sin borrado en esta versión | `chat` |
| POL-12 | Alta de cuenta | Declarar ser mayor de **18 años** y aceptar los términos; **sin** verificación de correo | `auth` (`acceptTerms`, `isAdult`) |
| POL-13 | Fotos | De **1 a 5** por objeto y de 1 a 5 por etapa de evidencia; hasta **5 MB** cada una (`IMAGE_MAX_BYTES`); JPEG, PNG o WebP | `images/domain`, `items`, `handover` |
| POL-14 | Categorías | `herramientas_manuales` (Herramientas manuales), `herramientas_electricas` (Herramientas eléctricas), `construccion_jardineria` (Construcción y jardinería), `movilidad` (Patines, patinetas, bicicletas y similares), `deportes` (Deportes y aire libre), `hogar` (Hogar), `otros` (Otros) | `policy.POL14Categories`, `GET /api/v1/categories` |
| POL-15 | Sanciones | Advertencia o suspensión con fecha de fin, decididas a mano por administración; no cambian el promedio de calificaciones | `sanctions`, `RequireActive` |
| POL-16 | Contenido prohibido | Sexual, armas, drogas e ilegal: filtro por lista de palabras, reportes y revisión humana. La lista de palabras es provisional y la debe revisar el equipo (`internal/moderation/blocklist.go`) | `moderation`, `reports` |
| POL-17 | Baja de cuenta | Si hay reservas activas (aceptadas o en curso), se rechaza con 409 y no cambia nada. Si no, la baja es **atómica** (una sola transacción en MongoDB) y **borra o anonimiza**: la cuenta queda solo con id, estado, fecha de baja, correo anonimizado, «Cuenta eliminada», aceptación de términos y fechas; se borran sesiones, tokens de push, avisos, uso de IA y recomendaciones y los objetos con sus fotos (Cloudinary, con reintento); el texto de sus mensajes pasa a «Mensaje eliminado» y se quita el comentario de sus calificaciones (las estrellas y los promedios ajenos no cambian); las solicitudes pendientes se cierran con aviso a la contraparte. Incidencias, denuncias y sanciones donde participa se conservan **12 meses** desde la baja y luego se borran solas (con sus evidencias). Las reservas y su historial se conservan con ids seudónimos. Los respaldos fuera de MongoDB quedan fuera de alcance. Ningún campo queda en `null` en la base | `auth` (`DELETE /api/v1/me`) y un `Eraser` por módulo (spec 021) |

## Límites técnicos relacionados (no son POL, pero afectan a la persona usuaria)

| Límite | Valor por defecto | Variable |
|---|---|---|
| Peticiones por IP | 120 por minuto (Redis, con respaldo local si Redis cae) | constante en `cmd/api/main.go` |
| Registro, inicio de sesión y renovación | 10 por minuto por IP | `RATE_LIMIT_AUTH_PER_MIN` |
| Intentos de inicio de sesión por cuenta | 20 cada 15 minutos | `LOGIN_ATTEMPTS_PER_ACCOUNT` |
| Vida del token de acceso / refresh | 15 minutos / 30 días | `JWT_ACCESS_TTL` / `REFRESH_TTL` |
| Conservación de avisos | 90 días | constante `RetentionDays` (TTL en MongoDB) |
| Cuerpo de una petición | 1 MiB | constante `maxBodyBytes` en `router.go` |

## Detalle de la baja de cuenta (POL-17, spec 021)

Decisiones del responsable del 2026-11-26 que la baja cumple:

| ID | Qué se hace |
|---|---|
| D-1 | Mensajes de chat de la cuenta eliminada: el texto pasa a «Mensaje eliminado»; la conversación conserva orden y fechas y la contraparte la sigue viendo (solo lectura) |
| D-2 | Objetos publicados: se borran por completo junto con sus fotos en Cloudinary (asíncrono y con reintento; un fallo no deja la cuenta a medias). Las reservas pasadas conservan su `itemSnapshot` (sin la foto, que ya no existe) |
| D-3 | Valoraciones: las estrellas se conservan; el comentario escrito por la cuenta eliminada se quita |
| D-4 | Incidencias, denuncias y sanciones que involucran a la persona: 12 meses desde la baja (`purgeAt`, índice TTL); si las dos partes se dan de baja, cuenta la primera; el texto no se toca antes de purgar |
| D-5 | Ningún `null` ni dato sin normalizar en MongoDB: los opcionales se omiten y los validadores `$jsonSchema` los rechazan |
| D-6 | Se conserva `acceptedTermsAt` como prueba de consentimiento (no identifica a la persona) |

Fuera de alcance: copias de seguridad de MongoDB (`docs/respaldos.md`) y de Cloudinary, que pueden conservar datos de cuentas ya eliminadas hasta que caduquen; recuperar una baja; exportar los datos antes de borrarlos.
