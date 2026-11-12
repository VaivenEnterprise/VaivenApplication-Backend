#!/usr/bin/env bash
# deploy/production/provision-data.sh — Crea en la VM el MongoDB y el Redis de
# PRODUCCIÓN y el .env de la API (spec 013, T001; Art. XIII.6). Sin SSH: lo
# ejecuta `az vm run-command invoke` desde Cloud Shell (docs/produccion.md §2),
# que antepone dos variables:
#   INIT_B64     deploy/production/init-env.sh en base64
#   COMPOSE_B64  deploy/production/docker-compose.yml en base64
# Idempotente: no pisa contraseñas, keyFile, usuario ni .env existentes.
# Nunca imprime valores secretos: solo estados y nombres de variables.
set -eu
D=/opt/vaiven/production
log() { echo "[prod-data] $*"; }

# 1. Contraseñas y keyFile propios (idempotente; no imprime valores).
printf '%s' "$INIT_B64" | base64 -d >/tmp/vaiven-init-env.sh
bash /tmp/vaiven-init-env.sh
rm -f /tmp/vaiven-init-env.sh

# 2. Stack de datos de producción.
install -d -m 0750 "$D/data"
printf '%s' "$COMPOSE_B64" | base64 -d >"$D/data/docker-compose.yml"
cd "$D/data"
docker compose --env-file "$D/data.env" up -d 2>&1 | tail -4
for _ in $(seq 1 40); do
    s=$(docker inspect -f '{{.State.Health.Status}}' vaiven-production-data-mongo-1 2>/dev/null || true)
    [ "$s" = healthy ] && break
    sleep 3
done
log "mongo: ${s:-sin estado}"

# 3. Replica set y usuario de la aplicación (contraseñas por variable de entorno).
set -a; . "$D/data.env"; set +a
docker exec -i -e MP="$MONGO_ROOT_PASSWORD" -e AP="$MONGO_APP_PASSWORD" vaiven-production-data-mongo-1 \
    mongosh --quiet --nodb <<"JS"
const c = connect("mongodb://root:" + process.env.MP + "@localhost:27017/admin?directConnection=true");
let rs = 0;
try { rs = c.adminCommand({ replSetGetStatus: 1 }).ok; } catch (e) { rs = 0; }
if (!rs) {
  const r = c.adminCommand({ replSetInitiate: { _id: "rs0", members: [{ _id: 0, host: "mongo:27017" }] } });
  print("replSetInitiate ok=" + r.ok);
} else { print("replica set ya iniciado"); }
for (let i = 0; i < 30; i++) {
  try { if (c.adminCommand({ hello: 1 }).isWritablePrimary) break; } catch (e) {}
  sleep(1000);
}
print("primario: " + c.adminCommand({ hello: 1 }).isWritablePrimary);
const p = c.getSiblingDB("vaiven_prod");
if (!p.getUser("vaiven_app")) {
  p.createUser({ user: "vaiven_app", pwd: process.env.AP, roles: [{ role: "readWrite", db: "vaiven_prod" }] });
  print("usuario vaiven_app creado (readWrite solo en vaiven_prod)");
} else { print("usuario vaiven_app ya existia"); }
JS

# 4. .env de la API de producción (solo si no existe). Parte del de staging,
#    pero MongoDB, Redis y JWT son nuevos (CA-02) y la carpeta de Cloudinary propia.
if [ -e "$D/.env" ]; then
    log "EXISTE $D/.env (no se toca)"
else
    T=$(mktemp)
    grep -vE '^(APP_ENV|VAIVEN_NETWORK|MONGO_URI|MONGO_DB|REDIS_URL|JWT_ACCESS_SECRET|CLOUDINARY_BASE_FOLDER|DOCS_ENABLED)=' \
        /opt/vaiven/staging/.env >"$T"
    {
        echo "VAIVEN_NETWORK=vaiven-production-net"
        printf 'MONGO_URI=mongodb://vaiven_app:%s@mongo:27017/vaiven_prod?replicaSet=rs0&authSource=vaiven_prod\n' "$MONGO_APP_PASSWORD"
        echo "MONGO_DB=vaiven_prod"
        printf 'REDIS_URL=redis://:%s@redis:6379/0\n' "$REDIS_PASSWORD"
        printf 'JWT_ACCESS_SECRET=%s\n' "$(openssl rand -hex 32)"
        echo "CLOUDINARY_BASE_FOLDER=vaiven/production"
        echo "DOCS_ENABLED=false"
    } >>"$T"
    install -m 0600 -o deploy -g deploy "$T" "$D/.env"
    rm -f "$T"
    log "CREADO $D/.env (600, deploy)"
fi
log "variables copiadas de staging sin cambio (solo nombres):"
grep -vE '^(#|$|APP_ENV=|VAIVEN_NETWORK=|MONGO_URI=|MONGO_DB=|REDIS_URL=|JWT_ACCESS_SECRET=|CLOUDINARY_BASE_FOLDER=|DOCS_ENABLED=)' \
    "$D/.env" | cut -d= -f1 | tr '\n' ' '; echo

# 5. Estado y memoria.
docker ps --filter name=vaiven-production-data --format '{{.Names}} {{.Status}}'
docker stats --no-stream --format '{{.Name}} {{.MemUsage}}'
free -m | sed -n 1,3p
