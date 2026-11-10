# Remediación de seguridad — noviembre 2026 (SEC-001 a SEC-007)

Este documento no contiene valores de secretos. Todo valor aparece como `<marcador>`.

## 1. Qué pasó

En el commit `7ad4f00` (spec 011, rama `spec/SPEC-11`, fusionado en `develop` con el PR #61) se versionó `Claude outputs/_tmp/repo11.tgz`. Ese archivo comprimido trae un `.env` local con:

| Línea del `.env` | Variable | Estado |
|---|---|---|
| 8 | `CLOUDINARY_URL` | **Credencial real**: su huella coincide con la cuenta en uso |
| 39 | `CLOUDINARY_API_KEY` | Real (la misma de la URL) |
| 40 | `CLOUDINARY_API_SECRET` | Real (el mismo de la URL) |
| 54 | `JWT_ACCESS_SECRET` | 64 caracteres; **no se pudo comparar** con el de staging |

`MONGO_URI` apunta a `localhost` sin credenciales, y las contraseñas de Mongo y Redis están vacías.

gitleaks no lo detectó en CI por dos motivos: no abre archivos comprimidos versionados y sus reglas por defecto no reconocen el formato `cloudinary://`.

El blob es `36f7ffe68452c5d6b4bbc0d52a523abbe222cf19`. Está en el historial de `develop` y de `spec/SPEC-11`; `main` no está afectada.

## 2. Estado por hallazgo

| ID | Hallazgo | Corrección en `fix/SPEC-11-seguridad` | Pendiente |
|---|---|---|---|
| SEC-001 | Credenciales reales de Cloudinary en el historial | El archivo se retiró del árbol | **P-1** (rotar y revocar) y **P-3** (limpiar el historial). Mientras no se haga P-1, la credencial sigue comprometida. |
| SEC-002 | `JWT_ACCESS_SECRET` en el historial | Se probó el comportamiento real de la rotación (sección 4) | **P-2** (rotar en staging) y **P-3** |
| SEC-003 | Artefactos `Claude outputs/` versionados | Los dos tgz y el ps1 salieron del árbol | P-3 para el historial |
| SEC-004 | CI no revisa archivos comprimidos ni el formato `cloudinary://` | `scripts/scan-archives.py` con pruebas, su paso en CI y la regla `.gitleaks.toml` | — |
| SEC-005 | `.gitignore` no ignora archivos comprimidos ni volcados | Se agregaron `*.tgz`, `*.zip`, `*.tar*`, `*.gz`, `*.sql`, `*.dump`, `_tmp/` y `Claude outputs/` | — |
| SEC-006 | El script de prueba tenía una contraseña fija y dejaba cuentas sin borrar | Ahora es `scripts/chat-e2e.ps1`: credenciales por entorno, guarda de entorno y limpieza en `finally` | Correrlo de punta a punta contra una API local (no se hizo) |
| SEC-007 | El lint no compilaba las pruebas de integración | Se agregó `build-tags: [integration]` en `.golangci.yml` y se corrigieron dos `errcheck` | — |

## 3. P-1: rotación de Cloudinary (operador)

Este procedimiento lo ejecuta solo una persona con acceso al panel de Cloudinary y a la VM. Claude Code no lo ejecuta.

1. En el panel de Cloudinary, ve a Settings → API Keys y genera una llave nueva. No borres todavía la vieja.
2. En la VM de staging, edita `/opt/vaiven/staging/.env` y reemplaza `CLOUDINARY_API_KEY` y `CLOUDINARY_API_SECRET` por los valores nuevos.
   - Hazlo con un editor en la VM; no pegues los valores en el chat ni en la terminal compartida.
   - Revisa que el archivo conserve sus permisos `600`.
3. Reinicia solo la API: `docker compose ... up -d api`, con el mismo comando de siempre en `deploy/`.
   - Verifica `/readyz`. Ojo: `/readyz` **no** consulta Cloudinary (solo MongoDB y Redis).
   - Valida la llave desde tu equipo, con las variables nuevas cargadas en tu entorno (no en un archivo del repositorio): `go run ./cmd/admin check-services` debe responder `OK` en Cloudinary. La imagen de la API no incluye este binario.
4. Prueba una subida real. Pide una firma con `POST /api/v1/uploads/signature`, sube una foto de prueba y bórrala después.
5. Actualiza `CLOUDINARY_URL` en tu entorno local (`az`, MCP) y en cualquier otro equipo que la tenga.
6. **Revoca la llave filtrada** en el panel. Antes, confirma con `check-services` y con la subida del paso 4 que staging ya usa la nueva.
7. Revisa en el panel el uso y la actividad (Reports → Usage y los registros de acceso) desde el 9 de noviembre: subidas, borrados o transformaciones que no reconozcas.
8. Anota en `memoria.md` la fecha y el resultado, sin valores.

## 4. P-2: rotación de `JWT_ACCESS_SECRET` (operador)

### Comportamiento real

Se probó con `TestIntegrationJWTRotation` en `internal/auth/adapter/http/rotation_integration_test.go`, sobre MongoDB real, con argon2id y JWT reales. La prueba simula reiniciar la API con otro secreto sobre la misma base.

| Qué | Resultado probado |
|---|---|
| Token de acceso firmado con el secreto anterior, aún vigente | **401 de inmediato**: no hay que esperar los 15 minutos de vida |
| Refresh token emitido antes de rotar | **Sigue valiendo**. Son opacos, se guardan como SHA-256 y no dependen del secreto. Al canjearlo se emite un acceso firmado con el secreto nuevo |
| Réplica que sigue con el secreto anterior | Rechaza los tokens nuevos. Todas las réplicas deben reiniciarse con el mismo secreto (hoy hay una sola) |
| Login después de rotar | Funciona |
| Reutilizar un refresh token ya canjeado | Sigue revocando la familia, igual que antes de rotar (CA-03) |

**Conclusión:** la rotación invalida al instante los tokens de acceso que alguien hubiera falsificado con el secreto filtrado. Además, **no cierra la sesión** de los usuarios legítimos: la app renueva su sesión de forma transparente con su refresh token.

**El secreto filtrado no permite fabricar refresh tokens.** Por eso, revocar todas las sesiones es opcional, no un requisito de esta rotación.

### Pasos

1. En la VM de staging, genera un secreto nuevo **dentro de la VM**, para que no pase por ningún chat: `openssl rand -hex 32`.
2. Reemplaza `JWT_ACCESS_SECRET` en `/opt/vaiven/staging/.env`; debe tener al menos 32 caracteres.
3. Reinicia solo la API y verifica `/readyz`.
4. Verifica con una cuenta de prueba: un `/me` con un token viejo responde 401, y `/auth/refresh` seguido de `/me` responde 200.
5. **Opcional:** si quieres forzar a todos a volver a iniciar sesión, márcalos como revocados en `mongosh` sobre la base de staging:
   `db.refresh_tokens.updateMany({ revokedAt: null }, { $set: { revokedAt: new Date() } })`
   No borra datos; es lo mismo que hace `RevokeAllForUser`, pero para todas las cuentas.
6. Anota en `memoria.md` la fecha y el resultado, sin valores.

## 5. P-3: limpiar el historial con `git filter-repo` (operador, coordinado)

> **Reescribe el historial y exige un push forzado.** Aquí solo se describe el procedimiento: nadie lo ejecuta sin acuerdo del equipo. **Rotar (P-1 y P-2) es lo que protege**; limpiar el historial solo reduce la exposición futura, porque cualquier clon o fork hecho desde el 9 de noviembre ya tiene el archivo.

1. Haz P-1 y P-2 antes.
2. Avisa a las 4 personas del equipo:
   - fusionen o respalden su trabajo pendiente;
   - nadie hace push mientras dure la limpieza.
3. Trabaja en un clon espejo nuevo, nunca en tu copia de trabajo:
   ```bash
   git clone --mirror https://github.com/VaivenEnterprise/VaivenApplication-Backend.git vaiven-mirror.git
   cd vaiven-mirror.git
   git filter-repo --invert-paths --path "Claude outputs/" --force
   ```
4. Verifica que ya no aparezca:
   - `git rev-list --all --objects | grep -E 'Claude outputs|36f7ffe6'` no debe devolver nada;
   - `python3 scripts/scan-archives.py --history` debe terminar con 0 hallazgos (córrelo desde un clon normal del resultado).
5. Haz el push forzado de las ramas afectadas (`develop`, `spec/SPEC-11` y cualquier otra que contenga `7ad4f00`). Si están protegidas, el admin debe permitirlo temporalmente.
6. Refs de PR: GitHub conserva `refs/pull/*` (por ejemplo, el PR #61) y el push no puede borrarlas. Pide a GitHub Support que purgue las vistas en caché y las refs de PR del blob `36f7ffe6…`.
7. Cada persona vuelve a clonar. No vale `git pull` sobre la copia vieja, porque reintroduciría el commit.
8. Revisa si hay forks (Insights → Forks); si existen, pide a sus dueños que los borren o los resincronicen.
9. Borra la línea del blob en `.archive-scan-baseline`, para que CI vuelva a fallar si reaparece.

## 6. Cómo funciona el escáner de archivos comprimidos

`python3 scripts/scan-archives.py --history --baseline .archive-scan-baseline` corre en CI, en el job de gitleaks.

- Abre `tar`, `tgz`, `tar.bz2`, `tar.xz`, `zip` y `gz` **en memoria**. Nunca extrae al disco, así que una ruta como `../x` no puede escribir fuera de ningún directorio; aun así se reporta como `unsafe-path`.
- Es recursivo y tiene límites:
  - profundidad 4;
  - 10,000 entradas;
  - 50 MiB por entrada y 300 MiB en total.
- Falla cerrado: si supera un límite, el archivo está corrupto o una entrada de zip está cifrada, lo trata como hallazgo.
- Detecta:
  - nombres sensibles: `.env` (sin incluir `.env.example`), llaves privadas y `credentials.json`;
  - URLs `cloudinary://` y URIs con contraseña;
  - llaves privadas PEM y tokens de GitHub y AWS;
  - asignaciones `*SECRET*`, `*PASSWORD*`, `*TOKEN*` y `*API_KEY*` con un valor real; los marcadores como `<x>`, `${X}` o `changeme` no cuentan.
- **Nunca imprime el valor:** solo `regla  contenedor!ruta:línea`.
- `.archive-scan-baseline` lista los blobs ya reportados que solo esperan P-3. No se agrega un blob nuevo para que CI pase: un secreto nuevo se rota y se retira.
- Las pruebas están en `scripts/scan_archives_test.py`: 15 casos positivos y negativos con secretos ficticios generados al vuelo, que además comprueban que el valor no aparezca en la salida.
