# Desde la raíz de la copia aislada. No publica puertos.
$ErrorActionPreference = 'Stop'
$network = 'revocation-018-' + [Guid]::NewGuid().ToString('N').Substring(0,10)
$redis = "$network-redis"
$root = (Get-Location).Path
$mods = go env GOMODCACHE
if ($LASTEXITCODE -ne 0) { throw 'No se pudo localizar la caché de módulos de Go.' }

podman network create --internal $network | Out-Null
if ($LASTEXITCODE -ne 0) { throw 'No se pudo crear la red interna.' }
try {
    podman run -d --pull=never --name $redis --network $network --network-alias lab-redis --memory 96m `
        docker.io/library/redis:7 redis-server --appendonly no | Out-Null
    if ($LASTEXITCODE -ne 0) { throw 'No se pudo iniciar Redis local.' }
    $deadline = [DateTime]::UtcNow.AddSeconds(15)
    do {
        $status = & podman exec $redis redis-cli ping 2>$null
        if ($LASTEXITCODE -eq 0 -and $status -eq 'PONG') { break }
        Start-Sleep -Seconds 1
    } while ([DateTime]::UtcNow -lt $deadline)
    if ($status -ne 'PONG') { throw 'Redis no respondió antes del plazo.' }

    podman run --rm --pull=never --network $network -v "${root}:/src:ro" -v "${mods}:/modules:ro" -w /src `
        -e GOPROXY=off -e GOSUMDB=off -e GOTOOLCHAIN=local -e GOMODCACHE=/modules -e GOMAXPROCS=3 `
        -e VAIVEN_LAB_REVOCATION=1 docker.io/library/golang:1.26.11 go test -race -p 2 -count=1 `
        -timeout 2m -tags=integration -v ./internal/platform/redis -run '^TestIntegrationRevocation'
    $testExit = $LASTEXITCODE
} finally {
    podman rm -f -v $redis 2>$null | Out-Null
    podman network rm $network 2>$null | Out-Null
}
exit $testExit
