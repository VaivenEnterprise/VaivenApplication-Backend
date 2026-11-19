# Piloto de Azure Standard Load Balancer

## Decisión técnica

La prueba local con Caddy balancea dos procesos de API en una VM. La red de Docker ofrece dos destinos al proxy, pero ambos comparten CPU, memoria, almacenamiento y dominio de fallo. Si cae la VM, se pierden ambos. Una arquitectura disponible ante fallos de VM necesita backends en VMs distintas y analizar también MongoDB, Redis, TLS y las dependencias compartidas.

El Standard Load Balancer de Azure balancea a destinos registrados en la VNet. Con una sola IP de NIC y una regla TCP `8443 → 8443`, dos contenedores siguen siendo un solo destino Azure. Una IP privada de la red bridge de Docker tampoco constituye un destino de ese pool.

| Opción | Alcance y recursos |
|---|---|
| Caddy local | Opción ya validada para demostrar reparto entre contenedores sin segunda VM. Para usarlo en la VM se necesita la enmienda, verificar terminación TLS, renovar certificados y conservar un camino de vuelta a la API directa. |
| Standard Load Balancer interno | Propuesta separada para verificar el servicio administrado. Un frontend privado, un pool por IP, una regla TCP y una sonda HTTPS. Candidato de costo mínimo: VM, NIC y VNet existentes, dos IP privadas de Azure para dos binds de API y un cliente de prueba existente dentro de la VNet. Solo considerar esta variante si se confirma que el mismo pool acepta ambos destinos sobre una VM y que la prueba no altera producción. |
| Alta disponibilidad entre VMs | Requiere otro dominio de fallo y capacidad adicional. Queda fuera del piloto propuesto. |

La plantilla mantiene el balanceador deshabilitado. Si el pool, los binds o el cliente de prueba interno no se pueden confirmar, conservar Caddy como demostración y detener el piloto administrado. El frontend interno queda fuera del flujo móvil público y deja el DNS, IP pública y rutas actuales con el acceso directo existente.

El ARM local exige un mínimo de dos IP de backend distintas antes de incluir el módulo. Esto evita aceptar una lista repetida como prueba de reparto. Queda por confirmar que las IP pertenezcan a configuraciones elegibles y que el pool admita la topología.

Fuentes: [Pools de Azure Load Balancer](https://learn.microsoft.com/en-us/azure/load-balancer/backend-pool-management) · [Varias IP en una NIC](https://learn.microsoft.com/en-us/azure/virtual-network/ip-services/virtual-network-multiple-ip-addresses-cli) · [Ejemplo con varias configuraciones IP](https://learn.microsoft.com/en-us/azure/load-balancer/load-balancer-multiple-ip)

## Presupuesto verificable

El catálogo público USD lista el SKU Standard regional con medidor **Global** y no devolvió un medidor separado para West US 2. Se usan las tarifas de lista como referencia conservadora. La elegibilidad para medidores gratuitos y el precio aplicable a la suscripción se deben confirmar antes de autorizar el piloto.

| Concepto | Tarifa pública USD |
|---|---:|
| Bloque de una a cinco reglas | 0,025 por hora facturable |
| Cada regla adicional | 0,010 por hora facturable |
| Datos procesados de entrada y salida | 0,005 por GB |
| Transferencia, zona y otros recursos asociados | Por confirmar según topología y contrato |

Fuentes: [API Azure Retail Prices](https://prices.azure.com/api/retail/prices?currencyCode=%27USD%27&%24filter=serviceName%20eq%20%27Load%20Balancer%27%20and%20armRegionName%20eq%20%27Global%27) · [Precios de Load Balancer](https://azure.microsoft.com/en-us/pricing/details/load-balancer/)

Azure anuncia para cuentas elegibles hasta 750 horas al mes, 15 GB procesados y cinco reglas de Standard Load Balancer durante 12 meses. El presupuesto de referencia no descuenta esa oferta hasta verificar su vigencia y consumo en VAIVÉN. [Servicios gratuitos](https://azure.microsoft.com/es-es/pricing/free-services) · [Uso de la cuota gratuita](https://learn.microsoft.com/en-us/azure/cost-management-billing/manage/check-free-service-usage)

Propuesta para revisión: **hasta dos horas transcurridas**, incluida la reversión. Presupuestar tres horas facturables como margen de redondeo y hasta 1 GB procesado. El cálculo del LB es `3 × 0,025 + 1 × 0,005 = USD 0,08`. Con una reserva propuesta de USD 2, el subtotal queda en **USD 2,08 más costos asociados**. La reserva cuenta contra el tope de USD 10. En un escenario sin crédito ni cuota gratuita, mantenerlo 730 horas costaría USD 18,25 por horas antes de datos y asociados.

Para una sola regla, el máximo estimado de horas facturables es `max(0, floor((10 − reserva − costos asociados − 0,005 × GB) / 0,025))`. Con USD 2 de reserva, 1 GB y asociados hipotéticamente cero, serían 319 horas facturables. Es un límite teórico de la tarifa pública. El plan operativo mantiene dos horas y requiere saldo, cuotas, facturación y aprobación confirmados. Las alertas de Azure Budgets sirven para avisar. La eliminación del recurso y la revisión de su facturación cierran el piloto.

## Consulta B-5 y requisito de aprobación

La lectura autorizada cubre saldo promocional, vencimiento, gasto, cuotas y red. Para solicitar la operación concreta, adjuntar el saldo y fecha de fin del crédito, gasto base acumulado y proyectado, tipo de suscripción y límite de gasto, cupos regionales para LB e IP privadas, precios aplicables y una lista de cambios exacta. Separar costos incrementales de la VM, disco e IP ya existentes. El crédito nominal de USD 200 y el tope propuesto de USD 10 requieren autorización de gasto posterior.

Estado al 18 de noviembre de 2026: el responsable autorizó B-5 para lectura. Azure solicitó MFA al renovar credenciales. El saldo, vencimiento, gasto, límites de facturación, cuotas disponibles y la configuración real de red quedan sin verificar. El presupuesto del piloto queda como estimación hasta que la cuenta autorizada se reautentique. Los límites públicos sirven de referencia y la consulta regional debe contrastarse con el uso real. [Límites públicos](https://learn.microsoft.com/en-us/azure/azure-resource-manager/management/azure-subscription-service-limits#azure-load-balancer-limits) · [Consulta de uso regional](https://learn.microsoft.com/en-us/azure/networking/check-usage-against-limits)

## Ventana del piloto y reversión

1. Antes de ejecutar, conseguir B-6, aprobación formal de la SPEC-018 y la enmienda aplicable. Autorizar `what-if` por separado. Inventariar los IDs de VM, NIC, NSG, IP, discos, VNet y subred, y guardar una copia de la configuración inicial sin secretos. Revisar el resultado incremental y, en particular, cualquier escritura en recursos existentes.
2. Preparar una ventana exclusiva de staging. Confirmar el soporte de dos destinos, la sonda HTTPS sobre `/healthz`, la validez del TLS en cada backend, el acceso desde un cliente de la VNet que no sea backend y las reglas efectivas del NSG para el servicio de sondas. Probar con conexiones TCP independientes y registrar la réplica. Detener una réplica y verificar el destino de las conexiones nuevas. Las conexiones preexistentes tienen semántica distinta.
3. A los 90 minutos comienza la reversión. Restablecer la configuración original de Compose y los binds de staging. Verificar HTTPS en `/healthz`, `/readyz` y `/api/v1/meta` en el acceso habitual antes de retirar el balanceador.
4. Retirar únicamente la regla, sonda, pool y frontend nuevos junto con el LB del piloto. Restaurar, con aprobación específica, las configuraciones IP secundarias y reglas de red del piloto que consten en el inventario de cambios. Mantener VM, discos, IP pública y NSG originales. Verificar ausencia del recurso nuevo y el gasto diferido en facturación. Si alguna operación falla, escalar con los IDs confirmados y mantener el acceso directo.

Fuentes: [Sondas de Azure Load Balancer](https://learn.microsoft.com/en-us/azure/load-balancer/load-balancer-custom-probe-overview) · [Azure Budgets](https://learn.microsoft.com/en-us/azure/cost-management-billing/costs/tutorial-acm-create-budgets)
