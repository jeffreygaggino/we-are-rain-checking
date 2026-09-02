package repository

import (
	"context"
	"fmt"

	"github.com/jmoiron/sqlx"

	"github.com/jeffreygaggino/we-are-rain-checking/backend/models"
)

type MeetingRepo struct{}

func NewMeetingRepo() *MeetingRepo { return &MeetingRepo{} }

// Upsert writes one Meeting, keyed on the upstream's own identifier (ADR-0002).
//
// DO UPDATE rather than DO NOTHING: the upstream corrects rows — an official name filled in, a date
// moved — and re-ingest is how those corrections arrive. Nothing is ever deleted, which is why the
// table has no deleted_at to filter.
func (r *MeetingRepo) Upsert(ctx context.Context, db sqlx.ExtContext, m *models.Meeting) error {
	const q = `
		INSERT INTO f1.meetings (meeting_key, year, name, official_name, circuit_id, country_name,
		                         location, date_start)
		VALUES (:meeting_key, :year, :name, :official_name, :circuit_id, :country_name,
		        :location, :date_start)
		ON CONFLICT (meeting_key) DO UPDATE SET
			year          = EXCLUDED.year,
			name          = EXCLUDED.name,
			official_name = EXCLUDED.official_name,
			circuit_id    = EXCLUDED.circuit_id,
			country_name  = EXCLUDED.country_name,
			location      = EXCLUDED.location,
			date_start    = EXCLUDED.date_start,
			updated_at    = NOW()`

	if _, err := sqlx.NamedExecContext(ctx, db, q, m); err != nil {
		return fmt.Errorf("meeting_repo.Upsert(%d): %w", m.MeetingKey, err)
	}
	return nil
}

// CountsByYear reports how many Meetings each season already holds. It is half of what makes
// resumption derived rather than checkpointed: the rows themselves say which seasons have landed.
func (r *MeetingRepo) CountsByYear(ctx context.Context, db sqlx.ExtContext) (map[int]int, error) {
	const q = `SELECT year, COUNT(*) FROM f1.meetings GROUP BY year`

	counts, err := countsByYear(ctx, db, q)
	if err != nil {
		return nil, fmt.Errorf("meeting_repo.CountsByYear: %w", err)
	}
	return counts, nil
}
