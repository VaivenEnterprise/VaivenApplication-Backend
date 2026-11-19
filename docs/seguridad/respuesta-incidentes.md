# Alertas de seguridad y respuesta a incidentes (spec 018, C-10)

- **Fecha:** 2026-11-18 · **Evaluador:** `scripts/security-alerts.mjs` · **Pruebas de activación:** `tests/security-controls.test.mjs` (en la CI, job Secrets)
- **Destinatarios:** **PENDIENTE DE OPERACIÓN.** El equipo no ha asignado quién recibe cada alerta ni por qué canal. Hasta asignarlos, las alertas de la cadena de suministro (A5 y A6) llegan como fallo de workflow a quien vigila el repositorio, y las de registros (A1 a A4) se revisan a mano con el evaluador. **Sin destinatarios asignados, C-10 no cuenta como cumplido.**
- **Sin servicios de pago:** no se usa Azure Monitor ni otro servicio con costo (Art. XIII.1).

## Las seis alertas

| ID | Qué detecta | Fuente | Umbral | Severidad | Estado de la fuente |
|---|---|---|---|---|---|
| A1 | Reutilización de un refresh token | Evento `auth.refresh_reuse` | **La primera** confirmada ya alerta; 3 o más en la ventana escalan a crítica | Alta → crítica | **Pendiente de BE-A**: el evento vive en `internal/auth` (C-02). Hoy la reutilización ya revoca la familia (probado en el balanceador local) pero no emite evento |
| A2 | Fallos de login anómalos | `POST /api/v1/auth/login` con 401 en los registros de petición (o el evento `auth.login_failed` cuando exista) | 20 en la ventana (15 min) | Media | **Disponible** en los registros actuales |
| A3 | Aumento de 5xx | Registros de petición (`status`) | ≥ 5 % con al menos 50 peticiones en la ventana | Alta | **Disponible** |
| A4 | Reinicios anómalos del contenedor | `docker inspect -f '{{.RestartCount}}'` de la API en la VM | 2 o más desde la revisión anterior | Alta | **Disponible** (lectura en la VM con `cloudshell-run.sh`) |
| A5 | Vulnerabilidad crítica | Trivy en la CI y en el despliegue (por digest) | Cualquier crítica o alta con corrección | Crítica | **Disponible**: el workflow falla y no se despliega |
| A6 | Falla de la verificación de firma | Job de despliegue (`cosign verify`) | Cualquier falla | Crítica | **Disponible**: el workflow emite `supply_chain.signature_failed` y no se despliega |

## Cómo se revisan

En Azure Cloud Shell, sobre los registros recientes de la API (no imprime líneas, solo conteos):

```bash
docker logs --since 15m vaiven-staging-api-1 2>&1 > /tmp/api.log   # dentro de la VM, por cloudshell-run.sh
node scripts/security-alerts.mjs /tmp/api.log --restarts "$(docker inspect -f '{{.RestartCount}}' vaiven-staging-api-1)"
```

El código de salida es 2 si se dispara alguna alerta.

## Respuesta

| Alerta | Primeros pasos (≤ 15 min) | Después |
|---|---|---|
| A1 Reutilización de refresh | Confirmar que la familia quedó revocada. Si hay varias o se escaló, revocar todas las sesiones del usuario afectado (baja de sesiones por usuario) | Avisar al usuario; revisar si el token salió de un dispositivo comprometido |
| A2 Fallos de login | Verificar que el límite por cuenta y por IP está activo (429). Identificar si es contra una cuenta o contra muchas | Si persiste, bajar `LOGIN_ATTEMPTS_PER_ACCOUNT`; nunca registrar contraseñas ni correos completos |
| A3 5xx | `/readyz` (MongoDB y Redis). Revisar registros por `requestId` | Si empezó con un despliegue: reversión (`deploy-staging.yml` → rollback) |
| A4 Reinicios | Memoria y disco de la VM; registros previos al reinicio | Ajustar límites o revertir la versión |
| A5 Vulnerabilidad crítica | No se despliega (lo bloquea la CI). Actualizar la imagen base o la dependencia | Una excepción solo con CVE concreto, dueño y vencimiento (`.trivyignore.yaml`) |
| A6 Firma fallida | No se despliega. Comprobar que la imagen viene de este workflow y de `develop` | Si alguien publicó una imagen ajena en GHCR: rotar tokens del repositorio y revisar el registro |

## Pruebas de activación

`node --test tests/security-controls.test.mjs` comprueba que:
- cada alerta se dispara con su condición;
- no se dispara con tráfico normal;
- se respeta el umbral (uno menos no alerta).

Corre en cada PR.
