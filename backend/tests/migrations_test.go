package tests_test

import (
	"strings"
	"testing"
	"time"

	"github.com/jmoiron/sqlx"

	"github.com/jeffreygaggino/we-are-rain-checking/backend/config"
	"github.com/jeffreygaggino/we-are-rain-checking/backend/db"
	"github.com/jeffreygaggino/we-are-rain-checking/backend/tests"
)

// These ids are copied from 000002 by hand, on purpose. If the migration ever regenerates them —
// gen_random_uuid(), a sequence, anything derived at migration time — this test is what fails, and
// it fails naming the person whose identity moved. See ADR-0003.
const (
	verstappenID = "7948efdf-1ca6-4274-bc5e-b392565c13f0"
	norrisID     = "d6588e29-d18d-43a3-85cd-3cb8c1e7d877"
	monzaID      = "d895bd57-b3c6-4f2f-b2a4-b5a013592277"
	silverstone  = "d5ffead2-0555-4abc-b5f0-734ccd124d13"
)

// The tables are not asserted here. A missing one fails every test that queries it, naming the query
// that broke — so a loop over information_schema would only restate 000001 and report the same
// breakage less clearly. The indexes are the opposite, and are the reason this test exists.
func TestMigrationsApplyCleanlyAndCreateEveryIndex(t *testing.T) {
	conn, cfg := tests.RequireDB(t)

	version, dirty, err := db.MigrateVersion(cfg)
	if err != nil {
		t.Fatalf("MigrateVersion: %v", err)
	}
	if dirty {
		t.Fatal("schema is dirty after a clean up migration")
	}
	if version != 2 {
		t.Errorf("version = %d, want 2", version)
	}

	// An index is invisible to correctness: drop one and every query still returns the right rows,
	// slower. Nothing else in this suite would notice, which is what makes asserting them worth the
	// lines. Reasoning for each lives in 000001 beside the table it serves.
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

// Coordinates are seeded, not geocoded (ADR-0003), so a Circuit that reached the table without one
// is a seed gap that no later code path would report — ingest resolves a Circuit by key and never
// reads its position.
//
// The seeded row counts are deliberately not asserted. Their numbers change whenever a season is
// added, and a test that has to be edited alongside the seed catches only the edit.
func TestEverySeededCircuitCarriesItsCoordinates(t *testing.T) {
	conn, _ := tests.RequireDB(t)

	var missingCoords int
	const q = `SELECT COUNT(*) FROM f1.circuits WHERE latitude = 0 AND longitude = 0`
	if err := conn.Get(&missingCoords, q); err != nil {
		t.Fatalf("checking coordinates: %v", err)
	}
	if missingCoords != 0 {
		t.Errorf("%d circuits have no coordinates", missingCoords)
	}
}

// The structural half of ADR-0003: a Racing Number cannot be an identity here because there is no
// column it could be one in. Which seeded person holds which id is asserted where it can regress —
// across a rebuild, below — and why the numbers collide is recorded in the ADR rather than restated
// as a row-by-row expectation of the seed.
func TestNoColumnLetsARacingNumberBecomeADriverIdentity(t *testing.T) {
	conn, _ := tests.RequireDB(t)

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
	conn, cfg := tests.RequireOwnSchema(t)

	before := seedFingerprint(t, conn)

	if err := db.MigrateDown(cfg); err != nil {
		t.Fatalf("migrate down: %v", err)
	}
	assertSeedTablesGone(t, conn)
	if err := db.MigrateUp(cfg); err != nil {
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

// The migration driver takes a connection out of the pool it is built over and holds it for its
// whole lifetime, so a migrator built over the caller's pool costs the caller one of ten for as long
// as it lives. A migrator that reached for the caller's pool again would show up here as a
// connection in use.
func TestDrivingMigrationsTakesNothingFromTheCallersPool(t *testing.T) {
	conn, cfg := tests.RequireOwnSchema(t)

	// Fatal rather than an error: every assertion below wants zero too, and letting them run against
	// an already-wrong pool would report three more failures pointing at the harness instead of at
	// the operation under test.
	if inUse := conn.Stats().InUse; inUse != 0 {
		t.Fatalf("%d connections are in use before this test ran, want 0", inUse)
	}

	// Ends on up, so the down in the middle is not what the next assertion runs against.
	operations := []struct {
		name string
		run  func() error
	}{
		{"version", func() error { _, _, err := db.MigrateVersion(cfg); return err }},
		{"down", func() error { return db.MigrateDown(cfg) }},
		{"up", func() error { return db.MigrateUp(cfg) }},
	}

	for _, operation := range operations {
		if err := operation.run(); err != nil {
			t.Fatalf("migrate %s: %v", operation.name, err)
		}

		if got := conn.Stats().InUse; got != 0 {
			t.Errorf("after migrate %s the pool has %d connections in use, want 0 — the migrator took one",
				operation.name, got)
		}

		// Usable, not merely accounted for: the way a migrator releases what it holds is to close
		// the pool it was built over, and the caller's must not be the pool it closes.
		var alive int
		if err := conn.Get(&alive, `SELECT 1`); err != nil {
			t.Fatalf("the pool is unusable after migrate %s: %v", operation.name, err)
		}
	}
}

// The migrator owns its pool now, so it is the only thing that can close it. A migrator that does
// not leaves a Postgres backend behind per operation — invisible to any pool statistic the caller
// can read, and bounded by nothing but the server's connection limit. So the count is read from the
// server: it is the only place the leak is observable.
func TestDrivingMigrationsRepeatedlyLeavesNoBackendsBehind(t *testing.T) {
	conn, cfg := tests.RequireDB(t)

	const checks = 25 // comfortably past a pool's cap of ten, and a quarter of the server's hundred

	before := backendCount(t, conn)
	for range checks {
		if _, _, err := db.MigrateVersion(cfg); err != nil {
			t.Fatalf("MigrateVersion: %v", err)
		}
	}

	// Closing a pool asks the server to end its backend, which it does a moment later rather than
	// synchronously. Only a count that stays high is a leak; one that settles is the close landing.
	//
	// The tolerance is for the count being the server's rather than this suite's: a psql session
	// someone left open would otherwise fail the test. A leak here is one backend per operation, so
	// twenty-five of them clear any tolerance this side of the connection limit.
	const tolerance = 5
	var after int
	for deadline := time.Now().Add(5 * time.Second); ; {
		after = backendCount(t, conn)
		if after <= before+tolerance || time.Now().After(deadline) {
			break
		}
		time.Sleep(100 * time.Millisecond)
	}

	if after > before+tolerance {
		t.Errorf("%d backends were open before %d version checks and %d after, want no more than %d — each migrator kept its pool",
			before, checks, after, before+tolerance)
	}
}

// backendCount is how many connections the server holds to this database, whoever opened them.
func backendCount(t *testing.T, conn *sqlx.DB) int {
	t.Helper()

	var n int
	const q = `SELECT COUNT(*) FROM pg_stat_activity WHERE datname = current_database()`
	if err := conn.Get(&n, q); err != nil {
		t.Fatalf("counting backends: %v", err)
	}
	return n
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
