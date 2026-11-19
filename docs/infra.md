# Infraestructura de VAIVÉN en Bicep

Borrador local de la SPEC-018, T016. La plantilla de inventario referencia recursos existentes y el módulo de balanceo es una propuesta desactivada por defecto. La validación local opera sobre archivos. La lectura de Azure requiere autorización B-5 y corresponde a `devops-mcp`.

## Recursos y alcance

| Recurso | Modelo | Pendiente de verificar con acceso de lectura |
|---|---|---|
| Máquina virtual | `vm-vaiven` en `rg-vaiven`, región documentada `westus2`. Una VM para ambos entornos de Compose | SKU, zona, perfil de seguridad, extensión y estado real |
| Interfaz, red y subred | Referencias `existing` condicionadas a parámetros confirmados | Identificadores, CIDR, configuración IP, asociaciones y rutas |
| NSG | Referencia existente `vm-vaivenNSG`. Se preservan las reglas actuales | Reglas efectivas en NIC y subred, prioridades y orígenes permitidos. El acceso administrativo queda restringido |
| IP pública | Referencia opcional a nombre confirmado. La plantilla conserva la IP asociada actualmente | SKU, zona, asociación y DNS reales |
| Discos | Referencias opcionales al disco del SO y a discos de datos verificados | Identificadores, SKU, tamaño, cifrado, LUN y asociación |
| Identidad | Referencia opcional a una identidad administrada asignada por el usuario | Confirmar si existe y está asociada. La credencial federada de GitHub pertenece a otro mecanismo |
| Load Balancer | Módulo Standard interno y opcional para staging | Subred, IP privada de frontend, IP privadas de los backends, capacidad y costos |

Los parámetros de staging y producción describen entornos de una misma VM. `bootstrap.sh`, Compose, certificados de Let's Encrypt y los scripts de despliegue siguen siendo el procedimiento vigente. La API continúa terminando TLS. MongoDB y Redis permanecen en redes privadas de Compose.

## Balanceador propuesto

El frontend privado se parametriza en una subred confirmada. El pool requiere al menos dos IP privadas distintas y confirmadas. La regla TCP de staging conserva el puerto 8443 y la sonda HTTPS usa `/healthz`. Validar en el diseño aprobado la conectividad del origen `AzureLoadBalancer`, el acceso de los clientes de staging, las asociaciones efectivas de NSG y la terminación TLS en cada backend. La configuración por defecto conserva las asociaciones actuales de NIC, NSG y DNS y mantiene el módulo desactivado en ambos entornos.

La topología de una sola VM conserva un único punto de fallo. Standard Load Balancer es un servicio administrado cuya disponibilidad y costo se deben verificar en la región y cuenta del proyecto. El diseño no solicita IP pública, VM ni discos adicionales.

El frontend privado necesita clientes con acceso a la VNet. El endpoint público y el DNS que usan las aplicaciones permanecen en la VM actual. Para evaluar capacidad, registrar el número de backends sanos, solicitudes por segundo, conexiones concurrentes, p95 y memoria de cada API, además del p95 de MongoDB y Redis. Dos procesos en la misma VM siguen compartiendo el mismo límite de CPU y memoria.

## Validación local

Usar el CLI de Bicep sin iniciar sesión en Azure.

```sh
bicep lint infra/main.bicep
bicep build infra/main.bicep --stdout
bicep build-params infra/params/staging.bicepparam --stdout
bicep build-params infra/params/production.bicepparam --stdout
```

En CI, revisar el resumen del job IaC para la compilación, el lint, la forma de la plantilla y el escaneo de secretos. Los parámetros base deben compilar con el balanceador apagado. Los recursos `existing` no deben convertirse en recursos administrados en el ARM generado. En producción debe permanecer desactivado el módulo incluso ante parámetros no esperados.

## Procedimiento futuro de what-if

1. Obtener autorización B-5 y confirmar la suscripción, identidad y alcance exactos. El equipo debe determinar si los permisos de lectura disponibles cubren la consulta. Azure puede exigir permisos de despliegue para `what-if`. Si los permisos son insuficientes, documentar el bloqueo y solicitar una decisión de acceso al responsable.
2. Inventariar las asociaciones efectivas de VM, discos, NIC, NSG, IP, subred e identidad. Completar los parámetros a partir de esa evidencia y comparar los IDs con el inventario actual.
3. Con el módulo de balanceo apagado, `devops-mcp` prepara la comparación en modo incremental. Ejemplo para el futuro, sujeto a autorización y a la versión del CLI.

```sh
az deployment group what-if \
  --resource-group <grupo-confirmado> \
  --template-file infra/main.bicep \
  --parameters infra/params/staging.bicepparam \
  --mode Incremental
```

4. Revisar cada operación y las dependencias. Una referencia válida puede apuntar al recurso equivocado. Detener la propuesta ante cualquier creación, modificación, eliminación, reemplazo o cambio ambiguo de un recurso actual. Contrastar por separado el estado de IP y DNS, los IDs de discos y el alcance de las reglas de red.
5. Si el equipo aprueba la enmienda y el costo, repetir la comparación para la propuesta de balanceo separada. Preparar la reversión con un inventario de asociaciones previas, comprobación de acceso directo y aprobación explícita para retirar únicamente recursos nuevos.

## Presupuesto y aprobaciones

El máximo nominal del crédito promocional indicado es de 200 USD. Antes de una propuesta remota, el responsable debe comprobar saldo real, vencimiento, gasto proyectado de la VM, disco e IP actuales, tarifas incrementales del balanceador, volumen procesado y margen de contingencia. Registrar también el escenario después del vencimiento y la decisión sobre cargos posteriores.

La estimación incremental suma las horas facturables por nivel de reglas, el volumen procesado por el balanceador y las tarifas aplicables de transferencia. Comparar el resultado y el gasto base proyectado contra el crédito restante después de descontar la reserva de contingencia. El plan usa una regla y una sonda para staging. Hasta confirmar tarifas, capacidad y vigencia, el costo remoto queda indeterminado.

La SPEC-018 está aprobada. El acceso B-5 se limita a lectura. El inventario y la facturación siguen pendientes por MFA de Azure CLI. `what-if`, B-6, la enmienda y la operación de escritura requieren autorización propia.

Para dos VMs independientes, usar el punto de entrada `infra/pilot.bicep`. Su archivo `infra/params/pilot.bicepparam` viene desactivado y conserva solo referencias de lectura a la VNet y la subred. El [piloto de dos VMs](seguridad/piloto-dos-vm-azure.md) documenta la propuesta vigente con máximo incremental de USD 15, ventana, reversión y retiro. La propuesta anterior de dos contenedores en una VM queda como alternativa para pruebas sin independencia de host.

## Propiedad de la infraestructura como código (Bicep y Terraform)

**Regla:** cada recurso tiene **un solo** dueño de estado IaC. Bicep es dueño de todo lo que vive en Azure. Terraform es dueño solo de la observabilidad externa (Better Stack). Ninguna configuración de Terraform declara recursos de Azure, así que no hay dos herramientas capaces de modificar el mismo recurso.

| Recurso | Bicep | Terraform | Existente o nuevo | Estado | Riesgo |
|---|---|---|---|---|---|
| VM1 `vm-vaiven`, su NIC, NSG, IP pública, disco del SO y VNet | Referencia `existing` (solo lectura) en `infra/main.bicep` y `infra/pilot.bicep` | No | Existentes (creados a mano) | Sin cambios por IaC | El what-if debe mostrar **cero** cambios en ellos; si muestra alguno, se detiene |
| VM2 del piloto, su NIC, NSG y disco | Dueño: `infra/pilot.bicep` y `modules/pilot-*.bicep` | No | Nuevos | Preparado; creación aprobada por el equipo, pendiente del what-if | Costo (≤ USD 15 en 4 h); se retiran al terminar |
| Standard Load Balancer interno del piloto | Dueño: `infra/modules/load-balancer.bicep` | No | Nuevo | Preparado | Interno: **no** reemplaza la URL pública actual |
| Monitores de Better Stack (healthz y readyz por entorno) | No | Dueño: `infra/terraform/observability` | Nuevos (externos a Azure) | Preparado y validado en la CI; sin aplicar (no hay cuenta) | Plan gratuito: verificar límites al crear la cuenta |
| Latido del escaneo de alertas | No | Dueño: `infra/terraform/observability` | Nuevo (externo) | Preparado | Si el escaneo se detiene, Better Stack alerta |

**Estado de Terraform:** nunca en Git (`.gitignore` excluye `*.tfstate` y `*.tfvars`). Para aplicarlo, el estado se guarda fuera del repositorio. El token va en `BETTERUPTIME_API_TOKEN`, nunca en archivos.

**Migración de propiedad:** si el equipo quisiera que Terraform administre recursos de Azure, se hace con `terraform import` y un `plan` sin cambios, y luego se retira el recurso de Bicep. Nunca las dos a la vez.
