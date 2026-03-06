// modules/container-apps-env.bicep -- Azure Container Apps Environment

param name                      string
param location                  string
param environment               string
param logAnalyticsWorkspaceCustomerId string
@secure()
param logAnalyticsKey                string

resource cae 'Microsoft.App/managedEnvironments@2024-03-01' = {
  name:     name
  location: location
  tags: {
    environment: environment
    application: 'quake-cloud'
  }
  properties: {
    appLogsConfiguration: {
      destination: 'log-analytics'
      logAnalyticsConfiguration: {
        customerId: logAnalyticsWorkspaceCustomerId
        sharedKey:  logAnalyticsKey
      }
    }
    zoneRedundant: false
  }
}

output id   string = cae.id
output name string = cae.name
