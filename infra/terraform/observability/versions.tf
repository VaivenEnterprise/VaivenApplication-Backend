# Observabilidad externa con Better Stack (spec 018, C-10). Terraform solo es
# dueño de recursos de Better Stack: nunca de recursos de Azure, que son de
# Bicep (docs/infra.md, matriz de propiedad). Estado fuera de Git.
terraform {
  required_version = ">= 1.9.0, < 2.0.0"

  required_providers {
    betteruptime = {
      source  = "BetterStackHQ/better-uptime"
      version = "= 0.22.4" # publicada el 2026-10-06 (≥ 2 semanas)
    }
  }
}
