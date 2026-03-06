import sys

filepath = sys.argv[1]

with open(filepath, 'r') as f:
    content = f.read()

old_block = """module gameWorker 'modules/game-worker.bicep' = {
  name: 'game-worker'
  dependsOn: [blobReaderRole, acrPullRole]
  params: {
    name:                         'ca-game-worker-${environment}'
    location:                     location
    environment:                  environment
    containerAppsEnvId:           containerAppsEnv.outputs.id
    acrLoginServer:               acr.outputs.loginServer
    imageTag:                     gameWorkerImageTag
    quakeMap:                     quakeMap
    quakeSkill:                   string(quakeSkill)
    appInsightsKey:               appInsights.outputs.instrumentationKey
    storageAccountName:           storageAccountName
    userAssignedIdentityId:       gameWorkerIdentity.id
    userAssignedIdentityClientId: gameWorkerIdentity.properties.clientId
  }
}"""

new_block = """module quakeStreaming 'modules/quake-streaming.bicep' = {
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
}"""

if old_block in content:
    content = content.replace(old_block, new_block)
    print("Replaced module block")
else:
    print("ERROR: old block not found!")
    sys.exit(1)

# Fix output reference
old_output = "output gameWorkerFqdn     string = gameWorker.outputs.fqdn"
new_output = "output quakeStreamingFqdn    string = quakeStreaming.outputs.fqdn"
if old_output in content:
    content = content.replace(old_output, new_output)
    print("Replaced output reference")
else:
    print("WARNING: output line not found (may already be updated)")

with open(filepath, 'w') as f:
    f.write(content)

print("File updated successfully")
