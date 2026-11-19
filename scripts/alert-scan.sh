#!/usr/bin/env bash
# scripts/alert-scan.sh — cuenta, EN LA VM, las señales de las alertas A1 a A4
# (spec 018, C-10) en los registros recientes de la API de un entorno. Solo
# lee: no cambia nada. Imprime UNA línea JSON con conteos; nunca imprime líneas
# de registro, ids de usuario, correos ni tokens.
#
#   bash alert-scan.sh <staging|production> [ventana, p. ej. 35m]
#
# La lanza .github/workflows/security-alerts.yml con az vm run-command (OIDC).
set -euo pipefail

ENV_NAME=${1:?uso: $0 <staging|production> [ventana]}
WINDOW=${2:-35m}
case "$ENV_NAME" in staging|production) ;; *) echo "entorno inválido" >&2; exit 2 ;; esac
case "$WINDOW" in *[!0-9smh]*|'') echo "ventana inválida" >&2; exit 2 ;; esac

C="vaiven-$ENV_NAME-api-1"
if ! docker inspect "$C" >/dev/null 2>&1; then
  echo "ALERT_COUNTS {\"error\":\"no existe el contenedor de la API\"}"
  exit 0
fi

L=$(docker logs --since "$WINDOW" "$C" 2>&1 || true)
count() { printf '%s\n' "$L" | grep -cE -- "$1" || true; }

requests=$(count 'msg=request|"msg":"request"')
s5xx=$(count '(status=5[0-9][0-9]|"status":5[0-9][0-9])')
login_failed=$(count 'auth\.login_failed')
refresh_reuse=$(count 'auth\.refresh_reuse')
restarts=$(docker inspect -f '{{.RestartCount}}' "$C")

printf 'ALERT_COUNTS {"env":"%s","window":"%s","requests":%d,"s5xx":%d,"loginFail":%d,"reuse":%d,"restarts":%d}\n' \
  "$ENV_NAME" "$WINDOW" "$requests" "$s5xx" "$login_failed" "$refresh_reuse" "$restarts"
