// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package automation

import (
	"context"
	"fmt"
	"slices"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
	"github.com/margince/margince/backend/internal/shared/ports/workflow"
)

// pendingWindow bounds how far back a write still counts as one an automation
// may react to. A run crashed between its claim and its outcome stays unfinished
// forever, and past this window it stops reading as pending.
const pendingWindow = "2 minutes"

// PendingFor counts the automation runs still due on the caller's own recent
// writes to any of these records. A run is due while its event is undelivered,
// or while a handler that fires on it has no outcome yet. Only the caller's
// writes count, so a listing reveals nothing about records another hand touched.
func (e *WorkflowEngine) PendingFor(ctx context.Context, targets []ids.Ref) (int, error) {
	caller, ok := principal.Actor(ctx)
	if len(targets) == 0 || !ok {
		return 0, nil
	}
	ws, err := e.db.Workspace(ctx)
	if err != nil {
		return 0, err
	}
	events, err := e.recentEventsOn(ctx, caller.ID, targets)
	if err != nil {
		return 0, err
	}
	if len(events) == 0 {
		return 0, nil
	}
	// Match reads the event as HandleEvent hands it over, workspace included.
	ctx = principal.WithWorkspaceID(ctx, ws.UUID)
	instances, err := e.liveInstances(ctx)
	if err != nil {
		return 0, fmt.Errorf("loading automation instances: %w", err)
	}
	pending := 0
	err = e.db.Tx(ctx, func(tx pgx.Tx) error {
		for _, raw := range events {
			ev, err := eventFromEnvelope(raw, ws.UUID)
			if err != nil {
				return err
			}
			for _, run := range e.expectedRuns(ctx, ev, instances) {
				var finished bool
				if err := tx.QueryRow(ctx, `SELECT EXISTS (SELECT 1 FROM workflow_run
					WHERE handler = $1 AND trigger_event = $2 AND ($3 = '' OR idempotency_key = $3)
					AND NOT (status = 'applied' AND applied IS NULL))`,
					run.handler, ev.ID, run.key).Scan(&finished); err != nil {
					return err
				}
				if !finished {
					pending++
				}
			}
		}
		return nil
	})
	return pending, err
}

// recentEventsOn reads the envelopes of the events the actor's writes emitted
// in the pending window about any of the targets, delivered or not.
func (e *WorkflowEngine) recentEventsOn(ctx context.Context, actor string, targets []ids.Ref) ([][]byte, error) {
	types, entityIDs := make([]string, 0, len(targets)), make([]string, 0, len(targets))
	for _, target := range targets {
		types = append(types, target.Type)
		entityIDs = append(entityIDs, target.ID.String())
	}
	var out [][]byte
	err := e.db.Tx(ctx, func(tx pgx.Tx) error {
		rows, err := tx.Query(ctx, `SELECT envelope->'entity'->>'type', envelope->'entity'->>'id', envelope
			FROM event_outbox
			WHERE envelope->'entity'->>'type' = ANY($1) AND envelope->'entity'->>'id' = ANY($2)
			AND envelope->'actor'->>'id' = $3 AND created_at > now() - $4::interval`,
			types, entityIDs, actor, pendingWindow)
		if err != nil {
			return err
		}
		defer rows.Close()
		for rows.Next() {
			var entityType, entityID string
			var envelope []byte
			if err := rows.Scan(&entityType, &entityID, &envelope); err != nil {
				return err
			}
			// The two lists also pair a type with another target's id.
			if slices.ContainsFunc(targets, func(t ids.Ref) bool { return t.Type == entityType && t.ID.String() == entityID }) {
				out = append(out, envelope)
			}
		}
		return rows.Err()
	})
	return out, err
}

// expectedRun is one firing HandleEvent makes on an event. key is empty for a
// list rule, whose fan-out claims one run per record it saw change.
type expectedRun struct {
	handler string
	key     string
}

// expectedRuns lists the firings HandleEvent makes on ev, from the same
// registry and enabled instances. A firing whose Match declines writes only a
// skip, so it is left out. A Match that errs is recorded as failed, not left
// running, so it is left out too.
func (e *WorkflowEngine) expectedRuns(ctx context.Context, ev workflow.Event, instances map[string][]automationInstance) []expectedRun {
	e.mu.RLock()
	handlers := append([]workflow.Handler(nil), e.handlers...)
	system := append([]workflow.Handler(nil), e.system...)
	e.mu.RUnlock()

	var out []expectedRun
	for _, h := range system {
		if h.Spec().Trigger.EventType == ev.Type && matches(ctx, h, ev) {
			out = append(out, expectedRun{handler: h.Spec().Name, key: runKey(h, ev)})
		}
	}
	for _, h := range handlers {
		if h.Spec().Trigger.EventType != ev.Type {
			continue
		}
		for _, inst := range instances[h.Spec().Name] {
			iev := ev
			iev.AutomationID, iev.OwnerID, iev.Params = inst.id.UUID, inst.owner, inst.params
			if _, list := h.(listRule); list {
				out = append(out, expectedRun{handler: h.Spec().Name})
				continue
			}
			if matches(ctx, h, iev) {
				out = append(out, expectedRun{handler: h.Spec().Name, key: runKey(h, iev)})
			}
		}
	}
	return out
}

func matches(ctx context.Context, h workflow.Handler, ev workflow.Event) bool {
	ok, err := h.Match(ctx, ev)
	return ok && err == nil
}
