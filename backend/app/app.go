// Package app is the dependency graph, in one function, so main and the HTTP test harness build the
// same router. Later tickets add their endpoint here, once. See plans/01-backend-v1.md, Deviation 5.
package app

import (
	"github.com/gin-gonic/gin"
	"github.com/jmoiron/sqlx"

	"github.com/jeffreygaggino/we-are-rain-checking/backend/handlers"
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
