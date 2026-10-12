// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package events

// Coalesced delivery: one handler call for every entry one read returned.
//
// A consumer whose effect is a RECOMPUTE from the base tables — a projection
// refold, not a side effect — does the same work for the tenth event about a
// record as for the first, and much of that work is shared between records:
// two messages in one mailbox name the same colleague, and refolding that
// colleague twice writes back what the first refold already wrote. Such a
// consumer can take the whole read at once and do the shared work once. That
// is only sound for an effect that reads its inputs from the database rather
// than from the envelopes, and that is idempotent, because a batch folds N
// events into one pass whose result must equal N passes in any order.
//
// The contract with the per-entry path is unchanged where it matters:
//
//   - nothing is acked until the effect succeeded, so at-least-once holds;
//   - a batch that fails is NOT left pending as a unit. Its entries go through
//     the ordinary one-at-a-time handler at once, so one entry the batch cannot
//     handle costs that entry its own retry and never holds the rest back;
//   - the dedupe marks are the per-event marks Dedupe writes, so an event is
//     absorbed as processed whichever of the two paths ran it.
//
// Coalescing costs nothing when the bus is quiet — a read that returns one
// entry is delivered the ordinary way — and pays off exactly when there is a
// backlog, which is when a read comes back full.

import (
	"context"
	"fmt"

	"github.com/redis/go-redis/v9"

	kevents "github.com/margince/margince/backend/internal/shared/kernel/events"
)

// BatchHandler processes every envelope one read returned from one stream, as
// one effect. Returning nil acks them all; returning an error hands each entry
// to the per-entry Handler instead (see deliverAll).
type BatchHandler func(ctx context.Context, envs []kevents.Envelope) error

// WithBatch makes this subscriber deliver each read's entries to h in one call.
// The per-entry handler stays in place and is still required: it is what a
// single-entry read and a failed batch fall back to. A nil h is ignored, as a
// non-positive WithMinIdle is.
func (s *Subscriber) WithBatch(h BatchHandler) *Subscriber {
	if h != nil {
		s.batchHandler = h
	}
	return s
}

// deliverAll dispatches the entries one read (or one reclaim page) returned
// from one stream.
func (s *Subscriber) deliverAll(ctx context.Context, stream string, entries []redis.XMessage) {
	if s.batchHandler == nil || len(entries) < 2 {
		for _, entry := range entries {
			s.deliver(ctx, stream, entry)
		}
		return
	}

	envs := make([]kevents.Envelope, 0, len(entries))
	kept := make([]redis.XMessage, 0, len(entries))
	for _, entry := range entries {
		env, err := decodeEnvelope(entry)
		if err != nil {
			// Same disposition as deliver: it can never succeed, so it is
			// acked away loudly rather than left to poison the reclaim pass.
			s.log.Error("bus: dropping undecodable entry", "stream", stream, "entry", entry.ID, "error", err)
			s.ack(ctx, stream, entry.ID)
			continue
		}
		envs = append(envs, env)
		kept = append(kept, entry)
	}
	if len(envs) == 0 {
		return
	}

	if err := s.batchHandler(ctx, envs); err != nil {
		s.log.Warn("bus: batch handler failed; delivering its entries one at a time",
			"stream", stream, "entries", len(kept), "error", err)
		for _, entry := range kept {
			s.deliver(ctx, stream, entry)
		}
		return
	}

	entryIDs := make([]string, len(kept))
	for i, entry := range kept {
		entryIDs[i] = entry.ID
	}
	if err := s.rdb.XAck(ctx, stream, s.group.Name, entryIDs...).Err(); err != nil && ctx.Err() == nil {
		// As in ack: the effect already ran, so a lost ack costs one
		// redelivery of each entry, which the dedupe marks absorb.
		s.log.Warn("bus: XACK failed for a batch; its entries will re-deliver",
			"stream", stream, "entries", len(entryIDs), "error", err)
	}
}

// DedupeBatch is Dedupe for a BatchHandler: events this group already
// processed are dropped before next runs, and every event next handled is
// marked processed only after it succeeded — the same run-THEN-mark order, for
// the same reason Dedupe gives. The marks are Dedupe's own keys, so the two
// wrappers recognise each other's work.
func DedupeBatch(rdb *redis.Client, group string, next BatchHandler) BatchHandler {
	return func(ctx context.Context, envs []kevents.Envelope) error {
		checks := rdb.Pipeline()
		seen := make([]*redis.IntCmd, len(envs))
		for i, env := range envs {
			seen[i] = checks.Exists(ctx, dedupeKey(group, env))
		}
		if _, err := checks.Exec(ctx); err != nil {
			return fmt.Errorf("bus: dedupe check for a batch of %d: %w", len(envs), err)
		}
		fresh := make([]kevents.Envelope, 0, len(envs))
		for i, env := range envs {
			if seen[i].Val() == 0 {
				fresh = append(fresh, env)
			}
		}
		if len(fresh) == 0 {
			return nil // all already processed: ack, no second effect
		}

		if err := next(ctx, fresh); err != nil {
			return err
		}

		// A failed mark surfaces as a batch failure, so the entries take the
		// per-entry path: each re-runs into its idempotent no-op and retries
		// its own mark, as Dedupe does for one event.
		marks := rdb.Pipeline()
		for _, env := range fresh {
			marks.Set(ctx, dedupeKey(group, env), 1, DedupeTTL)
		}
		if _, err := marks.Exec(ctx); err != nil {
			return fmt.Errorf("bus: marking a batch of %d processed: %w", len(fresh), err)
		}
		return nil
	}
}
