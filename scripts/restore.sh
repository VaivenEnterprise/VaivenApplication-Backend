#!/usr/bin/env bash
# scripts/restore.sh — restaura un respaldo cifrado de scripts/backup.sh y
# comprueba colecciones y conteos (spec 013, T006; CA-04). En la VM, como root:
#
#   bash restore.sh <ruta.enc | cloudinary:<public_id>> [--db vaiven_restore_check] [--keep]
#
# - Restaura SIEMPRE en el MongoDB de staging y en una base de comprobación
#   (por omisión vaiven_restore_check): nunca pisa vaiven_staging ni toca
#   producción. El nombre de la base debe empezar por vaiven_restore_.
# - cloudinary:<public_id> descarga la copia de fuera de la VM (y su
#   .counts.json) con las credenciales de staging (misma cuenta).
# - Compara cada colección con <archivo>.counts.json y sale con 1 si difiere.
# - Sin --keep, borra la base de comprobación al terminar.
# La recuperación real de producción (desastre) está en docs/respaldos.md §4.
set -euo pipefail

VAIVEN_DIR=${VAIVEN_DIR:-/opt/vaiven}
DOCKER=${DOCKER:-docker}
C=${MONGO_CONTAINER:-base-mongo-1}

die() { echo "restore: $*" >&2; exit 1; }
getv() { grep -m1 "^$1=" "$2" | cut -d= -f2- || true; }

[ $# -ge 1 ] || die "uso: $0 <ruta.enc | cloudinary:<public_id>> [--db vaiven_restore_x] [--keep]"
SRC=$1; shift
DST=vaiven_restore_check
KEEPDB=false
while [ $# -gt 0 ]; do
    case "$1" in
        --db) DST=${2:-}; shift 2 ;;
        --keep) KEEPDB=true; shift ;;
        *) die "opción desconocida: $1" ;;
    esac
done
case "$DST" in vaiven_restore_*) ;; *) die "la base destino debe empezar por vaiven_restore_ (no se pisa staging ni producción)" ;; esac
case "$DST" in *[!a-z0-9_]*) die "nombre de base inválido: $DST" ;; esac

KEY=$VAIVEN_DIR/backup/backup.key
[ -s "$KEY" ] || die "falta la frase $KEY"
ROOTPW=$(getv MONGO_ROOT_PASSWORD "$VAIVEN_DIR/base/.env")
[ -n "$ROOTPW" ] || die "no se encontró MONGO_ROOT_PASSWORD de staging"

umask 077
TMP=$(mktemp -d)
cleanup() {
    rm -rf "$TMP"
    "$DOCKER" exec "$C" rm -f /tmp/vaiven-restore.yml >/dev/null 2>&1 || true
}
trap cleanup EXIT

case "$SRC" in
    cloudinary:*)
        ENVF=$VAIVEN_DIR/staging/.env
        CN=$(getv CLOUDINARY_CLOUD_NAME "$ENVF"); AK=$(getv CLOUDINARY_API_KEY "$ENVF"); SEC=$(getv CLOUDINARY_API_SECRET "$ENVF")
        [ -n "$CN" ] && [ -n "$AK" ] && [ -n "$SEC" ] || die "faltan credenciales de Cloudinary en $ENVF"
        pid=${SRC#cloudinary:}
        F=$TMP/$(basename "$pid")
        for p in "$pid" "$pid.counts.json"; do
            ts=$(date +%s)
            sig=$(printf 'public_id=%s&timestamp=%s&type=authenticated%s' "$p" "$ts" "$SEC" | sha1sum | cut -d' ' -f1)
            out=$TMP/$(basename "$p")
            code=$(curl -sS -G -o "$out" -w '%{http_code}' --max-time 300 \
                --data-urlencode "public_id=$p" --data-urlencode "timestamp=$ts" \
                --data-urlencode "type=authenticated" --data-urlencode "api_key=$AK" \
                --data-urlencode "signature=$sig" \
                "https://api.cloudinary.com/v1_1/$CN/raw/download")
            [ "$code" = 200 ] || die "no se pudo descargar $(basename "$p") de Cloudinary (HTTP $code)"
        done
        echo "descargado de fuera de la VM: $(basename "$pid")"
        ;;
    *)
        F=$SRC
        [ -f "$F" ] || die "no existe $F"
        if [ -f "$F.sha256" ]; then
            [ "$(sha256sum "$F" | cut -d' ' -f1)" = "$(cat "$F.sha256")" ] || die "el sha256 de $F no coincide"
            echo "sha256 verificado"
        fi
        ;;
esac
[ -f "$F.counts.json" ] || die "falta $F.counts.json"
MANIFEST=$(cat "$F.counts.json")
SRCDB=$(printf '%s' "$MANIFEST" | sed -n 's/.*"db":"\([a-z0-9_]*\)".*/\1/p')
[ -n "$SRCDB" ] || die "no se pudo leer la base de origen del manifiesto"

printf 'uri: "mongodb://root@localhost:27017/?authSource=admin&directConnection=true"\npassword: "%s"\n' "$ROOTPW" |
    "$DOCKER" exec -i "$C" sh -c 'umask 077; cat >/tmp/vaiven-restore.yml'
openssl enc -d -aes-256-cbc -pbkdf2 -iter 200000 -pass "file:$KEY" -in "$F" |
    "$DOCKER" exec -i "$C" mongorestore --quiet --config /tmp/vaiven-restore.yml --archive --gzip \
        --nsFrom "$SRCDB.*" --nsTo "$DST.*" --drop ||
    die "falló la restauración"
echo "restaurado $SRCDB -> $DST (MongoDB de staging)"

CHECK_JS='const d = connect("mongodb://root:" + process.env.MP + "@localhost:27017/admin?directConnection=true").getSiblingDB(process.env.DST);
const want = JSON.parse(process.env.M).counts;
const got = {};
d.getCollectionNames().forEach((c) => { got[c] = d.getCollection(c).countDocuments({}); });
let bad = 0;
new Set([...Object.keys(want), ...Object.keys(got)]).forEach((c) => {
  const ok = want[c] === got[c];
  if (!ok) bad++;
  print((ok ? "OK        " : "DIFERENTE ") + c + ": respaldo " + want[c] + ", restaurado " + got[c]);
});
print("resumen: " + Object.keys(want).length + " colecciones en el respaldo, " + bad + " diferencias");
if (process.env.DROP === "si") { d.dropDatabase(); print("base " + process.env.DST + " borrada"); }
if (bad !== 0) { throw new Error("hay diferencias"); }'
set +e
"$DOCKER" exec -e MP="$ROOTPW" -e DST="$DST" -e M="$MANIFEST" -e DROP="$($KEEPDB && echo no || echo si)" "$C"     mongosh --quiet --nodb --eval "$CHECK_JS"
rc=$?
set -e
[ $rc -eq 0 ] && echo "RESTORE_RESULT=ok" || { echo "RESTORE_RESULT=diferencias"; exit 1; }
