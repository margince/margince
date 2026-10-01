// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package search

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// edgeTargets is everything one projection pass has to touch, gathered before
// anything is written so that apply can take the two tables' row locks in ONE
// order: graph_contact_edge, then graph_interaction_edge, each in key order.
//
// Two writers that take the tables in opposite orders deadlock however
// carefully each orders its own rows, and the bus delivers contact and
// activity events to replicas of one consumer group at the same time. Every
// writer of either table, and RebuildEdges' table lock, follows this order.
type edgeTargets struct {
	contactPairs []contactPair
	pairs        []pair
	// dropped are contacts whose edges are removed outright, after the folds
	// so a drop outlasts anything that could have refolded them.
	dropped []ids.UUID
}

// addActivities names the pairs the activities touch. Reads only: the pairs
// are resolved BEFORE any fold, so a pair whose rows have all gone is still
// named and can be deleted by it.
func (t *edgeTargets) addActivities(ctx context.Context, tx pgx.Tx, activityIDs []ids.UUID) error {
	if len(activityIDs) == 0 {
		return nil
	}
	contactPairs, err := affectedContactPairs(ctx, tx, activityIDs)
	if err != nil {
		return err
	}
	pairs, err := affectedPairs(ctx, tx, activityIDs)
	if err != nil {
		return err
	}
	t.contactPairs = append(t.contactPairs, contactPairs...)
	t.pairs = append(t.pairs, pairs...)
	return nil
}

// addContact names every edge touching one contact. Reads only.
func (t *edgeTargets) addContact(ctx context.Context, tx pgx.Tx, contactID ids.UUID) error {
	rows, err := tx.Query(ctx, `
		SELECT DISTINCT u.user_id
		  FROM activity_participant p
		  JOIN activity_participant u ON u.activity_id = p.activity_id
		 WHERE p.contact_id = $1 AND u.user_id IS NOT NULL
		 UNION
		SELECT user_id FROM graph_interaction_edge WHERE contact_id = $1`, contactID)
	if err != nil {
		return fmt.Errorf("search: resolving the colleagues who know a contact: %w", err)
	}
	defer rows.Close()
	for rows.Next() {
		var u ids.UUID
		if err := rows.Scan(&u); err != nil {
			return err
		}
		t.pairs = append(t.pairs, pair{user: u, contact: contactID})
	}
	if err := rows.Err(); err != nil {
		return err
	}
	contactPairs, err := contactPairsForContact(ctx, tx, contactID)
	if err != nil {
		return err
	}
	t.contactPairs = append(t.contactPairs, contactPairs...)
	return nil
}

// apply writes the gathered targets: contact table first, interaction second.
func (t *edgeTargets) apply(ctx context.Context, tx pgx.Tx) error {
	if err := recomputeContactPairs(ctx, tx, t.contactPairs); err != nil {
		return err
	}
	if len(t.dropped) > 0 {
		if _, err := tx.Exec(ctx, `DELETE FROM graph_contact_edge WHERE contact_a = ANY($1) OR contact_b = ANY($1)`, t.dropped); err != nil {
			return fmt.Errorf("search: dropping a contact's observed peer edges: %w", err)
		}
	}
	if err := recomputePairs(ctx, tx, t.pairs); err != nil {
		return err
	}
	if len(t.dropped) > 0 {
		if _, err := tx.Exec(ctx, `DELETE FROM graph_interaction_edge WHERE contact_id = ANY($1)`, t.dropped); err != nil {
			return fmt.Errorf("search: dropping a contact's interaction edges: %w", err)
		}
	}
	return nil
}
