// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package commissions

// The ledger meeting a field mask. An entry is the partner's margin tier
// applied to money and it says so three ways: margin_tier_at_accrual, the
// rate_bps the tier became, and amount_minor over basis_amount_minor, whose
// quotient IS that rate. Withholding the first alone leaves the other two
// standing, which is a mask that does not mask.
//
// The rate and the amount are required, non-nullable integers on the wire, so
// no rendering of them says "withheld". So the ROW leaves the read instead of a
// column leaving the row — the same answer the report engine gives an aggregate
// over a masked column, for the same reason: there is no per-row place to put
// "withheld" and a figure that is a function of the withheld one is the
// withheld one.
//
// The reach itself is declared in auth's group closure, not here and not
// between the two modules: contacts withholds the tier on its own record and
// the ledger asks which of its rows survive.

import (
	"context"
	"strings"

	"github.com/margince/margince/backend/internal/platform/auth"
)

// maskExcludedClause renders what every ledger read ANDs in: the rows this
// caller's masks leave them. Empty means no mask reaches a commission entry and
// the read is unnarrowed.
func maskExcludedClause(ctx context.Context, arg func(any) int) (string, error) {
	clauses, err := auth.MaskExclusionClauses(ctx, commissionObject, "", arg)
	if err != nil {
		return "", err
	}
	return strings.Join(clauses, " AND "), nil
}
