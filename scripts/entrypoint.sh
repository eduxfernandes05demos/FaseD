#!/bin/sh
set -e

# Skip download if game data already exists (container restart scenario)
if [ -f /game/id1/pak0.pak ] || [ -f /game/id1/PAK0.PAK ]; then
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
echo "Token length: ${#TOKEN} characters"
echo "Storage account: ${STORAGE_ACCOUNT_NAME}"
echo "curl version: $(curl --version | head -1)"

# --- Connectivity diagnostics ---
STORAGE_HOST="${STORAGE_ACCOUNT_NAME}.blob.core.windows.net"
echo "=== Connectivity check: ${STORAGE_HOST} ==="
echo "DNS resolution:"
getent hosts "${STORAGE_HOST}" 2>&1 || echo "  getent failed, trying curl..."
echo "TLS handshake test:"
curl -sSo /dev/null -w "  HTTP %{http_code}, connect=%{time_connect}s, total=%{time_total}s\n" \
  --connect-timeout 10 "https://${STORAGE_HOST}/" 2>&1 || echo "  TLS/connect test failed (exit $?)"
echo "=== End connectivity check ==="

# List blobs in the gamedata container via Azure Blob REST API
BLOB_LIST_URL="https://${STORAGE_HOST}/gamedata?restype=container&comp=list"
echo "Listing blobs: ${BLOB_LIST_URL}"

# Capture HTTP status code and body separately (no --fail-with-body for reliability)
HTTP_CODE=$(curl -sS -o /tmp/bloblist.xml -w '%{http_code}' \
  --connect-timeout 10 --max-time 30 \
  "${BLOB_LIST_URL}" \
  -H "Authorization: Bearer ${TOKEN}" \
  -H "x-ms-version: 2020-10-02" 2>/tmp/curlerr.log)
CURL_EXIT=$?

echo "Curl exit code: ${CURL_EXIT}"
echo "HTTP status: ${HTTP_CODE}"

if [ "${CURL_EXIT}" -ne 0 ]; then
  echo "ERROR: curl failed at network level"
  echo "ERROR: stderr: $(cat /tmp/curlerr.log)"
  exit 1
fi

if [ "${HTTP_CODE}" -ge 400 ] || [ "${HTTP_CODE}" = "000" ]; then
  echo "ERROR: Blob list request failed with HTTP ${HTTP_CODE}"
  echo "ERROR: Response body:"
  cat /tmp/bloblist.xml
  exit 1
fi

BLOB_LIST=$(cat /tmp/bloblist.xml)

# Extract blob names from the XML response
BLOBS=$(echo "$BLOB_LIST" | grep -o '<Name>[^<]*</Name>' | sed 's/<[^>]*>//g')

if [ -z "$BLOBS" ]; then
  echo "ERROR: No blobs found in gamedata container. Cannot start without game data."
  echo "Response was: $(head -c 500 /tmp/bloblist.xml)"
  exit 1
fi

for BLOB in $BLOBS; do
  # Quake on Linux expects lowercase filenames (pak0.pak, not PAK0.PAK)
  LOWER_BLOB=$(echo "$BLOB" | tr '[:upper:]' '[:lower:]')
  echo "Downloading ${BLOB} -> ${LOWER_BLOB}..."
  DL_CODE=$(curl -sS -o "/game/id1/${LOWER_BLOB}" -w '%{http_code}' \
    --connect-timeout 10 --max-time 120 \
    "https://${STORAGE_HOST}/gamedata/${BLOB}" \
    -H "Authorization: Bearer ${TOKEN}" \
    -H "x-ms-version: 2020-10-02" 2>/tmp/curlerr.log)
  DL_EXIT=$?
  if [ "${DL_EXIT}" -ne 0 ] || [ "${DL_CODE}" -ge 400 ]; then
    echo "ERROR: Failed to download blob ${BLOB} (curl exit=${DL_EXIT}, HTTP ${DL_CODE})"
    echo "ERROR: $(cat /tmp/curlerr.log)"
    exit 1
  fi
  FILE_SIZE=$(stat -c%s "/game/id1/${LOWER_BLOB}" 2>/dev/null || echo "0")
  echo "Downloaded ${LOWER_BLOB}: ${FILE_SIZE} bytes"
done

echo "Game data download complete. Files in /game/id1:"
ls -la /game/id1/
exec quake-worker "$@"
