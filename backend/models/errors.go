package models

import (
	"errors"
	"fmt"
)

var (
	// ErrNoUpcomingRace is a real answer, not a fault: the season's last Race has run.
	ErrNoUpcomingRace = errors.New("no upcoming race")

	// ErrCircuitNotFound means ingest saw a circuit_key with no seeded circuit. Circuits are
	// repo-owned, so this is a seed gap rather than upstream noise.
	ErrCircuitNotFound = errors.New("circuit not found")

	// ErrDriverNotResolved is ADR-0003 executing. An upstream name we have not seeded aborts
	// ingest rather than inserting a driver, because a new row here silently splits or merges a
	// person's results.
	ErrDriverNotResolved = errors.New("driver not resolved")

	// ErrUpstreamEmpty means a season that has ended came back with nothing. OpenF1 answers a wrong
	// base URL with the same "No results found." it uses for a season it does not carry, so this is
	// the only thing standing between a misconfiguration and a run that reports success over an
	// empty table. A season still in progress is exempt: it is empty until its first Meeting.
	ErrUpstreamEmpty = errors.New("upstream returned no data for a completed season")

	// ErrUnknownAxis is a bad request on the correlation endpoint.
	ErrUnknownAxis = errors.New("unknown correlation axis")
)

// UpstreamError carries an upstream's own status and message so a handler can name the dependency
// that failed instead of flattening every fault into one gateway error.
type UpstreamError struct {
	Upstream string
	Status   int
	Message  string
	Err      error
}

func (e *UpstreamError) Error() string {
	if e.Status != 0 {
		return fmt.Sprintf("%s: upstream returned %d: %s", e.Upstream, e.Status, e.Message)
	}
	return fmt.Sprintf("%s: %s", e.Upstream, e.Message)
}

func (e *UpstreamError) Unwrap() error { return e.Err }
