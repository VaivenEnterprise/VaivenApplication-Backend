#!/usr/bin/env bash
# deploy/remote-deploy.sh — pasos de despliegue que corren DENTRO de la VM, como
# root (spec 017; mismos pasos que scripts/deploy.sh, Art. XIII.5).
#
# Lo usa .github/workflows/deploy-staging.yml: el runner antepone a este archivo
# las variables de abajo y lo ejecuta con `az vm run-command invoke`
# (RunShellScript), sin SSH ni el puerto 22. También sirve por SSH:
#   ssh vm 'sudo env MODE=deploy TAG=abc1234 ... bash -s' < deploy/remote-deploy.sh
#
# Variables:
#   MODE            deploy | rollback
#   ENV_NAME        staging (production se despliega a mano, ver docs/devsecops.md)
#   API_PORT        8443 en staging
#   AZURE_HOST      nombre DNS del certificado (vaiven-staging.westus2.cloudapp.azure.com)
#   TAG             (deploy) commit corto; la imagen queda como vaiven-api:<TAG>
#   PULL_IMAGE      (deploy, opcional) referencia del registro, p. ej.
#                   ghcr.io/vaivenenterprise/vaiven-api:<TAG>; sin ella la imagen
#                   vaiven-api:<TAG> ya debe estar cargada (docker load por SSH)
#   REGISTRY_USER   (opcional) usuario para PULL_IMAGE
#   REGISTRY_TOKEN  (opcional) token de lectura; viaja a `docker login` por stdin
#                   y la sesión vive en un DOCKER_CONFIG temporal que se borra
#   COMPOSE_B64     (deploy, opcional) deploy/docker-compose.yml en base64
#   HOOK_B64        (deploy, opcional) deploy/certbot-deploy-hook.sh en base64
#
# La última línea de la salida es siempre VAIVEN_DEPLOY_RESULT=<resultado>:
#   deployed     la imagen nueva responde /healthz y /api/v1/meta con su versión
#   rolled_back  la nueva falló y se volvió a previous-tag (que sí responde)
#   failed       falló y no se pudo volver atrás (o no había a qué volver)
# Run Command no propaga el código de salida, por eso el workflow lee esa línea.
# Nunca imprime ni crea /opt/vaiven/<entorno>/.env.
set -euo pipefail

MODE=${MODE:-deploy}
ENV_NAME=${ENV_NAME:-staging}
API_PORT=${API_PORT:-8443}
AZURE_HOST=${AZURE_HOST:-vaiven-staging.westus2.cloudapp.azure.com}
REMOTE_DIR=/opt/vaiven/$ENV_NAME
RESULT=failed

log() { printf '[remote-deploy %s] %s\n' "$ENV_NAME" "$*"; }

DOCKER_CONFIG=$(mktemp -d)
export DOCKER_CONFIG
# shellcheck disable=SC2317 # se invoca desde el trap EXIT
cleanup() {
    rm -rf "$DOCKER_CONFIG"
    unset REGISTRY_TOKEN
    echo "VAIVEN_DEPLOY_RESULT=$RESULT"
}
trap cleanup EXIT

[ "$(id -u)" -eq 0 ] || { log "debe correr como root"; exit 1; }
case "$MODE" in deploy|rollback) ;; *) log "MODE inválido: $MODE"; exit 1 ;; esac
case "$ENV_NAME" in staging) ;; *) log "solo staging se despliega por aquí"; exit 1 ;; esac
[ -f "$REMOTE_DIR/.env" ] || { log "falta $REMOTE_DIR/.env (ver deploy/.env.example)"; exit 1; }

compose_up() {
    (cd "$REMOTE_DIR" && VAIVEN_ENV="$ENV_NAME" API_IMAGE_TAG="$1" API_PORT="$API_PORT" \
        docker compose up -d --remove-orphans api)
}

# Espera hasta 60 s a que la API responda por HTTPS validando el certificado
# (sin -k). --resolve conecta a 127.0.0.1 pero verifica el nombre real.
wait_healthy() {
    local want=$1 base="https://$AZURE_HOST:$API_PORT" meta
    local curl_vm=(curl -fsS --max-time 5 --resolve "$AZURE_HOST:$API_PORT:127.0.0.1")
    for _ in $(seq 1 30); do
        if "${curl_vm[@]}" "$base/healthz" >/dev/null 2>&1; then
            meta=$("${curl_vm[@]}" "$base/api/v1/meta" 2>/dev/null || true)
            if [ -z "$want" ] || printf '%s' "$meta" | grep -q "\"version\":\"$want\""; then
                log "OK: /healthz responde por HTTPS${want:+ con la versión $want}"
                return 0
            fi
        fi
        sleep 2
    done
    log "FALLA: la API no respondió en 60 s${want:+ con la versión $want}"
    docker logs --tail 20 "vaiven-$ENV_NAME-api-1" 2>&1 | tail -20 || true
    return 1
}

rollback_to_previous() {
    local previous
    previous=$(cat "$REMOTE_DIR/previous-tag" 2>/dev/null || true)
    if [ -z "$previous" ] || ! docker image inspect "vaiven-api:$previous" >/dev/null 2>&1; then
        log "no hay una imagen anterior a la que volver"
        return 1
    fi
    log "volviendo a vaiven-api:$previous"
    compose_up "$previous"
    echo "$previous" >"$REMOTE_DIR/current-tag"
    wait_healthy "$previous"
}

if [ "$MODE" = rollback ]; then
    if rollback_to_previous; then RESULT=rolled_back; fi
    exit 0
fi

: "${TAG:?falta TAG}"
case "$TAG" in *[!0-9a-f]*) log "TAG debe ser un commit corto en hexadecimal"; exit 1 ;; esac

# 1. Imagen: del registro (con sesión temporal) o ya cargada.
if [ -n "${PULL_IMAGE:-}" ]; then
    if [ -n "${REGISTRY_TOKEN:-}" ]; then
        printf '%s' "$REGISTRY_TOKEN" |
            docker login "${PULL_IMAGE%%/*}" -u "${REGISTRY_USER:-ci}" --password-stdin >/dev/null
    fi
    log "descargando $PULL_IMAGE"
    docker pull -q "$PULL_IMAGE" >/dev/null
    docker logout "${PULL_IMAGE%%/*}" >/dev/null 2>&1 || true
    docker tag "$PULL_IMAGE" "vaiven-api:$TAG"
    docker rmi "$PULL_IMAGE" >/dev/null 2>&1 || true # solo quita el alias
fi
docker image inspect "vaiven-api:$TAG" >/dev/null 2>&1 || { log "no existe vaiven-api:$TAG"; exit 1; }

# 2. Compose y hook del certificado (como scripts/deploy.sh).
install -d -m 0750 -o deploy -g deploy "$REMOTE_DIR"
if [ -n "${COMPOSE_B64:-}" ]; then
    printf '%s' "$COMPOSE_B64" | base64 -d >"$REMOTE_DIR/docker-compose.yml.new"
    install -m 0644 -o deploy -g deploy "$REMOTE_DIR/docker-compose.yml.new" "$REMOTE_DIR/docker-compose.yml"
    rm -f "$REMOTE_DIR/docker-compose.yml.new"
fi
if [ -n "${HOOK_B64:-}" ]; then
    hook=/etc/letsencrypt/renewal-hooks/deploy/vaiven-api.sh
    install -d -m 0755 "$(dirname "$hook")"
    printf '%s' "$HOOK_B64" | base64 -d >"$hook.new"
    install -m 0755 "$hook.new" "$hook"
    rm -f "$hook.new"
    [ -f /opt/vaiven/certs/privkey.pem ] || AZURE_HOST="$AZURE_HOST" "$hook"
fi

# 3. current-tag / previous-tag (no se pisa previous-tag al repetir el mismo commit).
current=$(cat "$REMOTE_DIR/current-tag" 2>/dev/null || true)
old_previous=$(cat "$REMOTE_DIR/previous-tag" 2>/dev/null || true)
if [ -n "$current" ] && [ "$current" != "$TAG" ]; then
    echo "$current" >"$REMOTE_DIR/previous-tag"
fi
echo "$TAG" >"$REMOTE_DIR/current-tag"

# 4. Arranque (corte de unos segundos) y verificación; si falla, rollback.
log "arrancando vaiven-api:$TAG"
if compose_up "$TAG" && wait_healthy "$TAG"; then
    RESULT=deployed
    # Solo se conservan la imagen actual y la anterior (disco de la VM).
    previous=$(cat "$REMOTE_DIR/previous-tag" 2>/dev/null || true)
    docker image ls vaiven-api --format '{{.Tag}}' | while read -r t; do
        if [ "$t" != "$TAG" ] && [ "$t" != "$previous" ]; then
            docker rmi "vaiven-api:$t" >/dev/null 2>&1 || true
        fi
    done
    exit 0
fi

log "la versión nueva falló"
if rollback_to_previous; then
    RESULT=rolled_back
    # Deja las etiquetas como estaban antes de este intento.
    if [ -n "$old_previous" ]; then echo "$old_previous" >"$REMOTE_DIR/previous-tag"; fi
fi
exit 0
