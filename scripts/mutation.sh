#!/usr/bin/env bash
# Pruebas de mutación con gremlins (spec 017, T010).
#
# Uso:   scripts/mutation.sh [directorio...]
# Por defecto: internal/domain internal/usecase internal/platform/security
#
# Recorre cada paquete Go (subpaquetes incluidos) de los directorios dados,
# ejecuta `gremlins unleash`, comprueba los umbrales y deja en $MUTATION_OUT:
#   <paquete>.json  reporte de gremlins (legible por máquina)
#   <paquete>.log   salida completa de gremlins
#   summary.md      tabla en Markdown con eficacia y cobertura de mutantes
#
# Variables opcionales:
#   MUTATION_EFFICACY  umbral de eficacia en % (KILLED / (KILLED + LIVED)); 75
#   MUTATION_MCOVER    umbral de cobertura de mutantes en %; 70
#   MUTATION_TIMEOUT_COEFFICIENT  multiplicador del tiempo de cada prueba; 5
#   MUTATION_OUT       carpeta de reportes; reports/mutation
#   GREMLINS           ruta del binario; gremlins
#
# Los directorios que no existen o los paquetes sin mutantes se informan y no
# fallan. Sale con 1 si algún paquete no alcanza un umbral.
set -euo pipefail

efficacy=${MUTATION_EFFICACY:-75}
mcover=${MUTATION_MCOVER:-70}
timeout_coefficient=${MUTATION_TIMEOUT_COEFFICIENT:-5}
out=${MUTATION_OUT:-reports/mutation}
gremlins=${GREMLINS:-gremlins}

for value in "$efficacy" "$mcover" "$timeout_coefficient"; do
  [[ $value =~ ^[0-9]+([.][0-9]+)?$ ]] || { echo "Valor numérico no válido: '$value'" >&2; exit 2; }
done

if [[ $# -gt 0 ]]; then
  targets=("$@")
else
  targets=(internal/domain internal/usecase internal/platform/security)
fi

command -v go >/dev/null || { echo "Falta go." >&2; exit 2; }
command -v "$gremlins" >/dev/null || {
  echo "Falta gremlins: go install github.com/go-gremlins/gremlins/cmd/gremlins@v0.6.0" >&2
  exit 2
}

mkdir -p "$out"
summary=$out/summary.md
module=$(go list -m)

# json_number <campo> <archivo>: número de un campo de primer nivel del JSON
# compacto de gremlins (sin depender de jq).
json_number() {
  sed -nE 's/.*"'"$1"'":([0-9.]+).*/\1/p' "$2" | head -n 1
}

{
  echo "## Pruebas de mutación (gremlins)"
  echo
  echo "Umbrales: eficacia >= ${efficacy} %, cobertura de mutantes >= ${mcover} %."
  echo
  echo "| Paquete | Eliminados | Vivos | Sin cubrir | Tiempo agotado | Eficacia | Cobertura | Resultado |"
  echo "|---|---:|---:|---:|---:|---:|---:|---|"
} >"$summary"

failed=0
for target in "${targets[@]}"; do
  target=${target%/}
  if [[ ! -d $target ]]; then
    echo "| \`$target\` | | | | | | | omitido: no existe |" >>"$summary"
    echo "Omitido $target: el directorio no existe."
    continue
  fi
  while IFS= read -r pkg; do
    rel=${pkg#"$module"/}
    name=${rel//\//_}
    report=$out/$name.json
    log=$out/$name.log
    echo "== $rel"
    rm -f "$report"
    status=0
    "$gremlins" unleash "./$rel" \
      --output "$report" \
      --timeout-coefficient "$timeout_coefficient" >"$log" 2>&1 || status=$?
    grep -v '^ *KILLED' "$log" | tail -n 8 || true

    if [[ ! -s $report ]]; then
      if [[ $status -eq 0 ]]; then
        echo "| \`$rel\` | | | | | | | sin mutantes |" >>"$summary"
      else
        echo "| \`$rel\` | | | | | | | error (código $status, ver log) |" >>"$summary"
        failed=1
      fi
      continue
    fi

    # gremlins v0.6.0 ignora --threshold-* en la línea de comandos (sale con 0
    # aunque no se cumplan), así que los umbrales se comprueban aquí.
    got_efficacy=$(json_number test_efficacy "$report")
    got_mcover=$(json_number mutations_coverage "$report")
    result="aprobado"
    if [[ $status -ne 0 ]]; then
      result="**error de gremlins (código $status)**"
      failed=1
    elif ! awk -v e="${got_efficacy:-0}" -v m="${got_mcover:-0}" -v te="$efficacy" -v tm="$mcover" \
      'BEGIN { exit !(e + 0 >= te + 0 && m + 0 >= tm + 0) }'; then
      result="**no alcanza el umbral**"
      failed=1
    fi
    # shellcheck disable=SC2016 # las comillas invertidas son Markdown literal
    printf '| `%s` | %s | %s | %s | %s | %.1f %% | %.1f %% | %s |\n' "$rel" \
      "$(json_number mutants_killed "$report")" \
      "$(json_number mutants_lived "$report")" \
      "$(json_number mutants_not_covered "$report")" \
      "$(grep -o '"status":"TIMED OUT"' "$report" | wc -l | tr -d ' ')" \
      "${got_efficacy:-0}" "${got_mcover:-0}" "$result" >>"$summary"
  done < <(go list "./$target/...")
done

echo
cat "$summary"
exit "$failed"
