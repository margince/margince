// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package dnsread

import (
	"context"
	"sync"
	"time"
)

// Pacer holds this installation to one resolver query per interval.
//
// It is the single-requester shape, not the per-site one: webread.Pacer paces
// toward the SITE being read and runs eight in flight because that budget
// belongs to each site, while every query here goes to the same resolver
// whatever domain it asks about. That makes the installation one requester, so
// Wait serializes it.
//
// It is not shared with the geocode pacer of the same shape because the two
// answer to different policies — Nominatim's usage policy fixes that one's
// floor, this one's is ours to choose — and a shared helper would invite
// changing one budget while reading the other's justification.
//
// Safe for concurrent use; concurrent callers queue.
type Pacer struct {
	mu        sync.Mutex
	interval  time.Duration
	lastStart time.Time

	// now and sleep are seams so the pacing is provable without a real clock —
	// a test that had to wait for a real interval to prove a floor would be
	// skipped, and a skipped test proves nothing.
	now   func() time.Time
	sleep func(ctx context.Context, d time.Duration) error
}

// NewPacer builds a real-clock pacer at the given floor.
func NewPacer(interval time.Duration) *Pacer {
	return &Pacer{interval: interval, now: time.Now, sleep: sleepCtx}
}

// Wait blocks until this installation may make its next query.
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

// sleepCtx sleeps, or gives up when the caller does — a query nobody is
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
