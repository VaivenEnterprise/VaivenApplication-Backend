#!/usr/bin/env bash
# scripts/set-ai-key.sh — guarda la clave de Gemini en el .env de un entorno de
# la VM y, si se pide, enciende la IA (spec 014, T014). Se ejecuta en Azure Cloud
# Shell (Bash), con la sesión de az de quien opera:
#
#   bash set-ai-key.sh staging                 # solo guarda la clave (IA apagada)
#   bash set-ai-key.sh staging --enable        # guarda la clave y enciende A y B
#   bash set-ai-key.sh staging --disable       # apaga A y B (la clave se conserva)
#
# Seguridad:
#   - La clave se lee sin mostrarla y se valida con Google ANTES de tocar la VM
#     (GET /v1beta/models, solo lectura, clave en cabecera).
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
NAME=set-ai-key

die() { echo "set-ai-key: $*" >&2; exit 1; }
[ $# -ge 1 ] && [ $# -le 2 ] || die "uso: $0 <staging|production> [--enable|--disable]"
ENV_NAME=$1
MODE=${2:-key}
case "$ENV_NAME" in staging) OTHER=production ;; production) OTHER=staging ;; *) die "entorno inválido: $ENV_NAME" ;; esac
case "$MODE" in key | --enable | --disable) ;; *) die "opción desconocida: $MODE" ;; esac
command -v az >/dev/null || die "falta az (usa Azure Cloud Shell)"

KEY=
if [ "$MODE" != --disable ]; then
    read -rsp "Clave de Gemini para $ENV_NAME (no se muestra): " KEY
    echo
    [ ${#KEY} -ge 20 ] || die "la clave parece incompleta (${#KEY} caracteres)"
    code=$(curl -s -o /dev/null -w '%{http_code}' --max-time 20 -H "x-goog-api-key: $KEY" \
        "https://generativelanguage.googleapis.com/v1beta/models?pageSize=1")
    [ "$code" = 200 ] || die "Google rechazó la clave (HTTP $code): no se cambió nada"
    echo "Clave válida para Google (HTTP 200)."
fi

# Script que corre en la VM. Recibe AI_KEY (protegido), ENV_NAME, OTHER y MODE.
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
  if [ -f "$O" ] && grep -qx "AI_API_KEY=$AI_KEY" "$O"; then
    echo "ES LA MISMA CLAVE DE $OTHER: no se cambió nada (CA-02)"; exit 1
  fi
  setv AI_API_KEY "$AI_KEY"
  grep -q "^AI_PROVIDER=" "$F" || setv AI_PROVIDER gemini
  grep -q "^AI_MODEL=" "$F" || setv AI_MODEL gemini-2.5-flash
  echo "AI_API_KEY guardada en $F (valor no mostrado)"
fi
case "$MODE" in
  --enable) setv AI_ENABLED true; setv ASSISTANT_ENABLED true; echo "IA ENCENDIDA en $ENV_NAME (se aplica al recrear la API)" ;;
  --disable) setv AI_ENABLED false; setv ASSISTANT_ENABLED false; echo "IA APAGADA en $ENV_NAME (se aplica al recrear la API)" ;;
esac
stat -c "permisos .env: %a %U" "$F"
grep -E "^(AI_ENABLED|ASSISTANT_ENABLED|AI_PROVIDER|AI_MODEL)=" "$F" || true
echo "SET_AI_KEY_RESULT=ok"'

az vm run-command create -g "$RG" --vm-name "$VM" --location "$LOCATION" --name "$NAME" \
    --async-execution false --timeout-in-seconds 120 \
    --parameters "ENV_NAME=$ENV_NAME" "OTHER=$OTHER" "MODE=$MODE" \
    --protected-parameters "AI_KEY=$KEY" \
    --script "$REMOTE" --query provisioningState -o tsv >/dev/null || true
unset KEY
az vm run-command show -g "$RG" --vm-name "$VM" --name "$NAME" --instance-view \
    --query "instanceView.{estado:executionState,salida:output,error:error}" -o json
az vm run-command delete -g "$RG" --vm-name "$VM" --name "$NAME" --yes
echo "Run command borrado."
