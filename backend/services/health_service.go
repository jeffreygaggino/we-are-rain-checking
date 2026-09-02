package services

import (
	"context"
	"fmt"

	"github.com/jmoiron/sqlx"

	"github.com/jeffreygaggino/we-are-rain-checking/backend/db"
	"github.com/jeffreygaggino/we-are-rain-checking/backend/models"
)

// HealthService answers whether this service's dependencies are reachable.
type HealthService struct {
	conn *sqlx.DB
}

func NewHealthService(conn *sqlx.DB) *HealthService {
	return &HealthService{conn: conn}
}

// Check verifies every dependency the service cannot answer a request without. Postgres is the only
// one: the two upstreams are reached per-request and degrade there, named in that response (#9).
//
// It calls db.Ping rather than issuing its own SELECT because Ping already bounds its wait — an
// unreachable database has to answer the health check, not hang it.
func (s *HealthService) Check(ctx context.Context) (models.HealthReport, error) {
	if err := db.Ping(ctx, s.conn); err != nil {
		return models.HealthReport{}, fmt.Errorf("services.HealthService.Check: %w", err)
	}
	return models.HealthReport{Database: models.HealthOK}, nil
}
