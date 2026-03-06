#!/bin/sh
set -e

# Skip download if game data already exists (container restart scenario)
if [ -f /game/id1/PAK0.PAK ]; then
  echo "Game data already present, skipping download."
  exec quake-worker "$@"
fi

echo "Downloading game data from Azure Blob Storage..."
mkdir -p /game/id1

# Wait for managed identity endpoint to become ready
MAX_RETRIES=30
TOKEN=""
for i in $(seq 1 $MAX_RETRIES); do
  TOKEN=$(curl -sf "${IDENTITY_ENDPOINT}?api-version=2019-08-01&resource=https%3A%2F%2Fstorage.azure.com%2F&client_id=${AZURE_CLIENT_ID}" \
    -H "X-IDENTITY-HEADER: ${IDENTITY_HEADER}" 2>/dev/null | \
    sed -n 's/.*"access_token":"\([^"]*\)".*/\1/p') && [ -n "$TOKEN" ] && break
  echo "Waiting for managed identity endpoint ($i/$MAX_RETRIES)..."
  sleep 2
done

if [ -z "$TOKEN" ]; then
  echo "ERROR: Failed to acquire managed identity token after $MAX_RETRIES retries"
  exit 1
fi

echo "Authenticated with managed identity."

# List blobs in the gamedata container via Azure Blob REST API
BLOB_LIST=$(curl -sf "https://${STORAGE_ACCOUNT_NAME}.blob.core.windows.net/gamedata?restype=container&comp=list" \
  -H "Authorization: Bearer ${TOKEN}" \
  -H "x-ms-version: 2020-10-02")

# Extract blob names from the XML response
BLOBS=$(echo "$BLOB_LIST" | grep -o '<Name>[^<]*</Name>' | sed 's/<[^>]*>//g')

if [ -z "$BLOBS" ]; then
  echo "WARNING: No blobs found in gamedata container"
else
  for BLOB in $BLOBS; do
    echo "Downloading ${BLOB}..."
    curl -sf "https://${STORAGE_ACCOUNT_NAME}.blob.core.windows.net/gamedata/${BLOB}" \
      -H "Authorization: Bearer ${TOKEN}" \
      -H "x-ms-version: 2020-10-02" \
      -o "/game/id1/${BLOB}"
  done
fi

echo "Game data download complete."
exec quake-worker "$@"
