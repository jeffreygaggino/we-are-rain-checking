package repository

import (
	"context"
	"fmt"

	"github.com/jmoiron/sqlx"

	"github.com/jeffreygaggino/we-are-rain-checking/backend/models"
)

type SessionResultRepo struct{}

func NewSessionResultRepo() *SessionResultRepo { return &SessionResultRepo{} }

// Upsert writes one classification, keyed on the Session and the Driver — never on the Racing
// Number, which belongs to a season rather than to a person (ADR-0003).
//
// One row per call rather than a multi-row INSERT, for the reason WeatherSampleRepo.Upsert gives:
// one statement's ON CONFLICT DO UPDATE cannot touch the same row twice, so an upstream repeating a
// Driver would fail the ingest instead of writing the row once.
func (r *SessionResultRepo) Upsert(ctx context.Context, db sqlx.ExtContext, result *models.SessionResult) error {
	const q = `
		INSERT INTO f1.session_results (session_key, driver_id, racing_number, position, points,
		                                number_of_laps, dnf, dns, dsq)
		VALUES (:session_key, :driver_id, :racing_number, :position, :points,
		        :number_of_laps, :dnf, :dns, :dsq)
		ON CONFLICT (session_key, driver_id) DO UPDATE SET
			racing_number  = EXCLUDED.racing_number,
			position       = EXCLUDED.position,
			points         = EXCLUDED.points,
			number_of_laps = EXCLUDED.number_of_laps,
			dnf            = EXCLUDED.dnf,
			dns            = EXCLUDED.dns,
			dsq            = EXCLUDED.dsq,
			updated_at     = NOW()`

	if _, err := sqlx.NamedExecContext(ctx, db, q, result); err != nil {
		return fmt.Errorf("session_result_repo.Upsert(%d, %s): %w", result.SessionKey, result.DriverID, err)
	}
	return nil
}

// SessionKeysWithResults reports which Sessions already hold a classification. It is what makes
// resumption derived rather than checkpointed at this stage, exactly as
// WeatherSampleRepo.SessionKeysWithSamples does one stage earlier: the rows say which Races landed.
func (r *SessionResultRepo) SessionKeysWithResults(ctx context.Context, db sqlx.ExtContext) (map[int]bool, error) {
	const q = `SELECT DISTINCT session_key FROM f1.session_results`

	var sessionKeys []int
	if err := sqlx.SelectContext(ctx, db, &sessionKeys, q); err != nil {
		return nil, fmt.Errorf("session_result_repo.SessionKeysWithResults: %w", err)
	}

	keys := make(map[int]bool, len(sessionKeys))
	for _, sessionKey := range sessionKeys {
		keys[sessionKey] = true
	}
	return keys, nil
}
