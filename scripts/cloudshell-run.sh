#!/usr/bin/env bash
# scripts/cloudshell-run.sh — ejecuta EN LA VM un script de este repositorio,
# fijado a un commit, sin SSH (spec 013). Se usa desde Azure Cloud Shell (Bash):
#
#   bash cloudshell-run.sh <commit> <ruta/del/script.sh> [argumentos...]
#   bash cloudshell-run.sh 1a2b3c4 scripts/backup.sh production --offsite
#
# Descarga el script de raw.githubusercontent.com en ese commit, le antepone
# `set -- <argumentos>` y lo ejecuta como root con `az vm run-command invoke`.
# Los argumentos no deben llevar secretos: el comando viaja a Azure.
# Variables opcionales: RG (rg-vaiven), VM (vm-vaiven).
set -euo pipefail

REPO=VaivenEnterprise/VaivenApplication-Backend
RG=${RG:-rg-vaiven}
VM=${VM:-vm-vaiven}

die() { echo "cloudshell-run: $*" >&2; exit 1; }
[ $# -ge 2 ] || die "uso: $0 <commit> <ruta/del/script.sh> [argumentos...]"
ref=$1 path=$2
shift 2
case "$ref" in *[!0-9a-f]* | "") die "el commit debe ser hexadecimal" ;; esac
case "$path" in scripts/*.sh | deploy/*.sh) ;; *) die "solo scripts .sh de scripts/ o deploy/" ;; esac

work=$(mktemp -d)
trap 'rm -rf "$work"' EXIT
curl -fsSL "https://raw.githubusercontent.com/$REPO/$ref/$path" -o "$work/script.sh" ||
    die "no se pudo descargar $path en $ref"
echo "script: $path @ $ref (sha256 $(sha256sum "$work/script.sh" | cut -c1-16)…)"
{
    # Run Command ejecuta el archivo directamente: la primera línea debe ser
    # el shebang de bash (sin ella corre con sh/dash y falla «set -o pipefail»).
    echo '#!/usr/bin/env bash'
    printf 'set --'
    for a in "$@"; do printf ' %q' "$a"; done
    printf '\n'
    cat "$work/script.sh"
} >"$work/payload.sh"
az vm run-command invoke -g "$RG" -n "$VM" --command-id RunShellScript \
    --scripts "@$work/payload.sh" --query "value[0].message" -o tsv
