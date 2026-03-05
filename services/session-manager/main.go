/*
Package main implements the Quake Cloud Session Manager.

REST API:
  POST   /api/sessions         - Create a new game session (returns WebSocket URL)
  GET    /api/sessions/{id}    - Get session status
  DELETE /api/sessions/{id}    - End a session and tear down the worker

Session lifecycle:
  1. Client authenticates (Entra ID / OAuth2 – stub in this implementation)
  2. POST /api/sessions provisions a game-worker + streaming-gateway pair
     in Azure Container Apps and returns the WebSocket signaling URL.
  3. The client connects to the gateway WebSocket and plays the game.
  4. DELETE /api/sessions/{id} stops the worker and gateway replicas.

Environment variables:
  LISTEN_ADDR         - HTTP listen address (default: :8080)
  AZURE_SUBSCRIPTION  - Azure subscription ID
  AZURE_RESOURCE_GROUP - Resource group containing the Container Apps env
  ACA_ENV_NAME        - Azure Container Apps environment name
  GATEWAY_IMAGE       - Full gateway image reference
  WORKER_IMAGE        - Full worker image reference
*/
package main

import (
	"encoding/json"
	"log"
	"net/http"
	"os"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/gorilla/mux"
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

var listenAddr = envOr("LISTEN_ADDR", ":8080")

// ---------------------------------------------------------------------------
// Session model
// ---------------------------------------------------------------------------

type SessionStatus string

const (
	StatusProvisioning SessionStatus = "provisioning"
	StatusRunning      SessionStatus = "running"
	StatusTerminating  SessionStatus = "terminating"
	StatusDone         SessionStatus = "done"
)

type Session struct {
	ID           string        `json:"id"`
	Status       SessionStatus `json:"status"`
	GatewayURL   string        `json:"gatewayUrl,omitempty"`
	Map          string        `json:"map"`
	Skill        int           `json:"skill"`
	CreatedAt    time.Time     `json:"createdAt"`
	UpdatedAt    time.Time     `json:"updatedAt"`
}

// ---------------------------------------------------------------------------
// In-memory session store (replace with Azure Cosmos DB / Redis in prod)
// ---------------------------------------------------------------------------

type SessionStore struct {
	mu       sync.RWMutex
	sessions map[string]*Session
}

func newSessionStore() *SessionStore {
	return &SessionStore{sessions: make(map[string]*Session)}
}

func (s *SessionStore) Create(sess *Session) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.sessions[sess.ID] = sess
}

func (s *SessionStore) Get(id string) (*Session, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	sess, ok := s.sessions[id]
	return sess, ok
}

func (s *SessionStore) Delete(id string) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, ok := s.sessions[id]; !ok {
		return false
	}
	delete(s.sessions, id)
	return true
}

func (s *SessionStore) Update(sess *Session) {
	s.mu.Lock()
	defer s.mu.Unlock()
	sess.UpdatedAt = time.Now()
	s.sessions[sess.ID] = sess
}

// ---------------------------------------------------------------------------
// Create session request / response
// ---------------------------------------------------------------------------

type CreateSessionRequest struct {
	Map   string `json:"map"`
	Skill int    `json:"skill"`
}

// ---------------------------------------------------------------------------
// Worker provisioner (stub)
// In production: uses the Azure Container Apps management API to
// dynamically create worker + gateway replica pairs.
// ---------------------------------------------------------------------------

func provisionWorker(sess *Session) {
	// Simulate provisioning delay
	time.Sleep(2 * time.Second)

	// In production, call the ACA REST API here:
	//   PUT https://management.azure.com/.../containerApps/<name>?api-version=...
	// and get back the FQDN of the gateway.

	sess.GatewayURL = "wss://gateway-" + sess.ID[:8] + ".example.com/signal"
	sess.Status = StatusRunning
}

func tearDownWorker(id string) {
	// In production: call the ACA API to stop/delete the replicas
	log.Printf("tearing down worker for session %s", id)
}

// ---------------------------------------------------------------------------
// HTTP handlers
// ---------------------------------------------------------------------------

var store = newSessionStore()

func createSessionHandler(w http.ResponseWriter, r *http.Request) {
	var req CreateSessionRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "bad request body", http.StatusBadRequest)
		return
	}

	if req.Map == "" {
		req.Map = "e1m1"
	}
	if req.Skill < 0 || req.Skill > 3 {
		req.Skill = 1
	}

	sess := &Session{
		ID:        uuid.NewString(),
		Status:    StatusProvisioning,
		Map:       req.Map,
		Skill:     req.Skill,
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
	}
	store.Create(sess)

	// Provision the worker asynchronously
	go func() {
		provisionWorker(sess)
		store.Update(sess)
		log.Printf("session %s ready: %s", sess.ID, sess.GatewayURL)
	}()

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusAccepted)
	json.NewEncoder(w).Encode(sess)
}

func getSessionHandler(w http.ResponseWriter, r *http.Request) {
	id := mux.Vars(r)["id"]
	sess, ok := store.Get(id)
	if !ok {
		http.Error(w, "session not found", http.StatusNotFound)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(sess)
}

func deleteSessionHandler(w http.ResponseWriter, r *http.Request) {
	id := mux.Vars(r)["id"]
	sess, ok := store.Get(id)
	if !ok {
		http.Error(w, "session not found", http.StatusNotFound)
		return
	}

	sess.Status = StatusTerminating
	store.Update(sess)

	go func() {
		tearDownWorker(id)
		store.Delete(id)
	}()

	w.WriteHeader(http.StatusNoContent)
}

func healthHandler(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/plain")
	w.Write([]byte("OK\n"))
}

// ---------------------------------------------------------------------------
// main
// ---------------------------------------------------------------------------

func main() {
	log.Printf("session-manager starting on %s", listenAddr)

	r := mux.NewRouter()
	r.HandleFunc("/healthz",            healthHandler).Methods(http.MethodGet)
	r.HandleFunc("/api/sessions",       createSessionHandler).Methods(http.MethodPost)
	r.HandleFunc("/api/sessions/{id}",  getSessionHandler).Methods(http.MethodGet)
	r.HandleFunc("/api/sessions/{id}",  deleteSessionHandler).Methods(http.MethodDelete)

	srv := &http.Server{
		Addr:         listenAddr,
		Handler:      r,
		ReadTimeout:  15 * time.Second,
		WriteTimeout: 15 * time.Second,
		IdleTimeout:  60 * time.Second,
	}

	if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		log.Fatalf("listen: %v", err)
	}
}
