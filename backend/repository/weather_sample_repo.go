package repository

import (
	"context"
	"fmt"

	"github.com/jmoiron/sqlx"

	"github.com/jeffreygaggino/we-are-rain-checking/backend/models"
)

type WeatherSampleRepo struct{}

func NewWeatherSampleRepo() *WeatherSampleRepo { return &WeatherSampleRepo{} }

// Upsert writes one Weather Sample, keyed on its Session and observation time (ADR-0002).
//
// One row per call rather than a multi-row INSERT: Postgres refuses to let one statement's
// ON CONFLICT DO UPDATE touch the same row twice, so a batch would turn a repeated `date` in the
// upstream's own response into a failed ingest instead of an idempotent write.
func (r *WeatherSampleRepo) Upsert(ctx context.Context, db sqlx.ExtContext, s *models.WeatherSample) error {
	const q = `
		INSERT INTO f1.weather_samples (session_key, observed_at, rainfall, air_temperature,
		                                track_temperature, humidity, pressure, wind_speed, wind_direction)
		VALUES (:session_key, :observed_at, :rainfall, :air_temperature,
		        :track_temperature, :humidity, :pressure, :wind_speed, :wind_direction)
		ON CONFLICT (session_key, observed_at) DO UPDATE SET
			rainfall          = EXCLUDED.rainfall,
			air_temperature   = EXCLUDED.air_temperature,
			track_temperature = EXCLUDED.track_temperature,
			humidity          = EXCLUDED.humidity,
			pressure          = EXCLUDED.pressure,
			wind_speed        = EXCLUDED.wind_speed,
			wind_direction    = EXCLUDED.wind_direction,
			updated_at        = NOW()`

	if _, err := sqlx.NamedExecContext(ctx, db, q, s); err != nil {
		return fmt.Errorf("weather_sample_repo.Upsert(%d, %s): %w", s.SessionKey, s.ObservedAt, err)
	}
	return nil
}

// SessionKeysWithSamples reports which Sessions already hold Weather Samples. It is what makes
// resumption derived rather than checkpointed at this stage: the rows themselves say which Sessions
// have landed, so an interrupted run picks up at the first one with nothing stored.
//
// A set rather than a tally: how many samples a Session has is not a question anything asks, and a
// count would invite a threshold nobody has justified.
func (r *WeatherSampleRepo) SessionKeysWithSamples(ctx context.Context, db sqlx.ExtContext) (map[int]bool, error) {
	const q = `SELECT DISTINCT session_key FROM f1.weather_samples`

	var sessionKeys []int
	if err := sqlx.SelectContext(ctx, db, &sessionKeys, q); err != nil {
		return nil, fmt.Errorf("weather_sample_repo.SessionKeysWithSamples: %w", err)
	}

	keys := make(map[int]bool, len(sessionKeys))
	for _, sessionKey := range sessionKeys {
		keys[sessionKey] = true
	}
	return keys, nil
}
