package tests_test

import (
	"context"
	"encoding/json"
	"errors"
	"slices"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jmoiron/sqlx"

	"github.com/jeffreygaggino/we-are-rain-checking/backend/models"
	"github.com/jeffreygaggino/we-are-rain-checking/backend/services"
	"github.com/jeffreygaggino/we-are-rain-checking/backend/tests"
)

// The Racing Number that follows the championship rather than a person. Number 1 resolves to Max
// VERSTAPPEN for 2023–2025 and Lando NORRIS for 2026 upstream, which is ADR-0003's own
// example — staged here so a test can prove the two seasons do not merge.
const championNumber = 1

// A Racing Number the entry list of a Race does not carry. Nothing seeded holds it; the point is
// that ingest cannot invent a Driver for it.
const unlistedNumber = 77

// An entrant this repo has not seeded. Paul ARON really does carry number 1 in a 2023 Practice, the
// same number Max VERSTAPPEN carries in every Race of that season — which is why stage 3 asks about
// Races only, and why an unseeded name is a failure rather than an insert. See
// plans/04-session-results.md.
const unseededDriver = "Paul ARON"

// gridEntry is one Driver's classification as the fixture stages it: the upstream row and the entry
// list row that resolves its number, kept together so the two cannot drift apart by accident.
type gridEntry struct {
	number   int
	fullName string
	position *int
	points   float64
	laps     int
	dnf      bool
	dns      bool
	dsq      bool
}

// The staged classification. It carries a finisher, a Retirement with no position, a Driver who
// never started, and a **classified** disqualification — a row flagged dsq that still holds a
// position, which is 31 real rows upstream and the reason the flags are stored separately from
// the position rather than derived from it.
func gridOf(year int) []gridEntry {
	return []gridEntry{
		{number: championNumber, fullName: championOf(year), position: position(1), points: 25, laps: 57},
		{number: 16, fullName: "Charles LECLERC", position: position(2), points: 18, laps: 57},
		{number: 44, fullName: "Lewis HAMILTON", position: position(3), points: 0, laps: 57, dsq: true},
		{number: 55, fullName: "Carlos SAINZ", laps: 38, dnf: true},
		{number: 23, fullName: "Alexander ALBON", laps: 0, dns: true},
	}
}

// championOf is the Driver holding number 1 in a season. The number moves once inside the seasons
// staged here, which is the whole point of it.
func championOf(year int) string {
	if year == services.FirstSeason {
		return "Max VERSTAPPEN"
	}
	return "Lando NORRIS"
}

// Every Race gets its classification, attributed to Drivers and carrying the Retirements. The
// Racing Number is stored beside the resolution rather than in place of it.
func TestIngestStoresARaceClassificationAttributedToDrivers(t *testing.T) {
	conn, _ := tests.RequireDB(t)
	stub := tests.NewOpenF1Stub(t)

	stageHealthySeasons(stub)
	stageHealthyResults(stub)

	seasons := coveredSeasons()
	summary, err := newIngest(t, conn, stub).Run(context.Background())
	if err != nil {
		t.Fatalf("Run: %v", err)
	}

	// One Race a season, the staged grid apiece.
	want := len(seasons) * len(gridOf(services.FirstSeason))
	if got := countRows(t, conn, "session_results"); got != want {
		t.Errorf("session_results = %d, want %d", got, want)
	}
	if summary.Results != want {
		t.Errorf("summary.Results = %d, want %d", summary.Results, want)
	}
	if summary.ResultRaces != len(seasons) {
		t.Errorf("summary.ResultRaces = %d, want %d", summary.ResultRaces, len(seasons))
	}

	year := services.FirstSeason
	race := raceOf(year)
	stored := resultsFor(t, conn, race.SessionKey)
	if len(stored) != len(gridOf(year)) {
		t.Fatalf("race %d holds %d results, want %d", race.SessionKey, len(stored), len(gridOf(year)))
	}

	byDriver := make(map[uuid.UUID]models.SessionResult, len(stored))
	for _, result := range stored {
		byDriver[result.DriverID] = result
	}

	for _, entry := range gridOf(year) {
		got, ok := byDriver[driverID(t, conn, entry.fullName)]
		if !ok {
			t.Errorf("%s has no row in race %d — the classification is attributed to Drivers",
				entry.fullName, race.SessionKey)
			continue
		}
		if got.RacingNumber != entry.number {
			t.Errorf("%s: racing_number = %d, want %d", entry.fullName, got.RacingNumber, entry.number)
		}
		if !samePosition(got.Position, entry.position) {
			t.Errorf("%s: position = %v, want %v — a Retirement has none, and a sentinel would sort",
				entry.fullName, describe(got.Position), describe(entry.position))
		}
		if got.Points != entry.points {
			t.Errorf("%s: points = %v, want %v", entry.fullName, got.Points, entry.points)
		}
		if got.NumberOfLaps == nil || *got.NumberOfLaps != entry.laps {
			t.Errorf("%s: number_of_laps = %v, want %d", entry.fullName, got.NumberOfLaps, entry.laps)
		}
		if got.DNF != entry.dnf || got.DNS != entry.dns || got.DSQ != entry.dsq {
			t.Errorf("%s: dnf/dns/dsq = %v/%v/%v, want %v/%v/%v — the Retirement flags are the "+
				"answer #10 counts, and none of them is derivable from the position",
				entry.fullName, got.DNF, got.DNS, got.DSQ, entry.dnf, entry.dns, entry.dsq)
		}
	}
}

// ADR-0003, executable. The number is reassigned between seasons, so keying on it merges two people
// — the bug that once produced a ten-point wet-weather advantage that was not there.
func TestTheSameRacingNumberInTwoSeasonsResolvesToTwoDrivers(t *testing.T) {
	conn, _ := tests.RequireDB(t)
	stub := tests.NewOpenF1Stub(t)

	stageHealthySeasons(stub)
	stageHealthyResults(stub)

	seasons := coveredSeasons()
	if len(seasons) < 2 {
		t.Fatalf("a run covers %v — this test needs two seasons to reassign a number across", seasons)
	}
	first, last := seasons[0], seasons[len(seasons)-1]
	if championOf(first) == championOf(last) {
		t.Fatalf("the fixture gives number %d to %s in both %d and %d — nothing is being proved",
			championNumber, championOf(first), first, last)
	}

	if _, err := newIngest(t, conn, stub).Run(context.Background()); err != nil {
		t.Fatalf("Run: %v", err)
	}

	firstHolder := resultForNumber(t, conn, raceOf(first).SessionKey, championNumber)
	lastHolder := resultForNumber(t, conn, raceOf(last).SessionKey, championNumber)

	if firstHolder.DriverID == lastHolder.DriverID {
		t.Fatalf("number %d resolved to one Driver (%s) in %d and %d — the two seasons have been "+
			"merged into one person", championNumber, firstHolder.DriverID, first, last)
	}
	if want := driverID(t, conn, championOf(first)); firstHolder.DriverID != want {
		t.Errorf("number %d in %d resolved to %s, want %s (%s)",
			championNumber, first, firstHolder.DriverID, want, championOf(first))
	}
	if want := driverID(t, conn, championOf(last)); lastHolder.DriverID != want {
		t.Errorf("number %d in %d resolved to %s, want %s (%s)",
			championNumber, last, lastHolder.DriverID, want, championOf(last))
	}
}

// The other half of ADR-0003: a name we have not seeded stops the run. Inserting a Driver here is
// what splits one person's results across two ids, and it would do it quietly.
func TestAnUnseededDriverNameAbortsIngestAndInsertsNothing(t *testing.T) {
	conn, _ := tests.RequireDB(t)
	stub := tests.NewOpenF1Stub(t)

	stageHealthySeasons(stub)
	stageHealthyResults(stub)

	// The oldest season's Race, so nothing is stored before the failure.
	race := raceOf(services.FirstSeason)
	entrants := driverFixture(race, gridOf(services.FirstSeason))
	entrants[0].FullName = unseededDriver
	stub.SetSessionDrivers(race.SessionKey, entrants)

	_, err := newIngest(t, conn, stub).Run(context.Background())
	if !errors.Is(err, models.ErrDriverNotResolved) {
		t.Fatalf("Run: %v, want ErrDriverNotResolved", err)
	}
	if !strings.Contains(err.Error(), unseededDriver) {
		t.Errorf("Run: %v — the error should name %q, which is the only way to know what to seed",
			err, unseededDriver)
	}

	if got := countRows(t, conn, "session_results"); got != 0 {
		t.Errorf("session_results = %d, want 0 — an unresolved Driver inserts nothing", got)
	}
	if got := countRows(t, conn, "drivers"); got != seededDrivers {
		t.Errorf("drivers = %d, want %d — ingest resolves against the seed and never adds to it",
			got, seededDrivers)
	}
}

// The entry list is what resolves a number, so a number missing from it has no Driver — and a row
// stored against a guess is the same silent merge as an invented Driver.
func TestARacingNumberTheEntryListDoesNotCarryAbortsIngest(t *testing.T) {
	conn, _ := tests.RequireDB(t)
	stub := tests.NewOpenF1Stub(t)

	stageHealthySeasons(stub)
	stageHealthyResults(stub)

	race := raceOf(services.FirstSeason)
	grid := gridOf(services.FirstSeason)
	results := resultFixture(race, grid)
	results[0].DriverNumber = unlistedNumber
	stub.SetSessionResults(race.SessionKey, results)

	_, err := newIngest(t, conn, stub).Run(context.Background())
	if !errors.Is(err, models.ErrDriverNotResolved) {
		t.Fatalf("Run: %v, want ErrDriverNotResolved", err)
	}
	if got := countRows(t, conn, "session_results"); got != 0 {
		t.Errorf("session_results = %d, want 0", got)
	}
}

// One number, two names, inside one Session. It does not happen upstream today — zero across all
// 9,949 entry rows, measured 2026-09-03 — and if it ever does, last-write-wins would attribute a
// Race to whichever name the upstream happened to send second.
func TestARacingNumberCarriedByTwoDriversInOneRaceAbortsIngest(t *testing.T) {
	conn, _ := tests.RequireDB(t)
	stub := tests.NewOpenF1Stub(t)

	stageHealthySeasons(stub)
	stageHealthyResults(stub)

	race := raceOf(services.FirstSeason)
	entrants := driverFixture(race, gridOf(services.FirstSeason))
	ambiguous := entrants[0]
	ambiguous.FullName = "Lando NORRIS"
	stub.SetSessionDrivers(race.SessionKey, append(entrants, ambiguous))

	_, err := newIngest(t, conn, stub).Run(context.Background())
	if !errors.Is(err, models.ErrDriverNotResolved) {
		t.Fatalf("Run: %v, want ErrDriverNotResolved", err)
	}
	if got := countRows(t, conn, "session_results"); got != 0 {
		t.Errorf("session_results = %d, want 0", got)
	}
}

// A Retirement flag the upstream omits, read as false, moves #10's headline number with nothing to
// show for it. Same rule as `rainfall` on a Weather Sample: the field a claim rests on is strict.
func TestAMissingRetirementFlagFailsTheIngest(t *testing.T) {
	conn, _ := tests.RequireDB(t)
	stub := tests.NewOpenF1Stub(t)

	stageHealthySeasons(stub)
	stageHealthyResults(stub)

	race := raceOf(services.FirstSeason)
	results := resultFixture(race, gridOf(services.FirstSeason))
	results[3].DNF = nil
	stub.SetSessionResults(race.SessionKey, results)

	_, err := newIngest(t, conn, stub).Run(context.Background())
	if err == nil {
		t.Fatal("Run succeeded with a result row carrying no dnf — a missing flag is not a finish")
	}
	if !strings.Contains(err.Error(), "dnf") {
		t.Errorf("Run: %v — the error should name the field that was missing", err)
	}
	if got := countRows(t, conn, "session_results"); got != 0 {
		t.Errorf("session_results = %d, want 0", got)
	}
}

// The criterion this ticket shares with every other ingest stage. Idempotence is two rules: a Race
// with results is skipped without a request, and the write underneath is an upsert.
func TestReIngestingProducesNoDuplicateResults(t *testing.T) {
	conn, _ := tests.RequireDB(t)
	stub := tests.NewOpenF1Stub(t)

	stageHealthySeasons(stub)
	stageHealthyResults(stub)

	ingest := newIngest(t, conn, stub)
	if _, err := ingest.Run(context.Background()); err != nil {
		t.Fatalf("first Run: %v", err)
	}
	afterFirst := countRows(t, conn, "session_results")

	stub.ResetRequests()
	summary, err := ingest.Run(context.Background())
	if err != nil {
		t.Fatalf("second Run: %v", err)
	}

	if got := countRows(t, conn, "session_results"); got != afterFirst {
		t.Errorf("session_results = %d after a second run, want %d", got, afterFirst)
	}
	if asked := resultRequests(stub); len(asked) != 0 {
		t.Errorf("second run asked for the results of races %v, want none — a Race with results is skipped", asked)
	}
	if summary.Results != 0 {
		t.Errorf("summary.Results = %d, want 0 on a run with nothing to fetch", summary.Results)
	}
	if summary.ResultRacesSkipped != len(coveredSeasons()) {
		t.Errorf("summary.ResultRacesSkipped = %d, want %d — every stored Race",
			summary.ResultRacesSkipped, len(coveredSeasons()))
	}
}

// Practice and qualifying carry a classification too, and asking for it is what would abort a run:
// 42 of the 71 names upstream appear only outside a Race, and none of them is seeded. See
// plans/04-session-results.md.
func TestResultsAreFetchedForRacesOnly(t *testing.T) {
	conn, _ := tests.RequireDB(t)
	stub := tests.NewOpenF1Stub(t)

	stageHealthySeasons(stub)
	stageHealthyResults(stub)

	// The Practice of the oldest season, staged with an entry list ingest must never resolve.
	practice := sessionOf(services.FirstSeason, 0)
	grid := gridOf(services.FirstSeason)
	entrants := driverFixture(practice, grid)
	entrants[0].FullName = unseededDriver
	stub.SetSessionResults(practice.SessionKey, resultFixture(practice, grid))
	stub.SetSessionDrivers(practice.SessionKey, entrants)

	if _, err := newIngest(t, conn, stub).Run(context.Background()); err != nil {
		t.Fatalf("Run: %v — a Practice is not something this service reads and must not be asked about", err)
	}

	if slices.Contains(resultRequests(stub), practice.SessionKey) {
		t.Errorf("the upstream was asked for session %d's results — it is a %s, not a Race",
			practice.SessionKey, practice.SessionName)
	}
	if got := len(resultsFor(t, conn, practice.SessionKey)); got != 0 {
		t.Errorf("session %d holds %d results, want 0", practice.SessionKey, got)
	}
}

// A Race the upstream holds no results for is an answer, not a fault — a cancelled Race is the live
// example, probed 2026-09-03 as `session_result?session_key=9086` answering 404 "No results found.".
// It also costs one request rather than two: with no results there is no number to resolve.
func TestARaceTheUpstreamHasNoResultsForIsNotAFailure(t *testing.T) {
	conn, _ := tests.RequireDB(t)
	stub := tests.NewOpenF1Stub(t)

	stageHealthySeasons(stub)
	stageHealthyResults(stub)

	silent := raceOf(services.FirstSeason)
	stub.SetSessionResults(silent.SessionKey, nil)

	ingest := newIngest(t, conn, stub)
	summary, err := ingest.Run(context.Background())
	if err != nil {
		t.Fatalf("Run: %v — a Race with no results upstream is an empty answer, not a failure", err)
	}

	if got := len(resultsFor(t, conn, silent.SessionKey)); got != 0 {
		t.Errorf("race %d holds %d results, want 0", silent.SessionKey, got)
	}
	if summary.ResultRaces != len(coveredSeasons())-1 {
		t.Errorf("summary.ResultRaces = %d, want %d — a Race that stored nothing is not an ingested one",
			summary.ResultRaces, len(coveredSeasons())-1)
	}
	if slices.Contains(driverRequests(stub), silent.SessionKey) {
		t.Errorf("the upstream was asked for race %d's entry list — with no results there is no "+
			"number to resolve", silent.SessionKey)
	}
	// Every other Race still landed, so the empty one did not abandon the stage.
	want := (len(coveredSeasons()) - 1) * len(gridOf(services.FirstSeason))
	if summary.Results != want {
		t.Errorf("summary.Results = %d, want %d", summary.Results, want)
	}

	stub.ResetRequests()
	if _, err := ingest.Run(context.Background()); err != nil {
		t.Fatalf("second Run: %v", err)
	}
	if asked := resultRequests(stub); len(asked) != 1 || asked[0] != silent.SessionKey {
		t.Errorf("second run asked for the results of races %v, want just [%d] — a Race with no rows "+
			"has nothing recording that it was asked, so it is asked again", asked, silent.SessionKey)
	}
}

// The classification that crosses the line is provisional. A run arriving before the stewards are
// done would store that grid and then skip the Race forever, because the skip asks only whether rows
// exist — so a penalty applied afterwards would never land. See services.ResultsSettleWindow.
func TestARaceThatEndedInsideTheSettleWindowIsNotFetchedYet(t *testing.T) {
	conn, _ := tests.RequireDB(t)
	stub := tests.NewOpenF1Stub(t)

	stageHealthySeasons(stub)
	stageHealthyResults(stub)

	current := time.Now().UTC().Year()
	meetings, sessions := seasonFixture(current)
	// Ended an hour ago: past, so the weather stage takes it, and inside the window, so this one
	// does not.
	justFinished := justFinishedRaceFixture(current, meetings[0].MeetingKey)
	stub.SetSeason(current, meetings, append(sessions, justFinished))
	grid := gridOf(current)
	stub.SetSessionResults(justFinished.SessionKey, resultFixture(justFinished, grid))
	stub.SetSessionDrivers(justFinished.SessionKey, driverFixture(justFinished, grid))

	if _, err := newIngest(t, conn, stub).Run(context.Background()); err != nil {
		t.Fatalf("Run: %v", err)
	}

	if got := len(resultsFor(t, conn, justFinished.SessionKey)); got != 0 {
		t.Errorf("race %d holds %d results, want 0 — its classification is still provisional",
			justFinished.SessionKey, got)
	}
	if slices.Contains(resultRequests(stub), justFinished.SessionKey) {
		t.Errorf("the upstream was asked for race %d's results %v after the flag, inside the %v "+
			"settle window", justFinished.SessionKey, time.Hour, services.ResultsSettleWindow)
	}
}

// The entry list is fetched whole, and it can name people the classification never mentions. Only
// the numbers a result row carries have to resolve: aborting on the rest would stop a run over a
// Driver it has nothing to store for.
func TestAnUnseededEntrantNobodyClassifiesDoesNotAbortIngest(t *testing.T) {
	conn, _ := tests.RequireDB(t)
	stub := tests.NewOpenF1Stub(t)

	stageHealthySeasons(stub)
	stageHealthyResults(stub)

	race := raceOf(services.FirstSeason)
	grid := gridOf(services.FirstSeason)
	entrants := driverFixture(race, grid)
	withdrawn := entrants[0]
	withdrawn.DriverNumber = unlistedNumber
	withdrawn.FullName = unseededDriver
	stub.SetSessionDrivers(race.SessionKey, append(entrants, withdrawn))

	if _, err := newIngest(t, conn, stub).Run(context.Background()); err != nil {
		t.Fatalf("Run: %v — %q is on the entry list but in no result row, so nothing needs resolving",
			err, unseededDriver)
	}
	if got := len(resultsFor(t, conn, race.SessionKey)); got != len(grid) {
		t.Errorf("race %d holds %d results, want %d — the classified Drivers still land",
			race.SessionKey, got, len(grid))
	}
}

// Two numbers resolving to one Driver would write two rows on one primary key: the second overwrites
// the first inside the transaction, and a Driver leaves the grid with the run reporting success.
func TestOneDriverClassifiedUnderTwoRacingNumbersAbortsIngest(t *testing.T) {
	conn, _ := tests.RequireDB(t)
	stub := tests.NewOpenF1Stub(t)

	stageHealthySeasons(stub)
	stageHealthyResults(stub)

	race := raceOf(services.FirstSeason)
	grid := gridOf(services.FirstSeason)
	entrants := driverFixture(race, grid)
	// The same person on the entry list twice, under a second number the classification also carries.
	twice := entrants[1]
	twice.DriverNumber = unlistedNumber
	stub.SetSessionDrivers(race.SessionKey, append(entrants, twice))

	results := resultFixture(race, grid)
	results[0].DriverNumber = unlistedNumber
	stub.SetSessionResults(race.SessionKey, results)

	_, err := newIngest(t, conn, stub).Run(context.Background())
	if !errors.Is(err, models.ErrDriverNotResolved) {
		t.Fatalf("Run: %v, want ErrDriverNotResolved", err)
	}
	if got := countRows(t, conn, "session_results"); got != 0 {
		t.Errorf("session_results = %d, want 0", got)
	}
}

// A Race still to run has no classification, and asking for one would store a partial grid that the
// "has rows" skip would then strand.
func TestResultsAreNotFetchedForARaceThatHasNotRunYet(t *testing.T) {
	conn, _ := tests.RequireDB(t)
	stub := tests.NewOpenF1Stub(t)

	stageHealthySeasons(stub)
	stageHealthyResults(stub)

	current := time.Now().UTC().Year()
	meetings, sessions := seasonFixture(current)
	upcoming := upcomingSessionFixture(current, meetings[0].MeetingKey)
	stub.SetSeason(current, meetings, append(sessions, upcoming))
	// Staged, so the assertion is about ingest declining to ask rather than the stub having nothing.
	grid := gridOf(current)
	stub.SetSessionResults(upcoming.SessionKey, resultFixture(upcoming, grid))
	stub.SetSessionDrivers(upcoming.SessionKey, driverFixture(upcoming, grid))

	if _, err := newIngest(t, conn, stub).Run(context.Background()); err != nil {
		t.Fatalf("Run: %v", err)
	}

	if got := len(resultsFor(t, conn, upcoming.SessionKey)); got != 0 {
		t.Errorf("race %d holds %d results, want 0 — it has not run yet", upcoming.SessionKey, got)
	}
	if slices.Contains(resultRequests(stub), upcoming.SessionKey) {
		t.Errorf("the upstream was asked for race %d's results before the Race had run", upcoming.SessionKey)
	}
}

// A Race is stored whole or not at all, and the next run resumes at it — the rule stage 1 keeps for
// a season and stage 2 for a Session's weather. Without it a half-written grid would have rows, and
// the skip would strand it.
func TestAFailedResultsFetchLeavesTheRaceUnstoredAndTheNextRunResumesAtIt(t *testing.T) {
	conn, _ := tests.RequireDB(t)
	stub := tests.NewOpenF1Stub(t)

	stageHealthySeasons(stub)
	stageHealthyResults(stub)

	seasons := coveredSeasons()
	stored, failed := raceOf(seasons[0]), raceOf(seasons[1])

	ingest := newIngest(t, conn, stub)
	stub.FailNextForSession("/session_result", failed.SessionKey, 500, `{"detail":"upstream fell over"}`)
	if _, err := ingest.Run(context.Background()); err == nil {
		t.Fatal("Run succeeded despite the upstream failing on a Race's results")
	}

	if got := len(resultsFor(t, conn, stored.SessionKey)); got != len(gridOf(seasons[0])) {
		t.Errorf("race %d holds %d results, want %d — the Races before the failure are committed",
			stored.SessionKey, got, len(gridOf(seasons[0])))
	}
	if got := len(resultsFor(t, conn, failed.SessionKey)); got != 0 {
		t.Errorf("race %d holds %d results, want 0 — a Race is stored whole or not at all",
			failed.SessionKey, got)
	}

	stub.ResetRequests()
	if _, err := ingest.Run(context.Background()); err != nil {
		t.Fatalf("second Run: %v", err)
	}

	if got := len(resultsFor(t, conn, failed.SessionKey)); got != len(gridOf(seasons[1])) {
		t.Errorf("race %d holds %d results after resuming, want %d",
			failed.SessionKey, got, len(gridOf(seasons[1])))
	}
	asked := resultRequests(stub)
	if len(asked) == 0 || asked[0] != failed.SessionKey {
		t.Errorf("second run asked for the results of races %v, want it to resume at %d",
			asked, failed.SessionKey)
	}
	if slices.Contains(asked, stored.SessionKey) {
		t.Errorf("second run re-asked for race %d's results, want it skipped — it has rows",
			stored.SessionKey)
	}
}

// ── fixtures ──────────────────────────────────────────────────────────────────

// seededDrivers is what 000002 inserts. Named here because the assertion that matters is that ingest
// never changes it.
const seededDrivers = 29

// stageHealthyResults gives every Race of every covered season its classification and its entry
// list, which is what the upstream looks like on a healthy day. A test about a Race with no results
// unsets that one, so the stub 404s it the way the live API does.
func stageHealthyResults(stub *tests.OpenF1Stub) {
	for _, year := range coveredSeasons() {
		race := raceOf(year)
		grid := gridOf(year)
		stub.SetSessionResults(race.SessionKey, resultFixture(race, grid))
		stub.SetSessionDrivers(race.SessionKey, driverFixture(race, grid))
	}
}

// resultFixture is one Race's classification in the upstream's wire shapes.
//
// gap_to_leader is staged as the union the live API sends — a number for the leader, a float for the
// gap, a string for a lapped car, absent for a Retirement. Nothing stores it; the fixture carries it
// so the client's decode is shown to be unbothered rather than merely untested.
func resultFixture(session tests.StubSession, grid []gridEntry) []tests.StubSessionResult {
	gaps := []json.RawMessage{
		json.RawMessage(`0`),
		json.RawMessage(`19.477`),
		json.RawMessage(`"+1 LAP"`),
		json.RawMessage(`null`),
		nil,
	}

	results := make([]tests.StubSessionResult, 0, len(grid))
	for i, entry := range grid {
		points, laps, dnf, dns, dsq := entry.points, entry.laps, entry.dnf, entry.dns, entry.dsq
		results = append(results, tests.StubSessionResult{
			Position:     entry.position,
			DriverNumber: entry.number,
			NumberOfLaps: &laps,
			Points:       &points,
			DNF:          &dnf,
			DNS:          &dns,
			DSQ:          &dsq,
			GapToLeader:  gaps[i%len(gaps)],
			MeetingKey:   session.MeetingKey,
			SessionKey:   session.SessionKey,
		})
	}
	return results
}

// driverFixture is the entry list that resolves the Race's Racing Numbers — `GET /drivers` for one
// Session, which is the only thing that says who carried a number that weekend.
func driverFixture(session tests.StubSession, grid []gridEntry) []tests.StubDriver {
	drivers := make([]tests.StubDriver, 0, len(grid))
	for _, entry := range grid {
		drivers = append(drivers, tests.StubDriver{
			MeetingKey:    session.MeetingKey,
			SessionKey:    session.SessionKey,
			DriverNumber:  entry.number,
			BroadcastName: strings.ToUpper(entry.fullName),
			FullName:      entry.fullName,
			NameAcronym:   acronymOf(entry.fullName),
			TeamName:      "Some Team",
		})
	}
	return drivers
}

// acronymOf is the upstream's three-letter form. Ingest reads none of it — it is here because the
// live row carries it, and a fixture missing fields the real response has hides decode surprises.
func acronymOf(fullName string) string {
	surname := fullName[strings.LastIndex(fullName, " ")+1:]
	return string([]rune(surname)[:3])
}

// justFinishedRaceFixture is a Race that ended an hour ago — over, and still inside the window its
// classification settles in.
func justFinishedRaceFixture(year, meetingKey int) tests.StubSession {
	end := time.Now().UTC().Add(-time.Hour)
	return tests.StubSession{
		SessionKey:       year*100 + 4,
		MeetingKey:       meetingKey,
		SessionType:      "Race",
		SessionName:      models.SessionNameRace,
		Location:         "Sakhir",
		CountryName:      "Bahrain",
		CircuitKey:       sakhirCircuitKey,
		CircuitShortName: "Sakhir",
		DateStart:        end.Add(-2 * time.Hour).Format(upstreamTimeLayout),
		DateEnd:          end.Format(upstreamTimeLayout),
		Year:             year,
	}
}

// raceOf and sessionOf name the seasonFixture Sessions by what they are, so a test does not index
// into the fixture and quietly get the Practice.
func raceOf(year int) tests.StubSession { return sessionOf(year, 1) }

func sessionOf(year, index int) tests.StubSession {
	_, sessions := seasonFixture(year)
	return sessions[index]
}

func position(p int) *int { return &p }

func samePosition(got, want *int) bool {
	if got == nil || want == nil {
		return got == nil && want == nil
	}
	return *got == *want
}

func describe(p *int) string {
	if p == nil {
		return "none"
	}
	return strconv.Itoa(*p)
}

// resultRequests and driverRequests are the Races the upstream was asked about, in order. Separate,
// because the two calls stage 3 makes are asserted on separately: one proves a Race was skipped, the
// other that a Race with no results never had its entry list fetched.
func resultRequests(stub *tests.OpenF1Stub) []int { return requestsTo(stub, "/session_result") }

func driverRequests(stub *tests.OpenF1Stub) []int { return requestsTo(stub, "/drivers") }

func requestsTo(stub *tests.OpenF1Stub, path string) []int {
	var asked []int
	for _, req := range stub.Requests() {
		if req.Path == path {
			asked = append(asked, req.SessionKey)
		}
	}
	return asked
}

func resultsFor(t *testing.T, conn *sqlx.DB, sessionKey int) []models.SessionResult {
	t.Helper()

	const q = `SELECT session_key, driver_id, racing_number, position, points, number_of_laps,
	                  dnf, dns, dsq, created_at, updated_at
	           FROM f1.session_results WHERE session_key = $1 ORDER BY racing_number`

	var results []models.SessionResult
	if err := conn.Select(&results, q, sessionKey); err != nil {
		t.Fatalf("fetching results for session %d: %v", sessionKey, err)
	}
	return results
}

func resultForNumber(t *testing.T, conn *sqlx.DB, sessionKey, racingNumber int) models.SessionResult {
	t.Helper()

	for _, result := range resultsFor(t, conn, sessionKey) {
		if result.RacingNumber == racingNumber {
			return result
		}
	}
	t.Fatalf("race %d holds no result for racing number %d", sessionKey, racingNumber)
	return models.SessionResult{}
}

func driverID(t *testing.T, conn *sqlx.DB, fullName string) uuid.UUID {
	t.Helper()

	var id uuid.UUID
	if err := conn.Get(&id, `SELECT id FROM f1.drivers WHERE full_name = $1`, fullName); err != nil {
		t.Fatalf("looking up the seeded driver %q: %v", fullName, err)
	}
	return id
}
