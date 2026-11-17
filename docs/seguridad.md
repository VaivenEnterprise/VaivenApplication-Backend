# Revisión de seguridad — spec 012, T008 (RF-05)

> Lista de comprobación fechada (plan P-08).
>
> - **Revisó:** Claude Code (Opus 5.5), a pedido de BE-B. T008 es de BE-A en el reparto, así que **falta su firma**.
> - **Fecha:** 2026-11-11.
> - **Rama:** `feat/SPEC-12-T008`, sobre `develop` `a56c642`.
> - Cada punto dice qué se comprobó y con qué evidencia. «Pendiente» significa que no está hecho, no que esté bien.

## 1. Lista de comprobación de RF-05

| # | Punto | Estado | Evidencia |
|---|---|---|---|
| 1 | Ninguna ruta de datos sensibles sin autenticación | ✅ | El contrato 0.11.0 declara sin sesión solo 7 operaciones: `/healthz`, `/readyz`, `/api/v1/meta` y `auth/register`, `login`, `refresh` y `logout` (`docs/api.md`). Las otras 56 exigen `Authenticate`, y las pruebas de cada módulo comprueban 401 sin token. |
| 2 | `RequireRole` en `/admin/*` | ✅ | Las 6 rutas de administración (incidencias ×3, reportes ×2, sanciones ×1) están dentro de un grupo con `m.admin`, que `cmd/api/main.go` arma con `authhttp.RequireRole(admin, userRepo.ByID)`. |
| 3 | Rol de admin comprobado contra la base (AUD-017, CA-06) | ✅ | `RequireRole` exige también que la cuenta tenga rol `admin` y esté activa en MongoDB. Si el token dice `admin` pero la cuenta es `user` → 403; si la cuenta fue dada de baja o no existe → 401; si falla la base → 5xx, nunca pasa. Las cuentas sin rol de admin no generan lecturas. Pruebas: `TestRequireRoleAndActive`, `TestRequireRoleLooksUpOnlyAdminTokens` y las de integración de auth e incidencias. |
| 4 | Ningún secreto en los registros | ✅ | El middleware `Logging` solo registra método, ruta, estado, duración e `requestId`, sin cabeceras ni cuerpos. Los secretos de configuración son del tipo `config.Secret`, que se oculta en `String`, `slog` y JSON. Los errores de Cloudinary y de MongoDB que muestran las utilidades ocultan la URI y las credenciales (`redact`). |
| 5 | CORS acotado | ✅ | `middleware.CORS` usa la lista explícita `CORS_ALLOWED_ORIGINS`: refleja solo un origen de la lista y `*` no funciona como comodín. |
| 6 | Evidencia solo con URL firmada | ✅ | Las fotos de evidencia se suben con entrega `authenticated` (privada) y se descargan con URLs firmadas que vencen a los 10 min (`downloadTTL`, `urlbuilder.go`). |
| 7 | Búsqueda de secretos en el historial | ✅ con 1 conocido | `gitleaks git --log-opts=--all` sin hallazgos. `scan-archives.py --history` revisa ahora **todos** los blobs (AUD-015): 0 hallazgos y 1 conocido (`repo11.tgz`, pendiente de P-3). |
| 8 | El escáner detecta comprimidos renombrados y formatos que no sabe abrir (AUD-015) | ✅ | Detecta por firma en el historial; `7z`, `rar` y `zstd` se reportan como `unsupported-archive`. 18 pruebas (3 nuevas). |
| 9 | Documento de remediación al día (AUD-018) | ✅ | `docs/seguridad/remediacion-2026-11.md` §2 y §6 corregidos; `.gitignore` cubre ahora también `*.tar.*`, `*.tbz2`, `*.txz`, `*.bz2`, `*.xz` y `*.zst`. |
| 10 | Cabeceras de seguridad y error de cuerpo demasiado grande en todos los módulos | ⏳ Pendiente | Es T003 (BE-A): `nosniff`, `no-store` y el mensaje unificado del límite de 1 MiB. El límite en sí ya se aplica (`limitBody`). |

## 2. Riesgos que siguen abiertos (no se cierran con código)

| ID | Riesgo | Responsable | Qué falta |
|---|---|---|---|
| P-1 | La llave de Cloudinary filtrada sigue válida | Operador | Rotarla y revocar la vieja (`docs/seguridad/remediacion-2026-11.md` §3). Es lo más urgente, y es requisito del `seed` en staging (T009). |
| P-2 | Si el `JWT_ACCESS_SECRET` filtrado es el de staging, se pueden fabricar tokens. Con AUD-017 ya **no** abren las rutas de administración, pero sí las de cualquier cuenta | Operador | Rotar el secreto en staging (§4 del documento de remediación). |
| P-3 | El comprimido filtrado sigue en el historial | Operador y equipo | Limpieza coordinada con `git filter-repo` (§5). |
| AUD-016 | Dos cuentas de prueba con contraseña conocida (del script viejo) podrían existir en staging | Operador | Buscarlas y darlas de baja. |
| — | La búsqueda reciente recorre todos los objetos activos | BE-B (spec 006) | Riesgo de rendimiento, no de seguridad (`docs/rendimiento.md` §5). |

## 3. Firmas

| Rol | Nombre | Fecha | Firma |
|---|---|---|---|
| Revisó (borrador) | Claude Code, a pedido de BE-B | 2026-11-11 | — |
| BE-A (dueña de T008) | | | |
