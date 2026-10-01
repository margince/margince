// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package gmail

// How one backfill page walks its messages: a few fetched at once, every
// capture written one at a time in the page's own order, a rate limit waited
// out by the whole pool, and a message the capture refuses walked past rather
// than ending the import.

import (
	"context"
	"errors"
	"sync"
	"time"

	"github.com/margince/margince/backend/internal/shared/kernel/backoff"
	"github.com/margince/margince/backend/internal/shared/ports/connector"
)

const (
	// backfillFetchWorkers is how many messages one page fetches at once. Gmail
	// meters a mailbox per second, so a few in flight cover the round trip of
	// each without spending the quota faster than a rate limit gives back.
	backfillFetchWorkers = 4

	// backfillLookahead bounds how far the fetches may run ahead of the
	// captures. A full message can be tens of megabytes, so a slow capture must
	// not let a whole page of downloads pile up in memory.
	backfillLookahead = 2 * backfillFetchWorkers

	// rateWaitInPage is the longest wait a page takes inside itself when Gmail
	// says slow down. A longer wait ends the page instead, and the engine comes
	// back after it, holding no worker meanwhile.
	rateWaitInPage = time.Minute

	// rateRetriesInPage bounds how often one call is retried after a rate limit
	// before the page gives up and the engine's ladder takes over.
	rateRetriesInPage = 5

	// rateBackoffBase and rateBackoffCap are the short ladder used when Gmail
	// rate-limits without saying how long to wait.
	rateBackoffBase = time.Second
	rateBackoffCap  = 30 * time.Second

	// failedInARowEndsPage ends a page whose messages keep failing one after
	// another. One message the capture refuses is that message's problem; a run
	// of them is almost always the database or the capture itself, and walking
	// past the rest would skip messages that would capture fine later.
	failedInARowEndsPage = 5
)

// rateGate holds every fetch of a page while Gmail has asked the mailbox to
// wait. A rate limit is a statement about the mailbox, not about one message,
// so the pool waits together rather than each worker discovering it in turn.
type rateGate struct {
	mu    sync.Mutex
	until time.Time
}

// wait blocks until the gate is open or ctx ends.
func (g *rateGate) wait(ctx context.Context) error {
	for {
		g.mu.Lock()
		d := time.Until(g.until)
		g.mu.Unlock()
		if d <= 0 {
			return ctx.Err()
		}
		timer := time.NewTimer(d)
		select {
		case <-ctx.Done():
			timer.Stop()
			return ctx.Err()
		case <-timer.C:
		}
	}
}

// pause closes the gate for d, never shortening a longer pause already set,
// and answers how much later the gate now opens.
func (g *rateGate) pause(d time.Duration) time.Duration {
	g.mu.Lock()
	defer g.mu.Unlock()
	now := time.Now()
	at := now.Add(d)
	if !at.After(g.until) {
		return 0
	}
	from := g.until
	if from.Before(now) {
		from = now
	}
	g.until = at
	return at.Sub(from)
}

// rateLimited runs one Gmail call through the gate, and on a rate limit closes the
// gate for the provider's wait (or the short ladder's, when it named none) and
// tries again. A wait longer than rateWaitInPage, or too many tries, returns
// the rate limit so the engine waits it out instead.
func rateLimited[T any](ctx context.Context, g *rateGate, fn func() (T, error)) (T, error) {
	var zero T
	for attempt := 0; ; attempt++ {
		if err := g.wait(ctx); err != nil {
			return zero, err
		}
		v, err := fn()
		if err == nil || !errors.Is(err, connector.ErrRateLimited) {
			return v, err
		}
		wait := backoff.Jittered(attempt, rateBackoffBase, rateBackoffCap)
		var limited *connector.RateLimitedError
		if errors.As(err, &limited) && limited.RetryAfter > wait {
			wait = limited.RetryAfter
		}
		// The gate closes either way: a wait too long for the page still holds
		// the other workers, so none of them spends Gmail's patience while the
		// page winds down and hands the wait to the engine.
		observeGatePause(ctx, g.pause(wait), err)
		if wait > rateWaitInPage || attempt+1 >= rateRetriesInPage {
			return zero, err
		}
	}
}

// inOrder fetches n items with a bounded pool and hands each result to consume
// in index order, so what consume writes never depends on which fetch finished
// first. consume returning an error stops the walk and is returned.
func inOrder[T any](
	ctx context.Context, n int,
	fetch func(ctx context.Context, i int) (T, error),
	consume func(i int, v T, err error) error,
) error {
	if n == 0 {
		return nil
	}
	ctx, cancel := context.WithCancel(ctx)
	type result struct {
		v   T
		err error
	}
	slots := make([]chan result, n)
	for i := range slots {
		slots[i] = make(chan result, 1)
	}
	tokens := make(chan struct{}, backfillLookahead)
	next := make(chan int)
	var wg sync.WaitGroup
	defer func() {
		cancel()
		wg.Wait()
	}()
	go func() {
		defer close(next)
		for i := range n {
			select {
			case tokens <- struct{}{}:
			case <-ctx.Done():
				return
			}
			select {
			case next <- i:
			case <-ctx.Done():
				return
			}
		}
	}()
	for range min(backfillFetchWorkers, n) {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for i := range next {
				v, err := fetch(ctx, i)
				slots[i] <- result{v: v, err: err}
			}
		}()
	}
	for i := range n {
		var r result
		select {
		case r = <-slots[i]:
		case <-ctx.Done():
			return ctx.Err()
		}
		<-tokens
		if err := consume(i, r.v, r.err); err != nil {
			return err
		}
	}
	return nil
}

// endsPage reports whether a message's failure is the connection's rather than
// the message's: the job ending, a rate limit the page could not wait out, a
// refused credential, an unreachable provider, a vanished cursor. Those stop
// the page without advancing it, and the engine decides whether to retry.
// Anything else is the capture refusing this one message.
func endsPage(ctx context.Context, err error) bool {
	return ctx.Err() != nil ||
		errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) ||
		errors.Is(err, connector.ErrRateLimited) ||
		errors.Is(err, connector.ErrAuthRejected) ||
		errors.Is(err, connector.ErrUnreachable) ||
		errors.Is(err, connector.ErrCursorGone)
}
