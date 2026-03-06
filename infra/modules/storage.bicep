// modules/storage.bicep -- Azure Storage Account for game assets

param name        string
param location    string
param environment string

resource storage 'Microsoft.Storage/storageAccounts@2023-01-01' = {
  name:     name
  location: location
  tags: {
    environment: environment
    application: 'quake-cloud'
  }
  kind: 'StorageV2'
  sku: {
    name: 'Standard_LRS'
  }
  properties: {
    accessTier:             'Hot'
    allowBlobPublicAccess:  false
    allowSharedKeyAccess:   false   // Managed identity only — no storage keys
    minimumTlsVersion:      'TLS1_2'
    supportsHttpsTrafficOnly: true
    publicNetworkAccess:    'Enabled'  // Required for CLI uploads and Container Apps without VNet
    networkAcls: {
      defaultAction: 'Allow'           // Container Apps needs access; tighten with VNet integration
      bypass:        'AzureServices'
    }
  }
}

// Blob container for game data
resource blobService 'Microsoft.Storage/storageAccounts/blobServices@2023-01-01' = {
  parent: storage
  name: 'default'
}

resource gameDataContainer 'Microsoft.Storage/storageAccounts/blobServices/containers@2023-01-01' = {
  parent: blobService
  name: 'gamedata'
  properties: {
    publicAccess: 'None'
  }
}

output name string = storage.name
output id   string = storage.id
