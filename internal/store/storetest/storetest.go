// Package storetest provisions a throwaway MySQL database per test, so
// store and handlers tests exercise the real driver/SQL instead of a fake.
// It's a separate package (rather than *_test.go helpers) so both
// internal/store and internal/handlers tests can import it.
package storetest

import (
	"context"
	"database/sql"
	"fmt"
	"os"
	"strings"
	"testing"
	"time"

	_ "github.com/go-sql-driver/mysql"
)

// NewDSN creates a uniquely-named database on the MySQL server pointed to
// by DOGAPP_MYSQL_TEST_DSN (default: the docker-compose dev server) and
// returns a DSN scoped to it. The database is dropped via t.Cleanup.
//
// If no MySQL server is reachable, the test is skipped rather than failed -
// CI always runs a mysql service container, so this only affects local
// `go test` runs without `docker compose up -d mysql` first.
func NewDSN(t testing.TB) string {
	t.Helper()

	adminDSN := os.Getenv("DOGAPP_MYSQL_TEST_DSN")
	if adminDSN == "" {
		adminDSN = "root:password@tcp(127.0.0.1:3306)/?parseTime=true&multiStatements=true&loc=UTC"
	}

	adminDB, err := sql.Open("mysql", adminDSN)
	if err != nil {
		t.Fatalf("open admin mysql connection: %v", err)
	}

	pingCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := adminDB.PingContext(pingCtx); err != nil {
		adminDB.Close()
		t.Skipf("no MySQL reachable at %s (run `docker compose up -d mysql`): %v", adminDSN, err)
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

	if !strings.Contains(adminDSN, "/?") {
		t.Fatalf(`DOGAPP_MYSQL_TEST_DSN must have an empty database name ("...)/?params"), got %q`, adminDSN)
	}
	return strings.Replace(adminDSN, "/?", "/"+dbName+"?", 1)
}
