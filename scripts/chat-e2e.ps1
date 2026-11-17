#Requires -Version 7
<#
.SYNOPSIS
  Prueba de extremo a extremo del chat (spec 008, T010) contra un entorno de
  desarrollo o de pruebas. Reemplaza a «Claude outputs/prueba-t010-chat.ps1»
  (SEC-006).

.DESCRIPTION
  - Credenciales SOLO por variables de entorno (nunca por parámetro, para que
    no queden en el historial de la terminal):
      VAIVEN_E2E_OWNER_EMAIL     cuenta A, dueña de al menos un objeto activo
      VAIVEN_E2E_OWNER_PASSWORD
    La cuenta A solo inicia sesión: nunca se registra ni se modifica.
  - Las cuentas B y C se registran con un id único por ejecución y una
    contraseña aleatoria que vive solo en memoria y nunca se imprime.
  - Guarda de entorno: -Environment local exige localhost/127.0.0.1/[::1];
    -Environment staging exige «staging» en el host. Producción se rechaza
    siempre. No hay URL por defecto.
  - Limpieza en finally, SOLO de lo que creó esta ejecución: cancela la reserva
    de prueba (si sigue abierta) y da de baja las cuentas B y C (DELETE /me,
    que anonimiza y revoca sesiones). Si la limpieza falla, lo dice y sale con 1.
  - Nunca imprime tokens, contraseñas ni cuerpos de respuestas de /auth.

.EXAMPLE
  $env:VAIVEN_E2E_OWNER_EMAIL = "..." ; $env:VAIVEN_E2E_OWNER_PASSWORD = "..."
  pwsh ./scripts/chat-e2e.ps1 -Environment local -BaseUrl http://localhost:8080/api/v1
#>
[CmdletBinding()]
param(
    [Parameter(Mandatory)] [ValidateSet("local", "staging")] [string] $Environment,
    [Parameter(Mandatory)] [ValidateNotNullOrEmpty()] [string] $BaseUrl,
    [ValidateRange(1, 365)] [int] $DaysAhead = 10,
    [switch] $SkipRateLimit
)
$ErrorActionPreference = "Stop"
Set-StrictMode -Version 1

# --- Guardas (antes de cualquier llamada de red) -----------------------------

function Assert-SafeTarget([string] $Url, [string] $EnvName) {
    $u = $null
    if (-not [Uri]::TryCreate($Url, [UriKind]::Absolute, [ref] $u) -or $u.Scheme -notin @("http", "https")) {
        throw "BaseUrl no es una URL http(s) absoluta."
    }
    if ($u.UserInfo) { throw "BaseUrl no debe llevar credenciales." }
    $h = $u.Host.ToLowerInvariant()
    if ($h -match '(^|[.-])(prod|production|produccion)([.-]|$)' -or $h -in @("vaiven.mx", "api.vaiven.mx", "www.vaiven.mx")) {
        throw "Destino de producción rechazado: esta prueba crea datos."
    }
    $isLocal = $h -in @("localhost", "127.0.0.1", "[::1]", "::1") -or $h.EndsWith(".localhost") -or $h.EndsWith(".test")
    if ($EnvName -eq "local" -and -not $isLocal) { throw "-Environment local exige un host local (localhost, 127.0.0.1, [::1], *.localhost, *.test)." }
    if ($EnvName -eq "staging" -and $h -notmatch '(^|[.-])staging([.-]|$)') { throw "-Environment staging exige un host con «staging»." }
    if ($EnvName -eq "staging" -and $u.Scheme -ne "https") { throw "staging exige https." }
    return $u.AbsoluteUri.TrimEnd("/")
}

function Get-RequiredEnv([string] $Name) {
    $v = [Environment]::GetEnvironmentVariable($Name)
    if ([string]::IsNullOrWhiteSpace($v)) { throw "Falta la variable de entorno $Name." }
    return $v
}

function New-RandomPassword {
    $bytes = [byte[]]::new(24)
    [System.Security.Cryptography.RandomNumberGenerator]::Fill($bytes)
    return "E2e-" + [Convert]::ToBase64String($bytes).Replace("+", "x").Replace("/", "y").TrimEnd("=")
}

$Base = Assert-SafeTarget $BaseUrl $Environment
$ownerEmail = Get-RequiredEnv "VAIVEN_E2E_OWNER_EMAIL"
$ownerPassword = Get-RequiredEnv "VAIVEN_E2E_OWNER_PASSWORD"
$runId = [Guid]::NewGuid().ToString("N").Substring(0, 12)

$script:ok = 0; $script:bad = 0
$created = [ordered]@{ Booking = $null; BookingClosed = $false; Accounts = [System.Collections.Generic.List[object]]::new() }

function Call {
    param([string] $Method, [string] $Path, [string] $Token, $Body)
    $p = @{ Uri = "$Base$Path"; Method = $Method; SkipHttpErrorCheck = $true; Headers = @{ "X-Request-Id" = "e2e-$runId-$([Guid]::NewGuid().ToString('N').Substring(0, 8))" } }
    if ($Token) { $p.Headers.Authorization = "Bearer $Token" }
    if ($null -ne $Body) { $p.ContentType = "application/json"; $p.Body = ($Body | ConvertTo-Json -Depth 6) }
    $r = Invoke-WebRequest @p
    $j = $null
    if ($r.Content) { try { $j = $r.Content | ConvertFrom-Json } catch { $j = $null } }
    [pscustomobject]@{ Status = [int] $r.StatusCode; Json = $j }
}
function Check([string] $Name, [bool] $Cond, [string] $Detail = "") {
    if ($Cond) { $script:ok++; Write-Host "  PASS  $Name" -ForegroundColor Green }
    else { $script:bad++; Write-Host "  FAIL  $Name  $Detail" -ForegroundColor Red }
}
function Login([string] $Email, [string] $Password) {
    $r = Call POST "/auth/login" $null @{ email = $Email; password = $Password }
    if ($r.Status -ne 200) { throw "No se pudo iniciar sesión con la cuenta A (estado $($r.Status))." }
    [pscustomobject]@{ Token = $r.Json.tokens.accessToken; Id = $r.Json.user.id }
}
function Register([string] $Label) {
    $email = "e2e-$Label-$runId@example.com"
    $password = New-RandomPassword
    $r = Call POST "/auth/register" $null @{ email = $email; password = $password; displayName = "E2E $Label"; acceptTerms = $true; isAdult = $true }
    if ($r.Status -ne 201) { throw "No se pudo registrar la cuenta $Label (estado $($r.Status))." }
    $acct = [pscustomobject]@{ Label = $Label; Email = $email; Password = $password; Token = $r.Json.tokens.accessToken; Id = $r.Json.user.id }
    $created.Accounts.Add($acct)  # se registra ANTES de seguir: el finally la limpia aunque algo falle
    return $acct
}
function Invoke-WithRetry([scriptblock] $Action) {
    for ($i = 1; $i -le 3; $i++) {
        $r = & $Action
        if ($r.Status -ne 429) { return $r }
        Start-Sleep -Seconds 30  # el límite de tasa es por minuto
    }
    return $r
}

$exitCode = 0
try {
    Write-Host "== Preparación ($Environment, ejecución $runId) ==" -ForegroundColor Cyan
    $A = Login $ownerEmail $ownerPassword
    $B = Register "b"
    $C = Register "c"

    $mine = Call GET "/me/items?status=active&limit=1" $A.Token
    if ($mine.Status -ne 200 -or @($mine.Json.items).Count -lt 1) { throw "La cuenta A no tiene objetos activos (estado $($mine.Status))." }
    $item = $mine.Json.items[0]
    $mode = if ($item.modality -eq "loan") { "none" } else { "full_on_delivery" }
    $s = (Get-Date).AddDays($DaysAhead).ToString("yyyy-MM-dd"); $e = (Get-Date).AddDays($DaysAhead + 1).ToString("yyyy-MM-dd")
    $q = Call GET "/items/$($item.id)/quote?startDate=$s&endDate=$e&paymentMode=$mode" $B.Token
    if ($q.Status -ne 200) { throw "La cotización falló (estado $($q.Status))." }
    $bk = Call POST "/bookings" $B.Token @{ itemId = $item.id; startDate = $s; endDate = $e; paymentMode = $mode; acceptedTotalCents = $q.Json.totalCents }
    if ($bk.Status -ne 201) { throw "No se pudo crear la reserva (estado $($bk.Status))." }
    $created.Booking = $bk.Json.id
    $cid = $bk.Json.conversationId; $bid = $bk.Json.id
    Write-Host "Reserva $bid, conversación $cid"

    Write-Host "`n== CA-01: la conversación existe con 2 participantes ==" -ForegroundColor Cyan
    $la = Call GET "/conversations" $A.Token
    $lb = Call GET "/conversations" $B.Token
    $ca = @($la.Json.items) | Where-Object id -eq $cid
    Check "A ve la conversación" ($la.Status -eq 200 -and $null -ne $ca)
    Check "B ve la conversación" ($lb.Status -eq 200 -and $null -ne (@($lb.Json.items) | Where-Object id -eq $cid))
    Check "la contraparte de A es B" ($null -ne $ca -and $ca.counterpart.id -eq $B.Id)

    Write-Host "`n== CA-02 / RF-05: una persona ajena recibe 404 ==" -ForegroundColor Cyan
    Check "C lee mensajes -> 404" ((Call GET "/conversations/$cid/messages" $C.Token).Status -eq 404)
    Check "C envía -> 404" ((Call POST "/conversations/$cid/messages" $C.Token @{ body = "intruso" }).Status -eq 404)
    Check "C marca leído -> 404" ((Call POST "/conversations/$cid/read" $C.Token).Status -eq 404)

    Write-Host "`n== Sesión y validación de ids ==" -ForegroundColor Cyan
    Check "sin token -> 401" ((Call GET "/conversations" $null).Status -eq 401)
    Check "id mal formado -> 404" ((Call GET "/conversations/xyz/messages" $A.Token).Status -eq 404)
    Check "id inexistente -> 404" ((Call GET "/conversations/000000000000000000000000/messages" $A.Token).Status -eq 404)

    Write-Host "`n== CA-06: no leídos ==" -ForegroundColor Cyan
    $base0 = (Call GET "/conversations/unread-count" $A.Token).Json.count
    $ids = @()
    foreach ($n in 1..3) {
        $m = Call POST "/conversations/$cid/messages" $B.Token @{ body = "Mensaje $n"; clientId = "e2e-$runId-$n" }
        Check "B envía el mensaje $n -> 201" ($m.Status -eq 201) "estado $($m.Status)"
        $ids += $m.Json.id
    }
    $ca = @((Call GET "/conversations" $A.Token).Json.items) | Where-Object id -eq $cid
    $cb = @((Call GET "/conversations" $B.Token).Json.items) | Where-Object id -eq $cid
    Check "A tiene unreadCount=3" ($ca.unreadCount -eq 3) "vale $($ca.unreadCount)"
    Check "B (remitente) sigue en 0" ($cb.unreadCount -eq 0) "vale $($cb.unreadCount)"
    Check "unread-count de A sube en 3" ((Call GET "/conversations/unread-count" $A.Token).Json.count -eq ($base0 + 3))
    Check "A marca leído -> 204" ((Call POST "/conversations/$cid/read" $A.Token).Status -eq 204)
    $ca = @((Call GET "/conversations" $A.Token).Json.items) | Where-Object id -eq $cid
    Check "A queda en 0" ($ca.unreadCount -eq 0)
    $cb = @((Call GET "/conversations" $B.Token).Json.items) | Where-Object id -eq $cid
    Check "leer no cambia nada para B" ($cb.unreadCount -eq 0)
    $raw = ($la.Json | ConvertTo-Json -Depth 8) + ($lb.Json | ConvertTo-Json -Depth 8)
    Check "no se expone lectura de la otra parte" ($raw -notmatch 'readAt|seenAt|lastRead|"read"|"seen"')

    Write-Host "`n== CA-03: idempotencia por clientId ==" -ForegroundColor Cyan
    $dup = Call POST "/conversations/$cid/messages" $B.Token @{ body = "Mensaje 1"; clientId = "e2e-$runId-1" }
    Check "repetir clientId -> 200 y el mismo id" ($dup.Status -eq 200 -and $dup.Json.id -eq $ids[0]) "estado $($dup.Status)"
    $all = Call GET "/conversations/$cid/messages" $A.Token
    Check "siguen siendo 3 mensajes, del más nuevo al más viejo" (@($all.Json.items).Count -eq 3 -and $all.Json.items[0].id -eq $ids[2])

    Write-Host "`n== RF-03: paginación y consulta periódica ==" -ForegroundColor Cyan
    $aft = Call GET "/conversations/$cid/messages?after=$($ids[0])" $A.Token
    Check "after devuelve los 2 más nuevos en orden ascendente" ($aft.Status -eq 200 -and @($aft.Json.items).Count -eq 2 -and $aft.Json.items[0].id -eq $ids[1])
    $pg = Call GET "/conversations/$cid/messages?limit=2" $A.Token
    Check "limit=2 trae 2 y nextCursor" (@($pg.Json.items).Count -eq 2 -and $pg.Json.nextCursor)
    Check "before + after juntos -> 400" ((Call GET "/conversations/$cid/messages?before=$($ids[2])&after=$($ids[0])" $A.Token).Status -eq 400)
    Check "limit=0 -> 400" ((Call GET "/conversations/$cid/messages?limit=0" $A.Token).Status -eq 400)

    Write-Host "`n== CA-05: validación del cuerpo ==" -ForegroundColor Cyan
    $big = Call POST "/conversations/$cid/messages" $B.Token @{ body = ("x" * 1001) }
    Check "1001 caracteres -> 400 validation_failed" ($big.Status -eq 400 -and $big.Json.error.code -eq "validation_failed")
    Check "1000 caracteres -> 201" ((Call POST "/conversations/$cid/messages" $B.Token @{ body = ("y" * 1000) }).Status -eq 201)
    Check "solo espacios -> 400" ((Call POST "/conversations/$cid/messages" $B.Token @{ body = "    " }).Status -eq 400)
    Check "sin body -> 400" ((Call POST "/conversations/$cid/messages" $B.Token @{ clientId = "z" }).Status -eq 400)

    Write-Host "`n== CA-04: reserva cancelada ==" -ForegroundColor Cyan
    $cx = Call POST "/bookings/$bid/cancel" $B.Token @{}
    Check "B cancela la reserva -> 200" ($cx.Status -eq 200) "estado $($cx.Status)"
    if ($cx.Status -eq 200) { $created.BookingClosed = $true }
    $w = Call POST "/conversations/$cid/messages" $B.Token @{ body = "ya cerrada" }
    Check "escribir -> 409 conflict" ($w.Status -eq 409 -and $w.Json.error.code -eq "conflict") "estado $($w.Status)"
    Check "el historial se sigue leyendo" (@((Call GET "/conversations/$cid/messages" $A.Token).Json.items).Count -ge 4)

    if (-not $SkipRateLimit) {
        Write-Host "`n== RF-11: límite de 30 mensajes por minuto ==" -ForegroundColor Cyan
        $limited = $false
        foreach ($n in 1..35) {
            if ((Call POST "/conversations/$cid/messages" $B.Token @{ body = "ráfaga $n" }).Status -eq 429) { $limited = $true; break }
        }
        Check "se alcanza un 429" $limited
    }
}
catch {
    $exitCode = 1
    Write-Host "ERROR: $($_.Exception.Message)" -ForegroundColor Red
}
finally {
    Write-Host "`n== Limpieza (solo lo creado en $runId) ==" -ForegroundColor Cyan
    $cleanupFailed = $false
    if ($created.Booking -and -not $created.BookingClosed) {
        $b = $created.Accounts | Where-Object Label -eq "b" | Select-Object -First 1
        $r = Invoke-WithRetry { Call POST "/bookings/$($created.Booking)/cancel" $b.Token @{} }
        if ($r.Status -eq 200) { Write-Host "  reserva $($created.Booking) cancelada" }
        else { $cleanupFailed = $true; Write-Host "  NO se pudo cancelar la reserva $($created.Booking) (estado $($r.Status))" -ForegroundColor Red }
    }
    foreach ($acct in $created.Accounts) {
        $r = Invoke-WithRetry { Call DELETE "/me" $acct.Token @{ password = $acct.Password } }
        if ($r.Status -eq 204) { Write-Host "  cuenta $($acct.Label) dada de baja" }
        else { $cleanupFailed = $true; Write-Host "  NO se pudo dar de baja la cuenta $($acct.Label) ($($acct.Email), estado $($r.Status))" -ForegroundColor Red }
        $acct.Password = $null; $acct.Token = $null
    }
    if ($cleanupFailed) { $exitCode = 1; Write-Host "  Limpieza incompleta: revisar a mano los recursos marcados." -ForegroundColor Red }
}

Write-Host "`n== Resultado ==" -ForegroundColor Cyan
Write-Host "PASS: $script:ok   FAIL: $script:bad"
if ($script:bad -gt 0) { $exitCode = 1 }
exit $exitCode
