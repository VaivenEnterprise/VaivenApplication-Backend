targetScope = 'resourceGroup'

import { NetworkInventory } from './modules/network.bicep'
import { ComputeInventory } from './modules/compute.bicep'
import { SecurityInventory } from './modules/security.bicep'
import { InternalLoadBalancerSettings } from './modules/load-balancer.bicep'

@description('Entorno de Docker Compose. Ambos entornos comparten recursos Azure.')
param environment 'staging' | 'production'

@description('Región de la VM y de la red, según inventario.')
param location string

@description('Nombres de red existentes en el grupo objetivo. Pendientes donde el nombre no se ha verificado.')
param network NetworkInventory

@description('Nombres de la VM y los discos existentes en el grupo objetivo.')
param compute ComputeInventory

@description('Nombres del NSG y de una identidad administrada asignada por usuario, si existe.')
param security SecurityInventory

@description('Propuesta de balanceador privado para staging. Dejar nulo sin autorización. No se habilita en producción.')
param loadBalancer InternalLoadBalancerSettings?

var vnetName = network.?vnetName ?? ''
var subnetName = network.?subnetName ?? ''
var nicName = network.?nicName ?? ''
var publicIpName = network.?publicIpName ?? ''
var osDiskName = compute.?osDiskName ?? ''
var dataDiskNames = compute.?dataDiskNames ?? []
var identityName = security.?userAssignedIdentityName ?? ''
var subnetKnown = !empty(vnetName) && !empty(subnetName)
var tlsPort = environment == 'staging' ? 8443 : 443
var backendIps = loadBalancer.?backendPrivateIps ?? []
var hasDistinctBackends = length(union(backendIps, backendIps)) == length(backendIps) && length(backendIps) >= 2
var loadBalancerEligible = environment == 'staging' && loadBalancer != null && subnetKnown && hasDistinctBackends

resource vnet 'Microsoft.Network/virtualNetworks@2024-05-01' existing = if (!empty(vnetName)) {
  name: vnetName
}

resource subnet 'Microsoft.Network/virtualNetworks/subnets@2024-05-01' existing = if (subnetKnown) {
  parent: vnet
  name: subnetName
}

resource nic 'Microsoft.Network/networkInterfaces@2024-05-01' existing = if (!empty(nicName)) {
  name: nicName
}

resource publicIp 'Microsoft.Network/publicIPAddresses@2024-05-01' existing = if (!empty(publicIpName)) {
  name: publicIpName
}

resource nsg 'Microsoft.Network/networkSecurityGroups@2024-05-01' existing = {
  name: security.nsgName
}

resource vm 'Microsoft.Compute/virtualMachines@2024-03-01' existing = {
  name: compute.vmName
}

resource osDisk 'Microsoft.Compute/disks@2024-03-02' existing = if (!empty(osDiskName)) {
  name: osDiskName
}

resource dataDisks 'Microsoft.Compute/disks@2024-03-02' existing = [
  for diskName in dataDiskNames: {
    name: diskName
  }
]

resource identity 'Microsoft.ManagedIdentity/userAssignedIdentities@2023-01-31' existing = if (!empty(identityName)) {
  name: identityName
}

var dataDiskIds = [for index in range(0, length(dataDiskNames)): dataDisks[index].id]

module proposedLoadBalancer './modules/load-balancer.bicep' = if (loadBalancerEligible) {
  name: 'vaiven-staging-lb-candidate'
  params: {
    location: location
    vnetId: vnet.id
    subnetId: subnet.id
    settings: loadBalancer!
    tlsPort: tlsPort
  }
}

// Las salidas son identificadores ARM, no lecturas de estado ni pruebas de existencia.
output inventory object = {
  vmId: vm.id
  vnetId: !empty(vnetName) ? vnet.id : null
  subnetId: subnetKnown ? subnet.id : null
  nicId: !empty(nicName) ? nic.id : null
  publicIpId: !empty(publicIpName) ? publicIp.id : null
  networkSecurityGroupId: nsg.id
  osDiskId: !empty(osDiskName) ? osDisk.id : null
  dataDiskIds: dataDiskIds
  userAssignedIdentityId: !empty(identityName) ? identity.id : null
}

output apiTlsPort int = tlsPort
output includesLoadBalancerCandidate bool = loadBalancerEligible
