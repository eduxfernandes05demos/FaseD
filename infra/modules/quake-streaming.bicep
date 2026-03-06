// modules/quake-streaming.bicep -- Combined Quake streaming Container App (sidecar pattern)
//
// Deploys the streaming-gateway as the main container and game-worker as a sidecar.
// Both containers share localhost, enabling the gateway to reach the game worker's
// TCP frame server on localhost:9000 with zero network overhead.

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

// 'placeholder' tag triggers MCR quickstart image for initial deploy
var isPlaceholder = gameWorkerImageTag == 'placeholder' || streamingGatewayImageTag == 'placeholder'
var gameWorkerImage = isPlaceholder
  ? 'mcr.microsoft.com/k8se/quickstart:latest'
  : '/quake-worker:'
var streamingGatewayImage = isPlaceholder
  ? 'mcr.microsoft.com/k8se/quickstart:latest'
  : '/streaming-gateway:'

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
      '': {}
    }
  }
  properties: {
    environmentId: containerAppsEnvId
    configuration: {
      ingress: {
        external:   true
        targetPort: isPlaceholder ? 80 : 8090
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
        // Main container: streaming-gateway (receives external ingress)
        {
          name:  'streaming-gateway'
          image: streamingGatewayImage
          resources: {
            cpu:    json('0.5')
            memory: '1Gi'
          }
          env: [
            {
              name:  'LISTEN_ADDR'
              value: ':8090'
            }
            {
              name:  'WORKER_ADDR'
              value: 'localhost:9000'
            }
            {
              name:  'TARGET_FPS'
              value: '30'
            }
          ]
          probes: isPlaceholder ? [] : [
            {
              type: 'Liveness'
              httpGet: {
                path:   '/healthz'
                port:   8090
                scheme: 'HTTP'
              }
              initialDelaySeconds: 30
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
              initialDelaySeconds: 15
              periodSeconds:       5
              failureThreshold:    3
            }
          ]
        }
        // Sidecar container: game-worker (TCP frame server on localhost:9000)
        {
          name:  'game-worker'
          image: gameWorkerImage
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
              value: 'InstrumentationKey='
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
        minReplicas: 0
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
