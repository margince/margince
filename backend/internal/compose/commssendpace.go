// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

// The send worker's pace, read from the installation's settings at the start
// of every delivery so an admin's change applies to the next send.

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/margince/margince/backend/internal/modules/comms"
	"github.com/margince/margince/backend/internal/modules/identity"
	"github.com/margince/margince/backend/internal/platform/settings"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// sendPace is how many messages one mailbox may send per window, and how long
// a delivery held back by that pace may wait before it parks with a reason.
type sendPace struct {
	limit  int
	window time.Duration
	maxAge time.Duration
}

// readSendPace reads the pace under the delivery's own scope: the worker's
// system principal on the job's workspace. One transaction, so an admin's save
// that moves the rate and its window together is never read half applied —
// every new pace rebuilds the dispatcher and starts every mailbox's count again.
func readSendPace(ctx context.Context, store *settings.Store) (sendPace, error) {
	var limit, windowSeconds, maxAgeHours int
	err := store.WriteTx(ctx, func(tx pgx.Tx) error {
		for _, f := range []struct {
			into  *int
			entry *settings.Entry[int]
		}{
			{&limit, identity.SendRateLimit},
			{&windowSeconds, identity.SendRateWindowSeconds},
			{&maxAgeHours, identity.SendMaxAgeHours},
		} {
			value, err := settings.GetTx(ctx, tx, f.entry)
			if err != nil {
				return err
			}
			*f.into = value
		}
		return nil
	})
	if err != nil {
		return sendPace{}, fmt.Errorf("compose: reading the send pace: %w", err)
	}
	return sendPace{
		limit:  limit,
		window: time.Duration(windowSeconds) * time.Second,
		maxAge: time.Duration(maxAgeHours) * time.Hour,
	}, nil
}

// pacedDispatcher keeps one dispatcher for as long as the pace it was built
// with stands. The mailbox rate counts live inside that dispatcher, so it is
// rebuilt only when the pace changes, and that change starts every mailbox's
// count afresh.
type pacedDispatcher struct {
	settings *settings.Store
	build    func(sendPace) deliveryDispatcher

	mu      sync.Mutex
	pace    sendPace
	current deliveryDispatcher
}

func newPacedDispatcher(pool *pgxpool.Pool, build func(sendPace) deliveryDispatcher) *pacedDispatcher {
	return &pacedDispatcher{settings: NewSettingsStore(pool), build: build}
}

// DispatchWithWait answers a failed pace read as a retry: the delivery is
// fine, and the next attempt reads again.
func (p *pacedDispatcher) DispatchWithWait(ctx context.Context, id ids.UUID) (comms.Outcome, time.Duration, error) {
	pace, err := readSendPace(ctx, p.settings)
	if err != nil {
		return comms.OutcomeRetry, 0, err
	}
	return p.dispatcherFor(pace).DispatchWithWait(ctx, id)
}

//nolint:ireturn // returns the deliveryDispatcher seam the worker is written against
func (p *pacedDispatcher) dispatcherFor(pace sendPace) deliveryDispatcher {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.current == nil || pace != p.pace {
		p.current, p.pace = p.build(pace), pace
	}
	return p.current
}
