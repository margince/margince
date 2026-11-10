// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package contracts

// Contract row visibility, which is INHERITED rather than owned.
//
// Every other row-scoped record in this system answers `owner_id`, and the
// shared row-scope helpers interpolate exactly those tables — they reject a
// table name they do not know rather than guessing, so `contract` cannot join
// them: there is no column for their clauses to test.
//
import (
	"context"

	"github.com/margince/margince/backend/internal/platform/auth"
)

// VisibleClause shares the contract anchor policy used by filing and delivery.
func VisibleClause(ctx context.Context, alias string, arg func(any) int) (string, error) {
	return auth.ContractVisibleClause(ctx, alias, arg)
}

func trueWhenEmpty(clause string) string {
	if clause == "" {
		return "TRUE"
	}
	return clause
}
