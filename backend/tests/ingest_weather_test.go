package tests_test

import (
	"context"
	"slices"
	"testing"
	"time"

	"github.com/jmoiron/sqlx"

	"github.com/jeffreygaggino/we-are-rain-checking/backend/models"
	"github.com/jeffreygaggino/we-are-rain-checking/backend/services"
	"github.com/jeffreygaggino/we-are-rain-checking/backend/tests"
)

// The weather endpoint's own wire format: the same numeric offset the rest of OpenF1 uses, plus six
// fractional digits. Probed 2026-09-03 — `2025-03-16T03:07:30.394000+00:00`.
const upstreamSampleTimeLayout = "2006-01-02T15:04:05.000000-07:00"

// One Session's worth of staged weather, as the upstream sends it. Both rainfall values appear, so
// an assertion on the presence flag cannot pass against a column written the same way every time,
// and one sample carries a wind speed of exactly zero — an observation, not an absence.
var stagedSamples = []struct {
	minute         int
	rainfall       int
	windSpeed      float64
	airTemperature float64
}{
	{minute: 0, rainfall: 0, windSpeed: 2.4, airTemperature: 20.5},
	{minute: 1, rainfall: 1, windSpeed: 5.6, airTemperature: 19.75},
	{minute: 2, rainfall: 1, windSpeed: 0, airTemperature: 19.5},
	{minute: 3, rainfall: 0, windSpeed: 3.25, airTemperature: 20},
}

// The same four samples as this repo stores them. Written out rather than derived from the rows
// above, so the assertion cannot agree with the client's mapping by construction.
var storedRainfall = []bool{false, true, true, false}

// The weather half of the eventual join: every Session that has samples gets them, keyed on the
// Session and the observation time, with Rainfall a presence flag and wind speed continuous.
func TestIngestStoresWeatherSamplesForEverySessionThatHasThem(t *testing.T) {
	conn, _ := tests.RequireDB(t)
	stub := tests.NewOpenF1Stub(t)

	stageHealthySeasons(stub)
	stageHealthyWeather(stub)

	seasons := coveredSeasons()
	summary, err := newIngest(t, conn, stub).Run(context.Background())
	if err != nil {
		t.Fatalf("Run: %v", err)
	}

	// Two Sessions a season, four samples apiece.
	want := len(seasons) * 2 * len(stagedSamples)
	if got := countRows(t, conn, "weather_samples"); got != want {
		t.Errorf("weather_samples = %d, want %d", got, want)
	}
	if summary.WeatherSamples != want {
		t.Errorf("summary.WeatherSamples = %d, want %d", summary.WeatherSamples, want)
	}

	_, sessions := seasonFixture(services.FirstSeason)
	race := sessions[1]
	stored := samplesFor(t, conn, race.SessionKey)
	if len(stored) != len(stagedSamples) {
		t.Fatalf("session %d has %d samples, want %d", race.SessionKey, len(stored), len(stagedSamples))
	}

	for i, staged := range stagedSamples {
		got := stored[i]

		wantAt := sessionStart(t, race).Add(time.Duration(staged.minute) * time.Minute)
		if !got.ObservedAt.Equal(wantAt) {
			t.Errorf("sample %d observed_at = %s, want %s", i, got.ObservedAt, wantAt)
		}
		if got.Rainfall != storedRainfall[i] {
			t.Errorf("sample %d rainfall = %v, want %v — upstream sent %d",
				i, got.Rainfall, storedRainfall[i], staged.rainfall)
		}
		if got.WindSpeed == nil || *got.WindSpeed != staged.windSpeed {
			t.Errorf("sample %d wind_speed = %v, want %v — a continuous value, stored as sent",
				i, got.WindSpeed, staged.windSpeed)
		}
		if got.AirTemperature == nil || *got.AirTemperature != staged.airTemperature {
			t.Errorf("sample %d air_temperature = %v, want %v", i, got.AirTemperature, staged.airTemperature)
		}
	}
}

// The criterion this ticket exists for. Idempotence here is two rules at once: a Session with
// samples is skipped without a request at all, and the write is an upsert underneath that.
func TestReIngestingProducesNoDuplicateWeatherSamples(t *testing.T) {
	conn, _ := tests.RequireDB(t)
	stub := tests.NewOpenF1Stub(t)

	stageHealthySeasons(stub)
	stageHealthyWeather(stub)

	ingest := newIngest(t, conn, stub)
	if _, err := ingest.Run(context.Background()); err != nil {
		t.Fatalf("first Run: %v", err)
	}
	afterFirst := countRows(t, conn, "weather_samples")

	stub.ResetRequests()
	summary, err := ingest.Run(context.Background())
	if err != nil {
		t.Fatalf("second Run: %v", err)
	}

	if got := countRows(t, conn, "weather_samples"); got != afterFirst {
		t.Errorf("weather_samples = %d after a second run, want %d", got, afterFirst)
	}
	if asked := weatherRequests(stub); len(asked) != 0 {
		t.Errorf("second run asked for the weather of sessions %v, want none — a Session with samples is skipped", asked)
	}
	if summary.WeatherSamples != 0 {
		t.Errorf("summary.WeatherSamples = %d, want 0 on a run with nothing to fetch", summary.WeatherSamples)
	}
	if summary.WeatherSessionsSkipped != len(coveredSeasons())*2 {
		t.Errorf("summary.WeatherSessionsSkipped = %d, want %d — every stored Session",
			summary.WeatherSessionsSkipped, len(coveredSeasons())*2)
	}
}

// The key is (Session, observation time), so an observation the upstream repeats is an update rather
// than a second row — including a repeat inside one response, which is why samples are written a row
// at a time rather than as one multi-row INSERT.
func TestAnObservationTimeTheUpstreamRepeatsIsStoredOnce(t *testing.T) {
	conn, _ := tests.RequireDB(t)
	stub := tests.NewOpenF1Stub(t)

	stageHealthySeasons(stub)
	stageHealthyWeather(stub)

	_, sessions := seasonFixture(services.FirstSeason)
	race := sessions[1]
	staged := weatherFixture(race)

	// The same instant again, carrying the other rainfall value so the surviving row says which won.
	repeated := staged[1]
	repeated.Rainfall = 0
	stub.SetWeather(race.SessionKey, append(staged, repeated))

	if _, err := newIngest(t, conn, stub).Run(context.Background()); err != nil {
		t.Fatalf("Run: %v", err)
	}

	stored := samplesFor(t, conn, race.SessionKey)
	if len(stored) != len(staged) {
		t.Fatalf("session %d has %d samples, want %d — one row per observation time",
			race.SessionKey, len(stored), len(staged))
	}
	if stored[1].Rainfall {
		t.Errorf("the repeated observation stored rainfall = true, want false — the later row wins")
	}
}

// A Session the upstream holds no weather for is an answer, not a fault — a cancelled Session is the
// live example, and `weather?session_key=9086` was probed answering 404 "No results found." on
// 2026-09-03. It is also the one place this stage's resumption is weaker than the season rule's: the
// Session never gets rows, so nothing records that it was asked, and every run asks again.
func TestASessionTheUpstreamHasNoWeatherForIsNotAFailure(t *testing.T) {
	conn, _ := tests.RequireDB(t)
	stub := tests.NewOpenF1Stub(t)

	stageHealthySeasons(stub)
	stageHealthyWeather(stub)

	// One completed Session left unset, so the stub 404s it exactly as the live API does.
	_, sessions := seasonFixture(services.FirstSeason)
	silent := sessions[1]
	stub.SetWeather(silent.SessionKey, nil)

	ingest := newIngest(t, conn, stub)
	summary, err := ingest.Run(context.Background())
	if err != nil {
		t.Fatalf("Run: %v — a Session with no weather upstream is an empty answer, not a failure", err)
	}

	if got := len(samplesFor(t, conn, silent.SessionKey)); got != 0 {
		t.Errorf("session %d has %d samples, want 0", silent.SessionKey, got)
	}
	// Every other Session still landed, so the empty one did not abandon the stage.
	want := (len(coveredSeasons())*2 - 1) * len(stagedSamples)
	if summary.WeatherSamples != want {
		t.Errorf("summary.WeatherSamples = %d, want %d", summary.WeatherSamples, want)
	}

	stub.ResetRequests()
	if _, err := ingest.Run(context.Background()); err != nil {
		t.Fatalf("second Run: %v", err)
	}
	if asked := weatherRequests(stub); len(asked) != 1 || asked[0] != silent.SessionKey {
		t.Errorf("second run asked for the weather of sessions %v, want just [%d] — a Session with no "+
			"samples has nothing recording that it was asked, so it is asked again",
			asked, silent.SessionKey)
	}
}

// The upstream is live only around a running Session, so a Session still to finish would be stored
// half-observed — and then skipped forever by a rule that asks only whether any samples exist.
func TestWeatherIsNotFetchedForASessionThatHasNotEndedYet(t *testing.T) {
	conn, _ := tests.RequireDB(t)
	stub := tests.NewOpenF1Stub(t)

	stageHealthySeasons(stub)
	stageHealthyWeather(stub)

	current := time.Now().UTC().Year()
	meetings, sessions := seasonFixture(current)
	upcoming := upcomingSessionFixture(current, meetings[0].MeetingKey)
	stub.SetSeason(current, meetings, append(sessions, upcoming))
	// Staged, so the assertion is about ingest declining to ask rather than the stub having nothing.
	stub.SetWeather(upcoming.SessionKey, weatherFixture(upcoming))

	if _, err := newIngest(t, conn, stub).Run(context.Background()); err != nil {
		t.Fatalf("Run: %v", err)
	}

	if got := len(samplesFor(t, conn, upcoming.SessionKey)); got != 0 {
		t.Errorf("session %d has %d samples, want 0 — it has not run yet", upcoming.SessionKey, got)
	}
	for _, sessionKey := range weatherRequests(stub) {
		if sessionKey == upcoming.SessionKey {
			t.Errorf("the upstream was asked for session %d's weather before the Session had ended",
				upcoming.SessionKey)
		}
	}
}

// A Session is stored whole or not at all, and the next run resumes at it — the season rule of #6,
// one level down. Without it a half-written Session would have rows, and the skip would strand it.
func TestAFailedWeatherFetchLeavesTheSessionUnstoredAndTheNextRunResumesAtIt(t *testing.T) {
	conn, _ := tests.RequireDB(t)
	stub := tests.NewOpenF1Stub(t)

	stageHealthySeasons(stub)
	stageHealthyWeather(stub)

	// The oldest season's Race: its Practice is fetched first, so the failure lands with one Session
	// already committed and the rest untouched.
	_, sessions := seasonFixture(services.FirstSeason)
	practice, race := sessions[0], sessions[1]

	ingest := newIngest(t, conn, stub)
	stub.FailNextForSession("/weather", race.SessionKey, 500, `{"detail":"upstream fell over"}`)
	if _, err := ingest.Run(context.Background()); err == nil {
		t.Fatal("Run succeeded despite the upstream failing on a Session's weather")
	}

	if got := len(samplesFor(t, conn, practice.SessionKey)); got != len(stagedSamples) {
		t.Errorf("session %d has %d samples, want %d — the Sessions before the failure are committed",
			practice.SessionKey, got, len(stagedSamples))
	}
	if got := len(samplesFor(t, conn, race.SessionKey)); got != 0 {
		t.Errorf("session %d has %d samples, want 0 — a Session is stored whole or not at all",
			race.SessionKey, got)
	}

	stub.ResetRequests()
	if _, err := ingest.Run(context.Background()); err != nil {
		t.Fatalf("second Run: %v", err)
	}

	if got := len(samplesFor(t, conn, race.SessionKey)); got != len(stagedSamples) {
		t.Errorf("session %d has %d samples after resuming, want %d", race.SessionKey, got, len(stagedSamples))
	}
	// The failure abandons the rest of the stage, so the second run re-asks from the failed Session
	// onward — but it starts there, and never re-asks the one already committed.
	asked := weatherRequests(stub)
	if len(asked) == 0 || asked[0] != race.SessionKey {
		t.Errorf("second run asked for the weather of sessions %v, want it to resume at %d",
			asked, race.SessionKey)
	}
	if slices.Contains(asked, practice.SessionKey) {
		t.Errorf("second run re-asked for session %d's weather, want it skipped — it has samples",
			practice.SessionKey)
	}
}

// ── fixtures ──────────────────────────────────────────────────────────────────

// stageHealthyWeather gives every Session of every covered season the staged samples, which is what
// the upstream looks like on a healthy day. A test about a Session with no weather leaves that one
// unset, so the stub 404s it the way the live API does.
func stageHealthyWeather(stub *tests.OpenF1Stub) {
	for _, year := range coveredSeasons() {
		_, sessions := seasonFixture(year)
		for _, session := range sessions {
			stub.SetWeather(session.SessionKey, weatherFixture(session))
		}
	}
}

// weatherFixture is one Session's Weather Samples in the upstream's wire shapes, observed a minute
// apart from the Session's own start.
func weatherFixture(session tests.StubSession) []tests.StubWeatherSample {
	start, err := time.Parse(upstreamTimeLayout, session.DateStart)
	if err != nil {
		panic("weatherFixture: session fixture carries an unparseable date_start: " + err.Error())
	}

	samples := make([]tests.StubWeatherSample, 0, len(stagedSamples))
	for _, staged := range stagedSamples {
		samples = append(samples, tests.StubWeatherSample{
			Date:             start.Add(time.Duration(staged.minute) * time.Minute).Format(upstreamSampleTimeLayout),
			SessionKey:       session.SessionKey,
			MeetingKey:       session.MeetingKey,
			WindDirection:    136,
			AirTemperature:   staged.airTemperature,
			Humidity:         58,
			Pressure:         1018.7,
			Rainfall:         staged.rainfall,
			WindSpeed:        staged.windSpeed,
			TrackTemperature: 52.5,
		})
	}
	return samples
}

// upcomingSessionFixture is a Session on the calendar that has not run — tomorrow, so it stays in
// the future however many seasons are stored.
func upcomingSessionFixture(year, meetingKey int) tests.StubSession {
	start := time.Now().UTC().AddDate(0, 0, 1)
	return tests.StubSession{
		SessionKey:       year*100 + 3,
		MeetingKey:       meetingKey,
		SessionType:      "Race",
		SessionName:      models.SessionNameRace,
		Location:         "Sakhir",
		CountryName:      "Bahrain",
		CircuitKey:       sakhirCircuitKey,
		CircuitShortName: "Sakhir",
		DateStart:        start.Format(upstreamTimeLayout),
		DateEnd:          start.Add(time.Hour).Format(upstreamTimeLayout),
		Year:             year,
	}
}

// weatherRequests is the Sessions the upstream was asked about, in order. A test asserts on this to
// show a Session was skipped rather than merely re-written with the same rows.
func weatherRequests(stub *tests.OpenF1Stub) []int {
	var asked []int
	for _, req := range stub.Requests() {
		if req.Path == "/weather" {
			asked = append(asked, req.SessionKey)
		}
	}
	return asked
}

func sessionStart(t *testing.T, session tests.StubSession) time.Time {
	t.Helper()

	start, err := time.Parse(upstreamTimeLayout, session.DateStart)
	if err != nil {
		t.Fatalf("parsing the fixture's date_start: %v", err)
	}
	return start.UTC()
}

func samplesFor(t *testing.T, conn *sqlx.DB, sessionKey int) []models.WeatherSample {
	t.Helper()

	const q = `SELECT session_key, observed_at, rainfall, air_temperature, track_temperature, humidity,
	                  pressure, wind_speed, wind_direction, created_at, updated_at
	           FROM f1.weather_samples WHERE session_key = $1 ORDER BY observed_at`

	var samples []models.WeatherSample
	if err := conn.Select(&samples, q, sessionKey); err != nil {
		t.Fatalf("fetching samples for session %d: %v", sessionKey, err)
	}
	return samples
}
