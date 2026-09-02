// Package tests holds the harness for this project's agreed test seams.
//
// It lives outside handlers/ and services/ so a harness that stands up the whole stack is not
// mistaken for a unit test of whichever package it happens to sit in.
//
// Real Postgres, never a mock. Idempotent re-ingest is ON CONFLICT behaviour and the correlation
// join is a windowed aggregate; a mock asserting on query strings would prove nothing about either.
package tests

import (
	"fmt"
	"os"
	"sync"
	"testing"

	"github.com/jmoiron/sqlx"

	"github.com/jeffreygaggino/we-are-rain-checking/backend/config"
	"github.com/jeffreygaggino/we-are-rain-checking/backend/db"
)

// The package's database: one schema, one pool, stood up on the first test that asks for one.
//
// Lazily rather than from a TestMain, so a run filtered down to tests that need no database never
// stands one up at all. Nothing closes the pool, for the same reason: the process exiting is what
// ends it, and a TestMain existing only to close it would put the setup back on every run.
var (
	setupOnce  sync.Once
	sharedPool *sqlx.DB
	setupErr   error
)

// RequireConfig loads the test configuration.
//
// It fails rather than skips when the test environment is absent. A silently skipped integration
// suite reports green while asserting nothing, which is the failure mode this repo exists to argue
// against. `make test` brings the database up first and sets the environment.
func RequireConfig(t *testing.T) *config.Config {
	t.Helper()

	if os.Getenv("DB_HOST") == "" {
		t.Fatal("tests: DB_HOST is unset — run `make test`, which starts Postgres and sets the test env")
	}

	config.LoadConfig()
	return config.GetConfig()
}

// RequireDB hands back the package's pool over a schema that is migrated, seeded, and empty of every
// ingested table.
//
// The clean slate is a truncation, not a rebuild. Seeded Circuits and Drivers are reference data
// this repo owns, written as literal constants so the same Driver carries the same id everywhere
// (ADR-0003), and nothing mutates them: ingest writes ingested tables only, and fails on an
// unrecognised Driver rather than inserting one. Re-inserting twenty-six Circuits and twenty-nine
// Drivers ahead of every test therefore buys no isolation the truncation does not already give.
func RequireDB(t *testing.T) (*sqlx.DB, *config.Config) {
	t.Helper()

	conn, cfg := packageDB(t)
	truncateIngested(t, conn)
	return conn, cfg
}

// RequireOwnSchema is RequireDB for a test that intends to destroy the schema and rebuild it — the
// migration tests, which exist to drive migrations up and down and assert on the result.
//
// However far down the test leaves it, the package's schema is migrated back up and emptied when the
// test ends, so the tests that run afterwards still find the one they were promised. The one state
// this cannot restore from is a dirty schema, which means a migration failed partway and every test
// in the package is going to fail anyway — so the cleanup says so and stops rather than papering
// over it.
func RequireOwnSchema(t *testing.T) (*sqlx.DB, *config.Config) {
	t.Helper()

	conn, cfg := packageDB(t)
	t.Cleanup(func() {
		if err := db.MigrateUp(cfg); err != nil {
			t.Errorf("tests: restoring the package schema: %v", err)
			return
		}
		truncateIngested(t, conn)
	})

	truncateIngested(t, conn)
	return conn, cfg
}

// packageDB stands the schema up on first use and hands every later caller the same pool.
func packageDB(t *testing.T) (*sqlx.DB, *config.Config) {
	t.Helper()

	cfg := RequireConfig(t)

	setupOnce.Do(func() {
		conn, err := db.Connect(cfg)
		if err != nil {
			setupErr = fmt.Errorf("connect (%s): %w", db.Describe(cfg), err)
			return
		}

		// Down first: a previous run that failed partway leaves a schema behind, and a test
		// asserting on counts would then pass or fail for reasons unrelated to the code under test.
		if err := db.MigrateDown(cfg); err != nil {
			setupErr = fmt.Errorf("migrate down: %w", err)
			return
		}
		if err := db.MigrateUp(cfg); err != nil {
			setupErr = fmt.Errorf("migrate up: %w", err)
			return
		}

		sharedPool = conn
	})
	if setupErr != nil {
		t.Fatalf("tests: standing up the package database: %v", setupErr)
	}

	return sharedPool, cfg
}

// truncateIngested empties every ingested table — the four ADR-0002 names — and leaves the seeded
// ones alone. All four, not only the two ingest writes today: a table this repo does not author the
// rows of is one a test must not inherit rows in, whenever the code that fills it lands.
//
// Every table holding a foreign key into one of these is itself in the list, so Postgres needs no
// CASCADE — and not having one is the point. A CASCADE would quietly extend the emptying to whatever
// table someone adds later, without anyone deciding it should be emptied.
func truncateIngested(t *testing.T, conn *sqlx.DB) {
	t.Helper()

	const q = `TRUNCATE f1.meetings, f1.sessions, f1.weather_samples, f1.session_results`
	if _, err := conn.Exec(q); err != nil {
		t.Fatalf("tests: emptying the ingested tables: %v", err)
	}
}
