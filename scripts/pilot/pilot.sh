#!/usr/bin/env bash
# scripts/pilot/pilot.sh — orquesta el piloto de dos VMs de la spec 018 desde
# Azure Cloud Shell (docs/seguridad/piloto-red-vm2.md). Ventana máxima de 4 h.
#
#   bash scripts/pilot/pilot.sh ca          # CA efímera del piloto (solo en Cloud Shell)
#   bash scripts/pilot/pilot.sh vm1-up      # pasarela mTLS y relevo en VM1 (solo staging)
#   bash scripts/pilot/pilot.sh vm2-up      # certificados, Docker, imagen y API en VM2
#   bash scripts/pilot/pilot.sh test        # reparto, sesión entre réplicas y caída de VM2
#   bash scripts/pilot/pilot.sh down        # retira pasarela, relevo y CA (los recursos Azure: ver docs)
#
# Reglas:
#   - La llave de la CA nunca sale de Cloud Shell. Las llaves de VM1 y VM2 se
#     generan DENTRO de cada VM; por run-command solo viajan solicitudes y
#     certificados (públicos).
#   - MongoDB y Redis no cambian ni publican puertos. Producción no se toca: la
#     pasarela solo se une a la red de staging.
#   - Ninguna salida imprime secretos ni el .env.
set -euo pipefail

RG=${RG:-rg-vaiven}
VM1=${VM1:-vm-vaiven}
VM2=${VM2:-vaiven-pilot-p1-vm}
VM2_IP=${VM2_IP:-10.0.0.10}
LB_IP=${LB_IP:-10.0.0.20}
PILOT_DIR=${PILOT_DIR:-$HOME/vaiven-pilot}
CA_DAYS=1

die() { echo "pilot: $*" >&2; exit 1; }

# --- PKI (también la usa tests/pilot/run-local.sh) -------------------------

# ossl evita que Git Bash (Windows) convierta «/CN=…» en una ruta; en Linux no
# tiene efecto.
ossl() { MSYS2_ARG_CONV_EXCL='*' openssl "$@"; }

# pki_init crea la CA del piloto en $1 (llave EC P-256, válida 1 día).
pki_init() {
  local dir=$1
  mkdir -p "$dir" && chmod 700 "$dir"
  [ -f "$dir/ca.key" ] && return 0
  ossl ecparam -name prime256v1 -genkey -noout -out "$dir/ca.key" 2>/dev/null
  chmod 600 "$dir/ca.key"
  ossl req -x509 -new -key "$dir/ca.key" -sha256 -days "$CA_DAYS" -subj "/CN=vaiven-pilot-ca" \
    -addext "basicConstraints=critical,CA:TRUE" -addext "keyUsage=critical,keyCertSign" -out "$dir/ca.pem" 2>/dev/null
}

# pki_sign firma la solicitud $2 con la CA de $1. $3: server|client. $4: SAN
# (p. ej. "IP:10.0.0.4,DNS:vaiven-pilot-gw"); vacío para clientes.
pki_sign() {
  local dir=$1 csr=$2 kind=$3 san=${4:-} ext
  ext="$dir/.ext.$$" # junto a la CA: válido en Linux y en Git Bash
  if [ "$kind" = server ]; then
    printf 'extendedKeyUsage=serverAuth\nkeyUsage=critical,digitalSignature\nsubjectAltName=%s\n' "$san" >"$ext"
  else
    printf 'extendedKeyUsage=clientAuth\nkeyUsage=critical,digitalSignature\n' >"$ext"
  fi
  ossl x509 -req -in "$csr" -CA "$dir/ca.pem" -CAkey "$dir/ca.key" -CAcreateserial \
    -days "$CA_DAYS" -sha256 -extfile "$ext" 2>/dev/null
  rm -f "$ext"
}

# rewrite_env adapta el .env de staging para VM2: MongoDB y Redis por los
# túneles locales (127.0.0.1) y conexión directa (el replica set anuncia el
# nombre interno «mongo»). Lee de stdin y escribe a stdout; no imprime nada más.
rewrite_env() {
  sed -E \
    -e '/^(TLS_CERT_FILE|TLS_KEY_FILE|HTTP_ADDR|APP_ENV)=/d' \
    -e 's#^(MONGO_URI=mongodb://([^@/]*@)?)mongo:27017#\1127.0.0.1:27017#' \
    -e '/^MONGO_URI=/{s#replicaSet=[^&]*&?##; s#[?&]$##; /\?/!s#$#?directConnection=true#; /directConnection/!s#$#\&directConnection=true#}' \
    -e 's#^(REDIS_URL=redis://([^@/]*@)?)redis:6379#\1127.0.0.1:6379#'
}

# --- run-command -----------------------------------------------------------

# vm_run ejecuta el script $2 en la VM $1 y devuelve su salida (sin la
# cabecera de Azure). Los argumentos nunca llevan secretos.
vm_run() {
  local vm=$1 script=$2
  # Sin esta línea, run-command usa sh (dash), que rechaza «set -o pipefail» y
  # termina sin imprimir nada (deploy-staging.yml también la antepone).
  sed -i '1i #!/usr/bin/env bash' "$script"
  az vm run-command invoke -g "$RG" -n "$vm" --command-id RunShellScript --scripts "@$script" \
    --query 'value[0].message' -o tsv
}

between() { sed -n "/^$1-BEGIN\$/,/^$1-END\$/p" | sed '1d;$d'; }

vm1_ip() { az vm list-ip-addresses -g "$RG" -n "$VM1" --query "[0].virtualMachine.network.privateIpAddresses[0]" -o tsv; }

# --- pasos -------------------------------------------------------------------

cmd_ca() {
  pki_init "$PILOT_DIR/ca"
  echo "CA del piloto lista en $PILOT_DIR/ca (vence en $CA_DAYS día; la llave no sale de Cloud Shell)"
}

cmd_vm1_up() {
  [ -f "$PILOT_DIR/ca/ca.key" ] || die "primero: pilot.sh ca"
  local ip s1 s2 csr cert ca_b64 cert_b64 compose_b64
  ip=$(vm1_ip)
  s1=$(mktemp)
  cat >"$s1" <<EOF
set -euo pipefail
. /etc/os-release
[ "\$ID" = ubuntu ] && [ "\$VERSION_ID" = 24.04 ] || { echo "OS-NO-SOPORTADO \$ID \$VERSION_ID"; exit 1; }
# 65532 = usuario nonroot de la imagen; no existe en la VM, por eso chown numérico.
install -d -m 750 /opt/vaiven/pilot/gw && chown 65532:65532 /opt/vaiven/pilot/gw
install -d -m 755 /opt/vaiven/pilot/relay
cd /opt/vaiven/pilot/gw
[ -f gw.key ] || openssl ecparam -name prime256v1 -genkey -noout -out gw.key 2>/dev/null
chown 65532:65532 gw.key && chmod 400 gw.key
echo CSR-BEGIN
openssl req -new -key gw.key -subj "/CN=vaiven-pilot-gw" 2>/dev/null
echo CSR-END
EOF
  csr=$(mktemp)
  vm_run "$VM1" "$s1" | between CSR >"$csr"
  grep -q 'BEGIN CERTIFICATE REQUEST' "$csr" || die "VM1 no devolvió la solicitud (¿Ubuntu 24.04?)"
  cert=$(pki_sign "$PILOT_DIR/ca" "$csr" server "IP:$ip,DNS:vaiven-pilot-gw")
  cert_b64=$(printf '%s\n' "$cert" | base64 -w0)
  ca_b64=$(base64 -w0 "$PILOT_DIR/ca/ca.pem")
  compose_b64=$(base64 -w0 deploy/pilot/gateway.compose.yml)

  s2=$(mktemp)
  cat >"$s2" <<EOF
set -euo pipefail
G=/opt/vaiven/pilot/gw R=/opt/vaiven/pilot/relay
echo '$cert_b64' | base64 -d >\$G/gw.pem
echo '$ca_b64' | base64 -d >\$G/ca.pem
echo '$compose_b64' | base64 -d >/opt/vaiven/pilot/gateway.compose.yml
chown 65532:65532 \$G/gw.pem \$G/ca.pem && chmod 444 \$G/gw.pem \$G/ca.pem
TAG=\$(cat /opt/vaiven/staging/current-tag)
IMG=vaiven-api:\$TAG
docker image inspect "\$IMG" >/dev/null
# Relevo: la imagen que staging ya verificó por firma y digest, y los paquetes de Docker.
docker save "\$IMG" | gzip -1 >\$R/image.tar.gz
docker image inspect -f '{{.Id}}' "\$IMG" >\$R/image.id
echo "\$IMG" >\$R/image.ref
rm -rf \$R/debs && mkdir -p \$R/debs && cd \$R/debs
apt-get download docker-ce docker-ce-cli containerd.io docker-compose-plugin >/dev/null 2>&1 \
  || apt-get download docker.io docker-compose-v2 containerd runc >/dev/null 2>&1
cd \$R && sha256sum image.tar.gz debs/*.deb >SHA256SUMS
# Configuración de la API para VM2 (no se imprime).
$(declare -f rewrite_env)
rewrite_env </opt/vaiven/staging/.env >\$R/api.env
chown -R 65532:65532 \$R && chmod 400 \$R/api.env
cd /opt/vaiven/pilot
# Exportadas: Compose las necesita en cada llamada (up y ps), no solo en up.
export PILOT_IMAGE="\$IMG" VM1_PRIVATE_IP=$ip
docker compose -f gateway.compose.yml up -d
docker compose -f gateway.compose.yml ps --format '{{.Service}} {{.State}}'
echo "relevo: \$(ls \$R/debs | wc -l) paquetes, imagen \$TAG"
EOF
  vm_run "$VM1" "$s2" | grep -vE 'api.env|PASSWORD|SECRET|TOKEN'
  rm -f "$s1" "$s2" "$csr"
}

cmd_vm2_up() {
  [ -f "$PILOT_DIR/ca/ca.key" ] || die "primero: pilot.sh ca y vm1-up"
  local ip s1 s2 out c1 c2 vm2_cert api_cert ca_b64 compose_b64
  ip=$(vm1_ip)
  s1=$(mktemp)
  cat >"$s1" <<'EOF'
set -euo pipefail
install -d -m 750 /opt/vaiven/pilot/certs
cd /opt/vaiven/pilot/certs
for k in vm2 api; do [ -f $k.key ] || openssl ecparam -name prime256v1 -genkey -noout -out $k.key 2>/dev/null; chmod 400 $k.key; done
echo VM2-BEGIN; openssl req -new -key vm2.key -subj "/CN=vaiven-pilot-vm2" 2>/dev/null; echo VM2-END
echo API-BEGIN; openssl req -new -key api.key -subj "/CN=vaiven-pilot-api" 2>/dev/null; echo API-END
EOF
  out=$(vm_run "$VM2" "$s1")
  c1=$(mktemp) c2=$(mktemp)
  printf '%s\n' "$out" | between VM2 >"$c1"
  printf '%s\n' "$out" | between API >"$c2"
  grep -q 'CERTIFICATE REQUEST' "$c1" || die "VM2 no devolvió la solicitud (¿run-command disponible?)"
  vm2_cert=$(pki_sign "$PILOT_DIR/ca" "$c1" client | base64 -w0)
  api_cert=$(pki_sign "$PILOT_DIR/ca" "$c2" server "IP:$VM2_IP,IP:$LB_IP,DNS:vaiven-pilot-api" | base64 -w0)
  ca_b64=$(base64 -w0 "$PILOT_DIR/ca/ca.pem")
  compose_b64=$(base64 -w0 deploy/pilot/vm2.compose.yml)

  s2=$(mktemp)
  cat >"$s2" <<EOF
set -euo pipefail
C=/opt/vaiven/pilot/certs W=/opt/vaiven/pilot/dl
echo '$vm2_cert' | base64 -d >\$C/vm2.pem
echo '$api_cert' | base64 -d >\$C/api.pem
echo '$ca_b64' | base64 -d >\$C/ca.pem
echo '$compose_b64' | base64 -d >/opt/vaiven/pilot/vm2.compose.yml
chown -R 65532:65532 \$C && chmod 444 \$C/*.pem
mkdir -p \$W && cd \$W
# Los .deb de Docker llevan la época codificada («5%3a28…»): el % va como %25
# en la URL o el relevo lo decodifica y responde 404.
get() { curl -fsS --tlsv1.3 --cacert \$C/ca.pem --cert \$C/vm2.pem --key \$C/vm2.key --resolve vaiven-pilot-gw:8444:$ip "https://vaiven-pilot-gw:8444/\${1//%/%25}" -o "\$1"; }
mkdir -p debs
get SHA256SUMS; get image.tar.gz; get image.id; get image.ref; get api.env
for d in \$(awk '{print \$2}' SHA256SUMS | grep '^debs/'); do get "\$d"; done
sha256sum -c SHA256SUMS >/dev/null && echo "relevo verificado (sha256)"
command -v docker >/dev/null || DEBIAN_FRONTEND=noninteractive dpkg -i debs/*.deb >/dev/null
docker load -i image.tar.gz >/dev/null
[ "\$(docker image inspect -f '{{.Id}}' "\$(cat image.ref)")" = "\$(cat image.id)" ] && echo "imagen idéntica a la verificada en VM1"
install -m 400 api.env /opt/vaiven/pilot/api.env && chown 65532:65532 /opt/vaiven/pilot/api.env && rm -f api.env
cd /opt/vaiven/pilot
PILOT_IMAGE="\$(cat \$W/image.ref)" VM1_PRIVATE_IP=$ip docker compose -f vm2.compose.yml up -d
sleep 8
curl -fsS --cacert \$C/ca.pem --resolve vaiven-pilot-api:8443:127.0.0.1 https://vaiven-pilot-api:8443/readyz
EOF
  vm_run "$VM2" "$s2" | grep -vE 'PASSWORD|SECRET|TOKEN'
  rm -f "$s1" "$s2" "$c1" "$c2"
}

cmd_test() {
  local s
  s=$(mktemp)
  cat >"$s" <<EOF
set -euo pipefail
LB=https://$LB_IP:8443
run=\$(date +%s)
ok=0; fail=0
for i in \$(seq 1 40); do
  if curl -fsSk -o /dev/null -H "X-Request-Id: pilot-\$run-\$i" \$LB/api/v1/meta; then ok=\$((ok+1)); else fail=\$((fail+1)); fi
done
echo "reparto: \$ok correctas, \$fail fallidas (por el balanceador)"
echo "VM1 atendió: \$(docker logs --since 5m vaiven-staging-api-1 2>&1 | grep -c "pilot-\$run-")"
echo "PILOT_RUN=\$run"
EOF
  vm_run "$VM1" "$s"
  rm -f "$s"
  echo "Para el reparto de VM2: az vm run-command en VM2 con 'docker logs vaiven-pilot-api-1 | grep -c pilot-<run>'."
  echo "Sesiones entre réplicas y caída: ver docs/seguridad/piloto-red-vm2.md §6."
}

cmd_down() {
  local s
  s=$(mktemp)
  cat >"$s" <<'EOF'
set -euo pipefail
if [ -f /opt/vaiven/pilot/gateway.compose.yml ]; then
  cd /opt/vaiven/pilot && PILOT_IMAGE=x VM1_PRIVATE_IP=127.0.0.1 docker compose -f gateway.compose.yml down
fi
rm -rf /opt/vaiven/pilot
docker ps --format '{{.Names}}' | grep -c vaiven-pilot-gw || true
echo "pasarela y relevo retirados de VM1 (MongoDB y Redis sin cambios)"
EOF
  vm_run "$VM1" "$s"
  rm -f "$s"
  rm -rf "$PILOT_DIR"
  echo "CA del piloto borrada de Cloud Shell."
  echo "Recursos de Azure del piloto (borrar por ID tras confirmar):"
  az resource list -g "$RG" --query "[?tags.purpose=='spec-018-pilot'].{name:name, type:type, id:id}" -o table
}

case "${1:-}" in
  ca) cmd_ca ;;
  vm1-up) cmd_vm1_up ;;
  vm2-up) cmd_vm2_up ;;
  test) cmd_test ;;
  down) cmd_down ;;
  __lib) ;; # solo carga las funciones (pruebas)
  *) die "uso: pilot.sh ca|vm1-up|vm2-up|test|down" ;;
esac
