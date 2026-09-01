// Package tests holds the harness for this project's agreed test seams.
//
// It lives outside handlers/ and services/ so a harness that stands up the whole stack is not
// mistaken for a unit test of whichever package it happens to sit in.
//
// Real Postgres, never a mock. Idempotent re-ingest is ON CONFLICT behaviour and the correlation
// join is a windowed aggregate; a mock asserting on query strings would prove nothing about either.
package tests

import (
	"os"
	"testing"

	"github.com/jmoiron/sqlx"

	"github.com/jeffreygaggino/we-are-rain-checking/backend/config"
	"github.com/jeffreygaggino/we-are-rain-checking/backend/db"
)

// RequireDB connects to the test database and applies the migrations, returning a pool wound back
// to a known-empty schema.
//
// It fails rather than skips when the database is absent. A silently skipped integration suite
// reports green while asserting nothing, which is the failure mode this repo exists to argue
// against. `make test` brings the database up first.
func RequireDB(t *testing.T) (*sqlx.DB, *config.Config) {
	t.Helper()

	if os.Getenv("DB_HOST") == "" {
		t.Fatal("tests: DB_HOST is unset — run `make test`, which starts Postgres and sets the test env")
	}

	config.LoadConfig()
	cfg := config.GetConfig()

	conn, err := db.Connect(cfg)
	if err != nil {
		t.Fatalf("tests: connect (%s): %v", db.Describe(cfg), err)
	}
	t.Cleanup(func() { _ = conn.Close() })

	// Down first: a previous run that failed partway leaves rows behind, and a test asserting on
	// counts would then pass or fail for reasons unrelated to the code under test.
	if err := db.MigrateDown(conn, cfg); err != nil {
		t.Fatalf("tests: migrate down: %v", err)
	}
	if err := db.MigrateUp(conn, cfg); err != nil {
		t.Fatalf("tests: migrate up: %v", err)
	}

	return conn, cfg
}
