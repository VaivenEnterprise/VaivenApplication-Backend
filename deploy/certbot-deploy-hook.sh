#!/usr/bin/env bash
# deploy/certbot-deploy-hook.sh — copia el certificado de Let's Encrypt para la API.
#
# La imagen de la API corre como usuario no root (distroless nonroot, uid 65532)
# y no puede leer /etc/letsencrypt (privkey.pem es solo de root). Este hook deja
# una copia en /opt/vaiven/certs legible solo por ese usuario; la API la monta de
# solo lectura y la recarga sola al cambiar (≤ 1 min), sin reiniciarse.
#
# Instalación (la hace scripts/deploy.sh): se copia a
# /etc/letsencrypt/renewal-hooks/deploy/vaiven-api.sh y certbot lo ejecuta tras
# cada renovación con RENEWED_LINEAGE. También se puede ejecutar a mano:
#   sudo AZURE_HOST=<dns> bash certbot-deploy-hook.sh
set -euo pipefail

API_UID=65532
DEST=/opt/vaiven/certs

if [ "$(id -u)" -ne 0 ]; then
    echo "Ejecuta con sudo" >&2
    exit 1
fi

if [ -n "${RENEWED_LINEAGE:-}" ]; then
    SRC="$RENEWED_LINEAGE"
elif [ -n "${AZURE_HOST:-}" ]; then
    SRC="/etc/letsencrypt/live/$AZURE_HOST"
else
    echo "Falta RENEWED_LINEAGE (certbot) o AZURE_HOST" >&2
    exit 1
fi

install -d -m 0750 -o "$API_UID" -g "$API_UID" "$DEST"
# Archivo temporal y mv: la API nunca ve un par a medio escribir.
install -m 0440 -o "$API_UID" -g "$API_UID" "$SRC/fullchain.pem" "$DEST/fullchain.pem.tmp"
install -m 0400 -o "$API_UID" -g "$API_UID" "$SRC/privkey.pem" "$DEST/privkey.pem.tmp"
mv -f "$DEST/fullchain.pem.tmp" "$DEST/fullchain.pem"
mv -f "$DEST/privkey.pem.tmp" "$DEST/privkey.pem"

echo "Certificado copiado a $DEST para la API (uid $API_UID)."
