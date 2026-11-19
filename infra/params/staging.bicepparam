using '../main.bicep'

param environment = 'staging'
param location = 'westus2'

// Los parámetros por confirmar tras B-5 se omiten hasta revisar el inventario.
param network = {}
param compute = {
  vmName: 'vm-vaiven'
}
param security = {
  nsgName: 'vm-vaivenNSG'
}

param loadBalancer = null
