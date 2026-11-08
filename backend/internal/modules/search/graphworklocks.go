// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package search

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5"
)

func (t *edgeTargets) lockContactPairs(ctx context.Context, tx pgx.Tx) ([]contactPair, error) {
	historical, _ := t.history.contactWork()
	pairs := append(append([]contactPair(nil), t.contactPairs...), historical...)
	if len(pairs) == 0 && len(t.dropped) == 0 {
		return nil, nil
	}
	first, second := contactPairKeys(pairs)
	args := []any{first, second, t.dropped}
	rows, err := tx.Query(ctx, fmt.Sprintf(`WITH targets AS (
 SELECT a,b FROM unnest($%[1]d::uuid[],$%[2]d::uuid[]) AS t(a,b)
 UNION SELECT contact_a,contact_b FROM graph_contact_edge WHERE contact_a=ANY($%[3]d::uuid[])
 UNION SELECT contact_a,contact_b FROM graph_contact_edge WHERE contact_b=ANY($%[3]d::uuid[])), locked AS MATERIALIZED (
 SELECT e.contact_a,e.contact_b FROM targets t JOIN graph_contact_edge e ON e.contact_a=t.a AND e.contact_b=t.b
 ORDER BY e.contact_a,e.contact_b FOR UPDATE OF e)
 SELECT t.a,t.b FROM targets t LEFT JOIN locked e ON e.contact_a=t.a AND e.contact_b=t.b
 WHERE e.contact_a IS NULL ORDER BY t.a,t.b`, len(args)-2, len(args)-1, len(args)), args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanContactPairs(rows)
}

func (t *edgeTargets) lockInteractionPairs(ctx context.Context, tx pgx.Tx) ([]pair, error) {
	historical, _ := t.history.interactionWork()
	pairs := append(append([]pair(nil), t.pairs...), historical...)
	if len(pairs) == 0 && len(t.dropped) == 0 {
		return nil, nil
	}
	users, contacts := interactionPairKeys(pairs)
	args := []any{users, contacts, t.dropped}
	rows, err := tx.Query(ctx, fmt.Sprintf(`WITH targets AS (
 SELECT u,c FROM unnest($%[1]d::uuid[],$%[2]d::uuid[]) AS t(u,c)
 UNION SELECT user_id,contact_id FROM graph_interaction_edge WHERE contact_id=ANY($%[3]d::uuid[])), locked AS MATERIALIZED (
 SELECT e.user_id,e.contact_id FROM targets t JOIN graph_interaction_edge e ON e.user_id=t.u AND e.contact_id=t.c
 ORDER BY e.user_id,e.contact_id FOR UPDATE OF e)
 SELECT t.u,t.c FROM targets t LEFT JOIN locked e ON e.user_id=t.u AND e.contact_id=t.c
 WHERE e.user_id IS NULL ORDER BY t.u,t.c`, len(args)-2, len(args)-1, len(args)), args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanPairs(rows)
}

// Row locks alone let a rebuild take its table lock while the writer still needs a lock upgrade.
func lockGraphWrites(ctx context.Context, tx pgx.Tx) error {
	_, err := tx.Exec(ctx, `LOCK TABLE graph_contact_edge, graph_interaction_edge IN ROW EXCLUSIVE MODE`)
	return err
}
