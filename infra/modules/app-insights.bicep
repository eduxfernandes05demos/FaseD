// modules/app-insights.bicep -- Application Insights

param name             string
param location         string
param environment      string
param logAnalyticsId   string

resource appInsights 'Microsoft.Insights/components@2020-02-02' = {
  name:     name
  location: location
  kind:     'web'
  tags: {
    environment: environment
    application: 'quake-cloud'
  }
  properties: {
    Application_Type:             'web'
    WorkspaceResourceId:          logAnalyticsId
    IngestionMode:                'LogAnalytics'
    publicNetworkAccessForIngestion: 'Enabled'
    publicNetworkAccessForQuery:     'Enabled'
  }
}

output instrumentationKey string = appInsights.properties.InstrumentationKey
output connectionString   string = appInsights.properties.ConnectionString
