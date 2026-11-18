#!/usr/bin/env bash
# scripts/set-reco.sh — enciende o apaga las recomendaciones (RECO_ENABLED,
# spec 015) en un entorno de la VM y recrea la API con la MISMA imagen para que
# tome el cambio. Corre EN LA VM como root, desde Azure Cloud Shell (Bash):
#
#   bash cloudshell-run.sh <commit> scripts/set-reco.sh staging --enable
#   bash cloudshell-run.sh <commit> scripts/set-reco.sh staging --disable
#   bash cloudshell-run.sh <commit> scripts/set-reco.sh production --enable --yes-production
#
# Solo cambia la línea RECO_ENABLED del .env (no lee ni imprime nada más; el
# .env conserva sus permisos). Después espera hasta 60 s a que /api/v1/meta
# responda por HTTPS, validando el certificado, con features.recommendations
# en el valor pedido. Imprime SET_RECO_RESULT=ok|failed al final.
set -euo pipefail

VAIVEN_DIR=${VAIVEN_DIR:-/opt/vaiven}
AZURE_HOST=${AZURE_HOST:-vaiven-staging.westus2.cloudapp.azure.com}
RESULT=failed
trap 'echo "SET_RECO_RESULT=$RESULT"' EXIT
die() { echo "set-reco: $*" >&2; exit 1; }

[ $# -ge 2 ] && [ $# -le 3 ] || die "uso: $0 <staging|production> <--enable|--disable> [--yes-production]"
ENV_NAME=$1
MODE=$2
case "$ENV_NAME" in staging) PORT=8443 ;; production) PORT=443 ;; *) die "entorno inválido: $ENV_NAME" ;; esac
case "$MODE" in --enable) VALUE=true ;; --disable) VALUE=false ;; *) die "opción desconocida: $MODE" ;; esac
# Encender en producción es una decisión del equipo (v1.1): pide confirmarlo.
if [ "$ENV_NAME:$VALUE" = production:true ] && [ "${3:-}" != --yes-production ]; then
    die "encender en production exige --yes-production (decisión del equipo; ver docs/produccion.md §12)"
fi
[ "$(id -u)" -eq 0 ] || die "debe correr como root"
DIR=$VAIVEN_DIR/$ENV_NAME
F=$DIR/.env
[ -f "$F" ] || die "no existe $F"
TAG=$(cat "$DIR/current-tag" 2>/dev/null || true)
[ -n "$TAG" ] || die "no hay current-tag en $DIR (¿la API está desplegada?)"

# Reemplaza o agrega la línea, conservando dueño y permisos del archivo.
T=$(mktemp)
awk -v v="$VALUE" 'BEGIN{done=0} index($0, "RECO_ENABLED=")==1 {print "RECO_ENABLED=" v; done=1; next} {print} END{if(!done) print "RECO_ENABLED=" v}' "$F" >"$T"
cat "$T" >"$F"
rm -f "$T"
echo "RECO_ENABLED=$VALUE en $F ($(stat -c '%a %U' "$F"))"

(cd "$DIR" && VAIVEN_ENV="$ENV_NAME" API_IMAGE_TAG="$TAG" API_PORT="$PORT" docker compose up -d --force-recreate api)

want="\"recommendations\":$VALUE"
for _ in $(seq 1 30); do
    meta=$(curl -fsS --max-time 5 --resolve "$AZURE_HOST:$PORT:127.0.0.1" "https://$AZURE_HOST:$PORT/api/v1/meta" 2>/dev/null || true)
    if printf '%s' "$meta" | grep -q "$want"; then
        echo "OK: la API de $ENV_NAME (vaiven-api:$TAG) responde con features.recommendations=$VALUE"
        RESULT=ok
        exit 0
    fi
    sleep 2
done
docker logs --tail 20 "vaiven-$ENV_NAME-api-1" 2>&1 | tail -20 || true
die "la API no respondió con features.recommendations=$VALUE en 60 s"
