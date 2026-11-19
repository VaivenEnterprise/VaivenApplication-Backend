targetScope = 'resourceGroup'

@sealed()
type PilotConfiguration = {
  @minLength(1)
  @maxLength(20)
  suffix: string
  @minLength(1)
  location: string
  @minLength(20)
  @maxLength(20)
  expiresAtUtc: string
  @minLength(1)
  vnetName: string
  @minLength(1)
  subnetName: string
  existingVmPrivateIp: string
  pilotVmPrivateIp: string
  frontendPrivateIp: string
  clientSourceCidr: string
  adminSourceCidr: string?
  mongoPrivateIp: string
  redisPrivateIp: string
  artifactRelayPrivateIp: string?
  @minLength(1)
  vmSize: string
  @minValue(1)
  osDiskSizeGb: int
  osDiskSku: 'Standard_LRS' | 'StandardSSD_LRS'
  @minLength(1)
  imageSku: string
  @minLength(1)
  imageVersion: string
  @minLength(1)
  @maxLength(32)
  adminUsername: string
  @minLength(64)
  sshPublicKey: string
}

@description('El piloto está limitado a staging.')
param environment 'staging' | 'production' = 'staging'

@description('Habilitación expresa del piloto. Mantener false antes de la aprobación de escritura.')
param enablePilot bool = false

@description('Confirmación registrada por el responsable sobre inventario, red, costos, cuota y permisos. El template no verifica esas aprobaciones.')
param inventoryAndBudgetConfirmed bool = false

@description('Solo valores de inventario verificados. La red existente se referencia por nombre en el grupo objetivo.')
param pilot PilotConfiguration?

func isDigits(value string) bool => !empty(value) && empty(reduce(range(0, 10), value, (remaining, digit) => replace(remaining, string(digit), '')))

func isIpv4Octet(value string) bool => (length(value) <= 3 && isDigits(value))
  ? int(value) <= 255 && string(int(value)) == value
  : false

func isPrivateIp(value string) bool => length(filter(split(value, '.'), part => isIpv4Octet(part))) == 4 && length(split(value, '.')) == 4
  ? startsWith(value, '10.') || startsWith(value, '192.168.') || (startsWith(value, '172.') && contains(range(16, 16), int(split(value, '.')[1])))
  : false

func privatePrefixFloor(address string) int => startsWith(address, '10.') ? 8 : startsWith(address, '172.') ? 12 : 16

func isPrivateCidr(value string) bool => length(split(value, '/')) == 2 && isPrivateIp(first(split(value, '/'))) && length(last(split(value, '/'))) <= 2 && isDigits(last(split(value, '/')))
  ? int(last(split(value, '/'))) >= privatePrefixFloor(first(split(value, '/'))) && int(last(split(value, '/'))) <= 32
  : false

func isLowercaseAlnum(value string) bool => !empty(value) && empty(filter(range(0, length(value)), index => !contains('abcdefghijklmnopqrstuvwxyz0123456789', substring(value, index, 1))))

func isPublicKey(value string) bool => !contains(value, '\n') && !contains(value, '\r') && (startsWith(value, 'ssh-ed25519 AAAAC3NzaC1lZDI1NTE5') || startsWith(value, 'ssh-rsa AAAAB3NzaC1yc2E'))

var suffix = pilot.?suffix ?? ''
var location = pilot.?location ?? ''
var expiresAtUtc = pilot.?expiresAtUtc ?? ''
var vnetName = pilot.?vnetName ?? ''
var subnetName = pilot.?subnetName ?? ''
var existingVmPrivateIp = pilot.?existingVmPrivateIp ?? ''
var pilotVmPrivateIp = pilot.?pilotVmPrivateIp ?? ''
var frontendPrivateIp = pilot.?frontendPrivateIp ?? ''
var clientSourceCidr = pilot.?clientSourceCidr ?? ''
var adminSourceCidr = pilot.?adminSourceCidr ?? ''
var mongoPrivateIp = pilot.?mongoPrivateIp ?? ''
var redisPrivateIp = pilot.?redisPrivateIp ?? ''
var artifactRelayPrivateIp = pilot.?artifactRelayPrivateIp ?? ''
var endpointIps = [existingVmPrivateIp, pilotVmPrivateIp, frontendPrivateIp]
var endpointsDistinct = length(union(endpointIps, endpointIps)) == 3
var frontendAndBackendsValid = endpointsDistinct && empty(filter(endpointIps, address => !isPrivateIp(address)))
var sourcesValid = isPrivateCidr(clientSourceCidr) && (empty(adminSourceCidr) || isPrivateCidr(adminSourceCidr))
var destinationsValid = isPrivateIp(mongoPrivateIp) && isPrivateIp(redisPrivateIp) && (empty(artifactRelayPrivateIp) || isPrivateIp(artifactRelayPrivateIp))
var addressShapeValid = frontendAndBackendsValid && sourcesValid && destinationsValid
var inventoryNamed = !empty(location) && !empty(vnetName) && !empty(subnetName)
var ttlShapeValid = length(expiresAtUtc) == 20 && endsWith(expiresAtUtc, 'Z')
var vmConfigValid = isLowercaseAlnum(suffix) && isPublicKey(pilot.?sshPublicKey ?? '') && toLower(pilot.?imageVersion ?? '') != 'latest'
var configurationReady = pilot != null && inventoryNamed && ttlShapeValid && addressShapeValid && vmConfigValid
var pilotEligible = environment == 'staging' && enablePilot && inventoryAndBudgetConfirmed && configurationReady
var namePrefix = 'vaiven-pilot-${suffix}'
var pilotTags = {
  project: 'vaiven'
  environment: 'staging'
  purpose: 'spec-018-pilot'
  expiresAtUtc: expiresAtUtc
}

resource vnet 'Microsoft.Network/virtualNetworks@2024-05-01' existing = if (pilotEligible) {
  name: vnetName
}

resource subnet 'Microsoft.Network/virtualNetworks/subnets@2024-05-01' existing = if (pilotEligible) {
  parent: vnet
  name: subnetName
}

module pilotNetwork './modules/pilot-network.bicep' = if (pilotEligible) {
  name: '${namePrefix}-network'
  params: {
    namePrefix: namePrefix
    location: location
    tags: pilotTags
    subnetId: subnet.id
    pilotVmPrivateIp: pilotVmPrivateIp
    clientSourceCidr: clientSourceCidr
    adminSourceCidr: adminSourceCidr
    mongoPrivateIp: mongoPrivateIp
    redisPrivateIp: redisPrivateIp
    artifactRelayPrivateIp: artifactRelayPrivateIp
  }
}

module pilotVm './modules/pilot-vm.bicep' = if (pilotEligible) {
  name: '${namePrefix}-compute'
  params: {
    namePrefix: namePrefix
    location: location
    tags: pilotTags
    nicId: pilotNetwork!.outputs.nicId
    vmSize: pilot!.vmSize
    osDiskSizeGb: pilot!.osDiskSizeGb
    osDiskSku: pilot!.osDiskSku
    imageSku: pilot!.imageSku
    imageVersion: pilot!.imageVersion
    adminUsername: pilot!.adminUsername
    sshPublicKey: pilot!.sshPublicKey
  }
}

module pilotLoadBalancer './modules/load-balancer.bicep' = if (pilotEligible) {
  name: '${namePrefix}-balancing'
  params: {
    location: location
    tags: pilotTags
    vnetId: vnet.id
    subnetId: subnet.id
    tlsPort: 8443
    settings: {
      name: '${namePrefix}-lb'
      frontendPrivateIp: frontendPrivateIp
      backendPrivateIps: [
        existingVmPrivateIp
        pilotVmPrivateIp
      ]
    }
  }
  dependsOn: [
    pilotVm
  ]
}

output pilotEligible bool = pilotEligible
output names object = pilotEligible ? {
  networkSecurityGroup: '${namePrefix}-nsg'
  networkInterface: '${namePrefix}-nic'
  virtualMachine: '${namePrefix}-vm'
  managedOsDisk: '${namePrefix}-os'
  loadBalancer: '${namePrefix}-lb'
} : {}
