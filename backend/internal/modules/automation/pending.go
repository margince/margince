// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package automation

import (
	"context"
	"fmt"
	"slices"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/platform/events"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// maxPendingEvents bounds one check to the caller's newest events on the
// records. The answer is a yes or no, so older ones would only add cost.
const maxPendingEvents = 50

// PendingFor says whether an automation is still due to act on the caller's own
// writes to any of these records. It is due while the event is undelivered, or
// while a handler that fires on it has no outcome yet. Only the caller's
// writes count.
//
// A delivered event whose consumer died is handled again within the reclaim
// window. Past it, a firing with no run row never gets one, and a claim with
// no outcome crashed.
func (e *WorkflowEngine) PendingFor(ctx context.Context, targets []ids.Ref) (bool, error) {
	caller, ok := principal.Actor(ctx)
	if len(targets) == 0 || !ok {
		return false, nil
	}
	ws, err := e.db.Workspace(ctx)
	if err != nil {
		return false, err
	}
	ctx = principal.WithWorkspaceID(ctx, ws.UUID)
	window := fmt.Sprintf("%d seconds", int(events.DefaultMinIdle.Seconds()))
	pending := false
	err = e.db.Tx(ctx, func(tx pgx.Tx) error {
		envelopes, err := recentEventsOn(ctx, tx, caller.ID, targets, window)
		if err != nil || len(envelopes) == 0 {
			return err
		}
		instances, err := e.liveInstances(ctx)
		if err != nil {
			return fmt.Errorf("loading automation instances: %w", err)
		}
		var handlers, keys []string
		for _, raw := range envelopes {
			ev, err := eventFromEnvelope(raw, ws.UUID)
			if err != nil {
				return err
			}
			system, instanced := e.firings(ev, instances)
			for _, f := range slices.Concat(system, instanced) {
				// A list rule fires on a scheduled check, never on the caller's write.
				if _, list := f.handler.(listRule); list || !wouldRun(ctx, f) {
					continue
				}
				handlers = append(handlers, f.handler.Spec().Name)
				keys = append(keys, runKey(f.handler, f.event))
			}
		}
		pending, err = anyRunUnfinished(ctx, tx, handlers, keys, window)
		return err
	})
	return pending, err
}

// wouldRun answers Match for a firing. A firing Match declines records only a
// skip, and one whose Match errs records a failure, so neither is left running.
func wouldRun(ctx context.Context, f firing) bool {
	ok, err := f.handler.Match(ctx, f.event)
	return ok && err == nil
}

// recentEventsOn reads the actor's newest events about any of the targets that
// are undelivered or were delivered within the window.
func recentEventsOn(ctx context.Context, tx pgx.Tx, actor string, targets []ids.Ref, window string) ([][]byte, error) {
	types, entityIDs := make([]string, 0, len(targets)), make([]string, 0, len(targets))
	for _, target := range targets {
		types = append(types, target.Type)
		entityIDs = append(entityIDs, target.ID.String())
	}
	rows, err := tx.Query(ctx, `SELECT envelope->'entity'->>'type', envelope->'entity'->>'id', envelope
		FROM event_outbox
		WHERE envelope->'entity'->>'type' = ANY($1) AND envelope->'entity'->>'id' = ANY($2)
		AND envelope->'actor'->>'id' = $3
		AND (published_at IS NULL OR published_at > now() - $4::interval)
		ORDER BY seq DESC LIMIT $5`, types, entityIDs, actor, window, maxPendingEvents)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out [][]byte
	for rows.Next() {
		var entityType, entityID string
		var envelope []byte
		if err := rows.Scan(&entityType, &entityID, &envelope); err != nil {
			return nil, err
		}
		// The two lists also pair a type with another target's id.
		if slices.ContainsFunc(targets, func(t ids.Ref) bool { return t.Type == entityType && t.ID.String() == entityID }) {
			out = append(out, envelope)
		}
	}
	return out, rows.Err()
}

// anyRunUnfinished says whether one of these (handler, key) runs has no row,
// or holds a claim made within the window that has no outcome yet.
func anyRunUnfinished(ctx context.Context, tx pgx.Tx, handlers, keys []string, window string) (bool, error) {
	if len(handlers) == 0 {
		return false, nil
	}
	var finished int
	err := tx.QueryRow(ctx, `SELECT count(*) FROM workflow_run
		WHERE (handler, idempotency_key) IN (SELECT * FROM unnest($1::text[], $2::text[]))
		AND NOT (`+runInFlightSQL+` AND created_at > now() - $3::interval)`,
		handlers, keys, window).Scan(&finished)
	return finished < len(handlers), err
}
