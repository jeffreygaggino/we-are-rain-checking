package repository

import (
	"context"
	"fmt"

	"github.com/jmoiron/sqlx"

	"github.com/jeffreygaggino/we-are-rain-checking/backend/models"
)

type SessionRepo struct{}

func NewSessionRepo() *SessionRepo { return &SessionRepo{} }

// Upsert writes one Session, keyed on the upstream's session_key. See MeetingRepo.Upsert for why
// this updates rather than doing nothing on conflict.
func (r *SessionRepo) Upsert(ctx context.Context, db sqlx.ExtContext, s *models.Session) error {
	const q = `
		INSERT INTO f1.sessions (session_key, meeting_key, circuit_id, session_type, session_name,
		                         year, date_start, date_end, is_cancelled)
		VALUES (:session_key, :meeting_key, :circuit_id, :session_type, :session_name,
		        :year, :date_start, :date_end, :is_cancelled)
		ON CONFLICT (session_key) DO UPDATE SET
			meeting_key  = EXCLUDED.meeting_key,
			circuit_id   = EXCLUDED.circuit_id,
			session_type = EXCLUDED.session_type,
			session_name = EXCLUDED.session_name,
			year         = EXCLUDED.year,
			date_start   = EXCLUDED.date_start,
			date_end     = EXCLUDED.date_end,
			is_cancelled = EXCLUDED.is_cancelled,
			updated_at   = NOW()`

	if _, err := sqlx.NamedExecContext(ctx, db, q, s); err != nil {
		return fmt.Errorf("session_repo.Upsert(%d): %w", s.SessionKey, err)
	}
	return nil
}

// CountsByYear reports how many Sessions each season already holds. Paired with the Meetings tally,
// it is what tells a run which seasons it can skip.
func (r *SessionRepo) CountsByYear(ctx context.Context, db sqlx.ExtContext) (map[int]int, error) {
	const q = `SELECT year, COUNT(*) FROM f1.sessions GROUP BY year`

	counts, err := countsByYear(ctx, db, q)
	if err != nil {
		return nil, fmt.Errorf("session_repo.CountsByYear: %w", err)
	}
	return counts, nil
}
