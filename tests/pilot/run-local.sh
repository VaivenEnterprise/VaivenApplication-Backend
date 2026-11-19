#!/usr/bin/env bash
# tests/pilot/run-local.sh — prueba LOCAL del camino de datos del piloto de dos
# VMs (spec 018): PKI y .env de scripts/pilot/pilot.sh, túneles mTLS de la
# imagen real y la API de «VM2» usando MongoDB y Redis de «VM1» sin que estos
# publiquen puertos. Necesita la imagen localhost/vaiven-api:pilot.
set -euo pipefail
cd "$(dirname "$0")/../.."
ENGINE=${ENGINE:-podman}
COMPOSE="$ENGINE compose -f tests/pilot/local.compose.yml"

# Funciones de PKI y .env del orquestador real.
# shellcheck source=/dev/null
. <(sed 's/^case "\${1:-}" in$/case __lib in/' scripts/pilot/pilot.sh)

PKI_DIR=$(mktemp -d)
# En Git Bash (Windows), Compose y openssl necesitan la ruta en formato C:/…
command -v cygpath >/dev/null && PKI_DIR=$(cygpath -m "$PKI_DIR")
export PKI_DIR
trap '$COMPOSE down -v >/dev/null 2>&1 || true; rm -rf "$PKI_DIR"' EXIT

pki_init "$PKI_DIR"
key_csr() { # $1 nombre, $2 CN
  ossl ecparam -name prime256v1 -genkey -noout -out "$PKI_DIR/$1.key" 2>/dev/null
  ossl req -new -key "$PKI_DIR/$1.key" -subj "/CN=$2" -out "$PKI_DIR/$1.csr" 2>/dev/null
}
key_csr gw vaiven-pilot-gw
key_csr vm2 vaiven-pilot-vm2
key_csr rogue otra-vm
pki_sign "$PKI_DIR" "$PKI_DIR/gw.csr" server "IP:172.31.252.4,IP:172.31.252.5,DNS:vaiven-pilot-gw" >"$PKI_DIR/gw.pem"
pki_sign "$PKI_DIR" "$PKI_DIR/vm2.csr" client >"$PKI_DIR/vm2.pem"
pki_sign "$PKI_DIR" "$PKI_DIR/rogue.csr" client >"$PKI_DIR/rogue.pem"
chmod 644 "$PKI_DIR"/*.pem "$PKI_DIR"/*.key # contenedores sin root, solo en la prueba

# .env «de staging» con nombres internos, reescrito como en VM1.
printf '%s\n' \
  "MONGO_URI=mongodb://mongo:27017/vaiven_pilot?replicaSet=rs0" \
  "MONGO_DB=vaiven_pilot" \
  "REDIS_URL=redis://:local-pilot-pass@redis:6379/0" \
  "JWT_ACCESS_SECRET=$(openssl rand -hex 32)" \
  "TLS_CERT_FILE=/certs/no-usar" | rewrite_env >"$PKI_DIR/api.env"
grep -q '127.0.0.1:27017' "$PKI_DIR/api.env" && grep -q 'directConnection=true' "$PKI_DIR/api.env"

$COMPOSE up -d >/dev/null 2>&1
fail=0
check() { if eval "$2"; then echo "OK   $1"; else echo "FALLA $1"; fail=1; fi; }
probe() { $ENGINE run --rm --network container:pilot-vm2 docker.io/library/busybox:1.37 "$@"; }

ready=""
for _ in $(seq 1 40); do
  ready=$(probe wget -qO- http://127.0.0.1:8080/readyz 2>/dev/null || true)
  case "$ready" in *'"ready"'*) break ;; esac
  sleep 2
done
check "VM2: /readyz listo con MongoDB y Redis por los túneles mTLS" '[[ "$ready" == *"\"status\":\"ready\""* && "$ready" == *"\"redis\":\"ok\""* ]]'

reg=$(probe wget -qO- --header 'Content-Type: application/json' \
  --post-data '{"email":"piloto@vaiven.invalid","password":"una-contrasena-larga","displayName":"Piloto","acceptTerms":true,"isAdult":true}' \
  http://127.0.0.1:8080/api/v1/auth/register 2>/dev/null || true)
check "VM2: registro escribe en el MongoDB de VM1 (transacción por el túnel)" '[[ "$reg" == *accessToken* ]]'

check "intruso: MongoDB de VM1 no es alcanzable desde la subred" '! $ENGINE exec pilot-intruder nc -z -w 2 mongo 27017 2>/dev/null'
check "intruso: Redis de VM1 no es alcanzable desde la subred" '! $ENGINE exec pilot-intruder nc -z -w 2 redis 6379 2>/dev/null'
before=$($ENGINE logs pilot-gw-mongo 2>&1 | grep -c 'conexión rechazada' || true)
$ENGINE exec pilot-intruder sh -c 'printf "hola\n" | nc -w 2 172.31.252.4 27018' >/dev/null 2>&1 || true
after=$($ENGINE logs pilot-gw-mongo 2>&1 | grep -c 'conexión rechazada' || true)
check "intruso: la pasarela rechaza TCP sin certificado de cliente" '[ "$after" -gt "$before" ]'

echo
[ "$fail" -eq 0 ] && echo "Camino de datos del piloto: todo en verde" || { echo "Hay fallas"; exit 1; }
