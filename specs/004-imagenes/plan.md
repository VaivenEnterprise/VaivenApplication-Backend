# Plan 004 — Imágenes con Cloudinary (backend)

- **Spec:** `spec.md` · **Dueño:** BE-B

## 1. Estructura
```
internal/images/
  domain/image.go                      # Image, Purpose, Delivery, errores
  usecase/{signature.go, resolve.go, urls.go, ports.go}
  adapter/cloudinary/{signer.go, admin_client.go, urlbuilder.go}
  adapter/http/{handler.go, dto.go, routes.go}
  service.go                           # images.Service (fachada usada por otras specs)
api/paths/uploads.yaml
```

## 2. Contrato
`POST /api/v1/uploads/signature` (Bearer)
```json
// petición
{ "purpose": "item_photo" }
// respuesta 200
{ "uploadUrl": "https://api.cloudinary.com/v1_1/<cloud>/image/upload",
  "apiKey": "123…", "timestamp": 1761700000, "signature": "ab12…",
  "publicId": "vaiven/staging/item_photo/<userId>/<uuid>",
  "type": "upload", "allowedFormats": "jpg,jpeg,png,webp" }
```
El cliente envía a `uploadUrl` un `multipart/form-data` con: `file`, `api_key`, `timestamp`, `signature`, `public_id`, `type`, `allowed_formats`.

Referencia que reciben otras specs: `{ "publicId": "…" }` → se guarda y se devuelve como
`Image` + `{ url, thumbUrl }` calculados al leer.

## 3. Firma (Cloudinary)
Cadena a firmar: parámetros ordenados alfabéticamente `allowed_formats=…&public_id=…&timestamp=…&type=…` + `API_SECRET`; `signature = hex(SHA-1(cadena))`. Se excluyen `file`, `api_key`, `resource_type`, `cloud_name`.

## 4. Decisiones
| # | Decisión | Art. |
|---|---|---|
| P-01 | Sin SDK; `crypto/sha1`, `net/http` | VIII |
| P-02 | `Resolve` consulta `GET /v1_1/<cloud>/resources/image/<type>/<publicId>` (autenticación básica) para obtener `width`, `height`, `format`, `bytes` | VI |
| P-03 | Descarga privada con vencimiento: `https://api.cloudinary.com/v1_1/<cloud>/image/download?api_key&expires_at&public_id&timestamp&type=authenticated&signature` (firmada igual que arriba, `expires_at = now+10 min`) | VI.7 |
| P-04 | `images.Service` es la **única** puerta: `Signature(ctx,userID,purpose)`, `Resolve(ctx,userID,purpose,refs)`, `URLs(img)` | XII |
| P-05 | Límite de firmas con `ratelimit` de 003 (clave = userId) | VI |
| P-06 | Reloj inyectado (`platform/clock`) para pruebas de vencimiento | VII |
| P-07 | Tipo de entrega por propósito: `item_photo` → `public`; `evidence` → `authenticated` | II.4 |

## 5. Variables
Las de 002 (`CLOUDINARY_*`) y `IMAGE_MAX_BYTES` (5242880), `IMAGE_SIGNATURE_PER_HOUR` (30).

## 6. Pruebas
Firma contra un vector conocido (cadena fija + secreto de prueba); prefijo de carpeta; `Resolve` con servidor Cloudinary simulado (200, 404, 5xx, formato inválido, tamaño excedido, carpeta ajena); URLs públicas y privadas (vencimiento con reloj falso); límite de tasa. Una prueba manual documentada con una cuenta real.

## 7. Verificación de constitución
| Art. | Cumple |
|---|---|
| II.4 | Binarios solo en Cloudinary; Mongo guarda `Image` |
| III | `domain`/`usecase` sin `net/http`; Cloudinary en `adapter` |
| VI | Secreto nunca sale; propiedad de carpeta; URLs con vencimiento |
| VIII | Sin dependencias nuevas |
| XII | Fachada `images.Service` para otras specs |
