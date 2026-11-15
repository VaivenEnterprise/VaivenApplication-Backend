#!/usr/bin/env bash
# scripts/deploy-production.sh — despliega la API de PRODUCCIÓN sin SSH (spec 013,
# T010; Art. XIII.5 y XIII.6). Se ejecuta en Azure Cloud Shell (Bash), con la
# sesión de `az` de quien despliega; producción no tiene workflow.
#
#   bash deploy-production.sh <commit>       # despliega vaiven-api:<commit corto>
#   bash deploy-production.sh --rollback     # vuelve a la imagen anterior
#
# Reglas:
#   - Solo despliega commits que ya están en origin/main (lo comprueba con la API
#     pública de GitHub: compare main...<commit> debe dar "behind" o "identical").
#   - Usa la imagen vaiven-api:<commit corto> ya cargada en la VM, es decir, la
#     misma que se construyó y probó en staging (no compila otra).
#   - Descarga deploy/remote-deploy.sh y deploy/docker-compose.yml FIJADOS a ese
#     commit y los ejecuta en la VM con `az vm run-command invoke`.
#   - Nunca lee ni imprime /opt/vaiven/production/.env.
#
# Variables opcionales: RG (rg-vaiven), VM (vm-vaiven),
# AZURE_HOST (vaiven-staging.westus2.cloudapp.azure.com).
set -euo pipefail

REPO=VaivenEnterprise/VaivenApplication-Backend
RG=${RG:-rg-vaiven}
VM=${VM:-vm-vaiven}
AZURE_HOST=${AZURE_HOST:-vaiven-staging.westus2.cloudapp.azure.com}

die() { echo "deploy-production: $*" >&2; exit 1; }
usage() { die "uso: $0 <commit> | --rollback"; }

[ $# -eq 1 ] || usage
command -v az >/dev/null || die "falta az (usa Azure Cloud Shell)"
command -v curl >/dev/null || die "falta curl"

work=$(mktemp -d)
trap 'rm -rf "$work"' EXIT

if [ "$1" = --rollback ]; then
    mode=rollback
    ref=main
    tag=
else
    mode=deploy
    sha=$1
    case "$sha" in *[!0-9a-f]* | "") die "el commit debe ser hexadecimal" ;; esac
    [ ${#sha} -ge 7 ] || die "usa al menos 7 caracteres del commit"
    curl -fsS "https://api.github.com/repos/$REPO/compare/main...$sha" -o "$work/compare.json" ||
        die "no se pudo consultar GitHub para $sha"
    status=$(grep -m1 -o '"status": *"[a-z]*"' "$work/compare.json" | sed 's/.*"\([a-z]*\)"$/\1/' || true)
    case "$status" in
        behind | identical) echo "OK: $sha ya está en origin/main ($status)" ;;
        "") die "no se pudo comprobar $sha contra main en GitHub" ;;
        *) die "$sha no está en origin/main ($status). Producción solo despliega lo fusionado en main." ;;
    esac
    ref=$sha
    tag=${sha:0:7}
fi

raw="https://raw.githubusercontent.com/$REPO/$ref"
for f in remote-deploy.sh docker-compose.yml; do
    curl -fsSL "$raw/deploy/$f" -o "$work/$f" || die "no se pudo descargar deploy/$f de $ref"
done
grep -q 'production:443' "$work/remote-deploy.sh" ||
    die "deploy/remote-deploy.sh de $ref no admite production (requiere spec 013 T010)"

{
    printf 'export MODE=%q ENV_NAME=production API_PORT=443 AZURE_HOST=%q TAG=%q\n' \
        "$mode" "$AZURE_HOST" "$tag"
    printf 'export COMPOSE_B64=%q\n' "$(base64 -w0 "$work/docker-compose.yml")"
    cat "$work/remote-deploy.sh"
} >"$work/payload.sh"

echo "Ejecutando en $VM ($mode${tag:+ vaiven-api:$tag})..."
out=$(az vm run-command invoke -g "$RG" -n "$VM" --command-id RunShellScript \
    --scripts "@$work/payload.sh" --query "value[0].message" -o tsv)
printf '%s\n' "$out"

result=$(printf '%s\n' "$out" | sed -n 's/^VAIVEN_DEPLOY_RESULT=//p' | tail -1)
case "$result" in
    deployed | rolled_back) echo "Resultado: $result" ;;
    *) die "resultado: ${result:-desconocido}" ;;
esac
[ "$result" = deployed ] || [ "$mode" = rollback ] || exit 1
