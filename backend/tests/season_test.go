package tests_test

import (
	"fmt"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/jmoiron/sqlx"

	"github.com/jeffreygaggino/we-are-rain-checking/backend/models"
	"github.com/jeffreygaggino/we-are-rain-checking/backend/tests"
)

// Seeded reference ids, which are literal constants precisely so a fixture can name them (ADR-0003).
const (
	silverstoneID = "d5ffead2-0555-4abc-b5f0-734ccd124d13"
	albonID       = "35d42e7f-a4f0-46e7-8ddc-dfa63a275129"
	sainzID       = "b1420115-4b78-4ed2-882d-5c0874b760af"
)

// A season inside the Season Range, far enough from the clock that the range never excludes it.
const fixtureSeason = 2024

// The four Races the fixture builds: one ordinary, one cancelled, one that has run but whose Weather
// Samples never landed, and one still to be run.
const (
	ranRace       = 900001
	cancelledRace = 900002
	weatherless   = 900003
	notRun        = 900004
)

// The headline: a season comes back as Races in date order, each holding its own results.
func TestASeasonReturnsItsRacesWithTheirResults(t *testing.T) {
	h := tests.RequireHarness(t)
	seedSeason(t, h.DB)

	got := requireSeason(t, h, fixtureSeason)

	// Every scheduled Race, not only the ones that have been run: the response is the season's
	// calendar, so a page can show the rounds still to come.
	if len(got.Races) != 4 {
		t.Fatalf("Races = %d, want 4", len(got.Races))
	}

	// Date order, which is what makes the response renderable as a calendar without sorting it.
	for i, want := range []int{ranRace, cancelledRace, weatherless, notRun} {
		if got.Races[i].SessionKey != want {
			t.Errorf("Race %d = %d, want %d — Races are ordered by date", i, got.Races[i].SessionKey, want)
		}
	}

	race := findRace(t, got, ranRace)
	if race.RaceName != "British Grand Prix" {
		t.Errorf("race name = %q", race.RaceName)
	}
	if len(race.Results) != 2 {
		t.Fatalf("results = %d, want 2", len(race.Results))
	}
	if race.Results[0].FullName != "Alexander ALBON" {
		t.Errorf("first result = %q, want the winner first", race.Results[0].FullName)
	}
	if race.Results[0].Position == nil || *race.Results[0].Position != 1 {
		t.Errorf("winner position = %v, want 1", race.Results[0].Position)
	}
}

// The AVG/COUNT trap, which is the reason each aggregate carries its own count.
//
// The fixture gives the Race four Weather Samples, only two of which carry a wind reading. A mean
// divided by the Session's sample count rather than the wind count reports 2.5 m/s for a Race that
// blew at 5 — a wrong number that looks entirely plausible, which is why it is asserted rather than
// left to review.
func TestAWeatherMeanIsComputedOverTheSamplesThatCarryIt(t *testing.T) {
	h := tests.RequireHarness(t)
	seedSeason(t, h.DB)

	race := findRace(t, requireSeason(t, h, fixtureSeason), ranRace)
	if race.Weather == nil {
		t.Fatal("Race has no weather block")
	}
	weather := *race.Weather

	if weather.SampleCount != 4 {
		t.Errorf("sampleCount = %d, want 4", weather.SampleCount)
	}
	if weather.RainfallSampleCount != 2 {
		t.Errorf("rainfallSampleCount = %d, want 2", weather.RainfallSampleCount)
	}
	if weather.WindSampleCount != 2 {
		t.Errorf("windSampleCount = %d, want 2 — two of the four samples carry no wind reading", weather.WindSampleCount)
	}
	if weather.WindSpeedMeanMps == nil || *weather.WindSpeedMeanMps != 5 {
		t.Errorf("windSpeedMeanMps = %v, want 5 (the mean of 4 and 6, not of 4, 6 and two NULLs)",
			deref(weather.WindSpeedMeanMps))
	}
	if weather.WindSpeedMaxMps == nil || *weather.WindSpeedMaxMps != 6 {
		t.Errorf("windSpeedMaxMps = %v, want 6", deref(weather.WindSpeedMaxMps))
	}

	// Track temperature is absent from every sample: the count is zero and the mean is nil, while
	// the block itself is still present. Absent readings and an absent Race are different answers.
	if weather.TrackTemperatureSampleCount != 0 {
		t.Errorf("trackTemperatureSampleCount = %d, want 0", weather.TrackTemperatureSampleCount)
	}
	if weather.TrackTemperatureMeanC != nil {
		t.Errorf("trackTemperatureMeanC = %v, want null when nothing recorded one", *weather.TrackTemperatureMeanC)
	}
}

// A Race that ran without Weather Samples reports null weather, never a zeroed block — which would
// read as a still, dry, freezing Race rather than as an absence of readings. Its results still land.
func TestARaceWithNoWeatherSamplesKeepsItsResults(t *testing.T) {
	h := tests.RequireHarness(t)
	seedSeason(t, h.DB)

	race := findRace(t, requireSeason(t, h, fixtureSeason), weatherless)

	if race.Weather != nil {
		t.Errorf("weather = %+v, want null for a Race with no Weather Samples", *race.Weather)
	}
	if len(race.Results) != 1 {
		t.Errorf("results = %d, want 1 — missing weather must not cost the Race its classification", len(race.Results))
	}
}

// A cancelled Race is reported, flagged, and carries nothing that would imply it was run.
//
// The fixture gives it results on purpose. A Race that did not happen has no classification, so one
// attached upstream would be an artefact — and reading it would put finishers in a Race nobody
// drove. The filter is on `is_cancelled`, which is what this asserts.
func TestACancelledRaceIsReportedButCarriesNothing(t *testing.T) {
	h := tests.RequireHarness(t)
	seedSeason(t, h.DB)

	race := findRace(t, requireSeason(t, h, fixtureSeason), cancelledRace)

	if !race.Cancelled {
		t.Error("cancelled = false, want true")
	}
	if race.RaceName != "Cancelled Grand Prix" || race.DateStart.IsZero() {
		t.Errorf("cancelled Race = %q on %v, want its calendar slot reported", race.RaceName, race.DateStart)
	}
	if race.Weather != nil {
		t.Errorf("weather = %+v, want null — there was nothing to observe", *race.Weather)
	}
	if len(race.Results) != 0 {
		t.Errorf("results = %d, want none for a Race that did not happen", len(race.Results))
	}
}

// The Races that ran are not flagged. Without this, a `cancelled: true` hardcoded everywhere would
// pass the test above.
func TestARaceThatRanIsNotFlaggedCancelled(t *testing.T) {
	h := tests.RequireHarness(t)
	seedSeason(t, h.DB)

	if race := findRace(t, requireSeason(t, h, fixtureSeason), ranRace); race.Cancelled {
		t.Error("a Race that ran is flagged cancelled")
	}
}

// The season is the whole calendar, not only what has been run. A round still to come is present
// with nothing recorded against it, so the page can show the rest of the year.
func TestARaceStillToBeRunIsInTheCalendar(t *testing.T) {
	h := tests.RequireHarness(t)
	seedSeason(t, h.DB)

	race := findRace(t, requireSeason(t, h, fixtureSeason), notRun)

	if race.Cancelled {
		t.Error("a Race still to be run is flagged cancelled — it is scheduled, not called off")
	}
	if race.Weather != nil || len(race.Results) != 0 {
		t.Errorf("weather = %v, results = %d; want nothing recorded against a Race that has not happened",
			race.Weather, len(race.Results))
	}
}

// A Race still to come and a Race that was called off carry the same empty payload, and `cancelled`
// is the only thing that separates them. Asserted because the page renders them differently — "13:00
// Sunday" against "cancelled" — and nothing else in the response can tell them apart.
func TestOnlyTheFlagSeparatesACancelledRaceFromAnUnrunOne(t *testing.T) {
	h := tests.RequireHarness(t)
	seedSeason(t, h.DB)

	season := requireSeason(t, h, fixtureSeason)
	cancelled := findRace(t, season, cancelledRace)
	upcoming := findRace(t, season, notRun)

	if cancelled.Cancelled == upcoming.Cancelled {
		t.Fatalf("cancelled flag = %v for both — the two states are indistinguishable", cancelled.Cancelled)
	}
	if cancelled.Weather != nil || upcoming.Weather != nil {
		t.Error("one of them carries weather; both should carry none")
	}
	if len(cancelled.Results) != 0 || len(upcoming.Results) != 0 {
		t.Error("one of them carries results; both should carry none")
	}
}

// Outside the Season Range is a refusal that names the range, not an empty season. 2019 cannot exist
// and a season with nothing stored merely has not happened — answering both the same way reports
// them as the same thing.
func TestASeasonOutsideTheRangeIsRefusedAndNamesTheRange(t *testing.T) {
	h := tests.RequireHarness(t)

	envelope := tests.DecodeError(t, h.GET(t, seasonPath(2019)), http.StatusBadRequest)

	if !strings.Contains(envelope.Message, "2019") {
		t.Errorf("message = %q, want it to name the season asked for", envelope.Message)
	}
	if !strings.Contains(envelope.Message, "2023") {
		t.Errorf("message = %q, want it to name the first season carried", envelope.Message)
	}
}

// A season inside the range holding nothing is an empty list — and `[]`, never `null`, so the page
// can iterate it without a guard.
func TestASeasonWithNothingStoredIsAnEmptyList(t *testing.T) {
	h := tests.RequireHarness(t)

	// Poisoned, so a decode that quietly does nothing fails loudly rather than reporting empty.
	got := models.Season{Races: []models.SeasonRace{{}}}
	tests.DecodeSuccess(t, h.GET(t, seasonPath(fixtureSeason)), http.StatusOK, &got)

	if len(got.Races) != 0 {
		t.Errorf("Races = %d, want 0 for a season with nothing stored", len(got.Races))
	}
	if body := h.GET(t, seasonPath(fixtureSeason)).Body.String(); !strings.Contains(body, `"races":[]`) {
		t.Errorf("body = %s, want races to encode as [] rather than null", body)
	}
}

// Results is never null either, for the same reason: a cancelled Race is the case that would
// otherwise hand the page a nil to range over.
func TestResultsEncodeAsAnEmptyListRatherThanNull(t *testing.T) {
	h := tests.RequireHarness(t)
	seedSeason(t, h.DB)

	if body := h.GET(t, seasonPath(fixtureSeason)).Body.String(); strings.Contains(body, `"results":null`) {
		t.Errorf("body contains \"results\":null, want [] — the page ranges over it unguarded")
	}
}

// A year that is not a number is a bad request rather than a 500 or a scan of season zero.
func TestANonNumericSeasonIsABadRequest(t *testing.T) {
	h := tests.RequireHarness(t)

	tests.DecodeError(t, h.GET(t, "/api/v1/seasons/last-year/races"), http.StatusBadRequest)
}

func seasonPath(year int) string {
	return fmt.Sprintf("/api/v1/seasons/%d/races", year)
}

func requireSeason(t *testing.T, h *tests.Harness, year int) models.Season {
	t.Helper()

	var got models.Season
	tests.DecodeSuccess(t, h.GET(t, seasonPath(year)), http.StatusOK, &got)
	return got
}

func findRace(t *testing.T, season models.Season, sessionKey int) models.SeasonRace {
	t.Helper()

	for _, race := range season.Races {
		if race.SessionKey == sessionKey {
			return race
		}
	}
	t.Fatalf("no Race %d in the season", sessionKey)
	return models.SeasonRace{}
}

// seedSeason writes three Races into one season: one that ran with weather, one cancelled, and one
// that ran with no Weather Samples at all.
func seedSeason(t *testing.T, conn *sqlx.DB) {
	t.Helper()

	base := time.Date(fixtureSeason, time.May, 5, 13, 0, 0, 0, time.UTC)

	meeting(t, conn, 800001, "British Grand Prix", base)
	meeting(t, conn, 800002, "Cancelled Grand Prix", base.AddDate(0, 0, 7))
	meeting(t, conn, 800003, "Unrecorded Grand Prix", base.AddDate(0, 0, 14))
	meeting(t, conn, 800004, "Forthcoming Grand Prix", base.AddDate(0, 0, 21))

	session(t, conn, ranRace, 800001, base, false)
	session(t, conn, cancelledRace, 800002, base.AddDate(0, 0, 7), true)
	session(t, conn, weatherless, 800003, base.AddDate(0, 0, 14), false)
	// Scheduled, not cancelled, nothing recorded against it — a round still to come.
	session(t, conn, notRun, 800004, base.AddDate(0, 0, 21), false)

	result(t, conn, ranRace, albonID, 23, ptr(1), 25)
	result(t, conn, ranRace, sainzID, 55, ptr(2), 18)
	// The cancelled Race carries results on purpose: it is what makes the is_cancelled filter
	// distinguishable from a filter written against emptiness.
	result(t, conn, cancelledRace, albonID, 23, ptr(1), 25)
	result(t, conn, weatherless, albonID, 23, ptr(1), 25)

	// Four samples, two recording Rainfall, two carrying a wind reading, none a track temperature.
	sample(t, conn, ranRace, base, true, ptr(4.0), ptr(20.0))
	sample(t, conn, ranRace, base.Add(10*time.Minute), true, ptr(6.0), ptr(20.0))
	sample(t, conn, ranRace, base.Add(20*time.Minute), false, nil, ptr(22.0))
	sample(t, conn, ranRace, base.Add(30*time.Minute), false, nil, ptr(22.0))
}

func meeting(t *testing.T, conn *sqlx.DB, key int, name string, start time.Time) {
	t.Helper()

	const q = `INSERT INTO f1.meetings (meeting_key, year, name, official_name, circuit_id, country_name, location, date_start)
	           VALUES ($1, $2, $3, $4, $5, 'United Kingdom', 'Silverstone', $6)`
	exec(t, conn, q, key, fixtureSeason, name, name, silverstoneID, start)
}

func session(t *testing.T, conn *sqlx.DB, key, meetingKey int, start time.Time, cancelled bool) {
	t.Helper()

	const q = `INSERT INTO f1.sessions (session_key, meeting_key, circuit_id, session_type, session_name, year, date_start, date_end, is_cancelled)
	           VALUES ($1, $2, $3, 'Race', $4, $5, $6, $7, $8)`
	exec(t, conn, q, key, meetingKey, silverstoneID, models.SessionNameRace, fixtureSeason,
		start, start.Add(2*time.Hour), cancelled)
}

func result(t *testing.T, conn *sqlx.DB, sessionKey int, driverID string, number int, position *int, points float64) {
	t.Helper()

	const q = `INSERT INTO f1.session_results (session_key, driver_id, racing_number, position, points, number_of_laps, dnf, dns, dsq)
	           VALUES ($1, $2, $3, $4, $5, 52, false, false, false)`
	exec(t, conn, q, sessionKey, driverID, number, position, points)
}

func sample(t *testing.T, conn *sqlx.DB, sessionKey int, at time.Time, rainfall bool, wind, airTemp *float64) {
	t.Helper()

	const q = `INSERT INTO f1.weather_samples (session_key, observed_at, rainfall, wind_speed, air_temperature)
	           VALUES ($1, $2, $3, $4, $5)`
	exec(t, conn, q, sessionKey, at, rainfall, wind, airTemp)
}

func exec(t *testing.T, conn *sqlx.DB, q string, args ...any) {
	t.Helper()

	if _, err := conn.Exec(q, args...); err != nil {
		t.Fatalf("seeding: %v", err)
	}
}

func ptr[T any](v T) *T { return &v }

func deref(v *float64) any {
	if v == nil {
		return "null"
	}
	return *v
}
