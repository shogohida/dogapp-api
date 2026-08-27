// Package handlers wires HTTP routes to the store and Claude checker.
package handlers

import (
	"encoding/json"
	"log"
	"net/http"

	"dogapp-api/internal/claude"
	"dogapp-api/internal/store"
)

type Server struct {
	Store   *store.Store
	Checker claude.Checker
}

// Routes builds the HTTP mux. Uses Go 1.22+'s method+pattern ServeMux
// (GET/POST + {param} wildcards) instead of a routing dependency.
func (s *Server) Routes() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /health", healthCheck)
	mux.HandleFunc("GET /owners/{ownerId}/dogs", s.listDogs)
	mux.HandleFunc("POST /dogs/{dogId}/ai-check", s.aiCheck)
	mux.HandleFunc("POST /dogs/{dogId}/gait-check", s.gaitCheck)
	mux.HandleFunc("POST /dogs/{dogId}/records", s.addRecord)
	mux.HandleFunc("GET /dogs/{dogId}/walks", s.listWalks)
	mux.HandleFunc("POST /dogs/{dogId}/walks", s.createWalk)
	return withCORS(withLogging(mux))
}

func withCORS(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Access-Control-Allow-Origin", "*")
		w.Header().Set("Access-Control-Allow-Methods", "GET, POST, OPTIONS")
		w.Header().Set("Access-Control-Allow-Headers", "Content-Type")
		if r.Method == http.MethodOptions {
			w.WriteHeader(http.StatusNoContent)
			return
		}
		next.ServeHTTP(w, r)
	})
}

func withLogging(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		log.Printf("%s %s", r.Method, r.URL.Path)
		next.ServeHTTP(w, r)
	})
}

func writeJSON(w http.ResponseWriter, status int, body any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	if err := json.NewEncoder(w).Encode(body); err != nil {
		log.Printf("write response: %v", err)
	}
}

func writeError(w http.ResponseWriter, status int, message string) {
	writeJSON(w, status, map[string]string{"error": message})
}

// healthCheck is a plain liveness probe - no store/Claude dependency, so it
// stays fast and answers even if those are degraded.
func healthCheck(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}
