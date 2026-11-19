// Nombres de los recursos de red de la VM compartida, pendientes de B-5.
@export()
@sealed()
type NetworkInventory = {
  vnetName: string?
  subnetName: string?
  nicName: string?
  publicIpName: string?
}
