# Quake Cloud Platform

**WinQuake modernized for the cloud** — the classic Quake engine running as a headless container on Azure Container Apps, with browser-based WebRTC streaming so anyone can play from a URL.

This project was built using the [spec2cloud](https://github.com/EmeaAppGbb/spec2cloud) AI-powered workflow, taking WinQuake from a 1996 Win32 desktop binary to a containerized, cloud-native game streaming platform on Azure.

## Architecture Overview

```
Browser (WebRTC)
    │
    ▼
┌──────────────────┐     ┌──────────────────┐
│ Streaming Gateway│────▶│   Game Worker     │
│   (Go, :8090)    │     │ (Headless Quake)  │
│ WebSocket signal │     │  RGBA framebuffer │
│ + HTML client    │     │  :8080 /healthz   │
└──────────────────┘     └──────────────────┘
    │                          │
    ▼                          ▼
┌──────────────────┐     ┌──────────────────┐
│ Session Manager  │     │  Telemetry API   │
│   (Go, :8080)    │     │   (Go, :8060)    │
│ POST/GET/DELETE  │     │ → App Insights   │
│ /api/sessions    │     │ /api/events      │
└──────────────────┘     └──────────────────┘
    │
    ▼
┌──────────────────┐
│   Assets API     │
│   (Go, :8070)    │
│ PAK file server  │
│ /api/assets/     │
└──────────────────┘
```

**Azure Resources** provisioned via Bicep:

| Resource | Purpose |
|----------|---------|
| Azure Container Registry | Stores container images |
| Azure Container Apps Environment | Hosts all containers |
| Game Worker Container App | Headless Quake engine (0→N replicas) |
| Azure Files | Game data (`id1/` directory) share |
| Key Vault | Secrets management (RBAC auth) |
| Application Insights + Log Analytics | Monitoring and telemetry |

---

## Prerequisites

- **Azure subscription** — [Create a free account](https://azure.microsoft.com/free/)
- **Azure CLI** (v2.60+) — [Install](https://learn.microsoft.com/cli/azure/install-azure-cli)
- **Docker** — [Install](https://docs.docker.com/get-docker/)
- **Quake game data** — You need a licensed `id1/` directory containing `pak0.pak` (shareware or full)

---

## Deploy to Azure

### Step 1: Clone the repo and log in to Azure

```bash
git clone https://github.com/kensondesu/quake-spec2cloud.git
cd quake-spec2cloud

az login
az account set --subscription "<YOUR_SUBSCRIPTION_ID>"
```

### Step 2: Create the resource group

```bash
az group create --name rg-quake-dev --location eastus
```

### Step 3: Deploy Azure infrastructure with Bicep

This provisions all resources (ACR, Container Apps Environment, Blob Storage, Key Vault, App Insights, a user-assigned managed identity with RBAC roles, and the game-worker Container App). Game data is stored in a blob container and downloaded into the container at startup by an init container—no storage account keys are used anywhere.

> **Note:** The Bicep template creates a user-assigned managed identity with **Storage Blob Data Reader** (on the storage account) and **AcrPull** (on ACR). These role assignments are completed before the container app is created, eliminating manual RBAC steps.

```bash
az deployment group create \
  --resource-group rg-quake-dev \
  --name main \
  --template-file infra/main.bicep \
  --parameters infra/parameters/dev.bicepparam
```

Capture the outputs — you'll need them in the next steps:

```bash
# Get deployment outputs
ACR_LOGIN_SERVER=$(az deployment group show \
  --resource-group rg-quake-dev \
  --name main \
  --query properties.outputs.acrLoginServer.value -o tsv)

STORAGE_ACCOUNT=$(az deployment group show \
  --resource-group rg-quake-dev \
  --name main \
  --query properties.outputs.storageAccountName.value -o tsv)

GAME_WORKER_FQDN=$(az deployment group show \
  --resource-group rg-quake-dev \
  --name main \
  --query properties.outputs.gameWorkerFqdn.value -o tsv)

echo "ACR:          $ACR_LOGIN_SERVER"
echo "Storage:      $STORAGE_ACCOUNT"
echo "Game Worker:  $GAME_WORKER_FQDN"
```

### Step 4: Upload game data to Blob Storage

Upload your Quake `id1/` directory (containing `pak0.pak`) to the `gamedata` blob container:

```bash
# Assign yourself Storage Blob Data Contributor on the storage account
CURRENT_USER=$(az ad signed-in-user show --query id -o tsv)
STORAGE_ID=$(az storage account show \
  --name "$STORAGE_ACCOUNT" \
  --resource-group rg-quake-dev \
  --query id -o tsv)

az role assignment create \
  --assignee "$CURRENT_USER" \
  --role "Storage Blob Data Contributor" \
  --scope "$STORAGE_ID"

# Wait a moment for the role assignment to propagate, then upload
az storage blob upload-batch \
  --account-name "$STORAGE_ACCOUNT" \
  --auth-mode login \
  --destination gamedata \
  --source /path/to/your/id1
```

### Step 5: Build and push the game worker container image

```bash
# Log in to ACR
az acr login --name "${ACR_LOGIN_SERVER%%.*}"

# Build and push the headless Quake worker
docker build -t "$ACR_LOGIN_SERVER/quake-worker:latest" .
docker push "$ACR_LOGIN_SERVER/quake-worker:latest"
```

> **Tip:** You can also use ACR Tasks to build in the cloud without a local Docker install:
> ```bash
> az acr build \
>   --registry "${ACR_LOGIN_SERVER%%.*}" \
>   --image quake-worker:latest .
> ```

### Step 6: Start the game worker

The Bicep deploy created the container app with `minReplicas: 0` (no pods running). Scale it up to start the game:

```bash
az containerapp update \
  --name ca-game-worker-dev \
  --resource-group rg-quake-dev \
  --min-replicas 1
```

The init container will download game data from blob storage using the managed identity, then the game worker starts.

---

## Build and Deploy the Microservices (optional)

The four Go microservices each have their own Dockerfile under `services/`:

```bash
# Build and push all services
for svc in streaming-gateway session-manager assets-api telemetry-api; do
  docker build -t "$ACR_LOGIN_SERVER/$svc:latest" "services/$svc"
  docker push "$ACR_LOGIN_SERVER/$svc:latest"
done
```

To deploy them as additional Container Apps, create matching Bicep modules or deploy via the CLI:

```bash
az containerapp create \
  --name ca-streaming-gateway-dev \
  --resource-group rg-quake-dev \
  --environment cae-quake-dev \
  --image "$ACR_LOGIN_SERVER/streaming-gateway:latest" \
  --target-port 8090 \
  --ingress external \
  --min-replicas 1 \
  --max-replicas 3 \
  --registry-server "$ACR_LOGIN_SERVER" \
  --registry-identity system
```

---

## See the Application Running

### Check the game worker health

```bash
# Verify the game-worker is healthy
az containerapp show \
  --name ca-game-worker-dev \
  --resource-group rg-quake-dev \
  --query "{Status:properties.runningStatus, FQDN:properties.configuration.ingress.fqdn}" \
  -o table
```

### View container logs

```bash
az containerapp logs show \
  --name ca-game-worker-dev \
  --resource-group rg-quake-dev \
  --follow
```

You should see log output like:

```
quake-worker: JSON log: engine initialized
quake-worker: JSON log: health server listening on :8080
quake-worker: JSON log: map=e1m1 skill=1
```

### Access the streaming gateway in a browser

If you deployed the streaming gateway with `--ingress external`:

```bash
# Get the gateway URL
GATEWAY_FQDN=$(az containerapp show \
  --name ca-streaming-gateway-dev \
  --resource-group rg-quake-dev \
  --query properties.configuration.ingress.fqdn -o tsv)

echo "Open in your browser: https://$GATEWAY_FQDN"
```

Navigate to that URL — the embedded HTML client loads a `<canvas>` and establishes a WebRTC connection to the game worker. You'll see the Quake game rendered in your browser with keyboard and mouse input forwarded to the engine.

### Monitor with Application Insights

```bash
# Open Application Insights in the Azure portal
az monitor app-insights component show \
  --app appi-quake-dev \
  --resource-group rg-quake-dev \
  --query "{Name:name, InstrumentationKey:instrumentationKey}" \
  -o table
```

Or open the [Azure Portal](https://portal.azure.com) → **Application Insights** → `appi-quake-dev` to see live metrics, traces, and telemetry events.

---

## Run Locally with Docker

For local development and testing without Azure:

```bash
# Build the headless game worker
docker build -t quake-worker:local .

# Run with your local game data
docker run -it --rm \
  -p 8080:8080 \
  -v /path/to/your/id1:/game/id1:ro \
  -e QUAKE_MAP=e1m1 \
  -e QUAKE_SKILL=1 \
  quake-worker:local
```

Check health: `curl http://localhost:8080/healthz`

---

## Configuration

The game worker is configured via environment variables:

| Variable | Default | Description |
|----------|---------|-------------|
| `QUAKE_BASEDIR` | `/game` | Base directory for game data |
| `QUAKE_MAP` | `e1m1` | Starting map |
| `QUAKE_SKILL` | `1` | Difficulty (0=Easy, 1=Normal, 2=Hard, 3=Nightmare) |
| `QUAKE_MEM_MB` | `32` | Engine memory allocation |
| `QUAKE_WIDTH` | `640` | Render width in pixels |
| `QUAKE_HEIGHT` | `480` | Render height in pixels |

---

## Project Structure

```
├── WinQuake/                  # Modernized Quake engine source (C)
│   ├── CMakeLists.txt         # CMake build (-DHEADLESS=ON -DNOASM=ON)
│   ├── vid_headless.c         # Headless video driver (framebuffer capture)
│   ├── snd_capture.c          # Audio capture driver
│   ├── in_inject.c            # Input injection (keyboard/mouse from network)
│   └── sys_container.c        # Container system layer (JSON logging, /healthz)
├── Dockerfile                 # Multi-stage build for quake-worker container
├── infra/                     # Azure Bicep infrastructure-as-code
│   ├── main.bicep             # Root template
│   ├── modules/               # ACR, Container Apps, Storage, Key Vault, etc.
│   └── parameters/dev.bicepparam
├── services/                  # Go microservices
│   ├── streaming-gateway/     # WebSocket signaling + WebRTC + browser client
│   ├── session-manager/       # Game session lifecycle REST API
│   ├── assets-api/            # PAK file content server
│   └── telemetry-api/         # Event ingestion → Application Insights
└── .github/workflows/
    └── build.yml              # CI: build headless worker + static analysis
```

---

## CI/CD

The GitHub Actions workflow in `.github/workflows/build.yml` runs on every push to `main`:
1. Builds the headless Quake worker binary
2. Runs `cppcheck` static analysis
3. Uploads the binary as a build artifact

---

## Clean Up

Remove all Azure resources when you're done:

```bash
az group delete --name rg-quake-dev --yes --no-wait
```

---

## License

See [LICENSE.md](LICENSE.md).

## Contributing

Contributions welcome! See [docs/contributing.md](docs/contributing.md).

---

**Built with [spec2cloud](https://github.com/EmeaAppGbb/spec2cloud) — from legacy code to cloud in minutes.**
