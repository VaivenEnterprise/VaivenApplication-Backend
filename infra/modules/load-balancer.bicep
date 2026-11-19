targetScope = 'resourceGroup'

@export()
@sealed()
type InternalLoadBalancerSettings = {
  @minLength(1)
  name: string
  @minLength(7)
  frontendPrivateIp: string
  @minLength(2)
  backendPrivateIps: string[]
}

@description('Región confirmada de la red y de sus backends.')
param location string

@description('Id de la red virtual existente que contiene los backends.')
param vnetId string

@description('Id de la subred existente para el frontend privado.')
param subnetId string

@description('Nombre e IP privadas confirmadas. Las direcciones deben pertenecer a la red virtual existente.')
param settings InternalLoadBalancerSettings

@description('Puerto HTTPS que ya sirve la API. El balanceador conserva TLS hasta el backend.')
@minValue(1)
@maxValue(65535)
param tlsPort int

@description('Tags de seguimiento. El inventario anterior conserva su configuración por defecto.')
param tags object = {}

var balancerId = resourceId('Microsoft.Network/loadBalancers', settings.name)
var frontendId = '${balancerId}/frontendIPConfigurations/private'
var backendPoolId = '${balancerId}/backendAddressPools/api'
var probeId = '${balancerId}/probes/api-https'

// Recurso propuesto. Una asociación por dirección IP evita escribir la NIC.
// No se modifican NSG, red virtual, VMs ni IP públicas.
resource balancer 'Microsoft.Network/loadBalancers@2024-05-01' = {
  name: settings.name
  location: location
  tags: tags
  sku: {
    name: 'Standard'
    tier: 'Regional'
  }
  properties: {
    frontendIPConfigurations: [
      {
        name: 'private'
        properties: {
          privateIPAllocationMethod: 'Static'
          privateIPAddress: settings.frontendPrivateIp
          privateIPAddressVersion: 'IPv4'
          subnet: {
            id: subnetId
          }
        }
      }
    ]
    backendAddressPools: [
      {
        name: 'api'
        properties: {
          loadBalancerBackendAddresses: [
            for (ip, index) in settings.backendPrivateIps: {
              name: 'api-${index + 1}'
              properties: {
                ipAddress: ip
                virtualNetwork: {
                  id: vnetId
                }
              }
            }
          ]
        }
      }
    ]
    probes: [
      {
        name: 'api-https'
        properties: {
          protocol: 'Https'
          port: tlsPort
          requestPath: '/healthz'
          intervalInSeconds: 15
          numberOfProbes: 2
        }
      }
    ]
    loadBalancingRules: [
      {
        name: 'api-tls'
        properties: {
          protocol: 'Tcp'
          frontendPort: tlsPort
          backendPort: tlsPort
          frontendIPConfiguration: {
            id: frontendId
          }
          backendAddressPool: {
            id: backendPoolId
          }
          probe: {
            id: probeId
          }
          enableFloatingIP: false
          disableOutboundSnat: true
        }
      }
    ]
  }
}
