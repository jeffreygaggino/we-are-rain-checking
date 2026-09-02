// Command api serves this service's HTTP API.
//
// It never migrates on boot — applying the schema is `cmd/migrate`, an explicit action with its own
// output. Boot connects, and fails loudly if it cannot.
package main

import (
	"log"

	"github.com/jeffreygaggino/we-are-rain-checking/backend/app"
	"github.com/jeffreygaggino/we-are-rain-checking/backend/config"
	"github.com/jeffreygaggino/we-are-rain-checking/backend/db"
	"github.com/jeffreygaggino/we-are-rain-checking/backend/docs"
)

// @title			we-are-rain-checking API
// @version		0.1
// @description	An honest weather-to-results service over four F1 seasons.
// @BasePath		/api/v1
func main() {
	config.LoadConfig()
	cfg := config.GetConfig()

	// Fatal on an unreachable database at boot: the service answers nothing without it, and a
	// process that starts and then 503s every request is harder to diagnose than one that does not
	// start. The health route covers the database going away afterwards.
	db.ConnectDatabase(cfg)

	// Where a proxy publishes this API, which is not where the router serves it.
	docs.SwaggerInfo.Host = cfg.PublicURL
	docs.SwaggerInfo.BasePath = cfg.APIBasePath

	router := app.New(db.DB)

	log.Printf("api: listening on %s", config.Address())
	if err := router.Run(config.Address()); err != nil {
		log.Fatalf("api: %v", err)
	}
}
