#!/usr/bin/env bash
# scripts/set-ai-key.sh — guarda la clave del proveedor de IA en el .env de un
# entorno de la VM y, si se pide, enciende la IA (spec 014, T014 y T016). Se
# ejecuta en Azure Cloud Shell (Bash), con la sesión de az de quien opera:
#
#   bash set-ai-key.sh staging                 # solo guarda la clave (IA apagada)
#   bash set-ai-key.sh staging --enable        # guarda la clave y enciende A y B
#   bash set-ai-key.sh staging --assistant-only # enciende solo B (sin modelo con visión)
#   bash set-ai-key.sh staging --disable       # apaga A y B (la clave se conserva)
#
# Proveedor (variable PROVIDER):
#   groq       asistente (AI_*); por defecto. API compatible con OpenAI.
#   openrouter sugerencias por foto (SUGGEST_*; spec 014, T017), modelo
#              gratuito con visión. --enable/--disable tocan solo AI_ENABLED.
#   gemini     alternativa para el asistente (AI_*).
# AI_MODEL cambia el modelo.
#
# Seguridad:
#   - La clave se lee sin mostrarla y se valida con el proveedor ANTES de tocar
#     la VM: lista de modelos y una generación mínima con el modelo que usará la
#     API. Si el proveedor exige pago o el modelo no existe, se detiene
#     (Art. XIII.1: costo cero).
#   - Viaja a la VM como parámetro protegido de un Managed Run Command (Azure lo
#     guarda cifrado y no lo devuelve) y el run command se borra al terminar.
#   - En la VM se rechaza si es igual a la del otro entorno (CA-02).
#   - Nunca se imprime; el .env conserva sus permisos (600).
# El cambio se aplica a la API al recrearla: en staging, con el siguiente
# despliegue; en production, con scripts/deploy-production.sh.
set -euo pipefail

RG=${RG:-rg-vaiven}
VM=${VM:-vm-vaiven}
LOCATION=${LOCATION:-westus2}
PROVIDER=${PROVIDER:-groq}
NAME=set-ai-key

die() { echo "set-ai-key: $*" >&2; exit 1; }
[ $# -ge 1 ] && [ $# -le 2 ] || die "uso: $0 <staging|production> [--enable|--assistant-only|--disable]"
ENV_NAME=$1
MODE=${2:-key}
case "$ENV_NAME" in staging) OTHER=production ;; production) OTHER=staging ;; *) die "entorno inválido: $ENV_NAME" ;; esac
case "$MODE" in key | --enable | --assistant-only | --disable) ;; *) die "opción desconocida: $MODE" ;; esac
PREFIX=AI
case "$PROVIDER" in
    groq) AI_PROVIDER=openai BASE=https://api.groq.com/openai/v1 MODEL=${AI_MODEL:-openai/gpt-oss-120b} ;;
    openrouter) AI_PROVIDER=openai BASE=https://openrouter.ai/api/v1 MODEL=${AI_MODEL:-google/gemma-4-31b-it:free} PREFIX=SUGGEST ;;
    gemini) AI_PROVIDER=gemini BASE=https://generativelanguage.googleapis.com MODEL=${AI_MODEL:-gemini-2.5-flash} ;;
    *) die "PROVIDER debe ser groq, openrouter o gemini" ;;
esac
[ "$PREFIX" = AI ] || [ "$MODE" != --assistant-only ] || die "--assistant-only es para el proveedor del asistente (groq o gemini)"
command -v az >/dev/null || die "falta az (usa Azure Cloud Shell)"

# check CODIGO: interpreta la respuesta de la generación de prueba.
check() {
    case "$1" in
        200) echo "Generación de prueba con $MODEL: OK (sin pago)." ;;
        401) die "el proveedor rechazó la clave (401). No se cambió nada." ;;
        429) die "el proveedor respondió 429: sin cuota gratuita o límite alcanzado. No se cambió nada; no actives pagos sin decisión del equipo." ;;
        403) die "el proveedor respondió 403: sin acceso (¿pide pago?). No se cambió nada." ;;
        404) die "el modelo $MODEL no existe para esta clave (404). No se cambió nada." ;;
        *) die "la generación de prueba falló (HTTP $1). No se cambió nada." ;;
    esac
}

KEY=
if [ "$MODE" != --disable ]; then
    read -rsp "Clave de $PROVIDER para $ENV_NAME (no se muestra): " KEY
    echo
    [ ${#KEY} -ge 20 ] || die "la clave parece incompleta (${#KEY} caracteres)"
    prompt='Responde solo: ok'
    if [ "$PROVIDER" = openrouter ]; then
        code=$(curl -s -o /dev/null -w '%{http_code}' --max-time 20 -H "Authorization: Bearer $KEY" "$BASE/key")
        [ "$code" = 200 ] || die "OpenRouter rechazó la clave (HTTP $code): no se cambió nada"
        # Prueba CON IMAGEN: confirma que el modelo acepta fotos (parte A).
        png='iVBORw0KGgoAAAANSUhEUgAAABAAAAAQCAIAAACQkWg2AAAAFklEQVR4nGO4oKBAEmIY1TCqYfhqAABLfxAQeNDhBQAAAABJRU5ErkJggg=='
        body=$(printf '{"model":"%s","max_tokens":20,"messages":[{"role":"user","content":[{"type":"text","text":"De que color es esta imagen? Responde una palabra."},{"type":"image_url","image_url":{"url":"data:image/png;base64,%s"}}]}]}' "$MODEL" "$png")
        code=$(curl -s -o /dev/null -w '%{http_code}' --max-time 60 -H "Authorization: Bearer $KEY" \
            -H "Content-Type: application/json" -d "$body" "$BASE/chat/completions")
    elif [ "$PROVIDER" = groq ]; then
        code=$(curl -s -o /dev/null -w '%{http_code}' --max-time 20 -H "Authorization: Bearer $KEY" "$BASE/models")
        [ "$code" = 200 ] || die "Groq rechazó la clave (HTTP $code): no se cambió nada"
        body=$(printf '{"model":"%s","messages":[{"role":"user","content":"%s"}],"max_tokens":8}' "$MODEL" "$prompt")
        code=$(curl -s -o /dev/null -w '%{http_code}' --max-time 30 -H "Authorization: Bearer $KEY" \
            -H "Content-Type: application/json" -d "$body" "$BASE/chat/completions")
    else
        code=$(curl -s -o /dev/null -w '%{http_code}' --max-time 20 -H "x-goog-api-key: $KEY" "$BASE/v1beta/models?pageSize=1")
        [ "$code" = 200 ] || die "Google rechazó la clave (HTTP $code): no se cambió nada"
        body=$(printf '{"contents":[{"role":"user","parts":[{"text":"%s"}]}],"generationConfig":{"maxOutputTokens":8}}' "$prompt")
        code=$(curl -s -o /dev/null -w '%{http_code}' --max-time 30 -H "x-goog-api-key: $KEY" \
            -H "Content-Type: application/json" -d "$body" "$BASE/v1beta/models/$MODEL:generateContent")
    fi
    echo "Clave válida para $PROVIDER."
    check "$code"
fi

# Script que corre en la VM. Recibe AI_KEY (protegido) y, en claro, ENV_NAME,
# OTHER, MODE, AI_PROVIDER, BASE, MODEL y PREFIX (AI o SUGGEST; ninguno es secreto).
REMOTE='set -eu
F=/opt/vaiven/$ENV_NAME/.env
O=/opt/vaiven/$OTHER/.env
[ -f "$F" ] || { echo "NO EXISTE $F"; exit 1; }
setv() { # setv CLAVE VALOR: reemplaza o agrega la línea, sin imprimir el valor
  T=$(mktemp)
  K="$1" V="$2" awk -v k="$1" '"'"'BEGIN{done=0} index($0, k"=")==1 {print k"="ENVIRON["V"]; done=1; next} {print} END{if(!done) print k"="ENVIRON["V"]}'"'"' "$F" > "$T"
  cat "$T" > "$F"; rm -f "$T"
}
if [ -n "${AI_KEY:-}" ]; then
  if [ -f "$O" ] && grep -qx "${PREFIX}_API_KEY=$AI_KEY" "$O"; then
    echo "ES LA MISMA CLAVE DE $OTHER: no se cambió nada (CA-02)"; exit 1
  fi
  setv "${PREFIX}_API_KEY" "$AI_KEY"
  setv "${PREFIX}_PROVIDER" "$AI_PROVIDER"
  setv "${PREFIX}_BASE_URL" "$BASE"
  setv "${PREFIX}_MODEL" "$MODEL"
  echo "${PREFIX}_API_KEY guardada en $F (valor no mostrado)"
fi
case "$PREFIX:$MODE" in
  SUGGEST:--enable) setv AI_ENABLED true; echo "SUGERENCIAS POR FOTO ENCENDIDAS en $ENV_NAME (se aplica al recrear la API)" ;;
  SUGGEST:--disable) setv AI_ENABLED false; echo "SUGERENCIAS POR FOTO APAGADAS en $ENV_NAME (se aplica al recrear la API)" ;;
  SUGGEST:*) ;;
  *:--enable) setv AI_ENABLED true; setv ASSISTANT_ENABLED true; echo "IA ENCENDIDA en $ENV_NAME (se aplica al recrear la API)" ;;
  *:--assistant-only) setv AI_ENABLED false; setv ASSISTANT_ENABLED true; echo "ASISTENTE ENCENDIDO y sugerencias por foto apagadas en $ENV_NAME (se aplica al recrear la API)" ;;
  *:--disable) setv AI_ENABLED false; setv ASSISTANT_ENABLED false; echo "IA APAGADA en $ENV_NAME (se aplica al recrear la API)" ;;
esac
stat -c "permisos .env: %a %U" "$F"
grep -E "^(AI_ENABLED|ASSISTANT_ENABLED|AI_PROVIDER|AI_BASE_URL|AI_MODEL|SUGGEST_PROVIDER|SUGGEST_BASE_URL|SUGGEST_MODEL)=" "$F" || true
echo "SET_AI_KEY_RESULT=ok"'

az vm run-command create -g "$RG" --vm-name "$VM" --location "$LOCATION" --name "$NAME" \
    --async-execution false --timeout-in-seconds 120 \
    --parameters "ENV_NAME=$ENV_NAME" "OTHER=$OTHER" "MODE=$MODE" "AI_PROVIDER=$AI_PROVIDER" "BASE=$BASE" "MODEL=$MODEL" "PREFIX=$PREFIX" \
    --protected-parameters "AI_KEY=$KEY" \
    --script "$REMOTE" --query provisioningState -o tsv >/dev/null || true
unset KEY
az vm run-command show -g "$RG" --vm-name "$VM" --name "$NAME" --instance-view \
    --query "instanceView.{estado:executionState,salida:output,error:error}" -o json
az vm run-command delete -g "$RG" --vm-name "$VM" --name "$NAME" --yes
echo "Run command borrado."
