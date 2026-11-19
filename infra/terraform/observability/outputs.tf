output "monitor_ids" {
  description = "Monitores creados, por entorno y comprobación."
  value       = { for k, m in betteruptime_monitor.api : k => m.id }
}

output "alert_scan_heartbeat_url" {
  description = "URL del latido: guardarla como secreto BETTERSTACK_HEARTBEAT_URL del repositorio (nunca en Git)."
  value       = betteruptime_heartbeat.alert_scan.url
  sensitive   = true
}
