#!/usr/bin/env bash
# Ejecuta los objetivos de fuzzing nativo de Go (spec 020, RF-10 a RF-12).
#   scripts/fuzz.sh [tiempo-por-objetivo]     (por omisión 20s)
# `go test -fuzz` solo admite un paquete y un objetivo por ejecución, por eso
# la lista es paquete|objetivo. Un hallazgo queda en testdata/fuzz/<Objetivo>
# del paquete: se versiona como regresión y se corre en cada `go test`.
set -uo pipefail
cd "$(dirname "$0")/.."
FUZZTIME="${1:-20s}"

TARGETS=(
  "./internal/bookings/adapter/http|FuzzDecodeCursor"
  "./internal/bookings/adapter/http|FuzzDecodeCreateRequest"
  "./internal/items/adapter/http|FuzzDecodeCursor"
  "./internal/items/adapter/http|FuzzDecodeCreateRequest"
  "./internal/items/adapter/http|FuzzDecodeUpdateRequest"
  "./internal/notifications/adapter/http|FuzzDecodeCursor"
  "./internal/chat/adapter/http|FuzzDecodeCursor"
  "./internal/incidents/adapter/http|FuzzDecodeCursor"
  "./internal/auth/adapter/http|FuzzDecodeAuthBodies"
  "./internal/platform/security|FuzzJWTParse"
  "./internal/handover/domain|FuzzParsePayload"
  "./internal/adapter/http/middleware|FuzzClientIP"
)

failed=0
for t in "${TARGETS[@]}"; do
  pkg="${t%%|*}"
  name="${t##*|}"
  echo "::group::fuzz ${name} (${pkg}) ${FUZZTIME}"
  if ! go test "$pkg" -run='^$' -fuzz="^${name}\$" -fuzztime="$FUZZTIME"; then
    echo "::error::el objetivo ${name} de ${pkg} encontró un fallo (ver testdata/fuzz)"
    failed=1
  fi
  echo "::endgroup::"
done
exit "$failed"
