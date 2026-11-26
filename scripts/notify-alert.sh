#!/usr/bin/env bash
# scripts/notify-alert.sh — entrega alertas de seguridad (spec 018, C-10).
#
# Canal principal: un issue de GitHub por alerta, con la etiqueta
# security-alert, asignado a los destinatarios de ALERT_ASSIGNEES (usuarios de
# GitHub separados por coma; variable de repositorio que fija un administrador
# cuando cada persona ACEPTÓ el rol). GitHub los avisa por correo y en la app.
#
# Privacidad: el repositorio es público, así que el issue solo lleva el id de
# la alerta, la severidad, el entorno y la hora. Nunca conteos, IP, usuarios,
# tokens ni líneas de registro. El detalle (ALERT_DETAIL) solo viaja por el
# webhook privado (ALERT_WEBHOOK_URL, secreto), si existe. Con ALERT_REPOSITORY
# y ALERT_REPO_TOKEN los issues se abren en un repositorio privado de operación.
#
# Duplicados: si ya hay un issue abierto de la misma alerta y entorno, se
# comenta en él en lugar de abrir otro (ALERT_DEDUPE=false lo desactiva; las
# pruebas sintéticas abren siempre uno nuevo como evidencia).
#
# Entrada:
#   ALERT_SUMMARY   «A1-refresh-reuse [high], A3-5xx [high]» (de security-alerts.mjs)
#   ALERT_ENV       staging | production
#   ALERT_SYNTHETIC true para pruebas de entrega
#   RUN_URL         enlace a la ejecución
# Sale con 3 si no hay destinatarios: el control C-10 NO es operativo.
set -euo pipefail

: "${ALERT_SUMMARY:?falta ALERT_SUMMARY}"
repo=${ALERT_REPOSITORY:-${GITHUB_REPOSITORY:?falta GITHUB_REPOSITORY}}
if [ -n "${ALERT_REPOSITORY:-}" ] && [ -n "${ALERT_REPO_TOKEN:-}" ]; then
  export GH_TOKEN=$ALERT_REPO_TOKEN
fi
env_name=${ALERT_ENV:-staging}
synthetic=${ALERT_SYNTHETIC:-false}
dedupe=${ALERT_DEDUPE:-true}
[ "$synthetic" = true ] && dedupe=false

assignees=$(printf '%s' "${ALERT_ASSIGNEES:-}" | tr -d ' ')
if [ -z "$assignees" ]; then
  echo "::error::C-10 no operativo: ALERT_ASSIGNEES está vacío (no hay destinatarios aceptados). La alerta no se entregó a nadie."
  exit 3
fi
case "$assignees" in *[!A-Za-z0-9,-]*) echo "::error::ALERT_ASSIGNEES solo admite usuarios de GitHub separados por coma"; exit 2 ;; esac
case "$env_name" in staging|production) ;; *) echo "::error::ALERT_ENV inválido"; exit 2 ;; esac

# retry ejecuta gh hasta 3 veces con espera creciente (fallas temporales de la API).
retry() {
  local n=0 out
  until out=$("$@" 2>&1); do
    n=$((n + 1))
    if [ "$n" -ge 3 ]; then
      echo "::error::GitHub falló 3 veces: $(printf '%s' "$out" | head -c 200)" >&2
      return 1
    fi
    sleep $((n * ${RETRY_BASE_SECONDS:-5}))
  done
  printf '%s' "$out"
}

retry gh label create security-alert --repo "$repo" --color B60205 \
  --description "Alerta de seguridad (spec 018, C-10)" >/dev/null 2>&1 || true

args=()
IFS=',' read -r -a people <<<"$assignees"
for p in "${people[@]}"; do
  [ -n "$p" ] && args+=(--assignee "$p")
done

now=$(date -u +%Y-%m-%dT%H:%M:%SZ)
delivered=0
# Una entrada por alerta: «ID [severidad]».
IFS=',' read -r -a entries <<<"$ALERT_SUMMARY"
for entry in "${entries[@]}"; do
  entry=$(printf '%s' "$entry" | sed 's/^ *//; s/ *$//')
  id=${entry%% *}
  sev=$(printf '%s' "$entry" | sed -n 's/.*\[\([a-z]*\)\].*/\1/p')
  case "$id" in A[1-6]-[a-z0-9-]*) ;; *) echo "::error::entrada de alerta inválida"; exit 2 ;; esac
  case "$sev" in low|medium|high|critical) ;; *) echo "::error::severidad inválida"; exit 2 ;; esac

  if [ "$synthetic" = true ]; then
    title="[PRUEBA] Alerta sintética $id ($env_name)"
    lead="**Prueba sintética de entrega de alertas (C-10). No es un incidente real.**"
  else
    title="Alerta de seguridad $id ($env_name)"
    lead="**Alerta de seguridad (spec 018, C-10).**"
  fi
  body=$(printf '%s\n\n| Alerta | Severidad | Entorno | Detectada (UTC) |\n|---|---|---|---|\n| %s | %s | %s | %s |\n\nEl detalle solo se envía por el canal privado. Respuesta: `docs/seguridad/respuesta-incidentes.md`.\n' \
    "$lead" "$id" "$sev" "$env_name" "$now")
  [ -n "${RUN_URL:-}" ] && body=$(printf '%s\nEjecución: %s\n' "$body" "$RUN_URL")

  existing=""
  if [ "$dedupe" = true ]; then
    existing=$(retry gh issue list --repo "$repo" --state open --label security-alert \
      --search "\"$title\" in:title" --json number,title \
      --jq "map(select(.title == \"$title\")) | first | .number // empty") || existing=""
  fi
  if [ -n "$existing" ]; then
    retry gh issue comment "$existing" --repo "$repo" \
      --body "Se repitió la alerta $id ($sev) a las $now UTC.${RUN_URL:+ Ejecución: $RUN_URL}" >/dev/null
    url="#$existing (comentario: la alerta se repitió)"
  else
    url=$(retry gh issue create --repo "$repo" --title "$title" --body "$body" \
      --label security-alert "${args[@]}")
  fi
  delivered=$((delivered + 1))
  echo "Alerta $id entregada: $url" | tee -a "${GITHUB_STEP_SUMMARY:-/dev/null}"
done

if [ -n "${ALERT_WEBHOOK_URL:-}" ]; then
  payload=$(jq -cn --arg s "$ALERT_SUMMARY" --arg e "$env_name" --arg d "${ALERT_DETAIL:-}" --arg r "${RUN_URL:-}" --arg t "$synthetic" \
    '{text: ("VAIVEN " + $e + ": " + $s + (if $d != "" then " — " + $d else "" end)), content: ("VAIVEN " + $e + ": " + $s), synthetic: $t, run: $r}')
  if curl -fsS --max-time 10 --retry 2 -H 'Content-Type: application/json' -d "$payload" "$ALERT_WEBHOOK_URL" >/dev/null; then
    echo "Webhook privado notificado" | tee -a "${GITHUB_STEP_SUMMARY:-/dev/null}"
  else
    # El webhook es secundario: su falla no oculta la alerta (el issue ya existe).
    echo "::warning::El webhook de alertas no respondió; la alerta sí quedó en el issue"
  fi
fi
echo "Alertas entregadas: $delivered"
