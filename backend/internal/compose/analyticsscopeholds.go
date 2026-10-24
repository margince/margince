// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

// Whether the population an Analytics answer measured holds any deal at all.
//
// An empty figure cannot say it. "No sales won" is true for a company with
// deals and none won, and for a rep who owns nothing. The screen needs that
// difference, and it must come from the same population clause the
// figures used, so the notice and the numbers cannot disagree.

import (
	"context"
	"fmt"
	"strings"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/platform/auth"
)

// scopeHoldsDeals answers whether the resolved population holds a deal,
// archived ones included. It is the only place the question is asked; the
// unowned flag is the one the answer's own figures pass.
func scopeHoldsDeals(
	ctx context.Context, tx pgx.Tx, requested RequestedScope, unowned unownedPopulation,
) (bool, error) {
	var args []any
	arg := func(v any) int { args = append(args, v); return len(args) }

	where := []string{sqlUnnarrowed}
	rowScope, err := auth.ScopeClauseFor(ctx, tableDeal, "d", arg)
	if err != nil {
		return false, err
	}
	if rowScope != "" {
		where = append(where, rowScope)
	}
	_, population, err := AnalyticsPopulationClause(ctx, tx, requested, "d", arg, unowned)
	if err != nil {
		return false, err
	}
	if population != "" {
		where = append(where, population)
	}
	var holds bool
	err = tx.QueryRow(ctx,
		`SELECT EXISTS (SELECT 1 FROM deal d WHERE `+strings.Join(where, " AND ")+`)`, args...).Scan(&holds)
	if err != nil {
		return false, fmt.Errorf("compose: asking whether the scope holds deals: %w", err)
	}
	return holds, nil
}
