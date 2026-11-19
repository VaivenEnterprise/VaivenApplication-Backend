targetScope = 'resourceGroup'

param namePrefix string
param location string
param tags object
param nicId string
param vmSize string
param osDiskSizeGb int
param osDiskSku 'Standard_LRS' | 'StandardSSD_LRS'
param imageSku string
param imageVersion string
param adminUsername string
param sshPublicKey string

// El disco administrado se crea de la imagen aprobada y se elimina junto con la VM
// solo cuando se apruebe la operación de limpieza. No se clonan discos existentes.
resource pilotVm 'Microsoft.Compute/virtualMachines@2024-03-01' = {
  name: '${namePrefix}-vm'
  location: location
  tags: tags
  properties: {
    hardwareProfile: {
      vmSize: vmSize
    }
    storageProfile: {
      imageReference: {
        publisher: 'Canonical'
        offer: 'ubuntu-24_04-lts'
        sku: imageSku
        version: imageVersion
      }
      osDisk: {
        name: '${namePrefix}-os'
        createOption: 'FromImage'
        deleteOption: 'Delete'
        diskSizeGB: osDiskSizeGb
        managedDisk: {
          storageAccountType: osDiskSku
        }
      }
    }
    osProfile: {
      computerName: '${namePrefix}-vm'
      adminUsername: adminUsername
      linuxConfiguration: {
        disablePasswordAuthentication: true
        ssh: {
          publicKeys: [
            {
              path: '/home/${adminUsername}/.ssh/authorized_keys'
              keyData: sshPublicKey
            }
          ]
        }
      }
    }
    networkProfile: {
      networkInterfaces: [
        {
          id: nicId
          properties: {
            primary: true
            deleteOption: 'Delete'
          }
        }
      ]
    }
    securityProfile: {
      securityType: 'TrustedLaunch'
      uefiSettings: {
        secureBootEnabled: true
        vTpmEnabled: true
      }
    }
  }
}
