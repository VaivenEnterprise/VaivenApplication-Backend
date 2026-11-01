# Plan 003 — Cuenta y autenticación (backend)

- **Spec:** `spec.md` · **Dueño:** BE-A

## 1. Estructura
```
internal/auth/
  domain/{user.go, errors.go}
  usecase/{register.go, login.go, refresh.go, logout.go, profile.go, deactivate.go, ports.go}
  adapter/http/{handler.go, dto.go, middleware.go, routes.go}
  adapter/mongo/{user_repo.go, refresh_repo.go}
internal/platform/{security/{password.go, jwt.go, tokens.go}, ratelimit/limiter.go}
internal/notify/{notifier.go}        # puerto + implementación vacía
internal/moderation/{checker.go}     # puerto + implementación vacía
internal/config/auth.go
cmd/api/modules.go · cmd/admin/create_admin.go
api/paths/auth.yaml · api/paths/users.yaml
```

## 2. Contrato (resumen; el detalle va en `api/paths/auth.yaml`)
| Método | Ruta | Auth | Cuerpo → Respuesta |
|---|---|---|---|
| POST | `/api/v1/auth/register` | — | `{email,password,displayName,acceptTerms,isAdult}` → 201 `{user,tokens}` |
| POST | `/api/v1/auth/login` | — | `{email,password}` → 200 `{user,tokens}` |
| POST | `/api/v1/auth/refresh` | — | `{refreshToken}` → 200 `{tokens}` |
| POST | `/api/v1/auth/logout` | — | `{refreshToken}` → 204 |
| GET | `/api/v1/me` | Sí | → 200 `User` |
| PATCH | `/api/v1/me` | Sí | `{displayName?,phone?}` → 200 `User` |
| DELETE | `/api/v1/me` | Sí | `{password}` → 204 |
| GET | `/api/v1/users/{id}` | Sí | → 200 `PublicUser` |

```
Tokens     = { accessToken, refreshToken, expiresIn }        # expiresIn en segundos
User       = { id, email, displayName, phone|null, role, ratingAverage|null, ratingCount, createdAt }
PublicUser = { id, displayName, ratingAverage|null, ratingCount, memberSince }
```

## 3. Datos (MongoDB)
**`users`**: `_id`, `email` (único, minúsculas), `passwordHash`, `displayName`, `phone`, `role`, `status` (`active`|`deactivated`), `acceptedTermsAt`, `ratingSum` (int), `ratingCount` (int), `suspendedUntil`, `createdAt`, `updatedAt`, `version`, `deactivatedAt`.
Índices: único `{email:1}`.
**`refresh_tokens`**: `_id`, `userId`, `familyId`, `tokenHash` (SHA-256), `expiresAt`, `usedAt`, `revokedAt`, `createdAt`.
Índices: único `{tokenHash:1}`; `{familyId:1}`; TTL `{expiresAt:1}` con `expireAfterSeconds:0`.

## 4. Decisiones
| # | Decisión | Art. |
|---|---|---|
| P-01 | `argon2id` (`x/crypto/argon2`): m=64 MiB, t=3, p=2, sal 16 B; formato PHC en cadena | VI |
| P-02 | JWT HS256 (`golang-jwt/jwt/v5`), claims `sub`, `role`, `exp`, `iat`, `jti`; secreto `JWT_ACCESS_SECRET` (≥ 32 caracteres) | VI |
| P-03 | Refresco: 32 bytes aleatorios en base64url; se guarda solo su hash | VI |
| P-04 | Rotación con detección de reutilización: si llega un token con `usedAt` → revocar la familia | VI |
| P-05 | Límite de tasa en memoria por IP (ventana deslizante); válido para una sola instancia (staging); se documenta | VIII |
| P-06 | `Module` interface y `modules.go` con una línea por módulo | XII |
| P-07 | `auth.UserID(ctx)` y `auth.Role(ctx)` son el único modo de leer la identidad | III |
| P-08 | Baja: `status=deactivated`, `deactivatedAt`, se revocan todos los refresh; el correo se conserva (impide reusarlo) | VI |
| P-09 | Validación con `go-playground/validator/v10` en DTOs; mensajes en español | VI |
| P-10 | Comprobación de reservas activas mediante puerto `ActiveBookingChecker` (vacío = false hasta 007) | XII |

## 5. Variables nuevas
`JWT_ACCESS_SECRET` (obligatoria, ≥ 32), `JWT_ACCESS_TTL` (15m), `REFRESH_TTL` (720h), `RATE_LIMIT_AUTH_PER_MIN` (10).

## 6. Dependencias nuevas (aprobadas por Art. VIII.4)
`github.com/go-playground/validator/v10`, `golang.org/x/crypto`, `github.com/golang-jwt/jwt/v5`.

## 7. Pruebas
Unitarias de cada caso de uso con repos falsos; `password` (hash/verify, formato); `jwt` (firma, expiración, algoritmo incorrecto rechazado); rotación y reutilización; límite de tasa. Integración (testcontainers): registro→login→refresh→logout, índice único de correo, TTL presente.

## 8. Verificación de constitución
| Art. | Cumple |
|---|---|
| III | Capas por módulo; el dominio no importa `jwt` ni `mongo` |
| IV | Errores con el envoltorio; `openapi` primero |
| VI | argon2id, sin secretos en logs, límite de intentos |
| VII | ≥ 80 % en domain/usecase |
| XII | `modules.go`, puertos vacíos, contrato en `api/paths/` |
