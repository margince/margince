// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package briefs

import (
	"context"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/platform/database"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// SnoozedDeals includes earlier runs, whose marks still suppress today's candidates.
func (e *BriefEngine) SnoozedDeals(ctx context.Context, now time.Time) ([]ids.UUID, error) {
	if err := auth.Require(ctx, "deal", principal.ActionRead); err != nil {
		return nil, err
	}
	user, err := briefUser(ctx)
	if err != nil {
		return nil, err
	}
	var deals []ids.UUID
	err = database.WithWorkspaceTx(ctx, e.pool, func(tx pgx.Tx) error {
		var args []any
		bind := func(value any) int { args = append(args, value); return len(args) }
		userParam, nowParam := fmt.Sprintf("$%d", bind(user)), fmt.Sprintf("$%d", bind(now.UTC()))
		scope, err := auth.ScopeClauseFor(ctx, "deal", "d", bind)
		if err != nil {
			return err
		}
		query := `SELECT DISTINCT bi.deal_id FROM brief_item bi
 JOIN brief_run br ON br.id = bi.brief_run_id JOIN deal d ON d.id = bi.deal_id
 WHERE br.user_id = ` + userParam + ` AND bi.state = 'snoozed' AND ` + briefSnoozeHoldsSQL("bi", nowParam)
		if scope != "" {
			query += " AND " + scope
		}
		rows, err := tx.Query(ctx, query, args...)
		if err != nil {
			return err
		}
		deals, err = pgx.CollectRows(rows, pgx.RowTo[ids.UUID])
		return err
	})
	return deals, err
}
