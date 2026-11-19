# Desde la raíz de la copia aislada. Usa Podman Machine en Windows y las
# imágenes ya disponibles. Los servicios de prueba usan datos sintéticos.
[CmdletBinding()]
param()

$ErrorActionPreference = 'Stop'
$root = (Get-Location).Path
$mods = go env GOMODCACHE
if ($LASTEXITCODE -ne 0) { throw 'No se pudo localizar la caché de módulos de Go.' }
$evidence = Join-Path (Split-Path -Parent $root) 'spec-018-test-evidence'
New-Item -ItemType Directory -Path $evidence -Force | Out-Null
$session = [Convert]::ToHexString([Security.Cryptography.RandomNumberGenerator]::GetBytes(32)).ToLowerInvariant()
$runner = "spec018-contract-$($session.Substring(0,12))"
$script = @'
set -eu
export CONTRACT_WORKSPACE="/evidence/contract-$TESTCONTAINERS_SESSION_ID"
mkdir "$CONTRACT_WORKSPACE"
wrapper=$(mktemp)
trap 'rm -f "$wrapper"' EXIT
cat > "$wrapper" <<'WRAPPER'
#!/bin/sh
set -eu
# Cada proceso abre archivos distintos en el volumen compartido con Windows.
capture=$(mktemp -d "$CONTRACT_WORKSPACE/package.XXXXXX")
export CONTRACT_COVERAGE="$capture/operations.txt"
export CONTRACT_COMPRESS_STATS="$capture/gzip.tsv"
export CONTRACT_COMPRESS=1
exec "$@"
WRAPPER
chmod 0700 "$wrapper"
packages=$(find internal -name '*integration_test.go' -type f -exec grep -l 'contract\.Wrap' {} + | sed 's|/[^/]*$||; s|^|./|' | sort -u)
test -n "$packages"
go test -p 2 -exec "$wrapper" -tags=integration -count=1 -timeout 3m $packages
operations="/evidence/linux-operations-$TESTCONTAINERS_SESSION_ID.txt"
stats="/evidence/linux-gzip-$TESTCONTAINERS_SESSION_ID.tsv"
find "$CONTRACT_WORKSPACE" -type f -name operations.txt -exec cat {} + | sort -u > "$operations"
find "$CONTRACT_WORKSPACE" -type f -name gzip.tsv -exec cat {} + > "$stats"
report=$(CONTRACT_COVERAGE_REPORT="$operations" go test -count=1 -v ./internal/adapter/http/contract -run '^TestCoverageReport$')
printf '%s\n' "$report"
printf '%s\n' "$report" | grep -Fq 'operaciones ejercidas: 69 de 69'
'@

podman image inspect docker.io/library/mongo:7 docker.io/library/redis:7 docker.io/library/golang:1.26.11 --format '{{.Id}}' | Out-Null
if ($LASTEXITCODE -ne 0) { throw 'Faltan imágenes en la caché de Podman.' }

try {
    $script | podman run --rm -i --name $runner --pull=never --network host --security-opt label=disable `
        -v /run/podman/podman.sock:/run/podman/podman.sock -v "${root}:/src:ro" -v "${mods}:/modules:ro" -v "${evidence}:/evidence:rw" `
        -w /src -e DOCKER_HOST=unix:///run/podman/podman.sock -e TESTCONTAINERS_PROVIDER=podman `
        -e TESTCONTAINERS_RYUK_DISABLED=true -e "TESTCONTAINERS_SESSION_ID=$session" -e TESTCONTAINERS_HOST_OVERRIDE=127.0.0.1 `
        -e GOPROXY=off -e GOSUMDB=off -e GOTOOLCHAIN=local -e GOMODCACHE=/modules -e GOMAXPROCS=3 `
        docker.io/library/golang:1.26.11 sh -c 'tr -d "\r" | sh'
    $testExit = $LASTEXITCODE
} finally {
    $owned = @(& podman ps -aq --filter "label=org.testcontainers.sessionId=$session")
    foreach ($id in $owned) {
        if ($id -match '^[a-f0-9]{12,64}$') { & podman rm -f -v $id | Out-Null }
    }
}
exit $testExit
