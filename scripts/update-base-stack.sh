#!/usr/bin/env bash
# scripts/update-base-stack.sh — actualiza el stack de datos de STAGING
# (MongoDB y Redis, proyecto «base») con deploy/base/docker-compose.yml de un
# commit (spec 013, T004: rotación de registros). En la VM, como root, vía
# cloudshell-run.sh:
#
#   bash cloudshell-run.sh <commit> scripts/update-base-stack.sh <commit>
#
# - Localiza el compose y el --env-file reales por las etiquetas del contenedor.
# - Muestra las diferencias; si hay algo más que `logging:`, se detiene (otra
#   diferencia necesita revisión humana).
# - Guarda una copia del compose anterior, aplica el nuevo con `up -d` (recrea
#   solo lo que cambió; los volúmenes se conservan: nunca `down -v`) y espera a
#   que MongoDB y Redis estén healthy.
set -euo pipefail

REPO=VaivenEnterprise/VaivenApplication-Backend
die() { echo "update-base-stack: $*" >&2; exit 1; }
[ $# -eq 1 ] || die "uso: $0 <commit>"
ref=$1
case "$ref" in *[!0-9a-f]* | "") die "el commit debe ser hexadecimal" ;; esac

label() { docker inspect -f "{{index .Config.Labels \"com.docker.compose.project.$1\"}}" base-mongo-1; }
CF=$(label config_files)
WD=$(label working_dir)
EF=$(label environment_file)
[ -f "$CF" ] || die "no se encontró el compose de base ($CF)"
echo "compose: $CF | dir: $WD | env-file: ${EF:-(ninguno)}"

new=$(mktemp)
trap 'rm -f "$new"' EXIT
curl -fsSL "https://raw.githubusercontent.com/$REPO/$ref/deploy/base/docker-compose.yml" -o "$new" ||
    die "no se pudo descargar deploy/base/docker-compose.yml en $ref"

if diff -q "$CF" "$new" >/dev/null; then
    echo "sin cambios: el compose de la VM ya es el de $ref"
else
    echo "== diferencias (VM -> $ref):"
    diff "$CF" "$new" || true
    # Solo se aceptan líneas agregadas o quitadas de rotación de registros y comentarios.
    other=$(diff "$CF" "$new" | grep -E '^[<>]' | grep -vE '^[<>]\s*(#.*)?$' |
        grep -vE '^[<>]\s+(logging:|driver: json-file|options:|max-size: "10m"|max-file: "3")\s*$' || true)
    [ -z "$other" ] || die "hay diferencias además de la rotación de registros; revisar a mano antes de aplicar"
    cp -p "$CF" "$CF.bak-$(date -u +%Y%m%dT%H%M%SZ)"
    install -m 0644 "$new" "$CF"
    echo "compose actualizado (copia del anterior junto a él)"
fi

cd "$WD"
args=(-f "$CF")
[ -n "$EF" ] && args=(--env-file "$EF" "${args[@]}")
docker compose "${args[@]}" up -d 2>&1 | tail -6
for c in base-mongo-1 base-redis-1; do
    for _ in $(seq 1 40); do
        s=$(docker inspect -f '{{.State.Health.Status}}' "$c" 2>/dev/null || true)
        [ "$s" = healthy ] && break
        sleep 3
    done
    echo "$c: ${s:-sin estado} | log=$(docker inspect -f '{{.HostConfig.LogConfig.Type}} {{.HostConfig.LogConfig.Config}}' "$c")"
done
curl -fsS --max-time 10 --resolve vaiven-staging.westus2.cloudapp.azure.com:8443:127.0.0.1 \
    https://vaiven-staging.westus2.cloudapp.azure.com:8443/readyz && echo
echo "UPDATE_BASE_RESULT=ok"
