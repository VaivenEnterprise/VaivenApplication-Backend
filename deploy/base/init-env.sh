#!/usr/bin/env bash
# deploy/base/init-env.sh — Crea /opt/vaiven/base/.env con contraseñas aleatorias
# (solo si no existe) y el enlace /opt/vaiven/.env que lee el servicio redis.
# Se ejecuta EN LA VM con sudo, una sola vez (spec 001, T006). Idempotente.
# Nunca imprime valores: solo nombres de variables y permisos.
set -euo pipefail

if [ "$(id -u)" -ne 0 ]; then
    echo "Ejecuta con sudo" >&2
    exit 1
fi

BASE=/opt/vaiven/base
ENVF="$BASE/.env"
mkdir -p "$BASE"

if [ -e "$ENVF" ]; then
    echo "EXISTE: $ENVF (no se sobrescribe)"
else
    (
        umask 077
        {
            printf 'MONGO_ROOT_PASSWORD=%s\n' "$(openssl rand -hex 24)"
            printf 'MONGO_APP_PASSWORD=%s\n'  "$(openssl rand -hex 24)"
            printf 'REDIS_PASSWORD=%s\n'      "$(openssl rand -hex 24)"
        } > "$ENVF"
    )
    echo "CREADO: $ENVF"
fi
chown deploy:deploy "$ENVF"
chmod 600 "$ENVF"

# deploy/base/docker-compose.yml lee /opt/vaiven/.env (env_file de redis).
if [ -L /opt/vaiven/.env ]; then
    echo "ENLACE: /opt/vaiven/.env ya existe"
elif [ -e /opt/vaiven/.env ]; then
    echo "ERROR: /opt/vaiven/.env existe y no es un enlace; no se toca" >&2
    exit 1
else
    ln -s "$ENVF" /opt/vaiven/.env
    chown -h deploy:deploy /opt/vaiven/.env
    echo "ENLACE: /opt/vaiven/.env -> $ENVF"
fi

ls -l "$ENVF" /opt/vaiven/.env
echo "Variables (sin valores):"
cut -d= -f1 "$ENVF"
