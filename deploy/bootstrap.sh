#!/usr/bin/env bash
set -euo pipefail

# =============================================================================
# deploy/bootstrap.sh — Bootstrap una VM Azure Ubuntu 24.04 LTS para VAIVÉN
# Idempotente: si se ejecuta dos veces no falla ni duplica nada.
# Soporta amd64 y arm64.
# =============================================================================

# -----------------------------------------------------------------------------
# Verifica que se ejecuta como root
# -----------------------------------------------------------------------------
if [ "$(id -u)" -ne 0 ]; then
    echo "Error: este script debe ejecutarse como root (o con sudo)." >&2
    exit 1
fi

# -----------------------------------------------------------------------------
# 1. User & directory -----------------------------------------------------------
# -----------------------------------------------------------------------------

# Crea el usuario 'deploy' solo si no existe (idempotencia)
if ! id -u deploy &>/dev/null; then
    useradd -m -s /bin/bash deploy
fi
# Añade al grupo docker después de instalar Docker (ver más abajo). Se hace aquí
# solo si el grupo ya existe para evitar errores en la primera corrida.
if getent group docker &>/dev/null; then
    usermod -aG docker deploy 2>/dev/null || true
fi

mkdir -p /opt/vaiven
# Asegura que 'deploy' es el dueño (idempotente: chown solo si distinto)
if [ "$(stat -c %U /opt/vaiven 2>/dev/null || echo root)" != "deploy" ]; then
    chown deploy:deploy /opt/vaiven
fi

# -----------------------------------------------------------------------------
# 2. Swap de 2 GB ---------------------------------------------------------------
# -----------------------------------------------------------------------------
SWAPFILE=/swapfile
SWAPSIZE=2G

# Solo crea el swap si no existe ya (idempotencia)
if [ ! -f "$SWAPFILE" ]; then
    fallocate -l "$SWAPSIZE" "$SWAPFILE" || dd if=/dev/zero of="$SWAPFILE" bs=1M count=2048 2>/dev/null
    chmod 600 "$SWAPFILE"
    mkswap "$SWAPFILE"
    swapon "$SWAPFILE"
    # Persistente en /etc/fstab (solo si no está ya)
    if ! grep -q "$SWAPFILE" /etc/fstab; then
        echo "/swapfile none swap sw 0 0" >> /etc/fstab
    fi
fi

# Asegura que el swap está activo ahora mismo (aunque ya estuviera en fstab)
swapon --show

# -----------------------------------------------------------------------------
# 3. Docker Engine desde el repositorio oficial (método nuevo, sin apt-key) -----
# -----------------------------------------------------------------------------

# Prerrequisitos: ca-certificates, curl
apt-get update -y
apt-get install -y ca-certificates curl

# Crea el keyring directory
install -m 0755 -d /etc/apt/keyrings

# Descarga la clave oficial de Docker (sobreescribir es idempotente)
curl -fsSL https://download.docker.com/linux/ubuntu/gpg -o /etc/apt/keyrings/docker.asc

# Ajusta permisos sobre el keyring
chmod a+r /etc/apt/keyrings/docker.asc

# Escribe el repositorio APT de Docker.
# Usa arch=$(dpkg --print-architecture) y el codename de /etc/os-release.
. /etc/os-release
echo \
  "deb [arch=$(dpkg --print-architecture) signed-by=/etc/apt/keyrings/docker.asc] \
   https://download.docker.com/linux/ubuntu \
   $VERSION_CODENAME stable" \
  > /etc/apt/sources.list.d/docker.list

# Actualiza e instala Docker Engine
apt-get update -y
apt-get install -y docker-ce docker-ce-cli containerd.io docker-buildx-plugin docker-compose-plugin

# -----------------------------------------------------------------------------
# 4. Grupo docker y usuario deploy ------------------------------------------------
# -----------------------------------------------------------------------------

# Crear el grupo docker si no existe, antes de agregar al usuario.
# Esto evita la llamada duplicada a usermod que había antes.
if ! getent group docker &>/dev/null; then
    groupadd docker
fi
# Agrega al usuario deploy al grupo docker (idempotente)
usermod -aG docker deploy 2>/dev/null || true

# -----------------------------------------------------------------------------
# 5. Directorio de secretos -----------------------------------------------------
# -----------------------------------------------------------------------------
mkdir -p -m 700 /opt/vaiven/secrets

# keyFile de MongoDB (lo monta deploy/base/docker-compose.yml). Debe existir ANTES
# del primer `docker compose up`: si falta, Docker crea una carpeta con ese nombre
# y mongod se niega a arrancar. El contenido nunca se imprime.
KEYFILE=/opt/vaiven/secrets/mongo-keyfile
if [ -d "$KEYFILE" ]; then
    rmdir "$KEYFILE"   # carpeta vacía dejada por Docker; rmdir falla si no está vacía
fi
if [ ! -f "$KEYFILE" ]; then
    (umask 077; openssl rand -base64 756 > "$KEYFILE")
fi
chown 999:999 "$KEYFILE"   # uid/gid de mongodb en la imagen oficial mongo:7
chmod 400 "$KEYFILE"

# -----------------------------------------------------------------------------
# 6. Actualizaciones de seguridad automáticas -----------------------------------
# -----------------------------------------------------------------------------
# Escribe el archivo de configuración APT aunque unattended-upgrades ya venga instalado.
cat > /etc/apt/apt.conf.d/20auto-upgrades <<'EOF'
APT::Get::AutomaticRemove "1";
APT::Periodic::Update-Package-Lists "1";
APT::Periodic::Unattended-Upgrade "1";
EOF

# Asegura que el paquete unattended-upgrades está instalado (puede que ya lo esté)
if ! dpkg -l | grep -q unattended-upgrades; then
    apt-get install -y unattended-upgrades
    dpkg-reconfigure -f noninteractive unattended-upgrades
fi

# -----------------------------------------------------------------------------
# 7. certbot (certificado de Let's Encrypt, ver deploy/base/tls.md) -------------
# -----------------------------------------------------------------------------
# El paquete de Ubuntu instala y activa el temporizador certbot.timer (renovación).
if ! command -v certbot &>/dev/null; then
    apt-get install -y certbot
fi

# -----------------------------------------------------------------------------
# 8. Resumen verificable ------------------------------------------------------
# -----------------------------------------------------------------------------
echo "========================================="
echo "Bootstrap completado — resumen verificable"
echo "========================================="
echo "Docker version: $(docker --version 2>/dev/null || echo 'N/A')"
echo "Docker compose version: $(docker compose version 2>/dev/null || echo 'N/A')"
echo "certbot: $(certbot --version 2>&1 || echo 'N/A')"
echo "Swap: $(swapon --show 2>/dev/null || echo 'N/A')"
echo "Memoria total: $(free -h 2>/dev/null || echo 'N/A')"
echo "Disk /: $(df -h / 2>/dev/null || echo 'N/A')"
echo "Usuario deploy: $(id deploy 2>/dev/null || echo 'N/A')"
echo "Directorio /opt/vaiven: $(ls -ld /opt/vaiven 2>/dev/null || echo 'N/A')"
echo "Directorio /opt/vaiven/secrets: $(ls -ld /opt/vaiven/secrets 2>/dev/null || echo 'N/A')"
echo "keyFile de MongoDB: $(stat -c '%F %u:%g %a' "$KEYFILE" 2>/dev/null || echo 'N/A')"
echo "========================================="