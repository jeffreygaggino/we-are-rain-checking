package services

import (
	"context"
	"fmt"
	"time"

	"github.com/jmoiron/sqlx"

	"github.com/jeffreygaggino/we-are-rain-checking/backend/models"
	"github.com/jeffreygaggino/we-are-rain-checking/backend/repository"
)

// SeasonRange is every season this service carries: FirstSeason to the current year, taken from the
// clock rather than configured, so the range grows on its own (CONTEXT.md).
//
// It shares FirstSeason with ingest deliberately. Two definitions of "which seasons exist" would let
// the endpoint refuse a season ingest had already stored.
func SeasonRange() (first, last int) {
	return FirstSeason, time.Now().UTC().Year()
}

type SeasonService struct {
	conn    *sqlx.DB
	seasons *repository.SeasonRepo
}

func NewSeasonService(conn *sqlx.DB, seasons *repository.SeasonRepo) *SeasonService {
	return &SeasonService{conn: conn, seasons: seasons}
}

// Season returns one year's Races in date order, each carrying its weather and its Drivers' results.
//
// A year outside the Season Range is ErrSeasonOutOfRange, not an empty result. A year inside it with
// nothing stored is empty — 2019 cannot exist and next March merely has not happened, and answering
// both the same way would report them as the same thing.
//
// The sentinel is wrapped with %w so it still matches an errors.Is check at the handler, which is
// the one place a status code is chosen for this resource.
//
// Cancelled Races are included and flagged, carrying no weather and no results — the season
// calendar has a slot for a Race that was called off, and hiding it would leave a silent gap
// between two weekends.
func (s *SeasonService) Season(ctx context.Context, year int) (models.Season, error) {
	first, last := SeasonRange()
	if year < first || year > last {
		return models.Season{}, fmt.Errorf("services.SeasonService.Season(%d): %w", year, models.ErrSeasonOutOfRange)
	}

	races, err := s.seasons.Season(ctx, s.conn, year)
	if err != nil {
		return models.Season{}, err
	}

	return models.Season{Races: races}, nil
}
