// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package vatcheck

import (
	"context"
	"sync"
	"time"
)

// Pacer holds this installation to one VAT consultation per interval.
//
// It differs from webread.Pacer in the one way that matters: there is NO
// concurrency, deliberately. The site reader paces per target site and may run
// several in flight, because the budget belongs to each site, while every
// consultation here goes to the same EU service whatever number it asks about.
// That makes the installation one requester, so Wait serializes it.
//
// It is not shared with the certlog, dnsread and geocode pacers of the same
// shape, for the reason those state about each other: each answers to a
// different policy. Nominatim's floor is fixed by published terms; this one is
// our reading of a service whose terms describe occasional verification and
// name no number. A shared helper would invite changing one budget while
// reading another's justification.
//
// Safe for concurrent use; concurrent callers queue.
type Pacer struct {
	mu        sync.Mutex
	interval  time.Duration
	lastStart time.Time

	// now and sleep are seams so the pacing is provable without a real clock —
	// a test that had to wait two seconds to prove a two-second floor would be
	// skipped, and a skipped test proves nothing.
	now   func() time.Time
	sleep func(ctx context.Context, d time.Duration) error
}

// NewPacer builds a real-clock pacer at the given floor.
func NewPacer(interval time.Duration) *Pacer {
	return &Pacer{interval: interval, now: time.Now, sleep: sleepCtx}
}

// Wait blocks until this installation may make its next consultation.
//
// It holds the lock ACROSS the sleep, which is what makes the pacer a queue
// rather than a suggestion: releasing it first would let every waiting caller
// compute the same deadline and start together, which is precisely the burst
// the interval exists to prevent.
func (p *Pacer) Wait(ctx context.Context) error {
	p.mu.Lock()
	defer p.mu.Unlock()

	if !p.lastStart.IsZero() {
		if wait := p.interval - p.now().Sub(p.lastStart); wait > 0 {
			if err := p.sleep(ctx, wait); err != nil {
				return err
			}
		}
	}
	p.lastStart = p.now()
	return nil
}

// sleepCtx sleeps, or gives up when the caller does — a consultation nobody is
// waiting for any more must not hold the queue.
func sleepCtx(ctx context.Context, d time.Duration) error {
	timer := time.NewTimer(d)
	defer timer.Stop()
	select {
	case <-timer.C:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}
