#!/usr/bin/env bash
# deploy/production/init-env.sh — Crea las contraseñas y el keyFile PROPIOS del
# MongoDB y el Redis de producción (spec 013, T001; Art. XIII.6), sin tocar los
# de staging (/opt/vaiven/base). Se ejecuta EN LA VM con sudo, una sola vez.
# Idempotente: si algo ya existe, no lo sobrescribe. Nunca imprime valores.
set -euo pipefail

if [ "$(id -u)" -ne 0 ]; then
    echo "Ejecuta con sudo" >&2
    exit 1
fi

DIR=/opt/vaiven/production
DATA_ENV="$DIR/data.env"
KEYFILE="$DIR/secrets/mongo-keyfile"
install -d -m 0750 -o deploy -g deploy "$DIR"
install -d -m 0700 "$DIR/secrets"

if [ -e "$DATA_ENV" ]; then
    echo "EXISTE: $DATA_ENV (no se sobrescribe)"
else
    (
        umask 077
        {
            printf 'MONGO_ROOT_PASSWORD=%s\n' "$(openssl rand -hex 24)"
            printf 'MONGO_APP_PASSWORD=%s\n' "$(openssl rand -hex 24)"
            printf 'REDIS_PASSWORD=%s\n' "$(openssl rand -hex 24)"
        } >"$DATA_ENV"
    )
    echo "CREADO: $DATA_ENV"
fi
chown deploy:deploy "$DATA_ENV"
chmod 600 "$DATA_ENV"

if [ -e "$KEYFILE" ]; then
    echo "EXISTE: $KEYFILE (no se sobrescribe)"
else
    (umask 077 && openssl rand -base64 756 >"$KEYFILE")
    echo "CREADO: $KEYFILE"
fi
# El proceso mongod de la imagen oficial corre con uid 999.
chown 999:999 "$KEYFILE"
chmod 400 "$KEYFILE"

echo "Listo. Variables: MONGO_ROOT_PASSWORD, MONGO_APP_PASSWORD, REDIS_PASSWORD (valores no mostrados)."
echo "Son distintas de las de staging (/opt/vaiven/base/.env): CA-02."
