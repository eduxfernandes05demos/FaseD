/*
Package main implements the Quake Cloud Assets API.

Serves extracted Quake PAK file contents from Azure Blob Storage.
In production, Azure CDN is placed in front for caching.

Endpoints:
  GET /healthz              - Liveness probe
  GET /api/assets           - List available assets
  GET /api/assets/{path...} - Serve a specific asset by path

Environment variables:
  LISTEN_ADDR       - HTTP listen address (default: :8070)
  ASSETS_BASE_DIR   - Local directory for assets (default: /game)
  AZURE_STORAGE_URL - Azure Blob Storage URL (when set, overrides local dir)
*/
package main

import (
	"log"
	"net/http"
	"os"
	"path/filepath"
	"strings"
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
	listenAddr   = envOr("LISTEN_ADDR", ":8070")
	assetsBase   = envOr("ASSETS_BASE_DIR", "/game")
)

func healthHandler(w http.ResponseWriter, r *http.Request) {
	w.Write([]byte("OK\n"))
}

func assetHandler(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	rawPath := vars["path"]

	// Reject path traversal in the raw (un-cleaned) path before any normalization.
	if strings.Contains(rawPath, "..") {
		http.Error(w, "forbidden", http.StatusForbidden)
		return
	}

	cleanPath := filepath.Clean("/" + rawPath)
	fullPath := filepath.Join(assetsBase, cleanPath)

	// Belt-and-suspenders: ensure the resolved path is still under assetsBase.
	if !strings.HasPrefix(fullPath, filepath.Clean(assetsBase)+string(os.PathSeparator)) {
		http.Error(w, "forbidden", http.StatusForbidden)
		return
	}

	http.ServeFile(w, r, fullPath)
}

func listAssetsHandler(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	// TODO: implement asset listing from Blob Storage or local dir
	w.Write([]byte(`{"assets":[],"note":"populate from PAK extraction pipeline"}`))
}

func main() {
	log.Printf("assets-api starting on %s (base=%s)", listenAddr, assetsBase)

	r := mux.NewRouter()
	r.HandleFunc("/healthz",              healthHandler).Methods(http.MethodGet)
	r.HandleFunc("/api/assets",           listAssetsHandler).Methods(http.MethodGet)
	r.PathPrefix("/api/assets/").HandlerFunc(assetHandler).Methods(http.MethodGet)

	srv := &http.Server{
		Addr:         listenAddr,
		Handler:      r,
		ReadTimeout:  15 * time.Second,
		WriteTimeout: 30 * time.Second,
		IdleTimeout:  60 * time.Second,
	}

	if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		log.Fatalf("listen: %v", err)
	}
}
