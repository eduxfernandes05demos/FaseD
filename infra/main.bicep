/*
  main.bicep
  Root Bicep template for the Quake Cloud Platform.

  Provisions:
    - Azure Container Registry (ACR)
    - Azure Container Apps Environment (with Log Analytics)
    - Azure Files storage account for game assets
    - Azure Key Vault
    - Application Insights
*/

targetScope = 'resourceGroup'

// ---------------------------------------------------------------------------
// Parameters
// ---------------------------------------------------------------------------

@description('Short environment identifier, e.g. dev / staging / prod.')
@allowed(['dev', 'staging', 'prod'])
param environment string = 'dev'

@description('Azure region for all resources.')
param location string = resourceGroup().location

@description('Name prefix applied to all resource names.')
@minLength(3)
@maxLength(12)
param prefix string = 'quake'

@description('Container image tag to deploy to the game-worker.')
param gameWorkerImageTag string = 'latest'

@description('Container image tag to deploy to the streaming-gateway.')
param streamingGatewayImageTag string = 'latest'

@description('Quake start map (e.g. e1m1).')
param quakeMap string = 'e1m1'

@description('Quake skill level (0–3).')
@minValue(0)
@maxValue(3)
param quakeSkill int = 1

// ---------------------------------------------------------------------------
// Variables
// ---------------------------------------------------------------------------

var resourceSuffix = '${prefix}-${environment}'
var acrName        = replace('${prefix}acr${environment}', '-', '')  // ACR names: alphanumeric only

// ---------------------------------------------------------------------------
// Modules
// ---------------------------------------------------------------------------

module logAnalytics 'modules/log-analytics.bicep' = {
  name: 'log-analytics'
  params: {
    name:        'law-${resourceSuffix}'
    location:    location
    environment: environment
  }
}

module appInsights 'modules/app-insights.bicep' = {
  name: 'app-insights'
  params: {
    name:                'appi-${resourceSuffix}'
    location:            location
    environment:         environment
    logAnalyticsId:      logAnalytics.outputs.workspaceId
  }
}

module acr 'modules/acr.bicep' = {
  name: 'acr'
  params: {
    name:        acrName
    location:    location
    environment: environment
  }
}

module keyVault 'modules/key-vault.bicep' = {
  name: 'key-vault'
  params: {
    name:        'kv-${resourceSuffix}'
    location:    location
    environment: environment
  }
}

module storage 'modules/storage.bicep' = {
  name: 'storage'
  params: {
    name:        storageAccountName
    location:    location
    environment: environment
  }
}

var storageAccountName = replace('st${prefix}${environment}', '-', '')

module containerAppsEnv 'modules/container-apps-env.bicep' = {
  name: 'container-apps-env'
  params: {
    name:                   'cae-${resourceSuffix}'
    location:               location
    environment:            environment
    logAnalyticsWorkspaceCustomerId: logAnalytics.outputs.workspaceCustomerId
    logAnalyticsKey:                 logAnalytics.outputs.primarySharedKey
  }
}

// ---------------------------------------------------------------------------
// User-Assigned Managed Identity & RBAC
// ---------------------------------------------------------------------------

resource gameWorkerIdentity 'Microsoft.ManagedIdentity/userAssignedIdentities@2023-01-31' = {
  name:     'id-game-worker-${resourceSuffix}'
  location: location
  tags: {
    environment: environment
    application: 'quake-cloud'
  }
}

// Existing references for role-assignment scoping
resource storageAccountRef 'Microsoft.Storage/storageAccounts@2023-01-01' existing = {
  name: storageAccountName
  dependsOn: [storage]
}

resource acrRef 'Microsoft.ContainerRegistry/registries@2023-07-01' existing = {
  name: acrName
  dependsOn: [acr]
}

// Storage Blob Data Reader – lets the init container download game data
var storageBlobDataReaderRoleId = '2a2b9908-6ea1-4ae2-8e65-a410df84e7d1'
resource blobReaderRole 'Microsoft.Authorization/roleAssignments@2022-04-01' = {
  name:  guid(storageAccountRef.id, gameWorkerIdentity.id, storageBlobDataReaderRoleId)
  scope: storageAccountRef
  properties: {
    roleDefinitionId: subscriptionResourceId('Microsoft.Authorization/roleDefinitions', storageBlobDataReaderRoleId)
    principalId:      gameWorkerIdentity.properties.principalId
    principalType:    'ServicePrincipal'
  }
}

// AcrPull – lets the container app pull images from ACR
var acrPullRoleId = '7f951dda-4ed3-4680-a7ca-43fe172d538d'
resource acrPullRole 'Microsoft.Authorization/roleAssignments@2022-04-01' = {
  name:  guid(acrRef.id, gameWorkerIdentity.id, acrPullRoleId)
  scope: acrRef
  properties: {
    roleDefinitionId: subscriptionResourceId('Microsoft.Authorization/roleDefinitions', acrPullRoleId)
    principalId:      gameWorkerIdentity.properties.principalId
    principalType:    'ServicePrincipal'
  }
}

module quakeStreaming 'modules/quake-streaming.bicep' = {
  name: 'quake-streaming'
  dependsOn: [blobReaderRole, acrPullRole]
  params: {
    name:                         'ca-quake-streaming-${environment}'
    location:                     location
    environment:                  environment
    containerAppsEnvId:           containerAppsEnv.outputs.id
    acrLoginServer:               acr.outputs.loginServer
    gameWorkerImageTag:           gameWorkerImageTag
    streamingGatewayImageTag:     streamingGatewayImageTag
    quakeMap:                     quakeMap
    quakeSkill:                   string(quakeSkill)
    appInsightsKey:               appInsights.outputs.instrumentationKey
    storageAccountName:           storageAccountName
    userAssignedIdentityId:       gameWorkerIdentity.id
    userAssignedIdentityClientId: gameWorkerIdentity.properties.clientId
  }
}

// ---------------------------------------------------------------------------
// Outputs
// ---------------------------------------------------------------------------

output acrLoginServer        string = acr.outputs.loginServer
output containerAppsEnvId    string = containerAppsEnv.outputs.id
output quakeStreamingFqdn    string = quakeStreaming.outputs.fqdn
output storageAccountName    string = storage.outputs.name
output keyVaultUri           string = keyVault.outputs.uri
