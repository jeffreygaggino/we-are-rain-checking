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

	conn, err := db.Connect(cfg)
	if err != nil {
		log.Fatalf("migrate: connect (%s): %v", db.Describe(cfg), err)
	}
	defer func() { _ = conn.Close() }()

	switch *direction {
	case "up":
		if err := db.MigrateUp(conn, cfg); err != nil {
			log.Fatalf("migrate: up: %v", err)
		}
	case "down":
		if err := db.MigrateDown(conn, cfg); err != nil {
			log.Fatalf("migrate: down: %v", err)
		}
	default:
		log.Fatalf("migrate: unknown direction %q, want up or down", *direction)
	}

	version, dirty, err := db.MigrateVersion(conn, cfg)
	if err != nil {
		log.Fatalf("migrate: version: %v", err)
	}
	log.Printf("migrate: %s complete, version=%d dirty=%v (%s)", *direction, version, dirty, db.Describe(cfg))
}
