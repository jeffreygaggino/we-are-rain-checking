package services

import (
	"context"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/jmoiron/sqlx"

	"github.com/jeffreygaggino/we-are-rain-checking/backend/client"
	"github.com/jeffreygaggino/we-are-rain-checking/backend/models"
	"github.com/jeffreygaggino/we-are-rain-checking/backend/repository"
)

// FirstSeason is the earliest year OpenF1 carries — probed, not assumed: meetings?year=2022 answers
// "No results found." The last season is the current year from the clock, so the range grows on its
// own and needs no configuration.
const FirstSeason = 2023

// ResultsSettleWindow is how long after a Race ends before its classification is fetched.
//
// Stage 2's resumption rests on a completed Session's weather never changing. A Race's
// classification does change: the grid that crosses the line is provisional until the stewards are
// done, and a penalty applied afterwards moves positions, points and a dsq flag — the three things
// #10 reads. Since a Race with rows is then skipped without a request, a run that arrived an hour
// after the flag would freeze the provisional grid permanently.
//
// A day is the judgement, not a measurement: it clears same-evening stewarding comfortably, and it
// costs data that is only ever read historically nothing at all. An appeal decided later than
// this is not picked up — delete the Race's rows and re-run, which the upsert makes safe. See
// plans/04-session-results.md.
const ResultsSettleWindow = 24 * time.Hour

// Summary is what a completed run reports to its operator. Seasons skipped is the number that says
// resumption worked; a run that re-fetched everything shows an empty one.
//
// Weather is counted rather than listed: the skipped ones are every stored Session on a healthy re-run,
// some five hundred Sessions, which is a number to read and not a list.
type Summary struct {
	SeasonsIngested        []int
	SeasonsSkipped         []int
	Meetings               int
	Sessions               int
	WeatherSamples         int
	WeatherSessions        int
	WeatherSessionsSkipped int
	Results                int
	ResultRaces            int
	ResultRacesSkipped     int
}

// IngestService fills the Meetings, Sessions, Weather Samples and Session Results tables from
// OpenF1.
//
// It is reached from cmd/ingest and never from a route: dropping the auth layer is only safe because
// the one operation worth protecting is not on the API at all (ADR-0001).
type IngestService struct {
	conn     *sqlx.DB
	openF1   *client.OpenF1Client
	circuits *repository.CircuitRepo
	drivers  *repository.DriverRepo
	meetings *repository.MeetingRepo
	sessions *repository.SessionRepo
	weather  *repository.WeatherSampleRepo
	results  *repository.SessionResultRepo
}

func NewIngestService(
	conn *sqlx.DB,
	openF1 *client.OpenF1Client,
	circuits *repository.CircuitRepo,
	drivers *repository.DriverRepo,
	meetings *repository.MeetingRepo,
	sessions *repository.SessionRepo,
	weather *repository.WeatherSampleRepo,
	results *repository.SessionResultRepo,
) *IngestService {
	return &IngestService{
		conn:     conn,
		openF1:   openF1,
		circuits: circuits,
		drivers:  drivers,
		meetings: meetings,
		sessions: sessions,
		weather:  weather,
		results:  results,
	}
}

// Run ingests every season the upstream covers, oldest first, skipping the ones already stored, then
// fills in the Weather Samples of every Session that has ended and the classification of every Race
// that has run.
//
// Oldest first is what makes an interrupted run resumable: the seasons behind the failure are
// complete, so the next run starts at the one that failed. The two later stages come second because
// both are keyed on the Sessions the first stores.
func (s *IngestService) Run(ctx context.Context) (Summary, error) {
	var summary Summary

	circuitIDs, err := s.circuits.KeysToIDs(ctx, s.conn)
	if err != nil {
		return summary, fmt.Errorf("services.IngestService.Run: %w", err)
	}
	if len(circuitIDs) == 0 {
		return summary, fmt.Errorf("services.IngestService.Run: no seeded circuits — run the migrations first")
	}

	stored, err := s.storedSeasons(ctx)
	if err != nil {
		return summary, fmt.Errorf("services.IngestService.Run: %w", err)
	}

	currentSeason := time.Now().UTC().Year()
	for year := FirstSeason; year <= currentSeason; year++ {
		// The season in progress is always re-fetched: Sessions are still being added to it, so
		// "already has rows" does not mean "complete" the way it does for a season that has ended.
		if year < currentSeason && stored[year] {
			summary.SeasonsSkipped = append(summary.SeasonsSkipped, year)
			continue
		}

		meetings, sessions, err := s.fetchSeason(ctx, year, circuitIDs)
		if err != nil {
			return summary, err
		}
		// A season that has ended and carries no Meetings is the misconfiguration signal; the season
		// in progress is exempt, being legitimately empty until its first Meeting is published. See
		// plans/01-backend-v1.md, "The guard asks about completed seasons only".
		if year < currentSeason && len(meetings) == 0 {
			return summary, fmt.Errorf("services.IngestService.Run: %d: %w", year, models.ErrUpstreamEmpty)
		}

		if err := s.storeSeason(ctx, meetings, sessions); err != nil {
			return summary, err
		}

		summary.SeasonsIngested = append(summary.SeasonsIngested, year)
		summary.Meetings += len(meetings)
		summary.Sessions += len(sessions)
	}

	if err := s.ingestWeather(ctx, &summary); err != nil {
		return summary, err
	}

	if err := s.ingestResults(ctx, &summary); err != nil {
		return summary, err
	}

	return summary, nil
}

// ingestWeather fills in the Weather Samples of every Session that has ended and holds none — the
// season rule of Run one level down, derived from the rows the same way, with the same
// fetch-before-the-transaction property. See plans/01-backend-v1.md, "Stage 2 — Weather Samples
// (#7)", including the gap it leaves open.
//
// The summary is written through rather than returned so a run that fails partway still reports the
// Sessions it got through — the same thing the season loop's counters do for an interrupted run.
func (s *IngestService) ingestWeather(ctx context.Context, summary *Summary) error {
	completed, err := s.sessions.CompletedKeys(ctx, s.conn, time.Now().UTC())
	if err != nil {
		return fmt.Errorf("services.IngestService.ingestWeather: %w", err)
	}
	stored, err := s.weather.SessionKeysWithSamples(ctx, s.conn)
	if err != nil {
		return fmt.Errorf("services.IngestService.ingestWeather: %w", err)
	}

	for _, sessionKey := range completed {
		if stored[sessionKey] {
			summary.WeatherSessionsSkipped++
			continue
		}

		upstream, err := s.openF1.WeatherSamples(ctx, sessionKey)
		if err != nil {
			return err
		}
		samples := samplesOf(upstream)
		if err := s.storeSamples(ctx, samples); err != nil {
			return err
		}

		summary.WeatherSessions++
		summary.WeatherSamples += len(samples)
	}

	return nil
}

// samplesOf maps the upstream's rows onto this repo's model. Unlike a Meeting or a Session there is
// nothing to resolve: a Weather Sample hangs off its Session, which already carries the Circuit.
func samplesOf(upstream []client.WeatherSample) []models.WeatherSample {
	samples := make([]models.WeatherSample, 0, len(upstream))
	for _, u := range upstream {
		samples = append(samples, models.WeatherSample{
			SessionKey:       u.SessionKey,
			ObservedAt:       u.ObservedAt,
			Rainfall:         u.Rainfall,
			AirTemperature:   u.AirTemperature,
			TrackTemperature: u.TrackTemperature,
			Humidity:         u.Humidity,
			Pressure:         u.Pressure,
			WindSpeed:        u.WindSpeed,
			WindDirection:    u.WindDirection,
		})
	}
	return samples
}

// storeSamples writes one Session's samples in one transaction, for the same reason storeSeason
// writes a season in one: a half-stored Session has rows, and a rule that skips any Session with
// rows would strand it forever.
func (s *IngestService) storeSamples(ctx context.Context, samples []models.WeatherSample) error {
	if len(samples) == 0 {
		return nil
	}

	tx, err := s.conn.BeginTxx(ctx, nil)
	if err != nil {
		return fmt.Errorf("services.IngestService.storeSamples: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	for i := range samples {
		if err := s.weather.Upsert(ctx, tx, &samples[i]); err != nil {
			return fmt.Errorf("services.IngestService.storeSamples: %w", err)
		}
	}

	if err := tx.Commit(); err != nil {
		return fmt.Errorf("services.IngestService.storeSamples commit: %w", err)
	}
	return nil
}

// ingestResults fills in the classification of every Race that has settled and holds none — the
// Session rule of ingestWeather again, one table across, with the settle window ResultsSettleWindow
// explains. See plans/04-session-results.md.
//
// Races only: an entry list outside a Race carries names this repo does not seed, and under
// ADR-0003 every one of them aborts the run.
func (s *IngestService) ingestResults(ctx context.Context, summary *Summary) error {
	driverIDs, err := s.drivers.NamesToIDs(ctx, s.conn)
	if err != nil {
		return fmt.Errorf("services.IngestService.ingestResults: %w", err)
	}
	if len(driverIDs) == 0 {
		return fmt.Errorf("services.IngestService.ingestResults: no seeded drivers — run the migrations first")
	}

	races, err := s.sessions.CompletedRaceKeys(ctx, s.conn, time.Now().UTC().Add(-ResultsSettleWindow))
	if err != nil {
		return fmt.Errorf("services.IngestService.ingestResults: %w", err)
	}
	stored, err := s.results.SessionKeysWithResults(ctx, s.conn)
	if err != nil {
		return fmt.Errorf("services.IngestService.ingestResults: %w", err)
	}

	for _, sessionKey := range races {
		if stored[sessionKey] {
			summary.ResultRacesSkipped++
			continue
		}

		results, err := s.fetchRaceResults(ctx, sessionKey, driverIDs)
		if err != nil {
			return err
		}
		if err := s.storeResults(ctx, results); err != nil {
			return err
		}
		// Counted only when it stored something. A Race the upstream has no classification for is
		// neither ingested nor skipped — it is the third state this stage has, and reporting it as
		// ingested would make the tally look stable while the same Races were re-asked every run.
		if len(results) == 0 {
			continue
		}

		summary.ResultRaces++
		summary.Results += len(results)
	}

	return nil
}

// fetchRaceResults reads one Race's classification and resolves every Racing Number in it to a
// seeded Driver, before storeResults opens a transaction — the property every stage keeps, and the
// one that makes "this Race has rows" a safe thing to skip on.
//
// The classification is read first: a Race the upstream has no results for needs no entry list, and
// there are fourteen of those today.
func (s *IngestService) fetchRaceResults(
	ctx context.Context,
	sessionKey int,
	driverIDs map[string]uuid.UUID,
) ([]models.SessionResult, error) {
	upstream, err := s.openF1.SessionResults(ctx, sessionKey)
	if err != nil {
		return nil, err
	}
	if len(upstream) == 0 {
		return nil, nil
	}

	names, err := s.raceEntryList(ctx, sessionKey)
	if err != nil {
		return nil, err
	}

	results := make([]models.SessionResult, 0, len(upstream))
	classified := make(map[uuid.UUID]int, len(upstream))
	for _, u := range upstream {
		fullName, ok := names[u.RacingNumber]
		if !ok {
			return nil, fmt.Errorf("session %d classifies racing number %d, which its entry list does "+
				"not carry: %w", sessionKey, u.RacingNumber, models.ErrDriverNotResolved)
		}
		driverID, ok := driverIDs[fullName]
		if !ok {
			return nil, fmt.Errorf("session %d classifies %q, who is not seeded: %w",
				sessionKey, fullName, models.ErrDriverNotResolved)
		}
		// Two numbers resolving to one Driver would write two rows on one primary key, and the
		// second would overwrite the first inside the transaction — a Driver dropped from the grid
		// with the run still reporting success.
		if number, ok := classified[driverID]; ok {
			return nil, fmt.Errorf("session %d classifies %q under racing numbers %d and %d: %w",
				sessionKey, fullName, number, u.RacingNumber, models.ErrDriverNotResolved)
		}
		classified[driverID] = u.RacingNumber

		results = append(results, models.SessionResult{
			SessionKey:   u.SessionKey,
			DriverID:     driverID,
			RacingNumber: u.RacingNumber,
			Position:     u.Position,
			Points:       u.Points,
			NumberOfLaps: u.NumberOfLaps,
			DNF:          u.DNF,
			DNS:          u.DNS,
			DSQ:          u.DSQ,
		})
	}
	return results, nil
}

// raceEntryList reads one Race's entry list as `racing number -> upstream name`. It is the middle
// step of ADR-0003's resolution, and it is per Session: a number reassigned between seasons — or
// mid-season — lands on the person who actually carried it that weekend.
//
// The names are not resolved to Drivers here. Only the numbers a result row classifies matter, and
// resolving the rest would let an entrant nobody classified — a withdrawal, a reserve listed and not
// run — abort a run that has nothing to store for them.
//
// One number under two names does abort, here where it is visible: last-write-wins over an ambiguous
// number resolves into a plausible-looking finding, which is the failure ADR-0003 was written about.
func (s *IngestService) raceEntryList(ctx context.Context, sessionKey int) (map[int]string, error) {
	entrants, err := s.openF1.SessionDrivers(ctx, sessionKey)
	if err != nil {
		return nil, err
	}

	names := make(map[int]string, len(entrants))
	for _, entrant := range entrants {
		if seen, ok := names[entrant.RacingNumber]; ok && seen != entrant.FullName {
			return nil, fmt.Errorf("session %d gives racing number %d to both %q and %q: %w",
				sessionKey, entrant.RacingNumber, seen, entrant.FullName, models.ErrDriverNotResolved)
		}
		names[entrant.RacingNumber] = entrant.FullName
	}
	return names, nil
}

// storeResults writes one Race's classification in one transaction, for the reason storeSamples and
// storeSeason each write theirs in one: a half-stored Race has rows, and the rule that skips any
// Race with rows would strand it.
func (s *IngestService) storeResults(ctx context.Context, results []models.SessionResult) error {
	if len(results) == 0 {
		return nil
	}

	tx, err := s.conn.BeginTxx(ctx, nil)
	if err != nil {
		return fmt.Errorf("services.IngestService.storeResults: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	for i := range results {
		if err := s.results.Upsert(ctx, tx, &results[i]); err != nil {
			return fmt.Errorf("services.IngestService.storeResults: %w", err)
		}
	}

	if err := tx.Commit(); err != nil {
		return fmt.Errorf("services.IngestService.storeResults commit: %w", err)
	}
	return nil
}

// storedSeasons is the resumption check, derived from the rows rather than a checkpoint table. A
// season counts as stored only with both Meetings and Sessions present — half a season is what a
// crashed run would leave, and skipping that would strand it forever.
func (s *IngestService) storedSeasons(ctx context.Context) (map[int]bool, error) {
	meetingCounts, err := s.meetings.CountsByYear(ctx, s.conn)
	if err != nil {
		return nil, err
	}
	sessionCounts, err := s.sessions.CountsByYear(ctx, s.conn)
	if err != nil {
		return nil, err
	}

	stored := make(map[int]bool, len(meetingCounts))
	for year, meetings := range meetingCounts {
		stored[year] = meetings > 0 && sessionCounts[year] > 0
	}
	return stored, nil
}

// fetchSeason reads a season from the upstream and resolves it into this repo's vocabulary.
//
// Every upstream call and every Circuit resolution happens here, before storeSeason opens a
// transaction — which is what makes a season all-or-nothing without holding one open across the
// network.
func (s *IngestService) fetchSeason(
	ctx context.Context,
	year int,
	circuitIDs map[int]uuid.UUID,
) ([]models.Meeting, []models.Session, error) {
	upstreamMeetings, err := s.openF1.Meetings(ctx, year)
	if err != nil {
		return nil, nil, err
	}
	upstreamSessions, err := s.openF1.Sessions(ctx, year)
	if err != nil {
		return nil, nil, err
	}

	meetings := make([]models.Meeting, 0, len(upstreamMeetings))
	for _, m := range upstreamMeetings {
		circuitID, ok := circuitIDs[m.CircuitKey]
		if !ok {
			return nil, nil, fmt.Errorf("meeting %d sits at circuit_key %d: %w",
				m.MeetingKey, m.CircuitKey, models.ErrCircuitNotFound)
		}
		meetings = append(meetings, models.Meeting{
			MeetingKey:   m.MeetingKey,
			Year:         m.Year,
			Name:         m.Name,
			OfficialName: m.OfficialName,
			CircuitID:    circuitID,
			CountryName:  m.CountryName,
			Location:     m.Location,
			DateStart:    m.DateStart,
		})
	}

	sessions := make([]models.Session, 0, len(upstreamSessions))
	for _, sess := range upstreamSessions {
		circuitID, ok := circuitIDs[sess.CircuitKey]
		if !ok {
			return nil, nil, fmt.Errorf("session %d sits at circuit_key %d: %w",
				sess.SessionKey, sess.CircuitKey, models.ErrCircuitNotFound)
		}
		sessions = append(sessions, models.Session{
			SessionKey:  sess.SessionKey,
			MeetingKey:  sess.MeetingKey,
			CircuitID:   circuitID,
			SessionType: sess.SessionType,
			SessionName: sess.SessionName,
			Year:        sess.Year,
			DateStart:   sess.DateStart,
			DateEnd:     sess.DateEnd,
			IsCancelled: sess.IsCancelled,
		})
	}

	return meetings, sessions, nil
}

// storeSeason writes a season in one transaction, Meetings first because Sessions reference them.
func (s *IngestService) storeSeason(ctx context.Context, meetings []models.Meeting, sessions []models.Session) error {
	tx, err := s.conn.BeginTxx(ctx, nil)
	if err != nil {
		return fmt.Errorf("services.IngestService.storeSeason: %w", err)
	}
	// A no-op once the transaction has committed, and the only thing standing between a failed
	// write and a half-stored season otherwise.
	defer func() { _ = tx.Rollback() }()

	for i := range meetings {
		if err := s.meetings.Upsert(ctx, tx, &meetings[i]); err != nil {
			return fmt.Errorf("services.IngestService.storeSeason: %w", err)
		}
	}
	for i := range sessions {
		if err := s.sessions.Upsert(ctx, tx, &sessions[i]); err != nil {
			return fmt.Errorf("services.IngestService.storeSeason: %w", err)
		}
	}

	if err := tx.Commit(); err != nil {
		return fmt.Errorf("services.IngestService.storeSeason commit: %w", err)
	}
	return nil
}
