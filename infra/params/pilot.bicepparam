using '../pilot.bicep'

// Estado de entrega: no se administra ningún recurso de Azure.
param environment = 'staging'
param enablePilot = false
param inventoryAndBudgetConfirmed = false
param pilot = null
