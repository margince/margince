// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package weekly

import (
	"context"

	"github.com/margince/margince/backend/internal/modules/deals"
	"github.com/margince/margince/backend/internal/platform/auth"
)

// standingDealScope is the caller's row scope on the deal table, narrowed to
// the deals that still stand for an opportunity. The review counts archived
// wins and losses, so a duplicate merged away must not count beside its survivor.
func standingDealScope(ctx context.Context, arg func(any) int) (string, error) {
	scope, err := auth.ScopeClauseFor(ctx, "deal", "d", arg)
	if err != nil {
		return "", err
	}
	if scope == "" {
		return deals.StandingSQL("d"), nil
	}
	return "(" + scope + ") AND " + deals.StandingSQL("d"), nil
}
