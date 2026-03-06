/*
Package main implements the Quake Cloud Streaming Gateway.

Architecture:
  - Connects to the game-worker sidecar via TCP loopback to receive
    JPEG video frames and PCM audio.
  - Streams JPEG frames to the browser over WebSocket (binary messages).
  - Forwards browser keyboard/mouse events back to the game-worker via
    the same TCP connection using a simple binary protocol.

Environment variables:

	WORKER_ADDR     - game-worker IPC address (default: localhost:9000)
	LISTEN_ADDR     - HTTP listen address (default: :8090)
	TARGET_FPS      - frame rate target (default: 30)
*/
package main

import (
	"encoding/binary"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net"
	"net/http"
	"os"
	"strconv"
	"sync"
	"time"

	"github.com/gorilla/websocket"
)

// ---------------------------------------------------------------------------
// Configuration
// ---------------------------------------------------------------------------

func envOr(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}

var (
	workerAddr = envOr("WORKER_ADDR", "localhost:9000")
	listenAddr = envOr("LISTEN_ADDR", ":8090")
	targetFPS  = envInt("TARGET_FPS", 30)
)

func envInt(key string, def int) int {
	if v := os.Getenv(key); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 {
			return n
		}
	}
	return def
}

// ---------------------------------------------------------------------------
// WebSocket upgrader
// ---------------------------------------------------------------------------

var upgrader = websocket.Upgrader{
	ReadBufferSize:  4096,
	WriteBufferSize: 256 * 1024, // enough for a JPEG frame
	CheckOrigin: func(r *http.Request) bool {
		return true // same-origin in sidecar, safe for demo
	},
}

// ---------------------------------------------------------------------------
// workerClient — speaks the binary IPC protocol to net_frame_server.c
// ---------------------------------------------------------------------------

type workerClient struct {
	mu   sync.Mutex
	conn net.Conn
}

// dial connects to the game worker with retries.
func dialWorker() (*workerClient, error) {
	var conn net.Conn
	var err error
	for i := 0; i < 20; i++ {
		conn, err = net.DialTimeout("tcp", workerAddr, 2*time.Second)
		if err == nil {
			// Set TCP_NODELAY for low latency
			if tc, ok := conn.(*net.TCPConn); ok {
				tc.SetNoDelay(true)
			}
			return &workerClient{conn: conn}, nil
		}
		log.Printf("worker connect attempt %d failed: %v", i+1, err)
		time.Sleep(500 * time.Millisecond)
	}
	return nil, fmt.Errorf("failed to connect to worker after retries: %w", err)
}

func (w *workerClient) Close() {
	w.mu.Lock()
	defer w.mu.Unlock()
	if w.conn != nil {
		w.conn.Close()
	}
}

// GetFrame sends 'F' and reads [width:4][height:4][jpeg_len:4][jpeg_data].
func (w *workerClient) GetFrame() ([]byte, int, int, error) {
	w.mu.Lock()
	defer w.mu.Unlock()

	if _, err := w.conn.Write([]byte{'F'}); err != nil {
		return nil, 0, 0, err
	}

	var header [12]byte
	if _, err := io.ReadFull(w.conn, header[:]); err != nil {
		return nil, 0, 0, err
	}

	width := int(binary.LittleEndian.Uint32(header[0:4]))
	height := int(binary.LittleEndian.Uint32(header[4:8]))
	jpegLen := int(binary.LittleEndian.Uint32(header[8:12]))

	if jpegLen == 0 {
		return nil, width, height, nil
	}

	// Sanity check: JPEG shouldn't exceed 2 MB for 640x480
	if jpegLen > 2*1024*1024 {
		return nil, 0, 0, fmt.Errorf("jpeg size %d exceeds limit", jpegLen)
	}

	jpeg := make([]byte, jpegLen)
	if _, err := io.ReadFull(w.conn, jpeg); err != nil {
		return nil, 0, 0, err
	}

	return jpeg, width, height, nil
}

// InjectKey sends 'K' + [key:4][down:1] and reads [ok:1].
func (w *workerClient) InjectKey(key int32, down bool) error {
	w.mu.Lock()
	defer w.mu.Unlock()

	var buf [6]byte
	buf[0] = 'K'
	binary.LittleEndian.PutUint32(buf[1:5], uint32(key))
	if down {
		buf[5] = 1
	}
	if _, err := w.conn.Write(buf[:]); err != nil {
		return err
	}

	var ok [1]byte
	_, err := io.ReadFull(w.conn, ok[:])
	return err
}

// InjectMouse sends 'M' + [dx:4][dy:4][buttons:4] and reads [ok:1].
func (w *workerClient) InjectMouse(dx, dy, buttons int32) error {
	w.mu.Lock()
	defer w.mu.Unlock()

	var buf [13]byte
	buf[0] = 'M'
	binary.LittleEndian.PutUint32(buf[1:5], uint32(dx))
	binary.LittleEndian.PutUint32(buf[5:9], uint32(dy))
	binary.LittleEndian.PutUint32(buf[9:13], uint32(buttons))
	if _, err := w.conn.Write(buf[:]); err != nil {
		return err
	}

	var ok [1]byte
	_, err := io.ReadFull(w.conn, ok[:])
	return err
}

// ---------------------------------------------------------------------------
// Session represents one connected browser peer
// ---------------------------------------------------------------------------

type Session struct {
	wsMu   sync.Mutex
	conn   *websocket.Conn
	worker *workerClient
	done   chan struct{}
}

func newSession(conn *websocket.Conn, worker *workerClient) *Session {
	return &Session{
		conn:   conn,
		worker: worker,
		done:   make(chan struct{}),
	}
}

// sendBinary sends a binary WebSocket message (JPEG frame).
func (s *Session) sendBinary(data []byte) error {
	s.wsMu.Lock()
	defer s.wsMu.Unlock()
	return s.conn.WriteMessage(websocket.BinaryMessage, data)
}

// sendJSON sends a JSON text WebSocket message.
func (s *Session) sendJSON(msg interface{}) error {
	data, err := json.Marshal(msg)
	if err != nil {
		return err
	}
	s.wsMu.Lock()
	defer s.wsMu.Unlock()
	return s.conn.WriteMessage(websocket.TextMessage, data)
}

// frameRelay reads frames from the worker and pushes them to the browser.
func (s *Session) frameRelay() {
	interval := time.Duration(1000/targetFPS) * time.Millisecond
	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	for {
		select {
		case <-s.done:
			return
		case <-ticker.C:
			jpeg, _, _, err := s.worker.GetFrame()
			if err != nil {
				log.Printf("frame relay: worker read error: %v", err)
				return
			}
			if jpeg == nil {
				continue // no frame available yet
			}
			if err := s.sendBinary(jpeg); err != nil {
				log.Printf("frame relay: ws write error: %v", err)
				return
			}
		}
	}
}

// ---------------------------------------------------------------------------
// Signaling / streaming handler
// ---------------------------------------------------------------------------

type inputMsg struct {
	Type    string `json:"type"`
	Kind    string `json:"kind"`
	Key     int32  `json:"key"`
	Down    bool   `json:"down"`
	Dx      int32  `json:"dx"`
	Dy      int32  `json:"dy"`
	Buttons int32  `json:"buttons"`
}

func streamHandler(w http.ResponseWriter, r *http.Request) {
	conn, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		log.Printf("websocket upgrade: %v", err)
		return
	}
	defer conn.Close()

	log.Printf("new session from %s", r.RemoteAddr)

	// Connect to game worker
	worker, err := dialWorker()
	if err != nil {
		log.Printf("worker connect failed: %v", err)
		return
	}
	defer worker.Close()

	sess := newSession(conn, worker)

	// Notify browser that streaming is ready
	if err := sess.sendJSON(map[string]interface{}{
		"type":   "ready",
		"width":  640,
		"height": 480,
		"fps":    targetFPS,
	}); err != nil {
		log.Printf("send ready: %v", err)
		return
	}

	// Start pushing frames
	go sess.frameRelay()

	// Read input messages from browser
	for {
		_, msg, err := conn.ReadMessage()
		if err != nil {
			if websocket.IsUnexpectedCloseError(err, websocket.CloseGoingAway, websocket.CloseNormalClosure) {
				log.Printf("session read error: %v", err)
			}
			break
		}

		var inp inputMsg
		if err := json.Unmarshal(msg, &inp); err != nil {
			log.Printf("bad input message: %v", err)
			continue
		}

		if inp.Type != "input" {
			continue
		}

		switch inp.Kind {
		case "key":
			if err := worker.InjectKey(inp.Key, inp.Down); err != nil {
				log.Printf("inject key error: %v", err)
				break
			}
		case "mouse":
			if err := worker.InjectMouse(inp.Dx, inp.Dy, inp.Buttons); err != nil {
				log.Printf("inject mouse error: %v", err)
				break
			}
		}
	}

	close(sess.done)
	log.Printf("session ended for %s", r.RemoteAddr)
}

// ---------------------------------------------------------------------------
// Health check
// ---------------------------------------------------------------------------

func healthHandler(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/plain")
	w.WriteHeader(http.StatusOK)
	w.Write([]byte("OK\n"))
}

// ---------------------------------------------------------------------------
// Static browser client
// ---------------------------------------------------------------------------

const clientHTML = `<!DOCTYPE html>
<html lang="en">
<head>
  <meta charset="UTF-8">
  <title>Quake Cloud</title>
  <style>
    * { margin: 0; padding: 0; box-sizing: border-box; }
    body { background: #000; display: flex; flex-direction: column;
           justify-content: center; align-items: center; height: 100vh;
           font-family: monospace; overflow: hidden; }
    canvas { cursor: crosshair; image-rendering: pixelated; }
    #overlay { position: absolute; top: 10px; left: 10px; color: #0f0;
               font-size: 12px; pointer-events: none; text-shadow: 1px 1px #000; z-index: 10; }
    #status { position: absolute; top: 50%; left: 50%; transform: translate(-50%,-50%);
              color: #e94560; font-size: 24px; text-shadow: 0 0 10px #e94560; z-index: 20; }
    #click-msg { position: absolute; bottom: 30px; color: #888; font-size: 14px; z-index: 20; }
  </style>
</head>
<body>
  <div id="overlay">FPS: --</div>
  <div id="status">Connecting...</div>
  <canvas id="c" width="640" height="480"></canvas>
  <div id="click-msg">Click to capture mouse</div>
  <script>
    const canvas = document.getElementById('c');
    const ctx = canvas.getContext('2d');
    const overlay = document.getElementById('overlay');
    const statusEl = document.getElementById('status');
    const clickMsg = document.getElementById('click-msg');

    // FPS tracking
    let frameCount = 0, lastFpsTime = performance.now(), fps = 0;

    // Quake key mapping (browser code → Quake keycode)
    const KEY_MAP = {
      'Backquote':96,'Digit1':49,'Digit2':50,'Digit3':51,'Digit4':52,'Digit5':53,
      'Digit6':54,'Digit7':55,'Digit8':56,'Digit9':57,'Digit0':48,
      'Minus':45,'Equal':61,'Backspace':127,
      'Tab':9,'KeyQ':113,'KeyW':119,'KeyE':101,'KeyR':114,'KeyT':116,
      'KeyY':121,'KeyU':117,'KeyI':105,'KeyO':111,'KeyP':112,
      'BracketLeft':91,'BracketRight':93,'Backslash':92,
      'KeyA':97,'KeyS':115,'KeyD':100,'KeyF':102,'KeyG':103,
      'KeyH':104,'KeyJ':106,'KeyK':107,'KeyL':108,
      'Semicolon':59,'Quote':39,'Enter':13,
      'ShiftLeft':304,'KeyZ':122,'KeyX':120,'KeyC':99,'KeyV':118,
      'KeyB':98,'KeyN':110,'KeyM':109,
      'Comma':44,'Period':46,'Slash':47,'ShiftRight':304,
      'ControlLeft':306,'AltLeft':308,'Space':32,'AltRight':308,'ControlRight':306,
      'ArrowUp':328,'ArrowDown':336,'ArrowLeft':331,'ArrowRight':333,
      'Escape':27,'F1':325,'F2':326,'F3':327,'F4':328,
      'PageUp':329,'PageDown':337,'Home':327,'End':335,
      'Insert':330,'Delete':127
    };

    // Pointer lock for FPS mouse controls
    canvas.addEventListener('click', () => {
      if (!document.pointerLockElement) canvas.requestPointerLock();
    });
    document.addEventListener('pointerlockchange', () => {
      clickMsg.style.display = document.pointerLockElement ? 'none' : 'block';
    });

    // WebSocket
    const wsProto = location.protocol === 'https:' ? 'wss:' : 'ws:';
    const ws = new WebSocket(wsProto + '//' + location.host + '/stream');
    ws.binaryType = 'arraybuffer';

    ws.onopen = () => { statusEl.textContent = 'Connected, waiting for frames...'; };
    ws.onerror = () => { statusEl.textContent = 'Connection error'; statusEl.style.color='#e94560'; };
    ws.onclose = () => { statusEl.textContent = 'Disconnected'; statusEl.style.color='#ff0'; };

    ws.onmessage = (ev) => {
      if (typeof ev.data === 'string') {
        // JSON control message
        const msg = JSON.parse(ev.data);
        if (msg.type === 'ready') {
          canvas.width = msg.width || 640;
          canvas.height = msg.height || 480;
          statusEl.style.display = 'none';
        }
        return;
      }

      // Binary = JPEG frame
      const blob = new Blob([ev.data], { type: 'image/jpeg' });
      createImageBitmap(blob).then(bmp => {
        ctx.drawImage(bmp, 0, 0, canvas.width, canvas.height);
        bmp.close();
        frameCount++;
        const now = performance.now();
        if (now - lastFpsTime >= 1000) {
          fps = frameCount;
          frameCount = 0;
          lastFpsTime = now;
          overlay.textContent = 'FPS: ' + fps;
        }
      }).catch(() => {});
    };

    // Input: keyboard
    function sendKey(code, down) {
      const qk = KEY_MAP[code];
      if (qk !== undefined && ws.readyState === 1) {
        ws.send(JSON.stringify({type:'input',kind:'key',key:qk,down:down}));
      }
    }
    document.addEventListener('keydown', (e) => { e.preventDefault(); sendKey(e.code, true); });
    document.addEventListener('keyup', (e) => { e.preventDefault(); sendKey(e.code, false); });

    // Input: mouse movement (only when pointer is locked)
    document.addEventListener('mousemove', (e) => {
      if (document.pointerLockElement && ws.readyState === 1) {
        ws.send(JSON.stringify({type:'input',kind:'mouse',dx:e.movementX,dy:e.movementY,buttons:0}));
      }
    });

    // Input: mouse buttons
    canvas.addEventListener('mousedown', (e) => {
      if (document.pointerLockElement && ws.readyState === 1) {
        ws.send(JSON.stringify({type:'input',kind:'mouse',dx:0,dy:0,buttons:(1<<e.button)}));
      }
    });
    canvas.addEventListener('mouseup', (e) => {
      if (document.pointerLockElement && ws.readyState === 1) {
        ws.send(JSON.stringify({type:'input',kind:'mouse',dx:0,dy:0,buttons:0}));
      }
    });

    // Prevent context menu in game area
    canvas.addEventListener('contextmenu', (e) => e.preventDefault());
  </script>
</body>
</html>`

func clientHandler(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.WriteHeader(http.StatusOK)
	w.Write([]byte(clientHTML))
}

// ---------------------------------------------------------------------------
// main
// ---------------------------------------------------------------------------

func main() {
	log.Printf("streaming-gateway starting (worker=%s listen=%s fps=%d)",
		workerAddr, listenAddr, targetFPS)

	mux := http.NewServeMux()
	mux.HandleFunc("/healthz", healthHandler)
	mux.HandleFunc("/stream", streamHandler)
	mux.HandleFunc("/signal", streamHandler) // keep old path working
	mux.HandleFunc("/", clientHandler)

	srv := &http.Server{
		Addr:         listenAddr,
		Handler:      mux,
		ReadTimeout:  30 * time.Second,
		WriteTimeout: 30 * time.Second,
		IdleTimeout:  120 * time.Second,
	}

	log.Printf("listening on %s", listenAddr)
	if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		log.Fatalf("listen: %v", err)
	}
}
