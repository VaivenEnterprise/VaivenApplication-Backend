#!/usr/bin/env bash
# scripts/backup.sh — respaldo CIFRADO de MongoDB (spec 013, T006; RF-07, P-05).
# Se ejecuta EN LA VM como root (sin SSH: con `az vm run-command`, ver
# docs/respaldos.md):
#
#   bash backup.sh <production|staging> [--offsite]
#
# 1. mongodump --archive --gzip dentro del contenedor de MongoDB del entorno,
#    cifrado al vuelo con openssl (aes-256-cbc, pbkdf2) con la frase de
#    /opt/vaiven/backup/backup.key: nunca queda una copia sin cifrar en disco.
# 2. Junto al archivo guarda <archivo>.sha256 y <archivo>.counts.json (solo
#    nombres de colecciones y conteos: lo usa restore.sh para comparar, CA-04).
# 3. Conserva los 8 respaldos más recientes por entorno.
# 4. Con --offsite sube el archivo cifrado y su .counts.json a Cloudinary como
#    raw "authenticated" (privado) en vaiven/backups/<entorno>/, con las
#    credenciales del .env del entorno: así queda una copia fuera de la VM.
# Nunca imprime contraseñas, la frase ni las credenciales.
set -euo pipefail

VAIVEN_DIR=${VAIVEN_DIR:-/opt/vaiven}
DOCKER=${DOCKER:-docker}
KEEP=${KEEP:-8}

die() { echo "backup: $*" >&2; exit 1; }
getv() { grep -m1 "^$1=" "$2" | cut -d= -f2- || true; }

[ $# -ge 1 ] && [ $# -le 2 ] || die "uso: $0 <production|staging> [--offsite]"
ENV_NAME=$1
OFFSITE=false
[ "${2:-}" = --offsite ] && OFFSITE=true
[ -z "${2:-}" ] || $OFFSITE || die "opción desconocida: $2"

case "$ENV_NAME" in
    production) C=${MONGO_CONTAINER:-vaiven-production-data-mongo-1} DB=vaiven_prod PWFILE=$VAIVEN_DIR/production/data.env ;;
    staging) C=${MONGO_CONTAINER:-base-mongo-1} DB=vaiven_staging PWFILE=$VAIVEN_DIR/base/.env ;;
    *) die "entorno inválido: $ENV_NAME" ;;
esac
KEY=$VAIVEN_DIR/backup/backup.key
[ -s "$KEY" ] || die "falta la frase $KEY (docs/respaldos.md §1)"
ROOTPW=$(getv MONGO_ROOT_PASSWORD "$PWFILE")
[ -n "$ROOTPW" ] || die "no se encontró MONGO_ROOT_PASSWORD en $PWFILE"

umask 077
OUT=$VAIVEN_DIR/backup/$ENV_NAME
mkdir -p "$OUT"
NAME=vaiven_${ENV_NAME}_$(date -u +%Y%m%dT%H%M%SZ).archive.gz.enc
F=$OUT/$NAME

# La configuración de mongodump (URI y contraseña) entra por stdin a un archivo
# temporal del contenedor: nunca aparece en la línea de comandos.
printf 'uri: "mongodb://root@localhost:27017/?authSource=admin&directConnection=true"\npassword: "%s"\n' "$ROOTPW" |
    "$DOCKER" exec -i "$C" sh -c 'f=$(mktemp); cat >"$f"; mongodump --quiet --config "$f" --db "$1" --archive --gzip; rc=$?; rm -f "$f"; exit $rc' sh "$DB" |
    openssl enc -aes-256-cbc -pbkdf2 -iter 200000 -salt -pass "file:$KEY" -out "$F" ||
    { rm -f "$F"; die "falló el respaldo de $DB"; }

# El JavaScript va por --eval (no tiene secretos); la contraseña, por entorno.
COUNT_JS='const d = connect("mongodb://root:" + process.env.MP + "@localhost:27017/admin?directConnection=true").getSiblingDB(process.env.DB);
const counts = {};
d.getCollectionNames().sort().forEach((c) => { counts[c] = d.getCollection(c).countDocuments({}); });
print(JSON.stringify({ db: process.env.DB, counts: counts }));'
"$DOCKER" exec -e MP="$ROOTPW" -e DB="$DB" "$C" mongosh --quiet --nodb --eval "$COUNT_JS" >"$F.counts.json" ||
    { rm -f "$F" "$F.counts.json"; die "no se pudieron contar las colecciones"; }
grep -q '^{"db":' "$F.counts.json" || { rm -f "$F" "$F.counts.json"; die "manifiesto de conteos inválido"; }
sha256sum "$F" | cut -d' ' -f1 >"$F.sha256"

echo "respaldo: $F ($(stat -c %s "$F") bytes, sha256 $(cut -c1-16 "$F.sha256")…)"
echo "conteos: $(cat "$F.counts.json")"

# Retención: los $KEEP más recientes (con sus archivos acompañantes).
ls -1t "$OUT"/*.enc 2>/dev/null | tail -n +"$((KEEP + 1))" | while read -r old; do
    rm -f "$old" "$old.sha256" "$old.counts.json"
    echo "retención: borrado $(basename "$old")"
done

if $OFFSITE; then
    ENVF=$VAIVEN_DIR/$ENV_NAME/.env
    CN=$(getv CLOUDINARY_CLOUD_NAME "$ENVF")
    AK=$(getv CLOUDINARY_API_KEY "$ENVF")
    SEC=$(getv CLOUDINARY_API_SECRET "$ENVF")
    [ -n "$CN" ] && [ -n "$AK" ] && [ -n "$SEC" ] || die "faltan credenciales de Cloudinary en $ENVF"
    for file in "$F" "$F.counts.json"; do
        pid="vaiven/backups/$ENV_NAME/$(basename "$file")"
        ts=$(date +%s)
        sig=$(printf 'public_id=%s&timestamp=%s&type=authenticated%s' "$pid" "$ts" "$SEC" | sha1sum | cut -d' ' -f1)
        code=$(curl -sS -o /dev/null -w '%{http_code}' --max-time 300 \
            -F "file=@$file" -F "public_id=$pid" -F "timestamp=$ts" -F "type=authenticated" \
            -F "api_key=$AK" -F "signature=$sig" \
            "https://api.cloudinary.com/v1_1/$CN/raw/upload")
        [ "$code" = 200 ] || die "copia fuera de la VM falló para $(basename "$file") (HTTP $code)"
        echo "fuera de la VM: cloudinary:$pid"
    done
fi
echo "BACKUP_RESULT=ok"
