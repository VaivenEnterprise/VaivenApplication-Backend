#!/usr/bin/env bash
# scripts/deploy.sh — despliega la API de VAIVÉN en la VM de Azure (spec 002, T030;
# Art. XIII.5 y XIII.7; plan P-S7).
#
#   scripts/deploy.sh staging                   # verifica, compila, envía, arranca y verifica
#   scripts/deploy.sh staging --check           # solo las verificaciones de git (sin SSH ni build)
#   scripts/deploy.sh staging --allow-unmerged  # staging desde una rama de spec aún sin fusionar
#   scripts/deploy.sh staging --rollback        # vuelve a la imagen anterior
#
# Verificaciones previas (antes de cualquier SSH o build; se omiten con --rollback,
# que reutiliza una imagen que ya está en la VM):
#   1. Árbol limpio: sin cambios en archivos versionados (preparados o no). Los
#      archivos sin seguimiento no se revisan.
#   2. Repositorio completo: un clon superficial (shallow) se rechaza.
#   3. HEAD contenido en la rama base del remoto, tras `git fetch origin <rama>`:
#      production -> origin/main (sin excepción); staging -> origin/develop.
#      Solo en staging, --allow-unmerged acepta un HEAD aún no fusionado si está
#      publicado en alguna rama de origin (tarea «Despliegue» de cada spec,
#      Art. XIII.5, que despliega la rama de la spec antes de fusionar su PR).
#
# Requisitos en la máquina de quien despliega: git, docker o podman, ssh, curl y
# la llave de la VM. Variables (con valores por defecto):
#   AZURE_HOST  vaiven-staging.westus2.cloudapp.azure.com
#   SSH_USER    azureuser
#   SSH_KEY     ~/.ssh/vaiven_azure
#   ENGINE      docker o podman (se detecta)
# En la VM debe existir /opt/vaiven/<entorno>/.env (600); este script nunca lo
# crea ni lo imprime. No hay registro de imágenes: la imagen viaja por SSH.
set -euo pipefail

usage() {
    echo "uso: $0 <staging|production> [--rollback|--check] [--allow-unmerged]" >&2
    echo "  --check           solo verifica el repositorio (sin SSH, build ni despliegue)" >&2
    echo "  --allow-unmerged  solo staging: permite un commit publicado aún no fusionado en develop" >&2
    echo "  --rollback        vuelve a la imagen anterior en la VM (sin verificaciones de git)" >&2
    exit 2
}

die() {
    echo "ERROR: $*" >&2
    exit 1
}

[ $# -ge 1 ] || usage
ENV_NAME=$1
shift
ROLLBACK=false
CHECK_ONLY=false
ALLOW_UNMERGED=false
for arg in "$@"; do
    case "$arg" in
        --rollback) ROLLBACK=true ;;
        --check) CHECK_ONLY=true ;;
        --allow-unmerged) ALLOW_UNMERGED=true ;;
        *) echo "Opción desconocida: $arg" >&2; usage ;;
    esac
done

case "$ENV_NAME" in
    staging) API_PORT=8443; BASE_BRANCH=develop ;;
    production) API_PORT=443; BASE_BRANCH=main ;;
    *) usage ;;
esac

if $ROLLBACK && $CHECK_ONLY; then
    echo "--rollback y --check no se combinan" >&2
    usage
fi
if $ROLLBACK && $ALLOW_UNMERGED; then
    echo "--allow-unmerged no aplica a --rollback (no compila ni verifica git)" >&2
    usage
fi
if $ALLOW_UNMERGED && [ "$ENV_NAME" = production ]; then
    die "--allow-unmerged no se acepta en production: solo se despliega lo que ya está en origin/main."
fi

REPO_ROOT=$(cd "$(dirname "$0")/.." && pwd)

# Verificaciones de git previas al despliegue (ver cabecera).
git_guard() {
    local g=(git -C "$REPO_ROOT")
    "${g[@]}" rev-parse --git-dir >/dev/null 2>&1         || die "$REPO_ROOT no es un repositorio git."

    if [ "$("${g[@]}" rev-parse --is-shallow-repository)" = true ]; then
        die "el repositorio es un clon superficial (shallow) y no se puede comprobar el historial. Ejecuta 'git fetch --unshallow' y vuelve a intentarlo."
    fi

    # Refresca el índice para no confundir cambios de fecha con cambios reales.
    "${g[@]}" update-index -q --refresh >/dev/null 2>&1 || true
    if ! "${g[@]}" diff --quiet HEAD --; then
        "${g[@]}" status --short --untracked-files=no >&2
        die "hay cambios sin commit en archivos versionados (arriba). Haz commit o 'git stash' y vuelve a intentarlo: la imagen debe corresponder exactamente a un commit."
    fi
    # El Dockerfile copia go.mod, go.sum, cmd/ e internal/: un archivo sin
    # seguimiento ahí entraría en la imagen sin estar en el commit.
    local untracked
    untracked=$("${g[@]}" ls-files --others --exclude-standard -- go.mod go.sum cmd internal)
    if [ -n "$untracked" ]; then
        echo "$untracked" >&2
        die "hay archivos sin seguimiento que entrarían en la imagen (arriba). Haz commit de ellos o bórralos y vuelve a intentarlo."
    fi

    "${g[@]}" fetch --quiet origin "$BASE_BRANCH"         || die "no se pudo ejecutar 'git fetch origin $BASE_BRANCH'. Revisa la conexión y el acceso al remoto."

    local head
    head=$("${g[@]}" rev-parse --short HEAD)
    if "${g[@]}" merge-base --is-ancestor HEAD "refs/remotes/origin/$BASE_BRANCH"; then
        echo "[deploy $ENV_NAME] OK: árbol limpio y $head está en origin/$BASE_BRANCH"
        return 0
    fi

    if [ "$ENV_NAME" = staging ] && $ALLOW_UNMERGED; then
        "${g[@]}" fetch --quiet origin             || die "no se pudo ejecutar 'git fetch origin'. Revisa la conexión y el acceso al remoto."
        local branches
        branches=$("${g[@]}" branch -r --contains HEAD --list 'origin/*')
        [ -n "$branches" ]             || die "$head no está publicado en ninguna rama de origin. Haz 'git push' de tu rama y vuelve a intentarlo."
        {
            echo "=================================================================="
            echo "AVISO: desplegando en staging $head, que NO está en origin/develop."
            echo "Solo para la tarea «Despliegue» de una spec (Art. XIII.5)."
            echo "Publicado en:"
            echo "$branches"
            echo "=================================================================="
        } >&2
        return 0
    fi

    if [ "$ENV_NAME" = production ]; then
        die "$head no está en origin/main. Producción solo despliega commits ya fusionados en main (PR develop -> main)."
    fi
    die "$head no está en origin/develop. Fusiona el PR en develop o, solo para la tarea «Despliegue» de una spec, usa --allow-unmerged con la rama ya publicada."
}

if ! $ROLLBACK; then
    git_guard
fi
if $CHECK_ONLY; then
    exit 0
fi

AZURE_HOST=${AZURE_HOST:-vaiven-staging.westus2.cloudapp.azure.com}
SSH_USER=${SSH_USER:-azureuser}
SSH_KEY=${SSH_KEY:-$HOME/.ssh/vaiven_azure}
REMOTE_DIR=/opt/vaiven/$ENV_NAME

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
    # El directorio es de deploy (750): se entra con sudo.
    ssh_vm "sudo sh -c \"cd '$REMOTE_DIR' && VAIVEN_ENV='$ENV_NAME' API_IMAGE_TAG='$tag' API_PORT='$API_PORT' docker compose up -d --remove-orphans api\"" </dev/null
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
    ssh_vm "sudo docker logs --tail 30 vaiven-$ENV_NAME-api-1" </dev/null || true
    return 1
}

ssh_vm "sudo test -f '$REMOTE_DIR/.env'" </dev/null || {
    echo "Falta $REMOTE_DIR/.env en la VM (ver deploy/.env.example). Créalo en la VM, nunca en el repositorio." >&2
    exit 1
}

if $ROLLBACK; then
    previous=$(ssh_vm "sudo cat '$REMOTE_DIR/previous-tag' 2>/dev/null" </dev/null || true)
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

# 2. Imagen etiquetada con el commit. Red de seguridad: si el árbol cambió
# desde la verificación inicial, se aborta (ya no existen etiquetas -dirty).
TAG=$(git -C "$REPO_ROOT" rev-parse --short HEAD)
git -C "$REPO_ROOT" diff --quiet HEAD --     || die "el árbol cambió durante el despliegue; no se compila una imagen que no corresponde a un commit."
log "Compilando vaiven-api:$TAG para linux/$ARCH con $ENGINE"
"$ENGINE" build --platform "linux/$ARCH" --build-arg "APP_VERSION=$TAG" -t "vaiven-api:$TAG" "$REPO_ROOT"

# 3. Envío por SSH (docker save | ssh docker load).
log "Enviando la imagen a la VM"
"$ENGINE" save "vaiven-api:$TAG" | ssh_vm "sudo docker load" >/dev/null
# Podman guarda la imagen como localhost/vaiven-api; Compose usa vaiven-api.
ssh_vm "sudo docker image inspect 'vaiven-api:$TAG' >/dev/null 2>&1 || sudo docker tag 'localhost/vaiven-api:$TAG' 'vaiven-api:$TAG'" </dev/null

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
ssh_vm "sudo sh -c \"cd '$REMOTE_DIR' && { [ -f current-tag ] && cp current-tag previous-tag || true; } && echo '$TAG' > current-tag\"" </dev/null
log "Arrancando vaiven-api:$TAG (corte de unos segundos)"
compose_up "$TAG"

# 6. Verificación por HTTPS.
wait_healthy
