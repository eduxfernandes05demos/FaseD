// modules/acr.bicep -- Azure Container Registry

param name        string
param location    string
param environment string

resource acr 'Microsoft.ContainerRegistry/registries@2023-07-01' = {
  name:     name
  location: location
  tags: {
    environment: environment
    application: 'quake-cloud'
  }
  sku: {
    name: environment == 'prod' ? 'Premium' : 'Basic'
  }
  properties: {
    adminUserEnabled: false
    publicNetworkAccess: 'Enabled'
  }
}

output loginServer string = acr.properties.loginServer
output id          string = acr.id
