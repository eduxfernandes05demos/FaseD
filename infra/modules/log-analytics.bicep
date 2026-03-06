// modules/log-analytics.bicep -- Azure Log Analytics workspace

param name        string
param location    string
param environment string

resource workspace 'Microsoft.OperationalInsights/workspaces@2023-09-01' = {
  name:     name
  location: location
  tags: {
    environment: environment
    application: 'quake-cloud'
  }
  properties: {
    sku: {
      name: 'PerGB2018'
    }
    retentionInDays: 30
    features: {
      enableLogAccessUsingOnlyResourcePermissions: true
    }
  }
}

output workspaceId       string = workspace.id
output workspaceCustomerId string = workspace.properties.customerId

@description('Log Analytics primary shared key. Marked secure to satisfy linter.')
@secure()
output primarySharedKey  string = workspace.listKeys().primarySharedKey
