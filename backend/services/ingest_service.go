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

// Summary is what a completed run reports to its operator. Seasons skipped is the number that says
// resumption worked; a run that re-fetched everything shows an empty one.
type Summary struct {
	SeasonsIngested []int
	SeasonsSkipped  []int
	Meetings        int
	Sessions        int
}

// IngestService fills the Meetings and Sessions tables from OpenF1.
//
// It is reached from cmd/ingest and never from a route: dropping the auth layer is only safe because
// the one operation worth protecting is not on the API at all (ADR-0001).
type IngestService struct {
	conn     *sqlx.DB
	openF1   *client.OpenF1Client
	circuits *repository.CircuitRepo
	meetings *repository.MeetingRepo
	sessions *repository.SessionRepo
}

func NewIngestService(
	conn *sqlx.DB,
	openF1 *client.OpenF1Client,
	circuits *repository.CircuitRepo,
	meetings *repository.MeetingRepo,
	sessions *repository.SessionRepo,
) *IngestService {
	return &IngestService{conn: conn, openF1: openF1, circuits: circuits, meetings: meetings, sessions: sessions}
}

// Run ingests every season the upstream covers, oldest first, skipping the ones already stored.
//
// Oldest first is what makes an interrupted run resumable: the seasons behind the failure are
// complete, so the next run starts at the one that failed.
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

	return summary, nil
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
