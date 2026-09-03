// Command ingest fills the Meetings, Sessions and Weather Samples tables from OpenF1.
//
// It is a scheduled binary and never an HTTP route. That is what makes the absent auth layer safe
// rather than negligent: the one operation worth protecting is not on the API at all. The two
// decisions stand or fall together (ADR-0001).
//
// Re-running is safe and cheap — a season already stored, and a Session that already has Weather
// Samples, are both skipped without an upstream call — so the recovery from any failure below is to
// run it again.
package main

import (
	"context"
	"errors"
	"fmt"
	"log"
	"os"
	"os/signal"
	"syscall"

	"github.com/jeffreygaggino/we-are-rain-checking/backend/app"
	"github.com/jeffreygaggino/we-are-rain-checking/backend/client"
	"github.com/jeffreygaggino/we-are-rain-checking/backend/config"
	"github.com/jeffreygaggino/we-are-rain-checking/backend/db"
)

// main holds only the exit code. The work is in run, because log.Fatal skips deferred functions and
// this binary has a pool and a signal handler to let go of.
func main() {
	if err := run(); err != nil {
		// Ingest failures live here and in the exit code. Nothing about them reaches the API, which
		// serves whatever is stored and says so.
		log.Printf("ingest: %v", err)
		os.Exit(1)
	}
}

func run() error {
	config.LoadConfig()
	cfg := config.GetConfig()

	db.ConnectDatabase(cfg)
	defer func() { _ = db.DB.Close() }()

	// A signal cancels the context rather than killing the process, so an interrupt stops between
	// seasons and leaves the last one whole. The next run picks up there.
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	openF1 := client.NewOpenF1Client(cfg.OpenF1BaseURL, cfg.UpstreamTimeout, cfg.IngestMinInterval)

	summary, err := app.NewIngest(db.DB, openF1).Run(ctx)

	// An interrupt is a successful stop, not a failure. A scheduler draining this binary with
	// SIGTERM leaves exactly the state the design intends — whole seasons, resumable — and a
	// non-zero exit there would page someone about a working system.
	if errors.Is(err, context.Canceled) {
		log.Printf("ingest: interrupted after seasons %v (skipped %v) and %d weather samples from "+
			"%d sessions — re-run to resume",
			summary.SeasonsIngested, summary.SeasonsSkipped, summary.WeatherSamples, summary.WeatherSessions)
		return nil
	}
	if err != nil {
		return fmt.Errorf("%w (ingested seasons %v, skipped %v, and %d weather samples from %d sessions "+
			"before failing)",
			err, summary.SeasonsIngested, summary.SeasonsSkipped, summary.WeatherSamples, summary.WeatherSessions)
	}

	log.Printf("ingest: complete — %d meetings and %d sessions across %v, skipped %v; "+
		"%d weather samples from %d sessions, skipped %d",
		summary.Meetings, summary.Sessions, summary.SeasonsIngested, summary.SeasonsSkipped,
		summary.WeatherSamples, summary.WeatherSessions, summary.WeatherSessionsSkipped)
	return nil
}
