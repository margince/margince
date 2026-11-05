// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package search

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

type contactPairWork struct {
	pair       contactPair
	generation int64
}
type interactionPairWork struct {
	pair       pair
	generation int64
}

// DrainPendingGraphTx folds a bounded cohort and acknowledges the generations it locked.
func DrainPendingGraphTx(ctx context.Context, tx pgx.Tx, limit int) (bool, error) {
	if err := auth.RequireSystem(ctx); err != nil {
		return false, err
	}
	if err := lockGraphWrites(ctx, tx); err != nil {
		return false, err
	}
	contacts, err := pendingContactPairs(ctx, tx, limit)
	if err != nil {
		return false, err
	}
	interactions, err := pendingInteractionPairs(ctx, tx, limit)
	if err != nil {
		return false, err
	}
	if err := settleContactPairs(ctx, tx, contacts); err != nil {
		return false, err
	}
	if err := settleInteractionPairs(ctx, tx, interactions); err != nil {
		return false, err
	}
	return len(contacts) == limit || len(interactions) == limit, nil
}

func pendingContactPairs(ctx context.Context, tx pgx.Tx, limit int) ([]contactPairWork, error) {
	args := []any{limit}
	rows, err := tx.Query(ctx, fmt.Sprintf(`SELECT contact_a,contact_b,requested_generation FROM graph_contact_edge
 WHERE requested_generation>processed_generation AND deferred_run_id IS NULL
 ORDER BY contact_a,contact_b LIMIT $%d FOR UPDATE SKIP LOCKED`, len(args)), args...)
	if err != nil {
		return nil, err
	}
	return pgx.CollectRows(rows, func(row pgx.CollectableRow) (contactPairWork, error) {
		var work contactPairWork
		err := row.Scan(&work.pair.a, &work.pair.b, &work.generation)
		return work, err
	})
}

func pendingInteractionPairs(ctx context.Context, tx pgx.Tx, limit int) ([]interactionPairWork, error) {
	args := []any{limit}
	rows, err := tx.Query(ctx, fmt.Sprintf(`SELECT user_id,contact_id,requested_generation FROM graph_interaction_edge
 WHERE requested_generation>processed_generation AND deferred_run_id IS NULL
 ORDER BY user_id,contact_id LIMIT $%d FOR UPDATE SKIP LOCKED`, len(args)), args...)
	if err != nil {
		return nil, err
	}
	return pgx.CollectRows(rows, func(row pgx.CollectableRow) (interactionPairWork, error) {
		var work interactionPairWork
		err := row.Scan(&work.pair.user, &work.pair.contact, &work.generation)
		return work, err
	})
}

func settleContactPairs(ctx context.Context, tx pgx.Tx, work []contactPairWork) error {
	if len(work) == 0 {
		return nil
	}
	pairs := make([]contactPair, 0, len(work))
	generations := make([]int64, 0, len(work))
	for _, request := range work {
		pairs = append(pairs, request.pair)
		generations = append(generations, request.generation)
	}
	if err := recomputeContactPairs(ctx, tx, pairs); err != nil {
		return err
	}
	a, b := contactPairKeys(pairs)
	args := []any{a, b, generations}
	_, err := tx.Exec(ctx, fmt.Sprintf(`UPDATE graph_contact_edge e SET processed_generation=GREATEST(e.processed_generation,t.g)
 FROM unnest($%[1]d::uuid[],$%[2]d::uuid[],$%[3]d::bigint[]) AS t(a,b,g)
 WHERE e.contact_a=t.a AND e.contact_b=t.b AND e.requested_generation>=t.g`, len(args)-2, len(args)-1, len(args)), args...)
	return err
}

func settleInteractionPairs(ctx context.Context, tx pgx.Tx, work []interactionPairWork) error {
	if len(work) == 0 {
		return nil
	}
	pairs := make([]pair, 0, len(work))
	generations := make([]int64, 0, len(work))
	for _, request := range work {
		pairs = append(pairs, request.pair)
		generations = append(generations, request.generation)
	}
	if err := recomputePairs(ctx, tx, pairs); err != nil {
		return err
	}
	users, contacts := interactionPairKeys(pairs)
	args := []any{users, contacts, generations}
	_, err := tx.Exec(ctx, fmt.Sprintf(`UPDATE graph_interaction_edge e SET processed_generation=GREATEST(e.processed_generation,t.g)
 FROM unnest($%[1]d::uuid[],$%[2]d::uuid[],$%[3]d::bigint[]) AS t(u,c,g)
 WHERE e.user_id=t.u AND e.contact_id=t.c AND e.requested_generation>=t.g`, len(args)-2, len(args)-1, len(args)), args...)
	return err
}

// ReleaseGraphHistoryTx transfers pending generations to the remaining import or releases them.
func ReleaseGraphHistoryTx(ctx context.Context, tx pgx.Tx, runs []ids.UUID, replacement *ids.UUID, limit int) (int64, error) {
	if err := auth.RequireSystem(ctx); err != nil {
		return 0, err
	}
	if err := lockGraphWrites(ctx, tx); err != nil {
		return 0, err
	}
	args := []any{runs, replacement, limit}
	contacts, err := tx.Exec(ctx, fmt.Sprintf(`WITH locked AS (
 SELECT contact_a,contact_b FROM graph_contact_edge WHERE deferred_run_id=ANY($%[1]d::uuid[])
 ORDER BY contact_a,contact_b LIMIT $%[3]d FOR UPDATE SKIP LOCKED)
 UPDATE graph_contact_edge e SET deferred_run_id=$%[2]d FROM locked
 WHERE e.contact_a=locked.contact_a AND e.contact_b=locked.contact_b`, len(args)-2, len(args)-1, len(args)), args...)
	if err != nil {
		return 0, err
	}
	interactions, err := tx.Exec(ctx, fmt.Sprintf(`WITH locked AS (
 SELECT user_id,contact_id FROM graph_interaction_edge WHERE deferred_run_id=ANY($%[1]d::uuid[])
 ORDER BY user_id,contact_id LIMIT $%[3]d FOR UPDATE SKIP LOCKED)
 UPDATE graph_interaction_edge e SET deferred_run_id=$%[2]d FROM locked
 WHERE e.user_id=locked.user_id AND e.contact_id=locked.contact_id`, len(args)-2, len(args)-1, len(args)), args...)
	return contacts.RowsAffected() + interactions.RowsAffected(), err
}

// HasPendingGraphWorkTx is an indexed wakeup check, independent of job retention.
func HasPendingGraphWorkTx(ctx context.Context, tx pgx.Tx) (bool, error) {
	if err := auth.RequireSystem(ctx); err != nil {
		return false, err
	}
	var pending bool
	err := tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM graph_contact_edge WHERE requested_generation>processed_generation AND deferred_run_id IS NULL)
 OR EXISTS(SELECT 1 FROM graph_interaction_edge WHERE requested_generation>processed_generation AND deferred_run_id IS NULL)`).Scan(&pending)
	return pending, err
}
