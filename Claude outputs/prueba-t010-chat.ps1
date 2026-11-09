# Prueba de la T010 (spec 008, chat) contra staging. PowerShell 7+.
# Uso:  pwsh .\prueba-t010-chat.ps1 -EmailA "tu@correo" -PassA "tu-clave"
# La cuenta A debe ser dueña de al menos un objeto ACTIVO. Las cuentas B y C se
# registran solas con correos aleatorios @example.com. Al final cancela la
# reserva de prueba (así también se prueba CA-04).
param(
    [Parameter(Mandatory)] [string] $EmailA,
    [Parameter(Mandatory)] [string] $PassA,
    [string] $Base = "https://vaiven-staging.westus2.cloudapp.azure.com:8443/api/v1"
)
$ErrorActionPreference = "Stop"
$script:ok = 0; $script:bad = 0

function Call {
    param([string]$Method, [string]$Path, [string]$Token, $Body)
    $p = @{ Uri = "$Base$Path"; Method = $Method; SkipHttpErrorCheck = $true; Headers = @{} }
    if ($Token) { $p.Headers.Authorization = "Bearer $Token" }
    if ($null -ne $Body) { $p.ContentType = "application/json"; $p.Body = ($Body | ConvertTo-Json -Depth 6) }
    $r = Invoke-WebRequest @p
    $j = $null
    if ($r.Content) { try { $j = $r.Content | ConvertFrom-Json } catch { } }
    [pscustomobject]@{ Status = [int]$r.StatusCode; Json = $j; Raw = [string]$r.Content }
}
function Check {
    param([string]$Name, [bool]$Cond, [string]$Detail = "")
    if ($Cond) { $script:ok++; Write-Host "  PASS  $Name" -ForegroundColor Green }
    else { $script:bad++; Write-Host "  FAIL  $Name  $Detail" -ForegroundColor Red }
}
function Session([string]$Email, [string]$Pass, [string]$Name) {
    $r = Call POST "/auth/login" $null @{ email = $Email; password = $Pass }
    if ($r.Status -ne 200) {
        $r = Call POST "/auth/register" $null @{ email = $Email; password = $Pass; displayName = $Name; acceptTerms = $true; isAdult = $true }
        if ($r.Status -ne 201) { throw "No se pudo iniciar sesión ni registrar $Email (estado $($r.Status)): $($r.Raw)" }
    }
    [pscustomobject]@{ Token = $r.Json.tokens.accessToken; Id = $r.Json.user.id }
}

Write-Host "== Preparación ==" -ForegroundColor Cyan
$suf = Get-Random -Maximum 999999
$A = Session $EmailA $PassA "Dueño"
$B = Session "chat-b-$suf@example.com" "Prueba12345" "Solicitante Prueba"
$C = Session "chat-c-$suf@example.com" "Prueba12345" "Ajeno Prueba"

$mine = Call GET "/me/items?status=active&limit=1" $A.Token
if ($mine.Status -ne 200 -or $mine.Json.items.Count -lt 1) { throw "La cuenta A no tiene objetos activos (estado $($mine.Status))." }
$item = $mine.Json.items[0]
$mode = if ($item.modality -eq "loan") { "none" } else { "full_on_delivery" }
$s = (Get-Date).AddDays(10).ToString("yyyy-MM-dd"); $e = (Get-Date).AddDays(11).ToString("yyyy-MM-dd")
$q = Call GET "/items/$($item.id)/quote?startDate=$s&endDate=$e&paymentMode=$mode" $B.Token
if ($q.Status -ne 200) { throw "Cotización falló (estado $($q.Status)): $($q.Raw)" }
$bk = Call POST "/bookings" $B.Token @{ itemId = $item.id; startDate = $s; endDate = $e; paymentMode = $mode; acceptedTotalCents = $q.Json.totalCents }
if ($bk.Status -ne 201) { throw "No se pudo crear la reserva (estado $($bk.Status)): $($bk.Raw)" }
$cid = $bk.Json.conversationId; $bid = $bk.Json.id
Write-Host "Reserva $bid, conversación $cid"

Write-Host "`n== CA-01: la conversación existe con 2 participantes ==" -ForegroundColor Cyan
$la = Call GET "/conversations" $A.Token
$lb = Call GET "/conversations" $B.Token
Check "A ve la conversación" ($la.Status -eq 200 -and ($la.Json.items | Where-Object id -eq $cid))
Check "B ve la conversación" ($lb.Status -eq 200 -and ($lb.Json.items | Where-Object id -eq $cid))
$ca = $la.Json.items | Where-Object id -eq $cid
Check "la contraparte de A es B" ($ca.counterpart.id -eq $B.Id)

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
    $m = Call POST "/conversations/$cid/messages" $B.Token @{ body = "Mensaje $n"; clientId = "t010-$suf-$n" }
    Check "B envía el mensaje $n -> 201" ($m.Status -eq 201) "estado $($m.Status)"
    $ids += $m.Json.id
}
$ca = (Call GET "/conversations" $A.Token).Json.items | Where-Object id -eq $cid
$cb = (Call GET "/conversations" $B.Token).Json.items | Where-Object id -eq $cid
Check "A tiene unreadCount=3" ($ca.unreadCount -eq 3) "vale $($ca.unreadCount)"
Check "B (remitente) sigue en 0" ($cb.unreadCount -eq 0) "vale $($cb.unreadCount)"
Check "unread-count de A sube en 3" ((Call GET "/conversations/unread-count" $A.Token).Json.count -eq ($base0 + 3))
Check "A marca leído -> 204" ((Call POST "/conversations/$cid/read" $A.Token).Status -eq 204)
$ca = (Call GET "/conversations" $A.Token).Json.items | Where-Object id -eq $cid
Check "A queda en 0" ($ca.unreadCount -eq 0)
$cb = (Call GET "/conversations" $B.Token).Json.items | Where-Object id -eq $cid
Check "leer no cambia nada para B" ($cb.unreadCount -eq 0)
Check "no se expone lectura de la otra parte" ($la.Raw -notmatch 'readAt|seenAt|lastRead|"read"|"seen"' -and $lb.Raw -notmatch 'readAt|seenAt|lastRead|"read"|"seen"')

Write-Host "`n== CA-03: idempotencia por clientId ==" -ForegroundColor Cyan
$dup = Call POST "/conversations/$cid/messages" $B.Token @{ body = "Mensaje 1"; clientId = "t010-$suf-1" }
Check "repetir clientId -> 200 y el mismo id" ($dup.Status -eq 200 -and $dup.Json.id -eq $ids[0]) "estado $($dup.Status)"
$all = Call GET "/conversations/$cid/messages" $A.Token
Check "siguen siendo 3 mensajes, del más nuevo al más viejo" ($all.Json.items.Count -eq 3 -and $all.Json.items[0].id -eq $ids[2])

Write-Host "`n== RF-03: paginación y consulta periódica ==" -ForegroundColor Cyan
$aft = Call GET "/conversations/$cid/messages?after=$($ids[0])" $A.Token
Check "after devuelve los 2 más nuevos en orden ascendente" ($aft.Status -eq 200 -and $aft.Json.items.Count -eq 2 -and $aft.Json.items[0].id -eq $ids[1])
$pg = Call GET "/conversations/$cid/messages?limit=2" $A.Token
Check "limit=2 trae 2 y nextCursor" ($pg.Json.items.Count -eq 2 -and $pg.Json.nextCursor)
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
Check "B cancela la reserva -> 200" ($cx.Status -eq 200) "estado $($cx.Status): $($cx.Raw)"
$w = Call POST "/conversations/$cid/messages" $B.Token @{ body = "ya cerrada" }
Check "escribir -> 409 conflict" ($w.Status -eq 409 -and $w.Json.error.code -eq "conflict") "estado $($w.Status)"
Check "el historial se sigue leyendo" ((Call GET "/conversations/$cid/messages" $A.Token).Json.items.Count -ge 4)

Write-Host "`n== RF-11: límite de 30 mensajes por minuto ==" -ForegroundColor Cyan
$limited = $false
foreach ($n in 1..35) {
    if ((Call POST "/conversations/$cid/messages" $B.Token @{ body = "ráfaga $n" }).Status -eq 429) { $limited = $true; break }
}
Check "se alcanza un 429" $limited

Write-Host "`n== Resultado ==" -ForegroundColor Cyan
Write-Host "PASS: $script:ok   FAIL: $script:bad"
Write-Host "Reserva de prueba: $bid  Conversación: $cid  (cuentas: chat-b-$suf / chat-c-$suf @example.com)"
if ($script:bad -gt 0) { exit 1 }
