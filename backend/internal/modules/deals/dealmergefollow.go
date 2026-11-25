// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package deals

import (
	"context"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// maxMergeHops bounds how far a redirect is followed. A survivor can itself be
// merged later, so the answer is the end of the chain.
const maxMergeHops = 8

// FollowMergeTx answers the deal that carries on for a merged-away deal, and the
// deal itself when it was not merged. Who is responsible for a retired duplicate
// is whoever is responsible for the deal that replaced it. The first deal is read under the
// caller's row scope. One it cannot see answers pgx.ErrNoRows, like one that
// does not exist.
func FollowMergeTx(ctx context.Context, tx pgx.Tx, id ids.UUID) (ids.UUID, error) {
	args := []any{id, maxMergeHops}
	arg := func(v any) int { args = append(args, v); return len(args) }
	scope, err := auth.ScopeClauseFor(ctx, "deal", "d", arg)
	if err != nil {
		return ids.UUID{}, err
	}
	if scope == "" {
		scope = "TRUE"
	}
	var end ids.UUID
	err = tx.QueryRow(ctx, `
		WITH RECURSIVE chain(deal_id, hop) AS (
			SELECT d.id, 0 FROM deal d WHERE d.id = $1 AND (`+scope+`)
			UNION ALL
			SELECT d.merged_into_id, c.hop + 1 FROM chain c JOIN deal d ON d.id = c.deal_id
			 WHERE d.merged_into_id IS NOT NULL AND c.hop < $2)
		SELECT deal_id FROM chain ORDER BY hop DESC LIMIT 1`, args...).Scan(&end)
	return end, err
}
