// infra/parameters/dev.bicepparam
// Development environment parameters for the Quake Cloud Platform.
// Deploy with:
//   az group create -n rg-quake-dev -l eastus
//   az deployment group create \
//     --resource-group rg-quake-dev \
//     --template-file infra/main.bicep \
//     --parameters infra/parameters/dev.bicepparam

using '../main.bicep'

param environment        = 'dev'
param location           = 'eastus'
param prefix             = 'quake'
param gameWorkerImageTag = 'latest'
param quakeMap           = 'e1m1'
param quakeSkill         = 1
