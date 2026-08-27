// Command dogapp-api serves the backend dogapp_flutter talks to: dog
// profiles/records, GPS walk history, and AI-based skin/gait checks via the
// Claude API. See README.md for setup and endpoint details.
package main

import (
	"context"
	"log"
	"net/http"
	"os"

	"dogapp-api/internal/claude"
	"dogapp-api/internal/handlers"
	"dogapp-api/internal/store"
)

func main() {
	ctx := context.Background()

	dbPath := envOr("DOGAPP_DB_PATH", "dogapp.db")
	db, err := store.Open(ctx, dbPath)
	if err != nil {
		log.Fatalf("open store: %v", err)
	}
	defer db.Close()

	if os.Getenv("ANTHROPIC_API_KEY") == "" {
		log.Println("warning: ANTHROPIC_API_KEY is not set - /ai-check and /gait-check will fail until it is")
	}

	server := &handlers.Server{
		Store:   db,
		Checker: claude.NewAnthropicChecker(),
	}

	addr := envOr("DOGAPP_ADDR", ":8080")
	log.Printf("dogapp-api listening on %s (db: %s)", addr, dbPath)
	if err := http.ListenAndServe(addr, server.Routes()); err != nil {
		log.Fatal(err)
	}
}

func envOr(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}
