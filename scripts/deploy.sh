#!/usr/bin/env bash
# scripts/deploy.sh — despliega la API de VAIVÉN en la VM de Azure (spec 002, T030;
# Art. XIII.5 y XIII.7; plan P-S7).
#
#   scripts/deploy.sh staging            # compila, envía, arranca y verifica
#   scripts/deploy.sh staging --rollback # vuelve a la imagen anterior
#
# Requisitos en la máquina de quien despliega: docker o podman, ssh, curl y la
# llave de la VM. Variables (con valores por defecto):
#   AZURE_HOST  vaiven-staging.westus2.cloudapp.azure.com
#   SSH_USER    azureuser
#   SSH_KEY     ~/.ssh/vaiven_azure
#   ENGINE      docker o podman (se detecta)
# En la VM debe existir /opt/vaiven/<entorno>/.env (600); este script nunca lo
# crea ni lo imprime. No hay registro de imágenes: la imagen viaja por SSH.
set -euo pipefail

usage() {
    echo "uso: $0 <staging|production> [--rollback]" >&2
    exit 2
}

[ $# -ge 1 ] || usage
ENV_NAME=$1
ROLLBACK=false
[ "${2:-}" = "--rollback" ] && ROLLBACK=true
[ $# -le 2 ] || usage

case "$ENV_NAME" in
    staging) API_PORT=8443 ;;
    production) API_PORT=443 ;;
    *) usage ;;
esac

AZURE_HOST=${AZURE_HOST:-vaiven-staging.westus2.cloudapp.azure.com}
SSH_USER=${SSH_USER:-azureuser}
SSH_KEY=${SSH_KEY:-$HOME/.ssh/vaiven_azure}
REMOTE_DIR=/opt/vaiven/$ENV_NAME
REPO_ROOT=$(cd "$(dirname "$0")/.." && pwd)

if [ -z "${ENGINE:-}" ]; then
    if command -v docker >/dev/null 2>&1 && docker info >/dev/null 2>&1; then
        ENGINE=docker
    elif command -v podman >/dev/null 2>&1; then
        ENGINE=podman
    else
        echo "Falta docker o podman" >&2
        exit 1
    fi
fi

ssh_vm() {
    ssh -i "$SSH_KEY" -o BatchMode=yes -o ConnectTimeout=20 "$SSH_USER@$AZURE_HOST" "$@"
}

log() { printf '[deploy %s] %s\n' "$ENV_NAME" "$*"; }

# Levanta la etiqueta indicada con Compose en la VM.
compose_up() {
    local tag=$1
    ssh_vm "cd '$REMOTE_DIR' && sudo env VAIVEN_ENV='$ENV_NAME' API_IMAGE_TAG='$tag' API_PORT='$API_PORT' docker compose up -d --remove-orphans api" </dev/null
}

# Espera /healthz por HTTPS, validando el certificado (sin -k), hasta 60 s.
wait_healthy() {
    local url="https://$AZURE_HOST:$API_PORT/healthz"
    for _ in $(seq 1 30); do
        if curl -fsS --max-time 5 "$url" >/dev/null 2>&1; then
            log "OK: $url responde por HTTPS"
            return 0
        fi
        sleep 2
    done
    log "FALLA: $url no respondió en 60 s"
    ssh_vm "cd '$REMOTE_DIR' && sudo docker compose -p 'vaiven-$ENV_NAME' logs --tail 30 api" </dev/null || true
    return 1
}

ssh_vm "sudo test -f '$REMOTE_DIR/.env'" </dev/null || {
    echo "Falta $REMOTE_DIR/.env en la VM (ver deploy/.env.example). Créalo en la VM, nunca en el repositorio." >&2
    exit 1
}

if $ROLLBACK; then
    previous=$(ssh_vm "cat '$REMOTE_DIR/previous-tag' 2>/dev/null" </dev/null || true)
    [ -n "$previous" ] || { echo "No hay una imagen anterior a la que volver" >&2; exit 1; }
    log "Volviendo a vaiven-api:$previous"
    compose_up "$previous"
    ssh_vm "echo '$previous' | sudo tee '$REMOTE_DIR/current-tag' >/dev/null" </dev/null
    wait_healthy
    exit $?
fi

# 1. Arquitectura de la VM (amd64 en B2ats_v2, arm64 en B2pts_v2).
case "$(ssh_vm 'uname -m' </dev/null)" in
    x86_64) ARCH=amd64 ;;
    aarch64) ARCH=arm64 ;;
    *) echo "Arquitectura de la VM no soportada" >&2; exit 1 ;;
esac

# 2. Imagen etiquetada con el commit (y -dirty si hay cambios sin commit).
TAG=$(git -C "$REPO_ROOT" rev-parse --short HEAD)
git -C "$REPO_ROOT" diff --quiet HEAD -- || TAG="$TAG-dirty"
log "Compilando vaiven-api:$TAG para linux/$ARCH con $ENGINE"
"$ENGINE" build --platform "linux/$ARCH" --build-arg "APP_VERSION=$TAG" -t "vaiven-api:$TAG" "$REPO_ROOT"

# 3. Envío por SSH (docker save | ssh docker load).
log "Enviando la imagen a la VM"
"$ENGINE" save "vaiven-api:$TAG" | ssh_vm "sudo docker load" >/dev/null

# 4. Compose y hook del certificado.
scp -q -i "$SSH_KEY" -o BatchMode=yes \
    "$REPO_ROOT/deploy/docker-compose.yml" "$REPO_ROOT/deploy/certbot-deploy-hook.sh" \
    "$SSH_USER@$AZURE_HOST:/tmp/"
ssh_vm "sudo install -d -m 0750 -o deploy -g deploy '$REMOTE_DIR' \
    && sudo install -m 0644 -o deploy -g deploy /tmp/docker-compose.yml '$REMOTE_DIR/docker-compose.yml' \
    && sudo install -m 0755 /tmp/certbot-deploy-hook.sh /etc/letsencrypt/renewal-hooks/deploy/vaiven-api.sh \
    && rm -f /tmp/docker-compose.yml /tmp/certbot-deploy-hook.sh \
    && sudo test -f /opt/vaiven/certs/privkey.pem || sudo AZURE_HOST='$AZURE_HOST' /etc/letsencrypt/renewal-hooks/deploy/vaiven-api.sh" </dev/null

# 5. Guarda la etiqueta anterior para --rollback y arranca la nueva.
ssh_vm "cd '$REMOTE_DIR' && { [ -f current-tag ] && sudo cp current-tag previous-tag || true; } && echo '$TAG' | sudo tee current-tag >/dev/null" </dev/null
log "Arrancando vaiven-api:$TAG (corte de unos segundos)"
compose_up "$TAG"

# 6. Verificación por HTTPS.
wait_healthy
