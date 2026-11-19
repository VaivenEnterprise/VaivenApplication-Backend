targetScope = 'resourceGroup'

param namePrefix string
param location string
param tags object
param subnetId string
param pilotVmPrivateIp string
param clientSourceCidr string
param adminSourceCidr string
param mongoPrivateIp string
param redisPrivateIp string
param artifactRelayPrivateIp string

var baseRules = [
  {
    name: 'api-private-clients'
    properties: {
      priority: 100
      direction: 'Inbound'
      access: 'Allow'
      protocol: 'Tcp'
      sourceAddressPrefix: clientSourceCidr
      sourcePortRange: '*'
      destinationAddressPrefix: pilotVmPrivateIp
      destinationPortRange: '8443'
    }
  }
  {
    name: 'api-health-probe'
    properties: {
      priority: 110
      direction: 'Inbound'
      access: 'Allow'
      protocol: 'Tcp'
      sourceAddressPrefix: 'AzureLoadBalancer'
      sourcePortRange: '*'
      destinationAddressPrefix: pilotVmPrivateIp
      destinationPortRange: '8443'
    }
  }
  {
    name: 'deny-other-ingress'
    properties: {
      priority: 4096
      direction: 'Inbound'
      access: 'Deny'
      protocol: '*'
      sourceAddressPrefix: '*'
      sourcePortRange: '*'
      destinationAddressPrefix: '*'
      destinationPortRange: '*'
    }
  }
  {
    name: 'mongo-private'
    properties: {
      priority: 100
      direction: 'Outbound'
      access: 'Allow'
      protocol: 'Tcp'
      sourceAddressPrefix: pilotVmPrivateIp
      sourcePortRange: '*'
      destinationAddressPrefix: mongoPrivateIp
      destinationPortRange: '27017'
    }
  }
  {
    name: 'redis-private'
    properties: {
      priority: 110
      direction: 'Outbound'
      access: 'Allow'
      protocol: 'Tcp'
      sourceAddressPrefix: pilotVmPrivateIp
      sourcePortRange: '*'
      destinationAddressPrefix: redisPrivateIp
      destinationPortRange: '6379'
    }
  }
  {
    name: 'platform-dns'
    properties: {
      priority: 120
      direction: 'Outbound'
      access: 'Allow'
      protocol: '*'
      sourceAddressPrefix: pilotVmPrivateIp
      sourcePortRange: '*'
      destinationAddressPrefix: 'AzurePlatformDNS'
      destinationPortRange: '53'
    }
  }
  {
    name: 'deny-other-egress'
    properties: {
      priority: 4096
      direction: 'Outbound'
      access: 'Deny'
      protocol: '*'
      sourceAddressPrefix: '*'
      sourcePortRange: '*'
      destinationAddressPrefix: '*'
      destinationPortRange: '*'
    }
  }
]

var adminRules = empty(adminSourceCidr) ? [] : [
  {
    name: 'ssh-private-admin'
    properties: {
      priority: 120
      direction: 'Inbound'
      access: 'Allow'
      protocol: 'Tcp'
      sourceAddressPrefix: adminSourceCidr
      sourcePortRange: '*'
      destinationAddressPrefix: pilotVmPrivateIp
      destinationPortRange: '22'
    }
  }
]

var relayRules = empty(artifactRelayPrivateIp) ? [] : [
  {
    name: 'artifact-relay-private'
    properties: {
      priority: 130
      direction: 'Outbound'
      access: 'Allow'
      protocol: 'Tcp'
      sourceAddressPrefix: pilotVmPrivateIp
      sourcePortRange: '*'
      destinationAddressPrefix: artifactRelayPrivateIp
      destinationPortRange: '443'
    }
  }
]

resource pilotNsg 'Microsoft.Network/networkSecurityGroups@2024-05-01' = {
  name: '${namePrefix}-nsg'
  location: location
  tags: tags
  properties: {
    securityRules: concat(baseRules, adminRules, relayRules)
  }
}

resource pilotNic 'Microsoft.Network/networkInterfaces@2024-05-01' = {
  name: '${namePrefix}-nic'
  location: location
  tags: tags
  properties: {
    enableIPForwarding: false
    networkSecurityGroup: {
      id: pilotNsg.id
    }
    ipConfigurations: [
      {
        name: 'pilot-ip'
        properties: {
          primary: true
          privateIPAllocationMethod: 'Static'
          privateIPAddressVersion: 'IPv4'
          privateIPAddress: pilotVmPrivateIp
          subnet: {
            id: subnetId
          }
        }
      }
    ]
  }
}

output nicId string = pilotNic.id
