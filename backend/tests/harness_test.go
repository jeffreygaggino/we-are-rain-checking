package tests_test

import (
	"testing"

	"github.com/jeffreygaggino/we-are-rain-checking/backend/tests"
)

// The headline property of #15: the schema is stood up for the package, not for each test. A harness
// that rebuilt it per test would still pass every other test in this suite — it would just take the
// cost — so this is the only place the claim is asserted.
//
// It is asserted through a seeded table, which is the half of the database nothing winds back. A
// harness that rebuilt the schema between the two acquisitions would drop this Circuit with the
// table; one that reseeded would fail on the duplicate key before that.
func TestThePackageSchemaSurvivesFromOneAcquisitionToTheNext(t *testing.T) {
	first, _ := tests.RequireDB(t)

	const marker = "00000000-0000-4000-8000-0000000015c1"
	t.Cleanup(func() {
		// The seeded corpus is exactly 26 Circuits to the tests that count them, so this one goes
		// back out however this test ends.
		if _, err := first.Exec(`DELETE FROM f1.circuits WHERE id = $1`, marker); err != nil {
			t.Errorf("removing the marker circuit: %v", err)
		}
	})
	const insert = `INSERT INTO f1.circuits (id, circuit_key, short_name, location, country_name, latitude, longitude)
	                VALUES ($1, -15, 'Marker', 'Nowhere', 'Nowhere', 0, 0)`
	if _, err := first.Exec(insert, marker); err != nil {
		t.Fatalf("writing the marker circuit: %v", err)
	}

	second, _ := tests.RequireDB(t)

	if second != first {
		t.Error("two acquisitions returned different pools; the package stands its database up once")
	}
	var survived int
	if err := second.Get(&survived, `SELECT COUNT(*) FROM f1.circuits WHERE id = $1`, marker); err != nil {
		t.Fatalf("looking for the marker circuit: %v", err)
	}
	if survived != 1 {
		t.Error("the marker Circuit did not survive a second acquisition — the schema is being rebuilt per test")
	}
}

// The isolation a test actually needs: ingest writes only Meetings and Sessions, so emptying those
// is the same clean slate reseeding gave, and the reference data ingest resolves against is still
// there to resolve against (ADR-0003).
func TestACleanSlateEmptiesTheIngestedTablesAndKeepsTheSeededReferenceData(t *testing.T) {
	conn, _ := tests.RequireDB(t)

	if _, err := conn.Exec(`INSERT INTO f1.meetings
		(meeting_key, year, name, official_name, circuit_id, country_name, location, date_start)
		SELECT 1, 2023, 'Left Behind', 'Left Behind', id, 'Bahrain', 'Sakhir', now()
		FROM f1.circuits WHERE short_name = 'Sakhir'`); err != nil {
		t.Fatalf("writing a row for the next acquisition to clear: %v", err)
	}

	// The second acquisition is what a following test sees.
	conn, _ = tests.RequireDB(t)

	for _, table := range []string{"meetings", "sessions", "weather_samples", "session_results"} {
		if got := countRows(t, conn, table); got != 0 {
			t.Errorf("f1.%s has %d rows on a clean slate, want 0", table, got)
		}
	}
	if got := countRows(t, conn, "circuits"); got != seededCircuits {
		t.Errorf("circuits = %d, want the %d seeded ones — reference data survives a clean slate", got, seededCircuits)
	}
	if got := countRows(t, conn, "drivers"); got != seededDrivers {
		t.Errorf("drivers = %d, want the %d seeded ones", got, seededDrivers)
	}
}
