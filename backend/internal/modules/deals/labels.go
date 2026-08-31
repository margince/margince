// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package deals

// Display names for a SET of deals, in one query.
//
// The attention feed names every subject on the page and the at-risk lane
// alone can carry a hundred deals. One gated get each is a hundred round
// trips whose count grows with how busy the pipeline is; together it is one
// query over the same page.
//
// It carries the object grant and the row-scope clause GetDeal carries, so a
// name is exactly as visible as the deal. A deal the caller cannot see is
// ABSENT from the answer rather than refused — one unreadable member is not a
// failure of a question about a set.

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// DealLabels answers each named deal's own name, under the caller's grants.
func (s *Store) DealLabels(ctx context.Context, want []ids.UUID) (map[ids.UUID]string, error) {
	if err := auth.Require(ctx, "deal", principal.ActionRead); err != nil {
		return nil, err
	}
	labels := map[ids.UUID]string{}
	if len(want) == 0 {
		return labels, nil
	}
	var args []any
	arg := func(v any) int { args = append(args, v); return len(args) }
	idsPos := arg(want)
	scope, err := auth.ScopeClauseFor(ctx, dealTable, "d", arg)
	if err != nil {
		return nil, err
	}
	if scope == "" {
		scope = "true"
	}
	err = s.Tx(ctx, func(tx pgx.Tx) error {
		rows, err := tx.Query(ctx, fmt.Sprintf(`
			SELECT d.id, coalesce(d.name, '')
			  FROM %s d
			 WHERE d.id = ANY($%d) AND d.archived_at IS NULL AND (%s)`,
			dealTable, idsPos, scope), args...)
		if err != nil {
			return err
		}
		defer rows.Close()
		for rows.Next() {
			var id ids.UUID
			var label string
			if err := rows.Scan(&id, &label); err != nil {
				return err
			}
			if label != "" {
				labels[id] = label
			}
		}
		return rows.Err()
	})
	if err != nil {
		return nil, fmt.Errorf("deals: reading deal names: %w", err)
	}
	return labels, nil
}
