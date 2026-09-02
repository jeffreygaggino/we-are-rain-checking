// Package app is the dependency graph, in one function, so main and the HTTP test harness build the
// same router. Later tickets add their endpoint here, once. See plans/01-backend-v1.md, Deviation 5.
package app

import (
	"github.com/gin-gonic/gin"
	"github.com/jmoiron/sqlx"

	"github.com/jeffreygaggino/we-are-rain-checking/backend/client"
	"github.com/jeffreygaggino/we-are-rain-checking/backend/handlers"
	"github.com/jeffreygaggino/we-are-rain-checking/backend/repository"
	"github.com/jeffreygaggino/we-are-rain-checking/backend/routes"
	"github.com/jeffreygaggino/we-are-rain-checking/backend/services"
)

// New wires repositories, then services, then handlers, then the router — the one order, always.
//
// The pool is an argument rather than db.DB so a caller can hand it a database that is not there.
func New(conn *sqlx.DB) *gin.Engine {
	// Services
	healthService := services.NewHealthService(conn)

	// Handlers
	healthHandler := handlers.NewHealthHandler(healthService)

	return routes.SetupRouter(healthHandler)
}

// NewIngest wires the ingest graph, for the same reason New wires the router: cmd/ingest and the
// ingest test seam then build the same service, and a repository added here reaches both.
//
// The client is an argument rather than built from config, so a test can point ingest at a stub
// upstream without going through the environment.
func NewIngest(conn *sqlx.DB, openF1 *client.OpenF1Client) *services.IngestService {
	// Repositories
	circuitRepo := repository.NewCircuitRepo()
	meetingRepo := repository.NewMeetingRepo()
	sessionRepo := repository.NewSessionRepo()

	// Services
	return services.NewIngestService(conn, openF1, circuitRepo, meetingRepo, sessionRepo)
}
