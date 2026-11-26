# Observabilidad con Better Stack (Terraform)

Terraform es dueño **solo** de recursos de Better Stack; Azure es de Bicep (`docs/infra.md`).

## Plan gratuito (consultado en betterstack.com/pricing el 2026-11-18)

| Límite publicado | Uso de este módulo |
|---|---|
| 10 monitores y latidos **en total** | 3 en staging (2 monitores y 1 latido) y 5 con producción |
| Avisos por correo y Slack (push, SMS y llamada no aparecen en el plan gratuito) | Solo correo (`email = true`; `push`, `sms` y `call` en `false`) |
| 1 página de estado | No se crea |
| Registros: 3 GB al mes con 3 días de retención | **No se envían registros** (solo comprobaciones HTTPS y un latido); los eventos de seguridad se quedan en la VM |
| Intervalo mínimo de comprobación | **No publicado**; se usan 180 s |
| Tarjeta de crédito | **No publicado** |
| Uso | La página lo describe como «para proyectos personales»: el equipo debe confirmar que su uso académico cabe en esos términos |

## Conectar una cuenta gratuita

```bash
# 1. Crear la cuenta en betterstack.com (plan gratuito) y un token de API de Uptime.
# 2. En tu terminal (nunca en archivos ni en el chat):
export BETTERUPTIME_API_TOKEN='…'
cd infra/terraform/observability
cp terraform.tfvars.example terraform.tfvars   # ignorado por Git
terraform init
terraform plan          # revisar: solo crea 2 monitores y 1 latido
terraform apply         # solo con autorización
terraform output -raw alert_scan_heartbeat_url   # guardarlo como secreto BETTERSTACK_HEARTBEAT_URL
```

El estado (`terraform.tfstate`) queda fuera de Git y contiene la URL del latido: guárdalo en un lugar privado.
