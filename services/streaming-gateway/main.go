/*
Package main implements the Quake Cloud Streaming Gateway.

Architecture:
  - Connects to the game-worker sidecar via a Unix domain socket or
    TCP loopback to receive RGBA video frames and PCM audio.
  - Encodes video to H.264 (via an external encoder stub) and audio to
    Opus (via an external encoder stub).
  - Serves a WebRTC signaling endpoint over WebSocket so browsers can
    establish a peer connection.
  - Forwards browser keyboard/mouse events back to the game-worker via
    the data channel.

Environment variables:
  WORKER_ADDR     - game-worker IPC address (default: localhost:9000)
  LISTEN_ADDR     - HTTP listen address (default: :8090)
  STUN_SERVER     - STUN server URI (default: stun:stun.l.google.com:19302)
*/
package main

import (
	"encoding/json"
	"log"
	"net/http"
	"os"
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
	stunServer = envOr("STUN_SERVER", "stun:stun.l.google.com:19302")
)

// ---------------------------------------------------------------------------
// WebSocket upgrader
// ---------------------------------------------------------------------------

var upgrader = websocket.Upgrader{
	ReadBufferSize:  4096,
	WriteBufferSize: 4096,
	CheckOrigin: func(r *http.Request) bool {
		// TODO(production): restrict to known origins
		return true
	},
}

// ---------------------------------------------------------------------------
// Session represents one connected browser peer
// ---------------------------------------------------------------------------

type Session struct {
	mu   sync.Mutex
	conn *websocket.Conn
	done chan struct{}
}

func newSession(conn *websocket.Conn) *Session {
	return &Session{
		conn: conn,
		done: make(chan struct{}),
	}
}

// sendJSON sends a JSON message to the browser peer.
func (s *Session) sendJSON(msg interface{}) error {
	data, err := json.Marshal(msg)
	if err != nil {
		return err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.conn.WriteMessage(websocket.TextMessage, data)
}

// ---------------------------------------------------------------------------
// Signaling handler
// Handles the WebRTC offer/answer exchange via WebSocket.
// ---------------------------------------------------------------------------

func signalingHandler(w http.ResponseWriter, r *http.Request) {
	conn, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		log.Printf("websocket upgrade: %v", err)
		return
	}
	defer conn.Close()

	sess := newSession(conn)
	log.Printf("new signaling session from %s", r.RemoteAddr)

	// Send gateway capabilities to the browser
	if err := sess.sendJSON(map[string]interface{}{
		"type":       "config",
		"stun":       stunServer,
		"workerAddr": workerAddr,
	}); err != nil {
		log.Printf("send config: %v", err)
		return
	}

	// Message dispatch loop
	for {
		_, msg, err := conn.ReadMessage()
		if err != nil {
			if websocket.IsUnexpectedCloseError(err, websocket.CloseGoingAway, websocket.CloseNormalClosure) {
				log.Printf("session read error: %v", err)
			}
			break
		}

		var envelope map[string]json.RawMessage
		if err := json.Unmarshal(msg, &envelope); err != nil {
			log.Printf("bad message: %v", err)
			continue
		}

		var msgType string
		if err := json.Unmarshal(envelope["type"], &msgType); err != nil {
			continue
		}

		switch msgType {
		case "offer":
			// Browser sends an SDP offer; we return an answer.
			// In a production build this drives pion/webrtc PeerConnection.
			log.Printf("received SDP offer (len=%d)", len(envelope["sdp"]))
			answer := map[string]interface{}{
				"type": "answer",
				"sdp":  "v=0\r\no=- 0 0 IN IP4 127.0.0.1\r\n", // placeholder SDP
			}
			if err := sess.sendJSON(answer); err != nil {
				log.Printf("send answer: %v", err)
			}

		case "ice":
			// ICE candidate from the browser – relay to pion.
			log.Printf("received ICE candidate")

		case "input":
			// Input event from the browser data channel.
			// Forward to the game-worker's IPC socket.
			log.Printf("input event: %s", string(envelope["data"]))

		default:
			log.Printf("unknown message type: %s", msgType)
		}
	}

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
// Static browser client (served from embedded HTML)
// ---------------------------------------------------------------------------

const clientHTML = `<!DOCTYPE html>
<html lang="en">
<head>
  <meta charset="UTF-8">
  <title>Quake Cloud</title>
  <style>
    body { margin: 0; background: #000; display: flex; justify-content: center; align-items: center; height: 100vh; }
    canvas { border: 2px solid #555; }
  </style>
</head>
<body>
  <canvas id="gameCanvas" width="640" height="480"></canvas>
  <script>
    const ws = new WebSocket('ws://' + location.host + '/signal');
    const pc = new RTCPeerConnection();
    const canvas = document.getElementById('gameCanvas');

    ws.onmessage = async (event) => {
      const msg = JSON.parse(event.data);
      if (msg.type === 'config') {
        // Configure STUN and start the WebRTC offer
        const offer = await pc.createOffer();
        await pc.setLocalDescription(offer);
        ws.send(JSON.stringify({ type: 'offer', sdp: offer.sdp }));
      } else if (msg.type === 'answer') {
        await pc.setRemoteDescription({ type: 'answer', sdp: msg.sdp });
      } else if (msg.type === 'ice') {
        await pc.addIceCandidate(msg.candidate);
      }
    };

    pc.onicecandidate = (e) => {
      if (e.candidate) ws.send(JSON.stringify({ type: 'ice', candidate: e.candidate }));
    };

    pc.ontrack = (e) => {
      // Attach video/audio tracks
      if (e.track.kind === 'video') {
        const video = document.createElement('video');
        video.srcObject = e.streams[0];
        video.autoplay = true;
        video.style.display = 'none';
        document.body.appendChild(video);
      }
    };

    // Forward keyboard events via data channel
    const dc = pc.createDataChannel('input');
    document.addEventListener('keydown', (e) => {
      if (dc.readyState === 'open') dc.send(JSON.stringify({ type: 'key', key: e.key, down: true }));
    });
    document.addEventListener('keyup', (e) => {
      if (dc.readyState === 'open') dc.send(JSON.stringify({ type: 'key', key: e.key, down: false }));
    });
    canvas.addEventListener('mousemove', (e) => {
      if (dc.readyState === 'open') dc.send(JSON.stringify({ type: 'mouse', dx: e.movementX, dy: e.movementY }));
    });
  </script>
</body>
</html>`

func clientHandler(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.WriteHeader(http.StatusOK)
	w.Write([]byte(clientHTML))
}

// ---------------------------------------------------------------------------
// Frame relay loop (stub)
// In production: reads RGBA frames from the game-worker IPC socket,
// encodes to H.264 NAL units, and writes RTP packets to pion tracks.
// ---------------------------------------------------------------------------

func startFrameRelay() {
	go func() {
		ticker := time.NewTicker(33 * time.Millisecond) // ~30 fps
		defer ticker.Stop()
		for range ticker.C {
			// TODO: read from game-worker IPC → encode → RTP
		}
	}()
}

// ---------------------------------------------------------------------------
// main
// ---------------------------------------------------------------------------

func main() {
	log.Printf("streaming-gateway starting (worker=%s listen=%s stun=%s)",
		workerAddr, listenAddr, stunServer)

	startFrameRelay()

	mux := http.NewServeMux()
	mux.HandleFunc("/healthz",  healthHandler)
	mux.HandleFunc("/signal",   signalingHandler)
	mux.HandleFunc("/",         clientHandler)

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
