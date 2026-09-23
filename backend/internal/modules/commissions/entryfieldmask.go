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
//
// The deal's amount reaches the ledger too, and by a different route. An
// entry's basis IS that amount, copied at accrual, so a mask on it that stopped
// at the deal would hand the figure back here. The closure cannot carry this
// one: a deal's mask may be conditioned on write authority, and which deals a
// caller may write is a question only the deal row answers — so the arm is
// resolved against that row, the way this module already resolves the deal's
// row scope.

import (
	"context"
	"strings"

	"github.com/margince/margince/backend/internal/platform/auth"
)

// dealAmountField is the WIRE field a deal's money mask names, which is not
// this module's vocabulary and is spelt here rather than reached for.
const dealAmountField = "amount_minor"

// maskExcludedClause renders what every ledger read ANDs in: the rows this
// caller's masks leave them. Empty means no mask reaches a commission entry and
// the read is unnarrowed.
func maskExcludedClause(ctx context.Context, arg func(any) int) (string, error) {
	clauses, err := auth.MaskExclusionClauses(ctx, commissionObject, "", arg)
	if err != nil {
		return "", err
	}
	basis, err := dealAmountExcludedClause(ctx, arg)
	if err != nil {
		return "", err
	}
	if basis != "" {
		clauses = append(clauses, basis)
	}
	return strings.Join(clauses, " AND "), nil
}

// dealAmountExcludedClause renders the arm that takes an entry out when the
// deal's own amount is withheld from this reader. Empty means no mask of theirs
// reaches it and the ledger is unnarrowed by this question.
//
// The rate is required and non-nullable on the wire like the rest, so there is
// no rendering of the basis that says "withheld" and the ROW leaves the read.
// Asked of the deal row rather than of the entry, because a conditioned mask
// lifts on the deals the caller may WRITE and the entry carries no such answer.
func dealAmountExcludedClause(ctx context.Context, arg func(any) int) (string, error) {
	clause, masked, err := auth.MaskExcludedClause(ctx, "deal", dealAmountField, "d", arg)
	if err != nil || !masked {
		return "", err
	}
	return "EXISTS (SELECT 1 FROM deal d WHERE d.id = deal_id AND " + clause + ")", nil
}
