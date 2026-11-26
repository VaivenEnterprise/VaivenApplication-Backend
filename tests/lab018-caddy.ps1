param(
    [Parameter(Mandatory)] [string] $Image,
    [Parameter(Mandatory)] [string] $Caddyfile
)

$ErrorActionPreference = 'Stop'
$suffix = [Guid]::NewGuid().ToString('N').Substring(0, 8)
$network = "vaiven-018-$suffix"
$created = [Collections.Generic.List[string]]::new()
$secret = [Convert]::ToHexString([Security.Cryptography.RandomNumberGenerator]::GetBytes(32))
$subnet = "10.221.$(Get-Random -Minimum 16 -Maximum 240)"
$prefix = "s018-$suffix"

function Invoke-Podman {
    & podman @args
    if ($LASTEXITCODE -ne 0) { throw "Falló podman $($args[0]) (código $LASTEXITCODE)." }
}

function Add-LabContainer([string] $name, [string[]] $options) {
    Invoke-Podman create --pull=never --name $name --network $network --label "vaiven.spec018.lab=$suffix" @options | Out-Null
    $created.Add($name)
}

function Wait-Lab([string] $stage, [scriptblock] $probe) {
    $deadline = [DateTime]::UtcNow.AddSeconds(60)
    do {
        if (& $probe) { return }
        Start-Sleep -Seconds 1
    } while ([DateTime]::UtcNow -lt $deadline)
    throw "No se alcanzó el estado $stage."
}

function Send-Batch([string] $phase, [int] $count) {
    $go = "set -e; i=1; while [ `$i -le $count ]; do wget -T 5 -q -O /dev/null --header `"X-Request-Id: $prefix-$phase-`$i`" http://127.0.0.1/healthz; i=`$((i+1)); done"
    Invoke-Podman exec "$prefix-caddy" sh -c $go | Out-Null
    $first = @(Invoke-Podman logs "$prefix-api1" 2>&1 | Select-String -Pattern "$prefix-$phase-\d+" -AllMatches).Count
    $second = @(Invoke-Podman logs "$prefix-api2" 2>&1 | Select-String -Pattern "$prefix-$phase-\d+" -AllMatches).Count
    if ($first + $second -ne $count) { throw "Distribución no atribuible: $phase API1=$first API2=$second total=$count." }
    [pscustomobject]@{ phase = $phase; api1 = $first; api2 = $second; total = $count }
}

try {
    Invoke-Podman network create --internal --subnet "$subnet.0/24" --label "vaiven.spec018.lab=$suffix" $network | Out-Null
    Add-LabContainer "$prefix-mongo" @('--network-alias','mongo','--memory','500m','docker.io/library/mongo:7','--replSet','rs0','--bind_ip_all','--wiredTigerCacheSizeGB','0.25')
    Add-LabContainer "$prefix-redis" @('--network-alias','redis','--memory','96m','docker.io/library/redis:7','redis-server','--save','','--appendonly','no')
    Invoke-Podman start "$prefix-mongo" "$prefix-redis" | Out-Null
    Wait-Lab 'MongoDB iniciado' {
        & podman exec "$prefix-mongo" mongosh --quiet --eval 'db.adminCommand({ping:1}).ok' 2>$null | Out-Null
        $LASTEXITCODE -eq 0
    }
    Invoke-Podman exec "$prefix-mongo" mongosh --quiet --eval "rs.initiate({_id:'rs0',members:[{_id:0,host:'mongo:27017'}]})" | Out-Null
    Wait-Lab 'MongoDB primario' {
        $state = & podman exec "$prefix-mongo" mongosh --quiet --eval 'db.hello().isWritablePrimary' 2>$null
        $LASTEXITCODE -eq 0 -and "$state".Trim() -eq 'true'
    }
    foreach ($replica in 1,2) {
        Add-LabContainer "$prefix-api$replica" @(
            '--network-alias',"api$replica",'--memory','192m',
            '-e','APP_ENV=development','-e','MONGO_URI=mongodb://mongo:27017/?replicaSet=rs0',
            '-e','MONGO_DB=vaiven_lab018','-e','REDIS_URL=redis://redis:6379/0',
            '-e',"JWT_ACCESS_SECRET=$secret",'-e','CLOUDINARY_CLOUD_NAME=lab-only',
            '-e','CLOUDINARY_API_KEY=000000','-e','CLOUDINARY_API_SECRET=lab-only',
            '-e','AI_ENABLED=false','-e','ASSISTANT_ENABLED=false','-e','RECO_ENABLED=false',
            '-e','PUSH_ENABLED=false','-e','DOCS_ENABLED=false',
            '-e','LOG_LEVEL=info','-e','HTTP_COMPRESSION_ENABLED=true',
            '-e',"TRUSTED_PROXIES=$subnet.10/32",$Image
        )
    }
    Add-LabContainer "$prefix-caddy" @('--ip',"$subnet.10",'--memory','96m','docker.io/library/caddy:2')
    Invoke-Podman cp $Caddyfile "${prefix}-caddy:/etc/caddy/Caddyfile" | Out-Null
    Invoke-Podman start "$prefix-api1" "$prefix-api2" "$prefix-caddy" | Out-Null
    foreach ($replica in 1,2) {
        Wait-Lab "api$replica listo" {
            & podman exec "$prefix-caddy" wget -T 3 -q -O /dev/null "http://api${replica}:8080/readyz" 2>$null
            $LASTEXITCODE -eq 0
        }
    }
    Start-Sleep -Seconds 3
    $normal = Send-Batch 'normal' 16
    if ($normal.api1 -ne 8 -or $normal.api2 -ne 8) { throw 'Round robin no distribuyó 8/8.' }
    $normal | ConvertTo-Json -Compress

    $crossReplica = @'
set -eu
post() {
  wget -T 15 -q -O - --header 'Content-Type: application/json' --post-data "$2" "$1"
}
field() {
  sed -n 's/.*"'"$1"'":"\([^"]*\)".*/\1/p'
}
registered=$(post 'http://api1:8080/api/v1/auth/register' '{"email":"lb-pilot@example.invalid","password":"Synthetic-Lab-Password-42","displayName":"Lab","acceptTerms":true,"isAdult":true}')
access=$(printf '%s' "$registered" | field accessToken)
refresh=$(printf '%s' "$registered" | field refreshToken)
test -n "$access" && test -n "$refresh"
wget -T 15 -q -O /dev/null --header "Authorization: Bearer $access" 'http://api2:8080/api/v1/me'
refreshed=$(post 'http://api2:8080/api/v1/auth/refresh' "$(printf '{"refreshToken":"%s"}' "$refresh")")
access=$(printf '%s' "$refreshed" | field accessToken)
refresh=$(printf '%s' "$refreshed" | field refreshToken)
test -n "$access" && test -n "$refresh"
wget -T 15 -q -O /dev/null --header "Authorization: Bearer $access" 'http://api1:8080/api/v1/me'
post 'http://api1:8080/api/v1/auth/logout' "$(printf '{"refreshToken":"%s"}' "$refresh")" > /dev/null
if failure=$(wget -T 15 -S -O /dev/null --header 'Content-Type: application/json' --post-data "$(printf '{"refreshToken":"%s"}' "$refresh")" 'http://api2:8080/api/v1/auth/refresh' 2>&1); then
  exit 3
fi
printf '%s\n' "$failure" | grep -q '401'
printf '%s\n' '{"phase":"crossReplicaSession","register":"api1","refresh":"api2","profile":"both","logout":"api1","refreshAfterLogoutStatus":401}'
'@
    $crossReplica | podman exec -i "$prefix-caddy" sh -c 'tr -d "\r" | sh'
    if ($LASTEXITCODE -ne 0) { throw 'El flujo sintético entre réplicas no se completó.' }

    Invoke-Podman stop --time 5 "$prefix-api1" | Out-Null
    Start-Sleep -Seconds 4
    $outage = Send-Batch 'outage' 12
    if ($outage.api1 -ne 0 -or $outage.api2 -ne 12) { throw 'La réplica caída recibió tráfico o hubo pérdidas.' }
    $outage | ConvertTo-Json -Compress

    Invoke-Podman start "$prefix-api1" | Out-Null
    Wait-Lab 'api1 recuperada' {
        & podman exec "$prefix-caddy" wget -T 3 -q -O /dev/null 'http://api1:8080/readyz' 2>$null
        $LASTEXITCODE -eq 0
    }
    Start-Sleep -Seconds 12
    $recovered = Send-Batch 'recovered' 16
    if ($recovered.api1 -ne 8 -or $recovered.api2 -ne 8) { throw 'La réplica recuperada no volvió al reparto 8/8.' }
    $recovered | ConvertTo-Json -Compress
} finally {
    foreach ($name in ($created.ToArray() | Sort-Object -Descending)) {
        & podman rm -f -v $name 2>$null | Out-Null
    }
    & podman network rm $network 2>$null | Out-Null
    $secret = $null
}
$remainingContainers = @(Invoke-Podman ps --all --filter "label=vaiven.spec018.lab=$suffix" --format '{{.Names}}')
$remainingNetworks = @(Invoke-Podman network ls --filter "label=vaiven.spec018.lab=$suffix" --format '{{.Name}}')
if ($remainingContainers.Count -ne 0 -or $remainingNetworks.Count -ne 0) {
    throw 'El retiro dejó recursos del ensayo.'
}
[pscustomobject]@{ phase = 'teardown'; containers = 0; networks = 0 } | ConvertTo-Json -Compress
