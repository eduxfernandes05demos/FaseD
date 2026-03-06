// modules/quake-streaming.bicep -- Combined streaming gateway + game worker sidecar

param name                         string
param location                     string
param environment                  string
param containerAppsEnvId           string
param acrLoginServer               string
param gameWorkerImageTag           string
param streamingGatewayImageTag     string
param quakeMap                     string
param quakeSkill                   string
param appInsightsKey               string
param storageAccountName           string
param userAssignedIdentityId       string
param userAssignedIdentityClientId string

var isPlaceholder = gameWorkerImageTag == 'placeholder'
var gameWorkerImage = isPlaceholder ? 'mcr.microsoft.com/k8se/quickstart:latest' : '${acrLoginServer}/quake-worker:${gameWorkerImageTag}'
var gatewayImage = '${acrLoginServer}/streaming-gateway:${streamingGatewayImageTag}'

resource quakeStreaming 'Microsoft.App/containerApps@2024-03-01' = {
  name:     name
  location: location
  tags: {
    environment: environment
    application: 'quake-cloud'
  }
  identity: {
    type: 'UserAssigned'
    userAssignedIdentities: {
      '${userAssignedIdentityId}': {}
    }
  }
  properties: {
    environmentId: containerAppsEnvId
    configuration: {
      ingress: {
        external:   true
        targetPort: 8090
        transport:  'http'
      }
      registries: isPlaceholder ? [] : [
        {
          server:   acrLoginServer
          identity: userAssignedIdentityId
        }
      ]
    }
    template: {
      containers: [
        // Main container: streaming gateway (serves browser UI + WebSocket)
        {
          name:  'streaming-gateway'
          image: gatewayImage
          resources: {
            cpu:    json('0.5')
            memory: '1Gi'
          }
          env: [
            {
              name:  'FRAME_ADDR'
              value: 'localhost:9000'
            }
            {
              name:  'LISTEN_ADDR'
              value: ':8090'
            }
          ]
          probes: [
            {
              type: 'Liveness'
              httpGet: {
                path:   '/healthz'
                port:   8090
                scheme: 'HTTP'
              }
              initialDelaySeconds: 10
              periodSeconds:       10
              failureThreshold:    3
            }
            {
              type: 'Readiness'
              httpGet: {
                path:   '/healthz'
                port:   8090
                scheme: 'HTTP'
              }
              initialDelaySeconds: 5
              periodSeconds:       5
              failureThreshold:    3
            }
          ]
        }
        // Sidecar: game worker (headless Quake engine + frame server on localhost:9000)
        {
          name:  'game-worker'
          image: isPlaceholder ? 'mcr.microsoft.com/k8se/quickstart:latest' : gameWorkerImage
          resources: {
            cpu:    json('1.0')
            memory: '2Gi'
          }
          env: [
            {
              name:  'QUAKE_BASEDIR'
              value: '/game'
            }
            {
              name:  'QUAKE_MAP'
              value: quakeMap
            }
            {
              name:  'QUAKE_SKILL'
              value: quakeSkill
            }
            {
              name:  'QUAKE_MEM_MB'
              value: '64'
            }
            {
              name:  'QUAKE_WIDTH'
              value: '640'
            }
            {
              name:  'QUAKE_HEIGHT'
              value: '480'
            }
            {
              name:  'APPLICATIONINSIGHTS_CONNECTION_STRING'
              value: 'InstrumentationKey=${appInsightsKey}'
            }
            {
              name:  'STORAGE_ACCOUNT_NAME'
              value: storageAccountName
            }
            {
              name:  'AZURE_CLIENT_ID'
              value: userAssignedIdentityClientId
            }
          ]
        }
      ]
      scale: {
        minReplicas: 1
        maxReplicas: environment == 'prod' ? 10 : 2
        rules: [
          {
            name: 'http-scaling'
            http: {
              metadata: {
                concurrentRequests: '5'
              }
            }
          }
        ]
      }
    }
  }
}

output fqdn string = quakeStreaming.properties.configuration.ingress.fqdn
output name string = quakeStreaming.name
output id   string = quakeStreaming.id
