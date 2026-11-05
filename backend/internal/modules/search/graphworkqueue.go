// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package search

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

type graphHistoryTargets struct {
	contacts     map[contactPair]*ids.UUID
	interactions map[pair]*ids.UUID
	sources      []string
}

func (h *graphHistoryTargets) add(targets edgeTargets, deferred *ids.UUID, sources []string) {
	h.sources = append(h.sources, sources...)
	if h.contacts == nil {
		h.contacts = make(map[contactPair]*ids.UUID)
		h.interactions = make(map[pair]*ids.UUID)
	}
	for _, p := range targets.contactPairs {
		if run, exists := h.contacts[p]; !exists || run != nil {
			h.contacts[p] = deferred
		}
	}
	for _, p := range targets.pairs {
		if run, exists := h.interactions[p]; !exists || run != nil {
			h.interactions[p] = deferred
		}
	}
}

func (h *graphHistoryTargets) contactWork() ([]contactPair, []*ids.UUID) {
	pairs := make([]contactPair, 0, len(h.contacts))
	runs := make([]*ids.UUID, 0, len(h.contacts))
	for p, run := range h.contacts {
		pairs = append(pairs, p)
		runs = append(runs, run)
	}
	return pairs, runs
}

func (h *graphHistoryTargets) interactionWork() ([]pair, []*ids.UUID) {
	pairs := make([]pair, 0, len(h.interactions))
	runs := make([]*ids.UUID, 0, len(h.interactions))
	for p, run := range h.interactions {
		pairs = append(pairs, p)
		runs = append(runs, run)
	}
	return pairs, runs
}

// Unmarked pairs can be missing or inserted after the marker scanned; both need coverage.
func (h *graphHistoryTargets) prepareContacts(ctx context.Context, tx pgx.Tx) error {
	pairs, _ := h.contactWork()
	uncovered, err := uncoveredContactPairs(ctx, tx, pairs, h.sources)
	if err != nil {
		return err
	}
	return recomputeContactPairs(ctx, tx, uncovered)
}

func (h *graphHistoryTargets) prepareInteractions(ctx context.Context, tx pgx.Tx) error {
	pairs, _ := h.interactionWork()
	uncovered, err := uncoveredInteractionPairs(ctx, tx, pairs, h.sources)
	if err != nil {
		return err
	}
	return recomputePairs(ctx, tx, uncovered)
}

func (h *graphHistoryTargets) markContacts(ctx context.Context, tx pgx.Tx) error {
	pairs, runs := h.contactWork()
	marked, err := queueContactPairs(ctx, tx, pairs, runs, h.sources)
	if err != nil {
		return err
	}
	for _, p := range marked {
		delete(h.contacts, p)
	}
	return nil
}

func (h *graphHistoryTargets) markInteractions(ctx context.Context, tx pgx.Tx) error {
	pairs, runs := h.interactionWork()
	marked, err := queueInteractionPairs(ctx, tx, pairs, runs, h.sources)
	if err != nil {
		return err
	}
	for _, p := range marked {
		delete(h.interactions, p)
	}
	return nil
}

func uncoveredContactPairs(ctx context.Context, tx pgx.Tx, pairs []contactPair, sources []string) ([]contactPair, error) {
	if len(pairs) == 0 {
		return nil, nil
	}
	a, b := contactPairKeys(pairs)
	args := []any{a, b, sources}
	rows, err := tx.Query(ctx, fmt.Sprintf(`SELECT DISTINCT t.a,t.b FROM unnest($%[1]d::uuid[],$%[2]d::uuid[]) AS t(a,b)
 LEFT JOIN graph_contact_edge e ON e.contact_a=t.a AND e.contact_b=t.b
 WHERE `+uncoveredGraphSourceSQL(fmt.Sprintf("$%d", len(args))), len(args)-2, len(args)-1), args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanContactPairs(rows)
}

func uncoveredInteractionPairs(ctx context.Context, tx pgx.Tx, pairs []pair, sources []string) ([]pair, error) {
	if len(pairs) == 0 {
		return nil, nil
	}
	users, contacts := interactionPairKeys(pairs)
	args := []any{users, contacts, sources}
	rows, err := tx.Query(ctx, fmt.Sprintf(`SELECT DISTINCT t.u,t.c FROM unnest($%[1]d::uuid[],$%[2]d::uuid[]) AS t(u,c)
 LEFT JOIN graph_interaction_edge e ON e.user_id=t.u AND e.contact_id=t.c
 WHERE `+uncoveredGraphSourceSQL(fmt.Sprintf("$%d", len(args))), len(args)-2, len(args)-1), args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanPairs(rows)
}

func queueContactPairs(ctx context.Context, tx pgx.Tx, pairs []contactPair, deferred []*ids.UUID, sources []string) ([]contactPair, error) {
	if len(pairs) == 0 {
		return nil, nil
	}
	a, b := contactPairKeys(pairs)
	args := []any{a, b, deferred, sources}
	rows, err := tx.Query(ctx, fmt.Sprintf(`WITH locked AS (
 SELECT e.contact_a,e.contact_b,t.r FROM graph_contact_edge e
 JOIN unnest($%[1]d::uuid[],$%[2]d::uuid[],$%[3]d::uuid[]) AS t(a,b,r)
 ON e.contact_a=t.a AND e.contact_b=t.b ORDER BY e.contact_a,e.contact_b FOR UPDATE OF e)
 UPDATE graph_contact_edge e SET requested_generation=e.requested_generation+1,deferred_run_id=locked.r
 FROM locked WHERE e.contact_a=locked.contact_a AND e.contact_b=locked.contact_b
 AND `+uncoveredGraphSourceSQL(fmt.Sprintf("$%d", len(args)))+`
 RETURNING e.contact_a,e.contact_b`, len(args)-3, len(args)-2, len(args)-1), args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanContactPairs(rows)
}

func queueInteractionPairs(ctx context.Context, tx pgx.Tx, pairs []pair, deferred []*ids.UUID, sources []string) ([]pair, error) {
	if len(pairs) == 0 {
		return nil, nil
	}
	users, contacts := interactionPairKeys(pairs)
	args := []any{users, contacts, deferred, sources}
	rows, err := tx.Query(ctx, fmt.Sprintf(`WITH locked AS (
 SELECT e.user_id,e.contact_id,t.r FROM graph_interaction_edge e
 JOIN unnest($%[1]d::uuid[],$%[2]d::uuid[],$%[3]d::uuid[]) AS t(u,c,r)
 ON e.user_id=t.u AND e.contact_id=t.c ORDER BY e.user_id,e.contact_id FOR UPDATE OF e)
 UPDATE graph_interaction_edge e SET requested_generation=e.requested_generation+1,deferred_run_id=locked.r
 FROM locked WHERE e.user_id=locked.user_id AND e.contact_id=locked.contact_id
 AND `+uncoveredGraphSourceSQL(fmt.Sprintf("$%d", len(args)))+`
 RETURNING e.user_id,e.contact_id`, len(args)-3, len(args)-2, len(args)-1), args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanPairs(rows)
}

// Unknown legacy sources stay conservative; commit order never follows ID order.
func uncoveredGraphSourceSQL(parameter string) string {
	return `(e.read_snapshot IS NULL OR COALESCE(cardinality(` + parameter + `::text[]),0)=0 OR EXISTS (
 SELECT 1 FROM unnest(` + parameter + `::text[]) AS source(xid)
 WHERE CASE WHEN source.xid='' THEN true ELSE NOT pg_visible_in_snapshot(source.xid::xid8,e.read_snapshot) END))`
}
