// Inventario de recursos existentes. Los discos son recursos administrados por Azure.
@export()
@sealed()
type ComputeInventory = {
  @minLength(1)
  vmName: string
  osDiskName: string?
  dataDiskNames: string[]?
}
