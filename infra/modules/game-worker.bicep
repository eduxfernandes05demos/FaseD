// modules/game-worker.bicep -- Quake game-worker Container App

param name               string
param location           string
param environment        string
param containerAppsEnvId string
param acrLoginServer     string
param imageTag           string
param quakeMap           string
param quakeSkill         string
param appInsightsKey     string

var imageName = '${acrLoginServer}/quake-worker:${imageTag}'

resource gameWorker 'Microsoft.App/containerApps@2024-03-01' = {
  name:     name
  location: location
  tags: {
    environment: environment
    application: 'quake-cloud'
  }
  identity: {
    type: 'SystemAssigned'
  }
  properties: {
    environmentId: containerAppsEnvId
    configuration: {
      ingress: {
        // Internal only – exposed to the streaming gateway via ACA internal networking
        external:   false
        targetPort: 8080
        transport:  'http'
      }
      registries: [
        {
          server:   acrLoginServer
          identity: 'system'
        }
      ]
    }
    template: {
      containers: [
        {
          name:  'game-worker'
          image: imageName
          resources: {
            cpu:    '1.0'
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
          ]
          probes: [
            {
              type: 'Liveness'
              httpGet: {
                path:   '/healthz'
                port:   8080
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
                port:   8080
                scheme: 'HTTP'
              }
              initialDelaySeconds: 15
              periodSeconds:       5
              failureThreshold:    3
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

output fqdn string = gameWorker.properties.configuration.ingress.fqdn
output id   string = gameWorker.id
