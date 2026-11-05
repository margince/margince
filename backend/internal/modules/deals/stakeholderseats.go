// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package deals

import (
	"context"
	"fmt"
	"strings"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// StakeholderSeats reads roles, without deriving an engagement score.
func StakeholderSeats(ctx context.Context, tx pgx.Tx, dealIDs []ids.UUID) (map[ids.UUID][]DealStakeholder, error) {
	if err := auth.Require(ctx, "deal", principal.ActionRead); err != nil {
		return nil, err
	}
	var args []any
	arg := func(v any) int { args = append(args, v); return len(args) }
	dealPos := arg(dealIDs)
	bound, err := edgeBound(ctx, "r", arg)
	if err != nil {
		return nil, err
	}
	contactScope, err := auth.ScopeClauseFor(ctx, "contact", "p", arg)
	if err != nil {
		return nil, err
	}
	dealScope, err := auth.ScopeClauseFor(ctx, dealTable, "d", arg)
	if err != nil {
		return nil, err
	}
	where := []string{fmt.Sprintf(`r.kind = 'deal_stakeholder' AND r.deal_id = ANY($%d)
		AND r.archived_at IS NULL AND d.archived_at IS NULL AND (%s)`, dealPos, bound)}
	for _, scope := range []string{contactScope, dealScope} {
		if scope != "" {
			where = append(where, scope)
		}
	}
	rows, err := tx.Query(ctx, `SELECT r.deal_id, r.contact_id, coalesce(r.role, '')
		FROM relationship r JOIN contact p ON p.id = r.contact_id AND p.archived_at IS NULL
		JOIN deal d ON d.id = r.deal_id WHERE `+strings.Join(where, " AND ")+` ORDER BY r.deal_id, r.contact_id`, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := map[ids.UUID][]DealStakeholder{}
	for rows.Next() {
		var dealID ids.UUID
		var seat DealStakeholder
		if err := rows.Scan(&dealID, &seat.ContactID, &seat.Role); err != nil {
			return nil, err
		}
		out[dealID] = append(out[dealID], seat)
	}
	return out, rows.Err()
}
