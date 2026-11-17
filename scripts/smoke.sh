#!/usr/bin/env bash
# Pruebas de humo de un despliegue de la API de VAIVÉN (spec 017, T010).
#
# Uso:   scripts/smoke.sh <url-base>
# Ej.:   scripts/smoke.sh https://vaiven-staging.westus2.cloudapp.azure.com:8443
#
# Comprueba por HTTPS, validando el certificado (nunca -k):
#   1. GET /healthz      -> 200 y {"status":"ok"}
#   2. GET /readyz       -> 200 y {"status":"ready"}
#   3. GET /api/v1/meta  -> 200 con name, version, environment y serverTime
#   4. GET /<inexistente> -> 404 con el envoltorio del Art. IV
#      {"error":{"code","message","requestId"}} y X-Request-Id igual al requestId.
#
# Variables opcionales:
#   SMOKE_EXPECT_ENV  entorno esperado en /api/v1/meta (p. ej. staging).
#   SMOKE_ALLOW_HTTP  =1 permite http:// (solo para la API levantada en local).
#   SMOKE_TIMEOUT     segundos por petición (10 por defecto).
#
# Usa jq si está instalado; si no, valida el JSON con expresiones regulares.
# Sale con 0 si todo pasa y con 1 si falla alguna comprobación.
set -euo pipefail

usage() {
  echo "Uso: $0 <url-base>   (p. ej. https://host:8443)" >&2
  exit 2
}

[[ $# -eq 1 ]] || usage
base_url=${1%/}
timeout=${SMOKE_TIMEOUT:-10}

case $base_url in
  https://*) ;;
  http://*)
    if [[ ${SMOKE_ALLOW_HTTP:-0} != 1 ]]; then
      echo "La URL debe usar https:// (Art. XIV). Para pruebas locales: SMOKE_ALLOW_HTTP=1." >&2
      exit 2
    fi
    ;;
  *) usage ;;
esac

command -v curl >/dev/null || { echo "Falta curl." >&2; exit 2; }
have_jq=0
command -v jq >/dev/null && have_jq=1

work_dir=$(mktemp -d)
trap 'rm -rf "$work_dir"' EXIT

failures=0
pass() { printf '  OK    %s\n' "$1"; }
fail() { printf '  FALLO %s\n' "$1" >&2; failures=$((failures + 1)); }

# request <ruta> [encabezado...]: deja el cuerpo en $body, los encabezados en
# $headers_file y el código en $status. Reintenta errores de red transitorios.
request() {
  local path=$1
  shift
  local args=(--silent --show-error --max-time "$timeout" --retry 2 --retry-delay 2
    --output "$work_dir/body" --dump-header "$work_dir/headers" --write-out '%{http_code}')
  if [[ $base_url == https://* ]]; then
    args+=(--proto '=https' --tlsv1.2)
  fi
  local h
  for h in "$@"; do args+=(--header "$h"); done
  if ! status=$(curl "${args[@]}" "$base_url$path"); then
    status=000
  fi
  body=$(cat "$work_dir/body" 2>/dev/null || true)
  headers_file=$work_dir/headers
}

# json_string <filtro-jq> <clave>: valor de una cadena del JSON en $body.
# Sin jq, busca la primera aparición de "clave":"valor".
json_string() {
  if [[ $have_jq == 1 ]]; then
    jq -er "$1 | strings" <<<"$body" 2>/dev/null || true
  else
    sed -nE 's/.*"'"$2"'"[[:space:]]*:[[:space:]]*"([^"]*)".*/\1/p' <<<"$body" | head -n 1
  fi
}

header_value() {
  # Encabezados sin distinguir mayúsculas; se quita el \r final de HTTP/1.1.
  { grep -i "^$1:" "$headers_file" || true; } | tail -n 1 | cut -d: -f2- | tr -d '\r' | sed -E 's/^[[:space:]]+//'
}

echo "Pruebas de humo contra $base_url"

# 1. /healthz
request /healthz
if [[ $status == 200 && $(json_string '.status' status) == ok ]]; then
  pass "/healthz 200 status=ok"
else
  fail "/healthz: código $status, cuerpo: ${body:0:200}"
fi

# 2. /readyz
request /readyz
if [[ $status == 200 && $(json_string '.status' status) == ready ]]; then
  pass "/readyz 200 status=ready"
else
  fail "/readyz: código $status, cuerpo: ${body:0:200}"
fi

# 3. /api/v1/meta
request /api/v1/meta
meta_ok=1
[[ $status == 200 ]] || meta_ok=0
for key in name version environment serverTime; do
  [[ -n $(json_string ".$key" "$key") ]] || meta_ok=0
done
environment=$(json_string '.environment' environment)
if [[ -n ${SMOKE_EXPECT_ENV:-} && $environment != "$SMOKE_EXPECT_ENV" ]]; then
  meta_ok=0
fi
if [[ $meta_ok == 1 ]]; then
  pass "/api/v1/meta 200 environment=$environment version=$(json_string '.version' version)"
else
  fail "/api/v1/meta: código $status, cuerpo: ${body:0:200}"
fi

# 4. Ruta inexistente con X-Request-Id propio (CA-05 y CA-07 de la spec 002).
request_id="smoke-$(date +%s)-$RANDOM"
request "/smoke-no-existe-$RANDOM" "X-Request-Id: $request_id"
code=$(json_string '.error.code' code)
message=$(json_string '.error.message' message)
body_request_id=$(json_string '.error.requestId' requestId)
header_request_id=$(header_value X-Request-Id)
if [[ $status == 404 && $code == not_found && -n $message &&
  $body_request_id == "$request_id" && $header_request_id == "$request_id" ]]; then
  pass "ruta inexistente 404 con envoltorio de error y X-Request-Id=$request_id"
else
  fail "ruta inexistente: código $status, X-Request-Id='$header_request_id', cuerpo: ${body:0:200}"
fi

if ((failures > 0)); then
  echo "Resultado: $failures comprobación(es) fallida(s)." >&2
  exit 1
fi
echo "Resultado: todas las comprobaciones pasaron."
