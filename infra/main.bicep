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
    name:        replace('st${prefix}${environment}', '-', '')
    location:    location
    environment: environment
  }
}

module containerAppsEnv 'modules/container-apps-env.bicep' = {
  name: 'container-apps-env'
  params: {
    name:                   'cae-${resourceSuffix}'
    location:               location
    environment:            environment
    logAnalyticsWorkspaceId: logAnalytics.outputs.workspaceId
    logAnalyticsKey:        logAnalytics.outputs.primarySharedKey
  }
}

module gameWorker 'modules/game-worker.bicep' = {
  name: 'game-worker'
  params: {
    name:              'ca-game-worker-${environment}'
    location:          location
    environment:       environment
    containerAppsEnvId: containerAppsEnv.outputs.id
    acrLoginServer:    acr.outputs.loginServer
    imageTag:          gameWorkerImageTag
    quakeMap:          quakeMap
    quakeSkill:        string(quakeSkill)
    appInsightsKey:    appInsights.outputs.instrumentationKey
  }
}

// ---------------------------------------------------------------------------
// Outputs
// ---------------------------------------------------------------------------

output acrLoginServer     string = acr.outputs.loginServer
output containerAppsEnvId string = containerAppsEnv.outputs.id
output gameWorkerFqdn     string = gameWorker.outputs.fqdn
output storageAccountName string = storage.outputs.name
output keyVaultUri        string = keyVault.outputs.uri
