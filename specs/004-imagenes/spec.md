# Spec 004 — Imágenes con Cloudinary (backend)

- **Dueño:** BE-B · **Prioridad:** P0 · **Ventana:** 5–7 nov · **Depende de:** 001, 002, 003 (identidad)
- **Constitución:** v1.4.0 (Art. II.4, VI.7) · **Políticas:** POL-13 · **Estado:** Aprobada e implementada (2026-11-07)

## 1. Objetivo y por qué
Dar a toda la plataforma una manera segura y gratuita de recibir fotos (objetos y evidencias): la app sube directamente a Cloudinary con una **firma de un solo uso** del backend y después entrega solo una referencia. El backend valida que la imagen es de quien la envía y de qué tipo es, y sabe entregarla pública o con URL firmada de corta duración (evidencias privadas).

## 2. Historias
| # | Prio. | Historia |
|---|---|---|
| H-01 | P1 | Como persona usuaria, quiero subir fotos de mi objeto, para que otras personas vean su estado. |
| H-02 | P1 | Como persona usuaria, quiero adjuntar fotos de evidencia solo visibles para las partes de mi reserva, para proteger mi privacidad. |
| H-03 | P1 | Como **desarrollador de otra spec**, quiero un servicio que valide y expanda referencias de imagen, para no volver a implementar esto. |

## 3. Requisitos
| ID | Requisito |
|---|---|
| RF-01 | `POST /api/v1/uploads/signature` (autenticado) con `purpose` ∈ {`item_photo`, `evidence`} devuelve lo necesario para subir **una** imagen: `uploadUrl`, `apiKey`, `timestamp`, `signature`, `publicId`, `type`, `allowedFormats`, `overwrite` (`"false"`). |
| RF-02 | `item_photo` se sube con `type=upload` (público); `evidence` con `type=authenticated` (privado). |
| RF-03 | El `publicId` generado tiene la forma `<BaseFolder>/<purpose>/<userId>/<uuid>`; la firma cubre `allowed_formats` (`jpg,jpeg,png,webp`), `overwrite=false`, `public_id`, `timestamp` y `type` (con `overwrite=false` la firma no sirve para reemplazar una imagen ya verificada). |
| RF-04 | Límite de firmas: 30 por persona y por hora → 429 `rate_limited`. |
| RF-05 | Las demás specs reciben referencias como `{ "publicId": "…" }`. `images.Service.Resolve(ctx, userID, purpose, field, refs)` (`field` = nombre del campo de quien llama, para los detalles del error) valida: cantidad (1–5), prefijo de carpeta propio, que exista en Cloudinary (API de administración) y que cumpla POL-13 (formato, ≤ 5 MB); devuelve `Image` con metadatos **verificados por Cloudinary**, no por el cliente. |
| RF-06 | `Image = { publicId, width, height, format, bytes, delivery }` es lo que se guarda en MongoDB (Art. VI §6.10). |
| RF-07 | `images.Service.URLs(img)` devuelve `url` y `thumbUrl`: públicas con transformación (`c_limit,w_1200` y `c_fill,w_300,h_300`) o, para `authenticated`, **URL firmada con vencimiento de 10 min** (descarga privada de Cloudinary). |
| RF-08 | Si Cloudinary no responde → 503 `service_unavailable`; no se guarda nada parcial. |
| RNF-01 | El `API secret` nunca sale del servidor ni aparece en logs, respuestas o pruebas. |
| RNF-02 | Cobertura ≥ 80 % en `images`. |

## 4. Criterios de aceptación
| ID | Dado / Cuando / Entonces |
|---|---|
| CA-01 | Dado un usuario autenticado, cuando pide firma para `item_photo`, entonces recibe `publicId` con su `userId` y `type=upload`. |
| CA-02 | Dado un `publicId` de **otra** persona, cuando se resuelve, entonces falla con `validation_failed`. |
| CA-03 | Dado un archivo de más de 5 MB o formato no permitido ya subido, cuando se resuelve, entonces se rechaza. |
| CA-04 | Dado una imagen `authenticated`, cuando se piden sus URLs, entonces la URL caduca a los 10 min. |
| CA-05 | Dada una petición sin sesión, cuando pide firma, entonces recibe 401. |

## 5. Fuera de alcance
Borrado de imágenes huérfanas en Cloudinary, video, avatar de perfil, moderación visual automática.

## 6. Preguntas abiertas
- ~~¿SHA-1 o SHA-256?~~ **Resuelto en T002:** la cuenta acepta SHA-1 (T002) y también SHA-256, que es el que se usa desde la corrección de CodeQL (`memoria.md`).
- ~~¿Sirve la descarga privada con vencimiento?~~ **Resuelto en T002:** `image/download` firmada con `expires_at` devuelve la imagen (200) y, vencida, 401; no hace falta el plan alterno.
