/*
Package main implements the Quake Cloud Telemetry API.

Accepts structured game events from the streaming gateway and game-worker,
forwards them to Azure Application Insights via the Track Events API.

Endpoints:
  GET  /healthz        - Liveness probe
  POST /api/events     - Ingest one or more telemetry events

Event schema (JSON array):
  [
    { "name": "PlayerKill", "properties": { "weapon": "shotgun" }, "ts": "2024-..." },
    ...
  ]

Environment variables:
  LISTEN_ADDR            - HTTP listen address (default: :8060)
  APPINSIGHTS_KEY        - Application Insights instrumentation key
  MAX_EVENTS_PER_REQUEST - Maximum events per POST (default: 100)
*/
package main

import (
	"encoding/json"
	"log"
	"net/http"
	"os"
	"strconv"
	"time"

	"github.com/gorilla/mux"
)

func envOr(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}

var (
	listenAddr    = envOr("LISTEN_ADDR", ":8060")
	appInsightsKey = envOr("APPINSIGHTS_KEY", "")
	maxEvents, _  = strconv.Atoi(envOr("MAX_EVENTS_PER_REQUEST", "100"))
)

// TelemetryEvent matches the ingest schema.
type TelemetryEvent struct {
	Name       string            `json:"name"`
	Properties map[string]string `json:"properties,omitempty"`
	Timestamp  string            `json:"ts,omitempty"`
}

func healthHandler(w http.ResponseWriter, r *http.Request) {
	w.Write([]byte("OK\n"))
}

func eventsHandler(w http.ResponseWriter, r *http.Request) {
	var events []TelemetryEvent
	if err := json.NewDecoder(r.Body).Decode(&events); err != nil {
		http.Error(w, "invalid JSON", http.StatusBadRequest)
		return
	}

	if len(events) > maxEvents {
		http.Error(w, "too many events", http.StatusRequestEntityTooLarge)
		return
	}

	for _, ev := range events {
		// TODO: forward to Application Insights Track Events REST API
		// using appInsightsKey. For now, log locally.
		log.Printf("event: name=%s ts=%s props=%v", ev.Name, ev.Timestamp, ev.Properties)
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusAccepted)
	json.NewEncoder(w).Encode(map[string]int{"accepted": len(events)})
}

func main() {
	log.Printf("telemetry-api starting on %s", listenAddr)
	if appInsightsKey == "" {
		log.Printf("WARNING: APPINSIGHTS_KEY not set – events will be logged locally only")
	}

	r := mux.NewRouter()
	r.HandleFunc("/healthz",     healthHandler).Methods(http.MethodGet)
	r.HandleFunc("/api/events",  eventsHandler).Methods(http.MethodPost)

	srv := &http.Server{
		Addr:         listenAddr,
		Handler:      r,
		ReadTimeout:  10 * time.Second,
		WriteTimeout: 10 * time.Second,
		IdleTimeout:  60 * time.Second,
	}

	if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		log.Fatalf("listen: %v", err)
	}
}
