locals {
  # Un monitor por entorno y comprobación. Solo endpoints públicos sin datos
  # personales: /healthz (proceso vivo) y /readyz (MongoDB y Redis).
  environments = merge(
    { staging = var.staging_base_url },
    var.production_base_url == "" ? {} : { production = var.production_base_url },
  )

  checks = merge([
    for env, base in local.environments : {
      "${env}-healthz" = { env = env, url = "${base}/healthz", type = "keyword", keyword = "\"ok\"" }
      "${env}-readyz"  = { env = env, url = "${base}/readyz", type = "keyword", keyword = "\"ready\"" }
    }
  ]...)
}

resource "betteruptime_monitor" "api" {
  for_each = local.checks

  url                 = each.value.url
  monitor_type        = each.value.type
  required_keyword    = each.value.keyword
  pronounceable_name  = "VAIVEN ${each.key}"
  http_method         = "GET"
  check_frequency     = var.check_frequency_seconds
  request_timeout     = 15
  confirmation_period = 60  # una caída breve no abre incidente
  recovery_period     = 180 # notifica la recuperación tras 3 min sanos
  verify_ssl          = true
  ssl_expiration      = 14 # aviso 14 días antes de que venza el certificado
  regions             = ["us"]
  email               = true
  push                = false # el plan gratuito solo incluye correo y Slack
  sms                 = false # sin costo extra
  call                = false
}

# Latido del escaneo de alertas A1–A4: si security-alerts.yml deja de
# reportar, Better Stack abre un incidente (la vigilancia no falla en silencio).
resource "betteruptime_heartbeat" "alert_scan" {
  name   = "VAIVEN staging security-alerts scan"
  period = var.alert_scan_period_seconds
  grace  = 900
  email  = true
  push   = false # el plan gratuito solo incluye correo y Slack
}
