#!/usr/bin/env bash
# scripts/check-logs.sh — cuenta, en los registros recientes de la API de un
# entorno, las apariciones de textos que nunca deben registrarse (CA-06 de la
# 013; RF-07 y RF-18 de la 014). Solo imprime conteos, nunca líneas de registro.
# Se ejecuta en Azure Cloud Shell (Bash):
#
#   bash check-logs.sh staging [desde] [texto extra...]
#   bash check-logs.sh staging 2h librero videojuegos
#
# Todas las cuentas deben ser 0, salvo `lineas` y `rutas_ai`.
set -euo pipefail

RG=${RG:-rg-vaiven}
VM=${VM:-vm-vaiven}
die() { echo "check-logs: $*" >&2; exit 1; }
[ $# -ge 1 ] || die "uso: $0 <staging|production> [desde] [texto extra...]"
ENV_NAME=$1
SINCE=${2:-1h}
shift $(($# >= 2 ? 2 : 1))
case "$ENV_NAME" in staging | production) ;; *) die "entorno inválido: $ENV_NAME" ;; esac
[[ "$SINCE" =~ ^[0-9]+[smh]$ ]] || die "desde inválido: $SINCE (p. ej. 30m o 2h)"
command -v az >/dev/null || die "falta az (usa Azure Cloud Shell)"

EXTRA=""
for t in "$@"; do
    [[ "$t" =~ ^[[:alnum:]._@-]+$ ]] || die "texto extra inválido: $t (solo letras, números y . _ @ -)"
    EXTRA="$EXTRA $t"
done

az vm run-command invoke -g "$RG" -n "$VM" --command-id RunShellScript --query "value[0].message" -o tsv --scripts "#!/usr/bin/env bash
C=\$(docker ps --format '{{.Names}}' | grep -E '^vaiven-$ENV_NAME-api' | head -1)
[ -n \"\$C\" ] || { echo 'no hay contenedor de la API de $ENV_NAME'; exit 1; }
echo \"contenedor=\$C imagen=\$(docker inspect -f '{{.Config.Image}}' \"\$C\")\"
L=\$(docker logs --since $SINCE \"\$C\" 2>&1)
echo \"lineas=\$(printf '%s\n' \"\$L\" | wc -l) rutas_ai=\$(printf '%s\n' \"\$L\" | grep -c '/api/v1/ai/')\"
for p in password 'Bearer ' refreshToken accessToken api_secret gsk_ sk-or- AI_API_KEY SUGGEST_API_KEY @vaiven.invalid$EXTRA; do
  printf '%-24s %s\n' \"\$p\" \"\$(printf '%s\n' \"\$L\" | grep -cF -- \"\$p\")\"
done"
