using '../main.bicep'

param environment = 'production'
param location = 'westus2'

// Usa la misma VM y red de Azure. La separación de entornos vive en Compose.
param network = {}
param compute = {
  vmName: 'vm-vaiven'
}
param security = {
  nsgName: 'vm-vaivenNSG'
}

param loadBalancer = null
