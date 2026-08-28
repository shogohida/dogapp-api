// Package storetest provisions a throwaway Postgres database per test, so
// store and handlers tests exercise the real driver/SQL instead of a fake.
// It's a separate package (rather than *_test.go helpers) so both
// internal/store and internal/handlers tests can import it.
package storetest

import (
	"context"
	"database/sql"
	"fmt"
	"net/url"
	"os"
	"testing"
	"time"

	_ "github.com/lib/pq"
)

// NewDSN creates a uniquely-named database on the Postgres server pointed to
// by DOGAPP_POSTGRES_TEST_DSN (default: the docker-compose dev server) and
// returns a DSN scoped to it. The database is dropped via t.Cleanup.
//
// If no Postgres server is reachable, the test is skipped rather than
// failed - CI always runs a postgres service container, so this only
// affects local `go test` runs without `docker compose up -d postgres`
// first.
func NewDSN(t testing.TB) string {
	t.Helper()

	adminDSN := os.Getenv("DOGAPP_POSTGRES_TEST_DSN")
	if adminDSN == "" {
		adminDSN = "postgres://postgres:password@127.0.0.1:5432/postgres?sslmode=disable"
	}

	adminDB, err := sql.Open("postgres", adminDSN)
	if err != nil {
		t.Fatalf("open admin postgres connection: %v", err)
	}

	pingCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := adminDB.PingContext(pingCtx); err != nil {
		adminDB.Close()
		t.Skipf("no Postgres reachable at %s (run `docker compose up -d postgres`): %v", adminDSN, err)
	}

	dbName := fmt.Sprintf("dogapp_test_%d", time.Now().UnixNano())
	if _, err := adminDB.ExecContext(pingCtx, "CREATE DATABASE "+dbName); err != nil {
		adminDB.Close()
		t.Fatalf("create test database %s: %v", dbName, err)
	}

	t.Cleanup(func() {
		dropCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		if _, err := adminDB.ExecContext(dropCtx, "DROP DATABASE "+dbName); err != nil {
			t.Logf("drop test database %s: %v", dbName, err)
		}
		adminDB.Close()
	})

	u, err := url.Parse(adminDSN)
	if err != nil {
		t.Fatalf("parse DOGAPP_POSTGRES_TEST_DSN %q: %v", adminDSN, err)
	}
	u.Path = "/" + dbName
	return u.String()
}
