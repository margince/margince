// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package events

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"time"

	"github.com/redis/go-redis/v9"

	"github.com/margince/margince/backend/internal/shared/kernel/backoff"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// ResetChannel carries data-reset notifications to every running process.
//
// Pub/sub, not the outbox bus: this is a process-lifecycle signal, not a
// domain event. It has no envelope, no audit trail and no replay, and it must
// NOT enter the spec-pinned event catalog — a subscriber that missed it has a
// stale cache entry that its own TTL retires, whereas a domain event that
// went missing would be a lost fact.
const ResetChannel = "gw:control:reset"

// maxResetPayloadLen bounds what this channel will even look at. A payload is
// one workspace UUID (36 bytes); the allowance is generous rather than tight
// because refusing a legitimate id would silently stop a reset from flushing
// caches, while the only thing a larger one can be is noise.
const maxResetPayloadLen = 64

// PublishReset announces that ws was reset, so every process drops what it
// cached for that workspace. Delivery is best-effort by construction: pub/sub
// reaches whoever is listening now, which is precisely the set of processes
// that hold caches.
func PublishReset(ctx context.Context, rdb *redis.Client, ws ids.UUID) error {
	if err := rdb.Publish(ctx, ResetChannel, ws.String()).Err(); err != nil {
		return fmt.Errorf("bus: announcing the reset of workspace %s: %w", ws, err)
	}
	return nil
}

// SubscribeReset runs until ctx is canceled, invoking fn for every reset
// announcement. A subscription that fails or drops is logged and reopened after
// a backoff. Announcements published meanwhile are lost, and the caches' own
// TTLs retire what they held.
//
// An unparseable payload is logged and skipped, so one bad publisher cannot
// stop cache flushing for the process.
func SubscribeReset(ctx context.Context, rdb *redis.Client, log *slog.Logger, fn func(ids.UUID)) {
	superviseReset(ctx, log, func(ctx context.Context, ready chan<- struct{}) error {
		return subscribeResetWithReady(ctx, rdb, log, fn, ready)
	}, backoff.Sleep)
}

const (
	resubscribeBase    = time.Second
	resubscribeCeiling = 30 * time.Second
)

// superviseReset reopens a subscription until ctx ends. A confirmed subscription
// restarts the failure count, so a drop after hours of uptime retries quickly.
// sleep is injected so tests do not wait.
func superviseReset(ctx context.Context, log *slog.Logger, run func(context.Context, chan<- struct{}) error, sleep func(context.Context, time.Duration) error) {
	failures := 0
	for ctx.Err() == nil {
		ready := make(chan struct{})
		err := run(ctx, ready)
		if ctx.Err() != nil {
			return
		}
		select {
		case <-ready:
			failures = 0
		default:
		}
		delay := backoff.Jittered(failures, resubscribeBase, resubscribeCeiling)
		failures++
		log.Error("bus: the reset control subscription ended; resubscribing", "error", err, "retry_in", delay)
		if sleep(ctx, delay) != nil {
			return
		}
	}
}

// errSubscriptionClosed distinguishes "the Redis subscription channel closed
// out from under us" from a clean ctx.Done() shutdown: both once returned a
// bare nil, which left a caller with no way to tell "no more resets will ever
// arrive until restart" from "we were asked to stop."
var errSubscriptionClosed = errors.New("bus: reset control subscription channel closed")

func subscribeResetWithReady(ctx context.Context, rdb *redis.Client, log *slog.Logger, fn func(ids.UUID), ready chan<- struct{}) error {
	sub := rdb.Subscribe(ctx, ResetChannel)
	defer func() {
		if err := sub.Close(); err != nil && ctx.Err() == nil {
			log.Error("bus: closing the reset control subscription", "error", err)
		}
	}()
	if _, err := sub.Receive(ctx); err != nil {
		return fmt.Errorf("bus: subscribing to %s: %w", ResetChannel, err)
	}
	if ready != nil {
		close(ready)
	}
	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case msg, ok := <-sub.Channel():
			if !ok {
				return errSubscriptionClosed
			}
			if len(msg.Payload) > maxResetPayloadLen {
				// Length only, never the payload: a publisher chooses this
				// string and Redis admits a very large one, so quoting it into
				// a log line — which ids.Parse's error does — hands whoever
				// reaches the bus a cheap amplifier into every subscriber's
				// log. The same reasoning bounds ratelimit's map keys.
				log.Warn("bus: skipping oversized reset payload", "bytes", len(msg.Payload))
				continue
			}
			ws, err := ids.Parse(msg.Payload)
			if err != nil {
				log.Warn("bus: skipping unparseable reset payload", "error", err)
				continue
			}
			fn(ws)
		}
	}
}
