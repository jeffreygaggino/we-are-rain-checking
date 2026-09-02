package client

import (
	"context"
	"sync"
	"time"
)

// pacer spaces outbound calls so a run of ingest stays under an upstream's rate limit.
//
// It reserves the next free instant under the lock and then waits for it, rather than sleeping for
// the interval after each call. The difference matters when two calls arrive together: reserving
// hands them distinct slots, where sleeping would let both wake at once and burst.
type pacer struct {
	mu   sync.Mutex
	min  time.Duration
	next time.Time
}

// wait blocks until this caller's slot. A cancelled context returns its error instead of finishing
// the wait, which is what makes an interrupted ingest stop within one interval rather than one run.
func (p *pacer) wait(ctx context.Context) error {
	p.mu.Lock()
	slot := p.next
	if now := time.Now(); slot.Before(now) {
		slot = now
	}
	p.next = slot.Add(p.min)
	p.mu.Unlock()

	delay := time.Until(slot)
	if delay <= 0 {
		// Still an interruption point: an unpaced client must not run a whole season after a cancel.
		return ctx.Err()
	}

	timer := time.NewTimer(delay)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-timer.C:
		return nil
	}
}
