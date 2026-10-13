// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

// The send worker's pace, read from the installation's settings at the start
// of every delivery so an admin's change applies to the next send.

import (
	"context"
	"encoding/json"
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

// readSendPace reads the pace in one statement, so an admin's save that moves
// the rate and its window together is never read half applied.
func readSendPace(ctx context.Context, store *settings.Store) (sendPace, error) {
	var limit, windowSeconds, maxAgeHours int
	err := store.WriteTx(ctx, func(tx pgx.Tx) error {
		raw, err := settings.ApplyManyTx(ctx, tx, identity.SendRateLimit, identity.SendRateWindowSeconds, identity.SendMaxAgeHours)
		if err != nil {
			return err
		}
		for _, f := range []struct {
			into *int
			key  string
		}{
			{&limit, identity.SendRateLimit.Key()},
			{&windowSeconds, identity.SendRateWindowSeconds.Key()},
			{&maxAgeHours, identity.SendMaxAgeHours.Key()},
		} {
			if err := json.Unmarshal(raw[f.key], f.into); err != nil {
				return fmt.Errorf("%s does not hold a whole number: %w", f.key, err)
			}
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
// with stands. A worker without the shared rate store counts each mailbox
// inside that dispatcher's limiter, so rebuilding it on every send would let
// every mailbox send without limit; it is rebuilt only when the pace changes.
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
