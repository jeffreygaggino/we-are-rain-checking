package repository

import (
	"context"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/jmoiron/sqlx"

	"github.com/jeffreygaggino/we-are-rain-checking/backend/models"
)

type SeasonRepo struct{}

func NewSeasonRepo() *SeasonRepo { return &SeasonRepo{} }

// raceRow is the flat scan target for a Race and its weather. The weather columns arrive as a
// nullable block that is assembled into a nested object or dropped entirely, and sampleCount is the
// pointer that decides which: the LEFT JOIN yields NULL for a Race with no Weather Samples, which is
// what distinguishes "no samples" from "samples, none of which recorded rain".
type raceRow struct {
	SessionKey       int       `db:"session_key"`
	MeetingKey       int       `db:"meeting_key"`
	RaceName         string    `db:"race_name"`
	DateStart        time.Time `db:"date_start"`
	CircuitShortName string    `db:"circuit_short_name"`
	CountryName      string    `db:"country_name"`
	Location         string    `db:"location"`
	Cancelled        bool      `db:"is_cancelled"`

	SampleCount         *int `db:"sample_count"`
	RainfallSampleCount *int `db:"rainfall_sample_count"`

	WindSampleCount  *int     `db:"wind_sample_count"`
	WindSpeedMeanMps *float64 `db:"wind_speed_mean_mps"`
	WindSpeedMaxMps  *float64 `db:"wind_speed_max_mps"`

	AirTemperatureSampleCount *int     `db:"air_temperature_sample_count"`
	AirTemperatureMeanC       *float64 `db:"air_temperature_mean_c"`

	TrackTemperatureSampleCount *int     `db:"track_temperature_sample_count"`
	TrackTemperatureMeanC       *float64 `db:"track_temperature_mean_c"`
}

// resultRow is one Driver's end result, carrying the Race it belongs to so the assembly can file it.
type resultRow struct {
	SessionKey int `db:"session_key"`

	DriverID     uuid.UUID `db:"driver_id"`
	FullName     string    `db:"full_name"`
	ShortName    string    `db:"short_name"`
	RacingNumber int       `db:"racing_number"`

	Position     *int    `db:"position"`
	Points       float64 `db:"points"`
	NumberOfLaps *int    `db:"number_of_laps"`
	DNF          bool    `db:"dnf"`
	DNS          bool    `db:"dns"`
	DSQ          bool    `db:"dsq"`
}

// Season returns one year's Races in date order, each carrying its weather and its Drivers' end
// results. Cancelled Races are included, flagged, and carry neither.
//
// Two queries rather than one join. A single flat query would repeat each Race and its whole weather
// block once per Driver — about 22 times over — for rows that are then collapsed again in memory,
// and its inner join to the classification would drop a cancelled Race entirely, which is precisely
// the row that has to survive.
//
// Measured with EXPLAIN ANALYZE against the filled dev database, 2026 season:
//
//   - sessions_year_name_idx serves the year + Race filter, as intended (Bitmap Index Scan).
//   - The weather aggregate does NOT use weather_samples' composite key. Postgres reads all 48,714
//     samples sequentially and hash-aggregates them, which is the right call at this size: the
//     season wants 13 Sessions' worth spread through the table, and a sequential read beats that
//     many random ones. Worth knowing the plan is a full scan; not worth an index yet.
//   - session_results and the three seeded tables are scanned whole. All are small enough that
//     their indexes would cost more than they save.
//
// Total 8.8 ms. **No new index**, and none proposed: nothing here is measured slow, and the schema's
// own comment on weather_samples argues against a second index that ingest would carry on every
// write. Revisit if a season's rows or the sample table grow by an order of magnitude.
func (r *SeasonRepo) Season(ctx context.Context, db sqlx.ExtContext, year int) ([]models.SeasonRace, error) {
	raceRows, err := r.races(ctx, db, year)
	if err != nil {
		return nil, err
	}

	resultRows, err := r.results(ctx, db, year)
	if err != nil {
		return nil, err
	}

	// Never nil: a season with nothing stored encodes as [] rather than null.
	races := make([]models.SeasonRace, 0, len(raceRows))
	// Index by Session so results file in one pass rather than a scan per Race.
	at := make(map[int]int, len(raceRows))

	for _, row := range raceRows {
		at[row.SessionKey] = len(races)
		races = append(races, models.SeasonRace{
			SessionKey:       row.SessionKey,
			MeetingKey:       row.MeetingKey,
			RaceName:         row.RaceName,
			DateStart:        row.DateStart,
			CircuitShortName: row.CircuitShortName,
			CountryName:      row.CountryName,
			Location:         row.Location,
			Cancelled:        row.Cancelled,
			Weather:          row.weather(),
			// Never nil, so a caller ranges over it without a guard. A cancelled Race keeps the
			// empty slice it starts with.
			Results: []models.DriverResult{},
		})
	}

	for _, row := range resultRows {
		i, ok := at[row.SessionKey]
		if !ok {
			// Unreachable while both queries filter on the same season: a result whose Race is not
			// in the list would be dropped silently, so it is named rather than ignored.
			return nil, fmt.Errorf("season_repo.Season(%d): result for session %d, which is not a Race of that season",
				year, row.SessionKey)
		}
		races[i].Results = append(races[i].Results, models.DriverResult{
			DriverID:     row.DriverID,
			FullName:     row.FullName,
			ShortName:    row.ShortName,
			RacingNumber: row.RacingNumber,
			Position:     row.Position,
			Points:       row.Points,
			NumberOfLaps: row.NumberOfLaps,
			DNF:          row.DNF,
			DNS:          row.DNS,
			DSQ:          row.DSQ,
		})
	}

	return races, nil
}

// races reads the season's Races with their weather, cancelled ones included.
//
// The LEFT JOIN is what lets a Race with no Weather Samples through at all — an inner join would
// drop every cancelled Race, and every Race whose samples have not landed yet.
//
// COUNT(*) counts rows; COUNT(col) counts non-NULL values. wind_speed, air_temperature and
// track_temperature are independently nullable, so each mean is reported beside the count it was
// actually computed over rather than beside the Session's total.
func (r *SeasonRepo) races(ctx context.Context, db sqlx.ExtContext, year int) ([]raceRow, error) {
	const q = `
		WITH race AS (
			SELECT s.session_key, s.meeting_key, s.date_start, s.is_cancelled,
			       m.name AS race_name, m.country_name, m.location,
			       c.short_name AS circuit_short_name
			FROM f1.sessions s
			JOIN f1.meetings m ON m.meeting_key = s.meeting_key
			JOIN f1.circuits c ON c.id = s.circuit_id
			WHERE s.year = $1
			  AND s.session_name = $2
		),
		weather AS (
			SELECT w.session_key,
			       COUNT(*)                            AS sample_count,
			       COUNT(*) FILTER (WHERE w.rainfall)  AS rainfall_sample_count,
			       COUNT(w.wind_speed)                 AS wind_sample_count,
			       AVG(w.wind_speed)                   AS wind_speed_mean_mps,
			       MAX(w.wind_speed)                   AS wind_speed_max_mps,
			       COUNT(w.air_temperature)            AS air_temperature_sample_count,
			       AVG(w.air_temperature)              AS air_temperature_mean_c,
			       COUNT(w.track_temperature)          AS track_temperature_sample_count,
			       AVG(w.track_temperature)            AS track_temperature_mean_c
			FROM f1.weather_samples w
			JOIN race r ON r.session_key = w.session_key
			GROUP BY w.session_key
		)
		SELECT r.session_key, r.meeting_key, r.race_name, r.date_start, r.is_cancelled,
		       r.circuit_short_name, r.country_name, r.location,
		       wx.sample_count, wx.rainfall_sample_count,
		       wx.wind_sample_count, wx.wind_speed_mean_mps, wx.wind_speed_max_mps,
		       wx.air_temperature_sample_count, wx.air_temperature_mean_c,
		       wx.track_temperature_sample_count, wx.track_temperature_mean_c
		FROM race r
		LEFT JOIN weather wx ON wx.session_key = r.session_key
		ORDER BY r.date_start`

	var rows []raceRow
	if err := sqlx.SelectContext(ctx, db, &rows, q, year, models.SessionNameRace); err != nil {
		return nil, fmt.Errorf("season_repo.races(%d): %w", year, err)
	}
	return rows, nil
}

// results reads every end result of the season's Races, ordered as they are classified.
//
// Cancelled Races are excluded here, on the flag. A Race that did not happen has no classification;
// if the upstream ever attached one it would be an artefact, and reading it would put finishers in a
// Race nobody drove. Filtering on `is_cancelled` rather than on an absence of rows is the same
// choice the Race query makes, so the two cannot disagree about which Races ran.
//
// Retirements sort last: position is NULL for them, and NULLS LAST keeps the classification in
// finishing order with the non-finishers behind it, rather than leading with them.
func (r *SeasonRepo) results(ctx context.Context, db sqlx.ExtContext, year int) ([]resultRow, error) {
	const q = `
		SELECT sr.session_key,
		       d.id AS driver_id, d.full_name, d.short_name, sr.racing_number,
		       sr.position, sr.points, sr.number_of_laps, sr.dnf, sr.dns, sr.dsq
		FROM f1.session_results sr
		JOIN f1.sessions s ON s.session_key = sr.session_key
		JOIN f1.drivers d ON d.id = sr.driver_id
		WHERE s.year = $1
		  AND s.session_name = $2
		  AND NOT s.is_cancelled
		ORDER BY s.date_start, sr.position NULLS LAST, d.full_name`

	var rows []resultRow
	if err := sqlx.SelectContext(ctx, db, &rows, q, year, models.SessionNameRace); err != nil {
		return nil, fmt.Errorf("season_repo.results(%d): %w", year, err)
	}
	return rows, nil
}

// weather assembles the nested block, or returns nil when the Race has no Weather Samples.
//
// Guarded on sample_count being present rather than on it being greater than zero: the aggregate
// only produces a row when the Session has samples, so NULL is the absent case and a zero could
// only come from a bug. Returning a zeroed block instead would report a still, freezing, dry Race.
func (row raceRow) weather() *models.RaceWeather {
	if row.SampleCount == nil {
		return nil
	}
	return &models.RaceWeather{
		SampleCount:                 *row.SampleCount,
		RainfallSampleCount:         derefInt(row.RainfallSampleCount),
		WindSampleCount:             derefInt(row.WindSampleCount),
		WindSpeedMeanMps:            row.WindSpeedMeanMps,
		WindSpeedMaxMps:             row.WindSpeedMaxMps,
		AirTemperatureSampleCount:   derefInt(row.AirTemperatureSampleCount),
		AirTemperatureMeanC:         row.AirTemperatureMeanC,
		TrackTemperatureSampleCount: derefInt(row.TrackTemperatureSampleCount),
		TrackTemperatureMeanC:       row.TrackTemperatureMeanC,
	}
}

// derefInt reads a count that COUNT() cannot actually return NULL for. The pointers exist because
// the LEFT JOIN can null the whole block at once; once sample_count is present the rest are too.
func derefInt(n *int) int {
	if n == nil {
		return 0
	}
	return *n
}
