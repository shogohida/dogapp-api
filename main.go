// Command dogapp-api serves the backend dogapp_flutter talks to: dog
// profiles/records, GPS walk history, and AI-based skin/gait checks via the
// Claude API. See README.md for setup and endpoint details.
package main

import (
	"context"
	"errors"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"dogapp-api/internal/claude"
	"dogapp-api/internal/handlers"
	"dogapp-api/internal/mailer"
	"dogapp-api/internal/store"
)

func main() {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	// DOGAPP_POSTGRES_DSN takes priority so it can be set explicitly; DATABASE_URL
	// is the name Render (and most other platforms) auto-injects when a
	// managed Postgres instance is linked to this service.
	dsn := envOr("DOGAPP_POSTGRES_DSN", envOr("DATABASE_URL", "postgres://postgres:password@127.0.0.1:5432/dogapp?sslmode=disable"))
	db, err := store.Open(ctx, dsn)
	if err != nil {
		log.Fatalf("open store: %v", err)
	}
	defer db.Close()

	if os.Getenv("ANTHROPIC_API_KEY") == "" {
		log.Println("warning: ANTHROPIC_API_KEY is not set - /ai-check and /gait-check will fail until it is")
	}

	var mail mailer.Mailer
	if apiKey := os.Getenv("RESEND_API_KEY"); apiKey != "" {
		fromEmail := envOr("RESEND_FROM_EMAIL", "onboarding@resend.dev")
		mail = mailer.NewResendMailer(apiKey, fromEmail)
	} else {
		log.Println("warning: RESEND_API_KEY is not set - signup welcome emails will be skipped")
		mail = mailer.NoopMailer{}
	}

	server := &handlers.Server{
		Store:   db,
		Checker: claude.NewAnthropicChecker(),
		Mailer:  mail,
	}

	addr := envOr("DOGAPP_ADDR", ":8080")
	httpServer := &http.Server{
		Addr:              addr,
		Handler:           server.Routes(),
		ReadHeaderTimeout: 10 * time.Second,
		// Body reads are capped per-handler (see handlers.maxVideoUploadBytes
		// and the ai-check request size limit), so a generous overall
		// ReadTimeout just guards against a stalled connection rather than
		// cutting off legitimate large uploads.
		ReadTimeout:  2 * time.Minute,
		WriteTimeout: 2 * time.Minute,
		IdleTimeout:  2 * time.Minute,
	}

	go func() {
		<-ctx.Done()
		log.Println("shutting down...")
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		if err := httpServer.Shutdown(shutdownCtx); err != nil {
			log.Printf("shutdown: %v", err)
		}
	}()

	log.Printf("dogapp-api listening on %s", addr)
	if err := httpServer.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
		log.Fatal(err)
	}
}

func envOr(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}
