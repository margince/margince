// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package events

import (
	"context"
	"fmt"
	"time"

	"github.com/redis/go-redis/v9"

	kevents "github.com/margince/margince/backend/internal/shared/kernel/events"
)

// DedupeTTL is how long a processed event_id is remembered: ≥ the stream
// retention horizon, so a consumer offline for the whole trim window can
// never re-see an event after its dedupe entry expired (events.md §4.3
// pins 96h against the ≥72h retention default).
const DedupeTTL = 96 * time.Hour

// dedupe wraps a handler so its consumer group skips a redelivered event_id.
// NewSubscriber applies it, so every subscriber's handler runs behind it.
// The mark goes in after the handler succeeds. A mark written first would, on a
// crash in between, swallow the redelivery and drop the event. A late mark can
// only cause a re-run, which the effect's upsert by natural key absorbs. So
// dedupe saves work over that layer and never replaces it.
func dedupe(rdb *redis.Client, group string, next Handler) Handler {
	return func(ctx context.Context, env kevents.Envelope) error {
		key := dedupeKey(group, env)
		seen, err := rdb.Exists(ctx, key).Result()
		if err != nil {
			return fmt.Errorf("bus: dedupe check for %s: %w", env.EventID, err)
		}
		if seen == 1 {
			return nil // already processed: ack, no second effect
		}

		if err := next(ctx, env); err != nil {
			return err // no mark yet, so the redelivery retries naturally
		}

		// A failed mark is surfaced: the entry stays pending, the
		// redelivery re-runs the effect into the natural-key no-op and
		// retries the mark — strictly better than acking with the mark
		// unwritten and eating a re-run per redelivery forever.
		if err := rdb.Set(ctx, key, 1, DedupeTTL).Err(); err != nil {
			return fmt.Errorf("bus: marking %s processed: %w", env.EventID, err)
		}
		return nil
	}
}

// DedupeKeyPrefix is the namespace every processed-event mark shares. Named
// here rather than inlined so the reset's purge and the writer cannot drift
// apart: a key shape only one of them knows is a key the purge leaves behind.
const DedupeKeyPrefix = "gw:dedupe:"

// dedupeKey is per group: each consumer group owns its own processed set
// (events.md §4.3 — every group sees every event once).
func dedupeKey(group string, env kevents.Envelope) string {
	return DedupeKeyPrefix + group + ":" + env.EventID.String()
}
