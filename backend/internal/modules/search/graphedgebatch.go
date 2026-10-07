// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package search

// The cg:graph-edge consumer, one bus read at a time.
//
// The refold is priced by the pairs it names, not by the events that asked for
// it, and the pairs are heavily shared. affectedPairs names every edge of every
// colleague on an activity (the relink arm), so one message to a rep who knows
// two thousand contacts refolds two thousand pairs — and the next message in
// that rep's mailbox refolds the same two thousand again. A capture burst is
// exactly that: a run of events from the same few mailboxes. Folding the run as
// one target set does the shared work once.
//
// This is sound because nothing here reads the envelopes for anything but
// WHICH record changed. Every refold recomputes from the base tables as they
// stand, so folding N events together leaves the same rows as folding them one
// by one, in any order — the property the per-event path already relies on for
// redelivery. The nightly RebuildEdges remains the backstop for anything a
// batch and its fallback both failed to reach.

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/shared/kernel/events"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// HandleBatch folds every envelope of one bus read in one transaction: the
// activities and the contact events are gathered into ONE target set, then
// applied in a single pass, contact table before interaction table (edgeTargets).
//
// Gathering first is what keeps the table order: folding the activities and
// then each contact event would take interaction locks and then ask for
// contact ones again. A merge's drop still lands after every fold in the read,
// which is the order the per-event path converges to as well. A contact event
// repeated within the read is applied once: the second application would
// recompute the same rows from the same base tables.
//
// One transaction, so a failure leaves nothing half-applied and the subscriber
// hands every entry to HandleEvent instead — an event the batch cannot fold
// costs its own retry, never its neighbours'.
func (g *GraphEdgeGen) HandleBatch(ctx context.Context, envs []events.Envelope) error {
	var activityIDs []ids.UUID
	var contactEnvs []events.Envelope
	seenActivity := map[ids.UUID]bool{}
	seenContact := map[string]bool{}
	for _, env := range envs {
		id := env.Entity.ID
		if id == ids.Nil {
			continue
		}
		switch env.Entity.Type {
		case entityActivity:
			if refoldsActivity(env.Type) && !seenActivity[id] {
				seenActivity[id] = true
				activityIDs = append(activityIDs, id)
			}
		case entityContact:
			key := env.Type + "/" + id.String()
			if !seenContact[key] {
				seenContact[key] = true
				contactEnvs = append(contactEnvs, env)
			}
		}
	}
	if len(activityIDs) == 0 && len(contactEnvs) == 0 {
		return nil
	}

	// The first envelope's correlation labels the pass. The projection writes
	// no audit row, so the correlation only ties its statements to one of the
	// events that caused them.
	ctx, err := g.projectionContext(ctx, envs[0])
	if err != nil {
		return err
	}
	return g.store.db.Tx(ctx, func(tx pgx.Tx) error {
		var targets edgeTargets
		if err := targets.addActivities(ctx, tx, activityIDs); err != nil {
			return fmt.Errorf("graph-edge: a batch of %d activities: %w", len(activityIDs), err)
		}
		for _, env := range contactEnvs {
			if err := targets.addContactEvent(ctx, tx, env, env.Entity.ID); err != nil {
				return fmt.Errorf("graph-edge: %s: %w", env.Type, err)
			}
		}
		if err := targets.apply(ctx, tx); err != nil {
			return fmt.Errorf("graph-edge: folding a batch of %d events: %w", len(envs), err)
		}
		return nil
	})
}
