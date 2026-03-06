# WebRTC Streaming Implementation Plan

## Status: DRAFT
## Owner: Engineering
## Priority: P0 (AI Summit demo)

---

## 1. Current State

### What Works
| Component | File | Status |
|---|---|---|
| Framebuffer capture | `WinQuake/vid_headless.c` | ✅ `VID_CaptureFrame()` returns RGBA, flag-based new-frame detection |
| Audio capture | `WinQuake/snd_capture.c` | ✅ `SND_CaptureAudio()` returns 11 kHz mono 16-bit PCM ring buffer |
| Input injection | `WinQuake/in_inject.c` | ✅ `IN_InjectKeyEvent()` / `IN_InjectMouseEvent()` via circular queues |
| Health endpoint | `WinQuake/sys_container.c` | ✅ HTTP on `:8080` (raw C sockets, pthread) |
| WebSocket signaling | `services/streaming-gateway/main.go` | ✅ Skeleton: config, offer/answer, ICE, input message types |
| Browser client | embedded in `main.go` | ✅ Splash screen with WSS connection |

### What's Missing
| Component | Gap |
|---|---|
| **Frame server** | Game worker has NO network endpoint for frame data. `WORKER_ADDR=localhost:9000` expected but nothing listens there. |
| **Video encoding** | No H.264/VP8 encoder anywhere. Raw RGBA frames are in-process only. |
| **Audio encoding** | No Opus encoder. PCM ring buffer is in-process only. |
| **WebRTC peer connection** | `pion/webrtc` not imported. SDP answer is a placeholder string. |
| **Input relay** | No forwarding path from browser → gateway → game worker. |
| **Container networking** | Two separate Container Apps can only reach each other via HTTP ingress, not raw TCP. |

---

## 2. Architecture Decision: Container Topology

### Problem
The game worker and streaming gateway are deployed as **separate Container Apps**. The gateway needs sub-millisecond access to frame data at 30 fps. Container Apps internal ingress adds HTTP overhead and doesn't support raw TCP between apps.

### Options

| Option | Pros | Cons |
|---|---|---|
| **A. Sidecar pattern** (recommended) | Shared localhost, zero-latency IPC, single deployment unit | Requires Bicep restructure, combined health probes |
| B. HTTP frame endpoint on `:8080` | No infra changes, uses existing HTTP server | High overhead (HTTP per frame), complex C code changes |
| C. Single combined Docker image | Simple networking | Messy multi-language Dockerfile, process management |

### Recommendation: Option A — Sidecar Pattern

Deploy both containers in the **same Container App**:

```
┌─────────────────────────────────────────────────────┐
│  Container App: ca-quake-streaming-dev               │
│  Ingress: external, port 8090 (streaming-gateway)    │
│                                                       │
│  ┌─────────────────────┐  ┌────────────────────────┐ │
│  │  streaming-gateway   │  │  game-worker (sidecar) │ │
│  │  :8090 (HTTP/WSS)    │  │  :8080 (healthz)       │ │
│  │  Main container       │←→│  :9000 (frame server)  │ │
│  │  pion/webrtc          │  │  TCP binary protocol   │ │
│  └─────────────────────┘  └────────────────────────┘ │
│         ↑ localhost:9000 (IPC)                        │
└─────────────────────────────────────────────────────┘
         ↕ HTTPS + WSS (external ingress)
    ┌──────────┐
    │  Browser  │
    └──────────┘
```

**Why this works:**
- Container Apps sidecars share `localhost` (same network namespace)
- Frame server on `localhost:9000` → zero network overhead
- External ingress targets the streaming gateway on `:8090`
- Health probes target the main container (gateway's `/healthz`)
- Single `az containerapp update` to deploy both

---

## 3. Architecture Decision: Streaming Protocol

### Problem
Azure Container Apps has **no UDP ingress**. WebRTC media transport (SRTP) uses UDP by default. Direct browser↔gateway WebRTC requires either a TURN relay or TCP ICE.

### Options

| Option | Latency | Complexity | Container Apps Compatible |
|---|---|---|---|
| **A. WebSocket MJPEG** (recommended for demo) | ~50-100ms | Low | ✅ Yes |
| B. WebRTC with TURN relay | ~30-50ms | High | ✅ (needs TURN deploy) |
| C. WebRTC with TCP ICE | ~40-60ms | High | ⚠️ Experimental |

### Recommendation: Option A for demo, upgrade to B later

**Phase 1 (demo):** MJPEG frames over the existing WebSocket connection.
- 640×480 JPEG at quality 70 ≈ 30-50 KB/frame
- At 30 fps ≈ 1-1.5 MB/s bandwidth (fine for Azure)
- Browser renders via `createImageBitmap()` → canvas
- Input events sent as JSON on same WebSocket
- Visually identical to WebRTC for the audience

**Phase 2 (post-demo):** Replace with pion/webrtc + TURN.
- VP8 encoding via libvpx (CGo)
- Opus audio encoding
- DataChannel for low-latency input
- Azure Communication Services or coturn for TURN relay

---

## 4. Implementation Phases

### Phase 1: Game Worker Frame Server (C)

**New file:** `WinQuake/net_frame_server.c`

Add a TCP server to the game worker that exposes frames, audio, and input injection over a simple binary protocol on port 9000.

#### Binary Protocol

All integers are **little-endian**. Each message starts with a 1-byte command.

**Request → Response:**

```
Client sends:  [cmd:1B]
Server replies: [payload]

Commands:
  'F' (0x46) — Get Frame
    → Response: [width:4B][height:4B][jpeg_len:4B][jpeg_data:*]
    
  'A' (0x41) — Get Audio  
    → Response: [samples:4B][rate:4B][pcm_data:*]
    
  'K' (0x4B) — Inject Key Event
    → Client sends: [key:4B][down:1B]
    → Response: [ok:1B] (0x01)
    
  'M' (0x4D) — Inject Mouse Event
    → Client sends: [dx:4B][dy:4B][buttons:4B]
    → Response: [ok:1B] (0x01)
```

**JPEG compression:** Use `stb_image_write.h` (single-header, no external dependency) to encode RGBA → JPEG in the C code. This keeps the gateway simple and reduces IPC bandwidth from ~1.2 MB/frame to ~40 KB/frame.

#### Implementation Details

```c
// net_frame_server.c — TCP frame server for streaming gateway IPC

#define FRAME_SERVER_PORT 9000  // Overridable via FRAME_SERVER_PORT env var

// Background thread (like healthz_thread)
static void *frame_server_thread(void *arg);

// Protocol handlers
static int handle_frame_request(int client_fd);   // 'F' → JPEG frame
static int handle_audio_request(int client_fd);   // 'A' → PCM audio
static int handle_key_inject(int client_fd);       // 'K' → inject key
static int handle_mouse_inject(int client_fd);     // 'M' → inject mouse
```

#### Changes Required

| File | Change |
|---|---|
| `WinQuake/net_frame_server.c` | **NEW** — TCP server + JPEG encoding |
| `WinQuake/stb_image_write.h` | **NEW** — Single-header JPEG encoder (public domain) |
| `WinQuake/CMakeLists.txt` | Add `net_frame_server.c` to HEADLESS source list |
| `WinQuake/sys_container.c` | Call `FrameServer_Init()` alongside healthz thread spawn |
| `WinQuake/vid_headless.c` | No changes needed (API already complete) |

#### Thread Safety

- `VID_CaptureFrame()` reads `vid_buffer` and `d_8to24table` — these are written by the engine's main loop. Add a **mutex** around the capture path.
- `IN_InjectKeyEvent()` / `IN_InjectMouseEvent()` are already queue-based with separate head/tail — safe for single-producer (network thread) / single-consumer (engine thread).
- `SND_CaptureAudio()` just returns a pointer to the ring buffer — the consumer must track its own read position.

---

### Phase 2: Streaming Gateway — Frame Pipeline (Go)

**Modify:** `services/streaming-gateway/main.go`

Replace the stub `startFrameRelay()` with a TCP client that connects to the game worker and relays frames to browser clients.

#### New Components

```go
// workerClient connects to the game worker's TCP frame server
type workerClient struct {
    conn net.Conn
    mu   sync.Mutex
}

func (w *workerClient) GetFrame() ([]byte, error)           // Send 'F', receive JPEG
func (w *workerClient) GetAudio() ([]byte, int, error)      // Send 'A', receive PCM  
func (w *workerClient) InjectKey(key int, down bool) error   // Send 'K' + payload
func (w *workerClient) InjectMouse(dx, dy, btns int) error   // Send 'M' + payload
```

#### Frame Relay Flow

```
Game Worker                  Streaming Gateway              Browser
     │                              │                          │
     │←── TCP 'F' request ─────────│                          │
     │─── JPEG bytes ──────────────→│                          │
     │                              │─── WS binary (JPEG) ────→│
     │                              │                          │── drawImage(canvas)
     │                              │                          │
     │                              │←── WS text (input) ──────│
     │←── TCP 'K'/'M' + payload ───│                          │
     │─── inject into queue ───→   │                          │
```

#### Changes Required

| File | Change |
|---|---|
| `services/streaming-gateway/main.go` | Replace stub relay with TCP client, WebSocket binary frame push, input forwarding |
| `services/streaming-gateway/go.mod` | No new deps needed (stdlib `net`, `encoding/binary`) |

#### Session Management

Each browser WebSocket connection gets:
1. A dedicated `workerClient` TCP connection to the game worker
2. A frame relay goroutine (30 fps ticker → request frame → send to browser)
3. Input forwarding (WS text message → parse → TCP inject to worker)

For the demo, support **1 concurrent session** (single player). The frame server accepts only one client at a time.

---

### Phase 3: Browser Client

**Modify:** Embedded HTML in `services/streaming-gateway/main.go`

Replace the splash screen with an interactive game client.

#### Features

1. **Video display**: Receive JPEG binary WebSocket messages → `createImageBitmap()` → draw on `<canvas>`
2. **Keyboard input**: `keydown`/`keyup` → map to Quake K_* constants → send as JSON over WebSocket
3. **Mouse input**: `mousemove` (pointer lock) → relative dx/dy → send as JSON over WebSocket
4. **Mouse buttons**: `mousedown`/`mouseup` → button bitmask → send as JSON
5. **Pointer lock**: Click canvas to capture mouse (essential for FPS controls)
6. **Status overlay**: FPS counter, connection state, latency indicator

#### Key Mapping (Browser → Quake)

```javascript
const KEY_MAP = {
    'KeyW': 119,    // 'w' → K_w (forward)
    'KeyA': 97,     // 'a' → K_a (strafe left)
    'KeyS': 115,    // 's' → K_s (back)
    'KeyD': 100,    // 'd' → K_d (strafe right)
    'Space': 32,    // K_SPACE (jump)
    'ShiftLeft': 304, // K_SHIFT (run)
    'Escape': 27,   // K_ESCAPE (menu)
    'Enter': 13,    // K_ENTER
    'ArrowUp': 328, // K_UPARROW
    'ArrowDown': 336, // K_DOWNARROW
    'ArrowLeft': 331, // K_LEFTARROW
    'ArrowRight': 333, // K_RIGHTARROW
    // ... etc
};
```

#### Input Message Format (Browser → Gateway)

```json
{"type":"input","kind":"key","key":119,"down":true}
{"type":"input","kind":"mouse","dx":5,"dy":-3,"buttons":1}
```

---

### Phase 4: Sidecar Container Deployment

**Modify:** Bicep infrastructure to deploy both containers in one Container App.

#### New Bicep Module

**New file:** `infra/modules/quake-streaming.bicep`

Replaces both `game-worker.bicep` and the CLI-created streaming gateway Container App.

```
Container App: ca-quake-streaming-{env}
├── Main container: streaming-gateway (:8090, external ingress)
├── Sidecar container: game-worker (:8080 health, :9000 frame server)
├── Health probe: streaming-gateway /healthz on :8090
└── Managed identity: id-game-worker-{env} (blob read + ACR pull)
```

#### Changes Required

| File | Change |
|---|---|
| `infra/modules/quake-streaming.bicep` | **NEW** — Combined container app with sidecar |
| `infra/main.bicep` | Replace `gameWorker` module call with `quakeStreaming` |
| `Dockerfile` (game-worker) | Add `EXPOSE 9000` |
| `services/streaming-gateway/Dockerfile` | Add codec libs if needed for Phase 5 |

---

### Phase 5 (Future): WebRTC Upgrade

> Not needed for the demo. Document for future reference.

Replace WebSocket MJPEG stream with proper WebRTC:

1. **Dependencies**: `pion/webrtc/v4`, `pion/interceptor`, libvpx (CGo), libopus (CGo)
2. **Video**: VP8 encoding from RGBA → pion video track → SRTP
3. **Audio**: Opus encoding from PCM → pion audio track → SRTP
4. **Input**: WebRTC DataChannel (lower latency than WebSocket)
5. **NAT traversal**: Deploy coturn or Azure Communication Services TURN relay
6. **Streaming gateway Dockerfile**: Switch to Ubuntu base for CGo + libvpx/libopus

---

## 5. File Change Summary

### New Files (4)
| File | Purpose |
|---|---|
| `WinQuake/net_frame_server.c` | TCP frame server (C, ~250 lines) |
| `WinQuake/stb_image_write.h` | JPEG encoder (public domain header, ~1500 lines) |
| `infra/modules/quake-streaming.bicep` | Combined sidecar Container App |
| `specs/features/webrtc-streaming.md` | This plan |

### Modified Files (5)
| File | Change |
|---|---|
| `WinQuake/CMakeLists.txt` | Add `net_frame_server.c` to HEADLESS sources, link math lib |
| `WinQuake/sys_container.c` | Spawn frame server thread at startup |
| `services/streaming-gateway/main.go` | Full rewrite of frame relay + client HTML |
| `Dockerfile` | Expose port 9000 |
| `infra/main.bicep` | Replace game-worker module with quake-streaming |

### Deleted Files (1)
| File | Reason |
|---|---|
| `infra/modules/game-worker.bicep` | Superseded by `quake-streaming.bicep` |

---

## 6. Estimated Scope

| Phase | Description | Files |
|---|---|---|
| **Phase 1** | Frame server in C | 3 new, 2 modified |
| **Phase 2** | Gateway frame pipeline in Go | 1 modified |
| **Phase 3** | Browser client | 1 modified (same file as Phase 2) |
| **Phase 4** | Sidecar deployment | 1 new, 2 modified, 1 deleted |
| **Phase 5** | WebRTC upgrade (future) | TBD |

---

## 7. Build & Deploy Sequence

```bash
# 1. Build game worker with frame server
cd WinQuake && cmake -B build -DHEADLESS=ON -DNOASM=ON && cmake --build build

# 2. Build & push game worker image  
docker build -t quakeacrdev.azurecr.io/quake-worker:v4 .
az acr login -n quakeacrdev
docker push quakeacrdev.azurecr.io/quake-worker:v4

# 3. Build & push streaming gateway image
cd services/streaming-gateway
docker build -t quakeacrdev.azurecr.io/streaming-gateway:v4 .
docker push quakeacrdev.azurecr.io/streaming-gateway:v4

# 4. Deploy combined container app
az deployment group create -g rg-quake-dev -f infra/main.bicep \
  --parameters gameWorkerImageTag=v4 environment=dev

# 5. Verify
curl https://ca-quake-streaming-dev.<env-fqdn>/healthz
# Open browser → https://ca-quake-streaming-dev.<env-fqdn>/
```

---

## 8. Risks & Mitigations

| Risk | Impact | Mitigation |
|---|---|---|
| JPEG encoding in C adds CPU | Frame serve latency | stb_image_write is O(n) and fast; 640×480 ≈ 2ms per frame |
| Thread safety on framebuffer access | Torn frames | Mutex on `VID_CaptureFrame()` path |
| Single-session limitation | Only one player at a time | Acceptable for demo; expand later |
| Container App sidecar startup order | Gateway can't connect to worker | Retry loop in TCP client with backoff |
| WebSocket bandwidth (1.5 MB/s) | Azure egress cost | Acceptable for demo; WebRTC with VP8 would reduce to ~500 KB/s |
| Browser keyboard capture conflicts | Keys go to browser, not game | Pointer lock API + `preventDefault()` on game keys |

---

## 9. Acceptance Criteria

- [ ] Browser loads game client page at gateway URL
- [ ] Live Quake gameplay visible on canvas (640×480, ~30 fps)
- [ ] WASD movement works via keyboard
- [ ] Mouse look works via pointer lock
- [ ] Mouse click fires weapon
- [ ] Status overlay shows FPS and connection state
- [ ] Single `az containerapp update` deploys the full stack
- [ ] Health probes pass for both containers
