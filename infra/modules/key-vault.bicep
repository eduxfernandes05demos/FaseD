// modules/key-vault.bicep -- Azure Key Vault

param name        string
param location    string
param environment string

resource kv 'Microsoft.KeyVault/vaults@2023-07-01' = {
  name:     name
  location: location
  tags: {
    environment: environment
    application: 'quake-cloud'
  }
  properties: {
    sku: {
      family: 'A'
      name:   'standard'
    }
    tenantId:                        tenant().tenantId
    enableRbacAuthorization:         true
    enableSoftDelete:                true
    softDeleteRetentionInDays:       7
    enablePurgeProtection:           true
    publicNetworkAccess:             'Enabled'
    networkAcls: {
      defaultAction: 'Allow'
      bypass:        'AzureServices'
    }
  }
}

output uri string = kv.properties.vaultUri
output id  string = kv.id
