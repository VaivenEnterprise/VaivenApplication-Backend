variable "staging_base_url" {
  description = "URL base HTTPS de staging (sin barra final)."
  type        = string
  default     = "https://vaiven-staging.westus2.cloudapp.azure.com:8443"

  validation {
    condition     = can(regex("^https://[^/]+$", var.staging_base_url))
    error_message = "Debe ser https://host[:puerto], sin ruta."
  }
}

variable "production_base_url" {
  description = "URL base HTTPS de producción. Vacío: no se crean monitores de producción."
  type        = string
  default     = ""

  validation {
    condition     = var.production_base_url == "" || can(regex("^https://[^/]+$", var.production_base_url))
    error_message = "Vacío o https://host[:puerto], sin ruta."
  }
}

variable "check_frequency_seconds" {
  description = "Frecuencia de las comprobaciones (180 s). La página de precios no publica el mínimo del plan gratuito (consultada el 2026-11-18): confirmarlo al crear la cuenta."
  type        = number
  default     = 180

  validation {
    condition     = var.check_frequency_seconds >= 30
    error_message = "Mínimo 30 s."
  }
}

variable "alert_scan_period_seconds" {
  description = "Cada cuánto llega el latido del escaneo de alertas (security-alerts.yml corre cada 30 min)."
  type        = number
  default     = 1800
}
