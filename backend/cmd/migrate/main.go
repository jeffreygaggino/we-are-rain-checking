// Command migrate applies or rolls back this service's schema.
//
// It is a separate binary rather than a startup step so that "apply the schema" is an explicit,
// observable action. The API binary never migrates on boot.
package main

import (
	"flag"
	"log"

	"github.com/jeffreygaggino/we-are-rain-checking/backend/config"
	"github.com/jeffreygaggino/we-are-rain-checking/backend/db"
)

func main() {
	direction := flag.String("direction", "up", "up or down")
	flag.Parse()

	config.LoadConfig()
	cfg := config.GetConfig()

	switch *direction {
	case "up":
		if err := db.MigrateUp(cfg); err != nil {
			log.Fatalf("migrate: up (%s): %v", db.Describe(cfg), err)
		}
	case "down":
		if err := db.MigrateDown(cfg); err != nil {
			log.Fatalf("migrate: down (%s): %v", db.Describe(cfg), err)
		}
	default:
		log.Fatalf("migrate: unknown direction %q, want up or down", *direction)
	}

	version, dirty, err := db.MigrateVersion(cfg)
	if err != nil {
		log.Fatalf("migrate: version (%s): %v", db.Describe(cfg), err)
	}
	log.Printf("migrate: %s complete, version=%d dirty=%v (%s)", *direction, version, dirty, db.Describe(cfg))
}
