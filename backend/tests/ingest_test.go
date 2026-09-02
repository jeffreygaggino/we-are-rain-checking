package tests_test

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/jmoiron/sqlx"

	"github.com/jeffreygaggino/we-are-rain-checking/backend/app"
	"github.com/jeffreygaggino/we-are-rain-checking/backend/client"
	"github.com/jeffreygaggino/we-are-rain-checking/backend/models"
	"github.com/jeffreygaggino/we-are-rain-checking/backend/services"
	"github.com/jeffreygaggino/we-are-rain-checking/backend/tests"
)

// The wire format OpenF1 actually emits: an explicit numeric offset, not RFC3339's "Z". Formatting
// fixtures with time.RFC3339 would produce a string the live API never sends.
const upstreamTimeLayout = "2006-01-02T15:04:05-07:00"

// Seeded in 000002. Ingest joins a Session to its Circuit on this key.
const (
	sakhirCircuitKey = 63
	monzaCircuitKey  = 39
	unseededCircuit  = 9999
)

// The seasons any run covers: OpenF1's first year through the current one, from the clock. See
// plans/01-backend-v1.md, "Seasons are a range, not a config knob".
func coveredSeasons() []int {
	var years []int
	for year := services.FirstSeason; year <= time.Now().UTC().Year(); year++ {
		years = append(years, year)
	}
	return years
}

func TestIngestPopulatesMeetingsAndSessionsAcrossEverySeasonUpstreamCovers(t *testing.T) {
	conn, _ := tests.RequireDB(t)
	stub := tests.NewOpenF1Stub(t)

	seasons := coveredSeasons()
	for _, year := range seasons {
		setSeason(stub, year)
	}

	summary, err := newIngest(t, conn, stub).Run(context.Background())
	if err != nil {
		t.Fatalf("Run: %v", err)
	}

	if got, want := countRows(t, conn, "meetings"), len(seasons); got != want {
		t.Errorf("meetings = %d, want %d — one per covered season", got, want)
	}
	if got, want := countRows(t, conn, "sessions"), len(seasons)*2; got != want {
		t.Errorf("sessions = %d, want %d", got, want)
	}
	if summary.Meetings != len(seasons) || summary.Sessions != len(seasons)*2 {
		t.Errorf("summary = %+v, want %d meetings and %d sessions", summary, len(seasons), len(seasons)*2)
	}

	// The Meeting is stored in this repo's vocabulary, not the upstream's field names.
	var meeting models.Meeting
	const q = `SELECT meeting_key, year, name, official_name, circuit_id, country_name, location, date_start,
	                  created_at, updated_at
	           FROM f1.meetings WHERE meeting_key = $1`
	if err := conn.Get(&meeting, q, meetingKey(services.FirstSeason)); err != nil {
		t.Fatalf("fetching the ingested meeting: %v", err)
	}
	if meeting.Name != "Bahrain Grand Prix" {
		t.Errorf("name = %q, want %q — meeting_name maps to name", meeting.Name, "Bahrain Grand Prix")
	}
	if want := time.Date(services.FirstSeason, time.March, 1, 12, 0, 0, 0, time.UTC); !meeting.DateStart.Equal(want) {
		t.Errorf("date_start = %s, want %s", meeting.DateStart, want)
	}
}

// The seeded Circuit is the join the whole corpus hangs off: a Session with the wrong one attributes
// its weather to the wrong place.
func TestIngestedSessionsReferenceTheirSeededCircuit(t *testing.T) {
	conn, _ := tests.RequireDB(t)
	stub := tests.NewOpenF1Stub(t)

	year := services.FirstSeason
	meetings, sessions := seasonFixture(year)
	sessions[1].CircuitKey = monzaCircuitKey
	sessions[1].CircuitShortName = "Monza"
	stub.SetSeason(year, meetings, sessions)

	if _, err := newIngest(t, conn, stub).Run(context.Background()); err != nil {
		t.Fatalf("Run: %v", err)
	}

	for _, tc := range []struct {
		sessionKey int
		wantShort  string
	}{
		{sessions[0].SessionKey, "Sakhir"},
		{sessions[1].SessionKey, "Monza"},
	} {
		var shortName string
		const q = `SELECT c.short_name FROM f1.sessions s JOIN f1.circuits c ON c.id = s.circuit_id
		           WHERE s.session_key = $1`
		if err := conn.Get(&shortName, q, tc.sessionKey); err != nil {
			t.Fatalf("resolving circuit for session %d: %v", tc.sessionKey, err)
		}
		if shortName != tc.wantShort {
			t.Errorf("session %d sits at %q, want %q", tc.sessionKey, shortName, tc.wantShort)
		}
	}
}

// A cancelled Session is stored, not dropped — dropping it would make the next re-ingest look like
// it lost rows. The flag is what #9 and #10 filter on, and a run against the live upstream finds 15
// of them across the corpus, three of which are Races. Both values are asserted because a tag that
// always wrote the same one would pass either assertion alone.
func TestACancelledSessionIsStoredWithItsFlagIntact(t *testing.T) {
	conn, _ := tests.RequireDB(t)
	stub := tests.NewOpenF1Stub(t)

	year := services.FirstSeason
	meetings, sessions := seasonFixture(year)
	sessions[1].IsCancelled = true
	stub.SetSeason(year, meetings, sessions)

	if _, err := newIngest(t, conn, stub).Run(context.Background()); err != nil {
		t.Fatalf("Run: %v", err)
	}

	for _, tc := range []struct {
		sessionKey int
		want       bool
	}{
		{sessions[0].SessionKey, false},
		{sessions[1].SessionKey, true},
	} {
		var cancelled bool
		const q = `SELECT is_cancelled FROM f1.sessions WHERE session_key = $1`
		if err := conn.Get(&cancelled, q, tc.sessionKey); err != nil {
			t.Fatalf("fetching session %d: %v", tc.sessionKey, err)
		}
		if cancelled != tc.want {
			t.Errorf("session %d is_cancelled = %v, want %v", tc.sessionKey, cancelled, tc.want)
		}
	}
}

// Idempotence is ON CONFLICT behaviour, which is why this runs against real Postgres.
func TestReIngestingTheSameSeasonsProducesNoDuplicateRows(t *testing.T) {
	conn, _ := tests.RequireDB(t)
	stub := tests.NewOpenF1Stub(t)

	seasons := coveredSeasons()
	for _, year := range seasons {
		setSeason(stub, year)
	}

	ingest := newIngest(t, conn, stub)
	if _, err := ingest.Run(context.Background()); err != nil {
		t.Fatalf("first Run: %v", err)
	}
	meetingsAfterFirst := countRows(t, conn, "meetings")
	sessionsAfterFirst := countRows(t, conn, "sessions")

	if _, err := ingest.Run(context.Background()); err != nil {
		t.Fatalf("second Run: %v", err)
	}

	if got := countRows(t, conn, "meetings"); got != meetingsAfterFirst {
		t.Errorf("meetings = %d after a second run, want %d", got, meetingsAfterFirst)
	}
	if got := countRows(t, conn, "sessions"); got != sessionsAfterFirst {
		t.Errorf("sessions = %d after a second run, want %d", got, sessionsAfterFirst)
	}
}

// A row the upstream corrected is updated in place. ADR-0002: re-ingest updates rather than deleting,
// which is the other half of why there is no deleted_at.
func TestReIngestingUpdatesAChangedRowInPlace(t *testing.T) {
	conn, _ := tests.RequireDB(t)
	stub := tests.NewOpenF1Stub(t)

	// The season in progress, because a completed one is skipped on the second run by design — an
	// upstream correction to a finished season is not something this service goes looking for.
	year := time.Now().UTC().Year()
	meetings, sessions := seasonFixture(year)
	stub.SetSeason(year, meetings, sessions)

	ingest := newIngest(t, conn, stub)
	if _, err := ingest.Run(context.Background()); err != nil {
		t.Fatalf("first Run: %v", err)
	}

	meetings[0].MeetingName = "Bahrain Grand Prix (renamed upstream)"
	stub.SetSeason(year, meetings, sessions)
	if _, err := ingest.Run(context.Background()); err != nil {
		t.Fatalf("second Run: %v", err)
	}

	var name string
	if err := conn.Get(&name, `SELECT name FROM f1.meetings WHERE meeting_key = $1`, meetingKey(year)); err != nil {
		t.Fatalf("fetching the meeting: %v", err)
	}
	if name != "Bahrain Grand Prix (renamed upstream)" {
		t.Errorf("name = %q, want the upstream's corrected value", name)
	}
	if got := countRows(t, conn, "meetings"); got != 1 {
		t.Errorf("meetings = %d, want 1 — an update, not a second row", got)
	}
}

// The criterion this ticket exists for: interrupt it, re-run, and it picks up where it stopped
// instead of re-fetching four seasons. Resumption is derived from the stored rows, not checkpointed.
func TestIngestResumesAtTheSeasonThatFailedRatherThanTheFirst(t *testing.T) {
	conn, _ := tests.RequireDB(t)
	stub := tests.NewOpenF1Stub(t)

	seasons := coveredSeasons()
	if len(seasons) < 3 {
		t.Fatalf("this test needs at least three covered seasons, got %d", len(seasons))
	}
	for _, year := range seasons {
		setSeason(stub, year)
	}
	interrupted := seasons[len(seasons)-2]

	ingest := newIngest(t, conn, stub)

	// Die on the second-to-last season's sessions call, after earlier seasons have committed.
	stub.FailNext("/sessions", interrupted, 500, `{"detail":"upstream fell over"}`)
	if _, err := ingest.Run(context.Background()); err == nil {
		t.Fatal("Run succeeded despite the upstream failing")
	}

	// Nothing of the failed season landed: both upstream calls precede the season's transaction.
	if got := countMeetingsForYear(t, conn, interrupted); got != 0 {
		t.Errorf("%d has %d meetings after the failed run, want 0 — a season is written whole or not at all",
			interrupted, got)
	}

	stub.ResetRequests()
	if _, err := ingest.Run(context.Background()); err != nil {
		t.Fatalf("second Run: %v", err)
	}

	var asked []int
	for _, req := range stub.Requests() {
		if req.Path == "/meetings" {
			asked = append(asked, req.Year)
		}
	}
	want := seasons[len(seasons)-2:]
	if fmt.Sprint(asked) != fmt.Sprint(want) {
		t.Errorf("second run asked for seasons %v, want %v — completed seasons are skipped without a request",
			asked, want)
	}

	for _, year := range seasons {
		if got := countMeetingsForYear(t, conn, year); got != 1 {
			t.Errorf("%d has %d meetings after resuming, want 1", year, got)
		}
	}
}

// The current season is never skipped: Sessions are still being added to it, so "already has rows"
// does not mean "complete".
func TestIngestAlwaysRefetchesTheCurrentSeason(t *testing.T) {
	conn, _ := tests.RequireDB(t)
	stub := tests.NewOpenF1Stub(t)

	seasons := coveredSeasons()
	current := seasons[len(seasons)-1]
	for _, year := range seasons {
		setSeason(stub, year)
	}

	ingest := newIngest(t, conn, stub)
	if _, err := ingest.Run(context.Background()); err != nil {
		t.Fatalf("first Run: %v", err)
	}

	// A Session added to the season in progress, which a skipped season would never see.
	meetings, sessions := seasonFixture(current)
	sessions = append(sessions, tests.StubSession{
		SessionKey:       sessions[1].SessionKey + 1,
		MeetingKey:       meetings[0].MeetingKey,
		SessionType:      "Race",
		SessionName:      "Sprint",
		Location:         "Sakhir",
		CountryName:      "Bahrain",
		CircuitKey:       sakhirCircuitKey,
		CircuitShortName: "Sakhir",
		DateStart:        time.Date(current, time.March, 2, 10, 0, 0, 0, time.UTC).Format(upstreamTimeLayout),
		DateEnd:          time.Date(current, time.March, 2, 11, 0, 0, 0, time.UTC).Format(upstreamTimeLayout),
		Year:             current,
	})
	stub.SetSeason(current, meetings, sessions)

	if _, err := ingest.Run(context.Background()); err != nil {
		t.Fatalf("second Run: %v", err)
	}

	var count int
	const q = `SELECT COUNT(*) FROM f1.sessions WHERE year = $1`
	if err := conn.Get(&count, q, current); err != nil {
		t.Fatalf("counting sessions for %d: %v", current, err)
	}
	if count != 3 {
		t.Errorf("%d has %d sessions, want 3 — the season in progress is re-fetched", current, count)
	}
}

// OpenF1 answers "this season does not exist" with a 404, the same status a genuine fault carries.
// Status alone cannot tell them apart, so the client reads the body.
func TestIngestReadsTheUpstreams404BodyToTellAnEmptySeasonFromAFault(t *testing.T) {
	conn, _ := tests.RequireDB(t)

	t.Run("no results found is an empty season", func(t *testing.T) {
		stub := tests.NewOpenF1Stub(t)
		year := services.FirstSeason
		setSeason(stub, year)
		// Every other covered season is unset, so the stub 404s it exactly as the live API does.

		summary, err := newIngest(t, conn, stub).Run(context.Background())
		if err != nil {
			t.Fatalf("Run: %v — a 404 carrying \"No results found.\" is an empty season, not a failure", err)
		}
		if summary.Meetings != 1 {
			t.Errorf("summary.Meetings = %d, want 1", summary.Meetings)
		}
	})

	t.Run("any other 404 body is a failure", func(t *testing.T) {
		conn, _ := tests.RequireDB(t)
		stub := tests.NewOpenF1Stub(t)
		for _, year := range coveredSeasons() {
			setSeason(stub, year)
		}
		stub.FailNext("/meetings", services.FirstSeason, 404, `{"detail":"Unrecognised path."}`)

		_, err := newIngest(t, conn, stub).Run(context.Background())
		if err == nil {
			t.Fatal("Run succeeded on a 404 that was not the upstream's empty answer")
		}

		// The upstream's own status and message survive, rather than flattening into one fault.
		var upstream *models.UpstreamError
		if !errors.As(err, &upstream) {
			t.Fatalf("error is %T (%v), want a *models.UpstreamError", err, err)
		}
		if upstream.Status != 404 {
			t.Errorf("status = %d, want 404 — the upstream's status is propagated", upstream.Status)
		}
		if !strings.Contains(upstream.Message, "Unrecognised path.") {
			t.Errorf("message = %q, want the upstream's own message", upstream.Message)
		}
	})
}

// The hole the 404 body check alone does not close: a wrong OPENF1_BASE_URL answers "No results
// found." for every path, so every season reads as empty and the run would otherwise exit 0 having
// stored nothing. An empty stub is exactly what a misconfigured base URL looks like from here.
func TestIngestFailsRatherThanReportingSuccessOverAnEmptyUpstream(t *testing.T) {
	conn, _ := tests.RequireDB(t)
	stub := tests.NewOpenF1Stub(t)

	summary, err := newIngest(t, conn, stub).Run(context.Background())
	if !errors.Is(err, models.ErrUpstreamEmpty) {
		t.Fatalf("error = %v, want models.ErrUpstreamEmpty", err)
	}
	if summary.Meetings != 0 {
		t.Errorf("summary.Meetings = %d, want 0", summary.Meetings)
	}
}

// The guard above must not fire on a run that legitimately had nothing to do: every season already
// stored is skipped, so no season is fetched and no season can come back empty.
func TestAFullyStoredCorpusIsNotMistakenForAnEmptyUpstream(t *testing.T) {
	conn, _ := tests.RequireDB(t)
	stub := tests.NewOpenF1Stub(t)

	for _, year := range coveredSeasons() {
		setSeason(stub, year)
	}
	ingest := newIngest(t, conn, stub)
	if _, err := ingest.Run(context.Background()); err != nil {
		t.Fatalf("first Run: %v", err)
	}

	// The current season is still re-fetched and still has rows, so the run is not empty — but if it
	// were the last season standing, an over-eager guard would fail a healthy re-run.
	if _, err := ingest.Run(context.Background()); err != nil {
		t.Fatalf("second Run: %v — a re-run over a stored corpus is not an empty upstream", err)
	}
}

// ADR-0003's circuit-shaped twin: Circuits are repo-owned, so a key we have not seeded is a seed gap
// to fix, never a row to invent.
func TestIngestRejectsAnUnseededCircuitWithoutWritingTheSeason(t *testing.T) {
	conn, _ := tests.RequireDB(t)
	stub := tests.NewOpenF1Stub(t)

	year := services.FirstSeason
	meetings, sessions := seasonFixture(year)
	meetings[0].CircuitKey = unseededCircuit
	stub.SetSeason(year, meetings, sessions)

	_, err := newIngest(t, conn, stub).Run(context.Background())
	if !errors.Is(err, models.ErrCircuitNotFound) {
		t.Fatalf("error = %v, want models.ErrCircuitNotFound", err)
	}
	if got := countRows(t, conn, "meetings"); got != 0 {
		t.Errorf("meetings = %d, want 0 — the season is resolved before its transaction opens", got)
	}
	if got := countRows(t, conn, "sessions"); got != 0 {
		t.Errorf("sessions = %d, want 0", got)
	}
}

// 30 req/min documented upstream. One limiter, shared by every call, or four seasons of ingest is a
// burst that earns a block.
func TestIngestPacesItsRequestsToTheConfiguredInterval(t *testing.T) {
	conn, _ := tests.RequireDB(t)
	stub := tests.NewOpenF1Stub(t)

	// Well clear of the database work the run interleaves between seasons — an unpaced run spans 15ms
	// end to end — so the floor below still has teeth if pacing is removed.
	const interval = 100 * time.Millisecond
	for _, year := range coveredSeasons() {
		setSeason(stub, year)
	}

	ingest := app.NewIngest(conn, client.NewOpenF1Client(stub.BaseURL(), 5*time.Second, interval))
	if _, err := ingest.Run(context.Background()); err != nil {
		t.Fatalf("Run: %v", err)
	}

	requests := stub.Requests()
	if len(requests) < 4 {
		t.Fatalf("served %d requests, want at least 4 for the elapsed floor to mean anything", len(requests))
	}

	// A slot reservation floors the run, never any one pair — plans/01-backend-v1.md, "Pacing is a
	// slot reservation, not a sleep". The pair-wise assertion this replaces was already failing
	// rather than merely over-strict: these timestamps are taken in the stub's handler, so each
	// carries its own scheduling latency, and over thirty runs on an idle machine the worst pair came
	// in at 22ms against a 40ms floor. Tolerance covers that jitter at the two endpoints and nothing
	// else; worst slack measured over the same runs was 2.5ms.
	const tolerance = 15 * time.Millisecond
	want := time.Duration(len(requests)-1) * interval
	if elapsed := requests[len(requests)-1].At.Sub(requests[0].At); elapsed < want-tolerance {
		t.Errorf("%d requests spanned %s, want at least %s — %s apiece", len(requests), elapsed, want, interval)
	}
}

// A client with no timeout inherits the OS's, which is minutes. This asserts the bound is the
// service's own.
func TestTheOutboundClientTimesOutRatherThanWaitingOnTheUpstream(t *testing.T) {
	conn, _ := tests.RequireDB(t)
	stub := tests.NewOpenF1Stub(t)

	setSeason(stub, services.FirstSeason)
	stub.Delay(500 * time.Millisecond)

	ingest := app.NewIngest(conn, client.NewOpenF1Client(stub.BaseURL(), 50*time.Millisecond, 0))

	started := time.Now()
	if _, err := ingest.Run(context.Background()); err == nil {
		t.Fatal("Run succeeded against an upstream slower than the client's timeout")
	}
	if elapsed := time.Since(started); elapsed > 400*time.Millisecond {
		t.Errorf("Run took %s, want it bounded near the 50ms client timeout", elapsed)
	}
}

// Interruption is a cancelled context in the binary too — cmd/ingest wires SIGINT and SIGTERM to it.
func TestIngestStopsOnACancelledContextWithoutWriting(t *testing.T) {
	conn, _ := tests.RequireDB(t)
	stub := tests.NewOpenF1Stub(t)

	for _, year := range coveredSeasons() {
		setSeason(stub, year)
	}

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	if _, err := newIngest(t, conn, stub).Run(ctx); !errors.Is(err, context.Canceled) {
		t.Fatalf("error = %v, want context.Canceled", err)
	}
	if got := countRows(t, conn, "meetings"); got != 0 {
		t.Errorf("meetings = %d, want 0", got)
	}
}

// ADR-0001: dropping the auth layer is only safe because the one operation worth protecting is not
// on the API at all. This is that decision as an assertion.
func TestNoHTTPRouteTriggersIngest(t *testing.T) {
	harness := tests.RouterOnly(t)

	for _, route := range harness.Router.Routes() {
		if strings.Contains(strings.ToLower(route.Path), "ingest") {
			t.Errorf("%s %s is routed; ingest is a binary, never an endpoint (ADR-0001)", route.Method, route.Path)
		}
	}

	rec := harness.GET(t, "/api/v1/ingest")
	tests.DecodeError(t, rec, 404)
}

// ── fixtures ──────────────────────────────────────────────────────────────────

func newIngest(t *testing.T, conn *sqlx.DB, stub *tests.OpenF1Stub) *services.IngestService {
	t.Helper()
	// No pacing interval: the pacing test is the one that asserts on it, and every other test would
	// only pay for it.
	return app.NewIngest(conn, client.NewOpenF1Client(stub.BaseURL(), 5*time.Second, 0))
}

func meetingKey(year int) int { return year*10 + 1 }

func setSeason(stub *tests.OpenF1Stub, year int) {
	meetings, sessions := seasonFixture(year)
	stub.SetSeason(year, meetings, sessions)
}

// seasonFixture is one Meeting and its two Sessions for a year, in the upstream's wire shapes. Keys
// are derived from the year because they are primary keys across every season in one table.
func seasonFixture(year int) ([]tests.StubMeeting, []tests.StubSession) {
	meetings := []tests.StubMeeting{{
		MeetingKey:          meetingKey(year),
		MeetingName:         "Bahrain Grand Prix",
		MeetingOfficialName: fmt.Sprintf("FORMULA 1 GULF AIR BAHRAIN GRAND PRIX %d", year),
		Location:            "Sakhir",
		CountryName:         "Bahrain",
		CircuitKey:          sakhirCircuitKey,
		CircuitShortName:    "Sakhir",
		DateStart:           time.Date(year, time.March, 1, 12, 0, 0, 0, time.UTC).Format(upstreamTimeLayout),
		Year:                year,
	}}

	session := func(offset int, sessionType, name string, hour int) tests.StubSession {
		return tests.StubSession{
			SessionKey:       year*100 + offset,
			MeetingKey:       meetingKey(year),
			SessionType:      sessionType,
			SessionName:      name,
			Location:         "Sakhir",
			CountryName:      "Bahrain",
			CircuitKey:       sakhirCircuitKey,
			CircuitShortName: "Sakhir",
			DateStart:        time.Date(year, time.March, 1, hour, 0, 0, 0, time.UTC).Format(upstreamTimeLayout),
			DateEnd:          time.Date(year, time.March, 1, hour+1, 0, 0, 0, time.UTC).Format(upstreamTimeLayout),
			Year:             year,
		}
	}
	return meetings, []tests.StubSession{
		session(1, "Practice", "Practice 1", 12),
		session(2, "Race", models.SessionNameRace, 15),
	}
}

func countRows(t *testing.T, conn *sqlx.DB, table string) int {
	t.Helper()

	var n int
	if err := conn.Get(&n, `SELECT COUNT(*) FROM f1.`+table); err != nil {
		t.Fatalf("counting %s: %v", table, err)
	}
	return n
}

func countMeetingsForYear(t *testing.T, conn *sqlx.DB, year int) int {
	t.Helper()

	var n int
	if err := conn.Get(&n, `SELECT COUNT(*) FROM f1.meetings WHERE year = $1`, year); err != nil {
		t.Fatalf("counting meetings for %d: %v", year, err)
	}
	return n
}
