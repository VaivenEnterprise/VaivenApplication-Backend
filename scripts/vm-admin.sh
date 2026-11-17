#!/usr/bin/env bash
# scripts/vm-admin.sh — corre `/admin` (cmd/admin) EN LA VM contra un entorno
# (spec 012 T009 y spec 013). Como root, normalmente vía cloudshell-run.sh:
#
#   bash vm-admin.sh <staging|production> <subcomando> [argumentos...]
#   bash cloudshell-run.sh <commit> scripts/vm-admin.sh production indexes
#
# Usa la imagen que corre ese entorno (vaiven-api:<current-tag>), su .env y su
# red de Docker, con APP_ENV y las rutas TLS que pone el compose. Para `smoke`
# agrega --connect-to api:8443, así valida TLS con el nombre real sin salir de
# la VM. create-admin lee de la entrada estándar: no se usa por aquí (ver
# docs/produccion.md, secretos).
set -euo pipefail

VAIVEN_DIR=${VAIVEN_DIR:-/opt/vaiven}
die() { echo "vm-admin: $*" >&2; exit 1; }
[ $# -ge 2 ] || die "uso: $0 <staging|production> <subcomando> [argumentos...]"
ENV_NAME=$1
shift
case "$ENV_NAME" in
    staging) NET=base_vaiven-net ;;
    production) NET=vaiven-production-net ;;
    *) die "entorno inválido: $ENV_NAME" ;;
esac
[ "$1" != create-admin ] || die "create-admin lee la contraseña por stdin: usa el procedimiento de docs/produccion.md"
TAG=$(cat "$VAIVEN_DIR/$ENV_NAME/current-tag" 2>/dev/null || true)
[ -n "$TAG" ] || die "no hay current-tag en $VAIVEN_DIR/$ENV_NAME (¿la API de $ENV_NAME está desplegada?)"
docker image inspect "vaiven-api:$TAG" >/dev/null 2>&1 || die "no existe vaiven-api:$TAG"

extra=()
[ "$1" = smoke ] && extra=(--connect-to api:8443)
echo "vm-admin: $ENV_NAME, vaiven-api:$TAG, $*"
set +e
docker run --rm --network "$NET" --env-file "$VAIVEN_DIR/$ENV_NAME/.env" \
    -e APP_ENV="$ENV_NAME" -e TLS_CERT_FILE=/certs/fullchain.pem -e TLS_KEY_FILE=/certs/privkey.pem \
    --entrypoint /admin "vaiven-api:$TAG" "$@" "${extra[@]}"
rc=$?
set -e
echo "ADMIN_EXIT=$rc"
exit "$rc"
