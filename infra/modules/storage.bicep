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
    minimumTlsVersion:      'TLS1_2'
    supportsHttpsTrafficOnly: true
  }
}

// Azure Files share for game data (id1/ directory)
resource fileShare 'Microsoft.Storage/storageAccounts/fileServices/shares@2023-01-01' = {
  name: '${storage.name}/default/gamedata'
  properties: {
    shareQuota:      5   // GiB
    enabledProtocols: 'SMB'
  }
}

output name             string = storage.name
output id               string = storage.id
output primaryFileEndpoint string = storage.properties.primaryEndpoints.file
