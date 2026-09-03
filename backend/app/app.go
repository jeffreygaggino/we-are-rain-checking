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
//
// The trusted proxies are an argument for a second reason: the policy has to belong to the router
// rather than to main. The test harness builds its router here too, so a policy applied after this
// function returned would leave tests trusting every proxy while the deployed service trusted none —
// and the first handler to read ClientIP() would be tested against the wrong answer. Empty trusts
// nothing, which makes ClientIP() the peer address rather than a caller's X-Forwarded-For.
func New(conn *sqlx.DB, trustedProxies []string) (*gin.Engine, error) {
	// Services
	healthService := services.NewHealthService(conn)

	// Handlers
	healthHandler := handlers.NewHealthHandler(healthService)

	router := routes.SetupRouter(healthHandler)

	// gin validates the entries, so the loader does not: a second parser would be a second
	// definition of "valid CIDR", free to drift from the one that actually decides.
	//
	// The error travels back rather than exiting here. This function is the one both main and the
	// harness build through, so a log.Fatalf would let a test kill its own binary — and would put
	// the boot failure this comment relies on beyond the reach of any test.
	// Unwrapped: both callers name the source themselves — main the environment variable, the
	// harness the list it passed — and a wrap here only stutters in front of gin's own message.
	if err := router.SetTrustedProxies(trustedProxies); err != nil {
		return nil, err
	}

	return router, nil
}

// NewIngest wires the ingest graph, for the same reason New wires the router: cmd/ingest and the
// ingest test seam then build the same service, and a repository added here reaches both.
//
// The client is an argument rather than built from config, so a test can point ingest at a stub
// upstream without going through the environment.
func NewIngest(conn *sqlx.DB, openF1 *client.OpenF1Client) *services.IngestService {
	// Repositories
	circuitRepo := repository.NewCircuitRepo()
	driverRepo := repository.NewDriverRepo()
	meetingRepo := repository.NewMeetingRepo()
	sessionRepo := repository.NewSessionRepo()
	weatherRepo := repository.NewWeatherSampleRepo()
	resultRepo := repository.NewSessionResultRepo()

	// Services
	return services.NewIngestService(conn, openF1, circuitRepo, driverRepo, meetingRepo, sessionRepo,
		weatherRepo, resultRepo)
}
