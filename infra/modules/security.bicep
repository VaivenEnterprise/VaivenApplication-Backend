@export()
@sealed()
type SecurityInventory = {
  @minLength(1)
  nsgName: string
  // Solo para una identidad administrada asignada por usuario, si se confirma.
  userAssignedIdentityName: string?
}
