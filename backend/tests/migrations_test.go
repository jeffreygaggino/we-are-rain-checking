package tests_test

import (
	"strings"
	"testing"
	"time"

	"github.com/jmoiron/sqlx"

	"github.com/jeffreygaggino/we-are-rain-checking/backend/config"
	"github.com/jeffreygaggino/we-are-rain-checking/backend/db"
	"github.com/jeffreygaggino/we-are-rain-checking/backend/models"
	"github.com/jeffreygaggino/we-are-rain-checking/backend/tests"
)

// These ids are copied from 000002 by hand, on purpose. If the migration ever regenerates them —
// gen_random_uuid(), a sequence, anything derived at migration time — this test is what fails, and
// it fails naming the person whose identity moved. See ADR-0003.
const (
	verstappenID = "7948efdf-1ca6-4274-bc5e-b392565c13f0"
	norrisID     = "d6588e29-d18d-43a3-85cd-3cb8c1e7d877"
	ricciardoID  = "9ddf4757-e218-47c4-b8b2-04f340422893"
	monzaID      = "d895bd57-b3c6-4f2f-b2a4-b5a013592277"
	silverstone  = "d5ffead2-0555-4abc-b5f0-734ccd124d13"
)

func TestMigrationsApplyAndCreateEverySchemaObject(t *testing.T) {
	conn, cfg := tests.RequireDB(t)

	version, dirty, err := db.MigrateVersion(conn, cfg)
	if err != nil {
		t.Fatalf("MigrateVersion: %v", err)
	}
	if dirty {
		t.Fatal("schema is dirty after a clean up migration")
	}
	if version != 2 {
		t.Errorf("version = %d, want 2", version)
	}

	for _, table := range []string{"circuits", "drivers", "meetings", "sessions", "weather_samples", "session_results"} {
		var exists bool
		const q = `SELECT EXISTS (SELECT 1 FROM information_schema.tables WHERE table_schema = 'f1' AND table_name = $1)`
		if err := conn.Get(&exists, q, table); err != nil {
			t.Fatalf("checking table %s: %v", table, err)
		}
		if !exists {
			t.Errorf("table f1.%s was not created", table)
		}
	}

	// The indexes are deliberate choices, not defaults, so their absence is a regression worth
	// naming. Reasoning for each lives in 000001 beside the table it serves.
	for _, index := range []string{"sessions_date_start_idx", "sessions_year_name_idx", "session_results_driver_idx"} {
		var exists bool
		const q = `SELECT EXISTS (SELECT 1 FROM pg_indexes WHERE schemaname = 'f1' AND indexname = $1)`
		if err := conn.Get(&exists, q, index); err != nil {
			t.Fatalf("checking index %s: %v", index, err)
		}
		if !exists {
			t.Errorf("index %s was not created", index)
		}
	}
}

// ADR-0002: ingested tables carry neither a creator column nor a soft-delete column, so nobody can
// write a query that has to remember to filter one.
func TestIngestedTablesHaveNoSoftDeleteOrCreatorColumns(t *testing.T) {
	conn, _ := tests.RequireDB(t)

	for _, table := range []string{"meetings", "sessions", "weather_samples", "session_results"} {
		for _, column := range []string{"deleted_at", "created_by"} {
			var exists bool
			const q = `SELECT EXISTS (
				SELECT 1 FROM information_schema.columns
				WHERE table_schema = 'f1' AND table_name = $1 AND column_name = $2)`
			if err := conn.Get(&exists, q, table, column); err != nil {
				t.Fatalf("checking %s.%s: %v", table, column, err)
			}
			if exists {
				t.Errorf("f1.%s has a %s column; ADR-0002 says ingested tables carry neither", table, column)
			}
		}
	}
}

func TestSeedPopulatesEveryCircuitAndDriverInTheCorpus(t *testing.T) {
	conn, _ := tests.RequireDB(t)

	var circuits, drivers int
	if err := conn.Get(&circuits, `SELECT COUNT(*) FROM f1.circuits`); err != nil {
		t.Fatalf("counting circuits: %v", err)
	}
	if err := conn.Get(&drivers, `SELECT COUNT(*) FROM f1.drivers`); err != nil {
		t.Fatalf("counting drivers: %v", err)
	}
	if circuits != 26 {
		t.Errorf("seeded %d circuits, want 26 — every distinct circuit across 2023-2026", circuits)
	}
	if drivers != 29 {
		t.Errorf("seeded %d drivers, want 29 — every driver appearing in a Race in the corpus", drivers)
	}

	// Coordinates are seeded, not geocoded, so every row must actually carry a position.
	var missingCoords int
	const q = `SELECT COUNT(*) FROM f1.circuits WHERE latitude = 0 AND longitude = 0`
	if err := conn.Get(&missingCoords, q); err != nil {
		t.Fatalf("checking coordinates: %v", err)
	}
	if missingCoords != 0 {
		t.Errorf("%d circuits have no coordinates", missingCoords)
	}
}

// A Driver row carries both the upstream display name ingest resolves on and the short form, so
// either can be shown without a second lookup.
func TestSeededDriverCarriesBothResolutionNameAndShortForm(t *testing.T) {
	conn, _ := tests.RequireDB(t)

	var driver models.Driver
	const q = `SELECT id, full_name, short_name, created_at, updated_at FROM f1.drivers WHERE id = $1`
	if err := conn.Get(&driver, q, verstappenID); err != nil {
		t.Fatalf("fetching seeded driver: %v", err)
	}
	if driver.FullName != "Max VERSTAPPEN" {
		t.Errorf("full_name = %q, want %q", driver.FullName, "Max VERSTAPPEN")
	}
	if driver.ShortName != "VER" {
		t.Errorf("short_name = %q, want %q", driver.ShortName, "VER")
	}
}

// The point of ADR-0003 in one assertion: the Racing Numbers that collide across this corpus belong
// to distinct seeded people. 1 covers both VERSTAPPEN and NORRIS; 3 covers both VERSTAPPEN and
// RICCIARDO. Nothing in the schema lets those merge, because no seeded identity is a number.
func TestCollidingRacingNumbersBelongToDistinctSeededDrivers(t *testing.T) {
	conn, _ := tests.RequireDB(t)

	ids := map[string]string{
		verstappenID: "Max VERSTAPPEN",
		norrisID:     "Lando NORRIS",
		ricciardoID:  "Daniel RICCIARDO",
	}
	for id, wantName := range ids {
		var name string
		if err := conn.Get(&name, `SELECT full_name FROM f1.drivers WHERE id = $1`, id); err != nil {
			t.Fatalf("fetching %s: %v", wantName, err)
		}
		if name != wantName {
			t.Errorf("id %s resolves to %q, want %q", id, name, wantName)
		}
	}

	// And there is no column a Racing Number could be an identity in.
	var exists bool
	const q = `SELECT EXISTS (
		SELECT 1 FROM information_schema.columns
		WHERE table_schema = 'f1' AND table_name = 'drivers' AND column_name LIKE '%number%')`
	if err := conn.Get(&exists, q); err != nil {
		t.Fatalf("checking drivers columns: %v", err)
	}
	if exists {
		t.Error("f1.drivers carries a number column; a Racing Number belongs to a season, not a person")
	}
}

// The criterion that makes seeded ids worth having: they survive a rebuild. A migration calling
// gen_random_uuid() passes every other test in this file and fails this one.
func TestSeededIdentifiersAreStableAcrossADownUpCycle(t *testing.T) {
	conn, cfg := tests.RequireDB(t)

	before := seedFingerprint(t, conn)

	if err := db.MigrateDown(conn, cfg); err != nil {
		t.Fatalf("migrate down: %v", err)
	}
	assertSeedTablesGone(t, conn)
	if err := db.MigrateUp(conn, cfg); err != nil {
		t.Fatalf("migrate up: %v", err)
	}

	after := seedFingerprint(t, conn)

	if len(before) != len(after) {
		t.Fatalf("seed row count changed across the cycle: %d then %d", len(before), len(after))
	}
	for key, id := range before {
		if after[key] != id {
			t.Errorf("%s: id was %s before the cycle and %s after — seeded ids must be literal constants (ADR-0003)",
				key, id, after[key])
		}
	}

	// Spot-check the named constants too, so a failure points at a person rather than a diff.
	for id, want := range map[string]string{
		verstappenID: "driver:Max VERSTAPPEN",
		norrisID:     "driver:Lando NORRIS",
		monzaID:      "circuit:Monza",
		silverstone:  "circuit:Silverstone",
	} {
		if after[want] != id {
			t.Errorf("%s has id %s after the cycle, want the literal %s", want, after[want], id)
		}
	}
}

// seedFingerprint maps a stable natural key to the repo-owned id, so a comparison across a rebuild
// names what moved rather than reporting that two sets differ.
func seedFingerprint(t *testing.T, conn *sqlx.DB) map[string]string {
	t.Helper()

	out := make(map[string]string)

	var drivers []struct {
		ID       string `db:"id"`
		FullName string `db:"full_name"`
	}
	if err := conn.Select(&drivers, `SELECT id, full_name FROM f1.drivers`); err != nil {
		t.Fatalf("fingerprinting drivers: %v", err)
	}
	for _, d := range drivers {
		out["driver:"+d.FullName] = d.ID
	}

	var circuits []struct {
		ID        string `db:"id"`
		ShortName string `db:"short_name"`
	}
	if err := conn.Select(&circuits, `SELECT id, short_name FROM f1.circuits`); err != nil {
		t.Fatalf("fingerprinting circuits: %v", err)
	}
	for _, c := range circuits {
		out["circuit:"+c.ShortName] = c.ID
	}

	return out
}

// Each down reverses exactly what its up created: 000002 removes its rows, 000001 removes the
// tables, and the schema is left with neither.
func assertSeedTablesGone(t *testing.T, conn *sqlx.DB) {
	t.Helper()

	var exists bool
	const q = `SELECT EXISTS (SELECT 1 FROM information_schema.tables WHERE table_schema = 'f1' AND table_name = 'drivers')`
	if err := conn.Get(&exists, q); err != nil {
		t.Fatalf("checking drivers table after down: %v", err)
	}
	if exists {
		t.Error("f1.drivers still exists after migrate down")
	}
}

// The migration driver takes a connection out of the pool when it is built. If nothing gives that
// connection back, every up, down and version check costs the caller one of ten for as long as the
// pool lives — and this suite is one pool per test, each of them driving migrations.
func TestMigrationOperationsReturnTheirConnectionToTheCallersPool(t *testing.T) {
	conn, cfg := tests.RequireDB(t)

	// RequireDB has already driven a down and an up of its own, so a migrator that keeps what it
	// borrows is visible before this test has done anything. Fatal rather than an error: every
	// assertion below wants zero too, and letting them run against an already-wrong pool would
	// report three more failures pointing at RequireDB instead of at the operation under test.
	if inUse := conn.Stats().InUse; inUse != 0 {
		t.Fatalf("%d connections are in use before this test ran, want 0 — RequireDB's own migrations kept them", inUse)
	}

	// Ends on up, so the schema this test leaves behind is the one the next test expects.
	operations := []struct {
		name string
		run  func() error
	}{
		{"version", func() error { _, _, err := db.MigrateVersion(conn, cfg); return err }},
		{"down", func() error { return db.MigrateDown(conn, cfg) }},
		{"up", func() error { return db.MigrateUp(conn, cfg) }},
	}

	for _, operation := range operations {
		if err := operation.run(); err != nil {
			t.Fatalf("migrate %s: %v", operation.name, err)
		}

		if got := conn.Stats().InUse; got != 0 {
			t.Errorf("after migrate %s the pool has %d connections in use, want 0 — the migrator kept one",
				operation.name, got)
		}

		// Usable, not merely accounted for: the obvious way to release the connection is to close
		// the migrator, which closes the caller's whole pool along with it.
		var alive int
		if err := conn.Get(&alive, `SELECT 1`); err != nil {
			t.Fatalf("the pool is unusable after migrate %s: %v", operation.name, err)
		}
	}
}

// A leaked connection is not bounded by the pool's cap of ten, it is subtracted from it. So a leak
// does not fail this test, it deadlocks it: the eleventh migrator waits for a connection that
// nobody will ever return. The deadline is what turns that into a failure a person can read.
func TestDrivingMigrationsRepeatedlyDoesNotExhaustThePool(t *testing.T) {
	conn, cfg := tests.RequireDB(t)

	const checks = 25 // comfortably past the pool's cap of ten

	done := make(chan error, 1)
	go func() {
		for range checks {
			if _, _, err := db.MigrateVersion(conn, cfg); err != nil {
				done <- err
				return
			}
		}
		done <- nil
	}()

	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("MigrateVersion: %v", err)
		}
	case <-time.After(30 * time.Second):
		stats := conn.Stats()
		t.Fatalf("%d version checks did not finish within 30s — %d connections open, %d in use, %d waiters: the migrator is not giving them back",
			checks, stats.OpenConnections, stats.InUse, stats.WaitCount)
	}

	if got := conn.Stats().InUse; got != 0 {
		t.Errorf("%d connections still in use after %d version checks, want 0", got, checks)
	}
}

// The DSN carries the password; the loggable description must not. Every log line in db/ uses
// Describe, so this asserts the property at its source.
func TestDescribeNeverCarriesThePassword(t *testing.T) {
	cfg := &config.Config{
		DBHost: "db.example.com", DBPort: "5432", DBName: "rainchecking",
		DBUser: "rainchecking", DBPassword: "hunter2-a-very-distinctive-secret",
		DBSchema: "f1", DBSSLMode: "require",
	}

	if got := db.Describe(cfg); strings.Contains(got, cfg.DBPassword) {
		t.Errorf("Describe() leaked the password: %q", got)
	}
	// The DSN is the one place it may appear, so the test proves Describe differs from DSN rather
	// than that the password is simply absent everywhere.
	if got := db.DSN(cfg); !strings.Contains(got, cfg.DBPassword) {
		t.Errorf("DSN() should carry the password, got %q", got)
	}
}
