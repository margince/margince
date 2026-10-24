// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package deals

// The money a project's deals add up to, for the project page header.
//
// It lives with the deals rather than with the project: every figure here is
// read from the deal table under the caller's DEAL row scope and the field
// masks that withhold its money, and priced in the installation's base
// currency, which is this module's Installation seam. modules/projects
// consumes it through a port compose injects.

import (
	"context"
	"strings"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/platform/database/storekit"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// ProjectDealTotals values open and won deals with the same base-currency rule.
// A nil sum with an unpriced count distinguishes missing prices from no work.
type ProjectDealTotals struct {
	OpenMinor    *int64
	WonMinor     *int64
	OpenUnpriced int
	WonUnpriced  int
	Currency     string
}

// ProjectDealTotalsTx sums the open and the won deals of a project over the
// caller's deal row scope and field masks, inside a caller-opened transaction.
// A total that counted a deal the caller's list would not show discloses that
// deal through arithmetic, which is why the narrowing is here and not only on
// the list.
func (s *Store) ProjectDealTotalsTx(ctx context.Context, tx pgx.Tx, id ids.ProjectID) (ProjectDealTotals, error) {
	if err := auth.Require(ctx, "deal", principal.ActionRead); err != nil {
		return ProjectDealTotals{}, err
	}
	base, err := s.installation.BaseCurrency(ctx, tx)
	if err != nil {
		return ProjectDealTotals{}, err
	}
	var args []any
	arg := func(v any) int { args = append(args, v); return len(args) }
	baseSQL := storekit.SQLf("$%d", arg(base))
	asOfSQL := storekit.SQLf("$%d", arg(s.clock()))
	where := []string{storekit.SQLf("d.project_id = $%d", arg(id)), "d.archived_at IS NULL"}
	scope, err := auth.ScopeClauseFor(ctx, dealTable, "d", arg)
	if err != nil {
		return ProjectDealTotals{}, err
	}
	if scope != "" {
		where = append(where, scope)
	}
	// A total is the one place a withheld figure reappears whole, and a project
	// carrying one deal makes the total that deal's amount. So a deal whose
	// money this caller reads as masked is outside the row set, the way the
	// projects report excludes it from the same two measures. It belongs to the
	// row set rather than to either sum because every figure this statement
	// answers is that money: a count would need its own unmasked filter.
	priceable, masked, err := auth.MaskExcludedClause(ctx, maskObject, dealAmountField, "d", arg)
	if err != nil {
		return ProjectDealTotals{}, err
	}
	if masked {
		where = append(where, priceable)
	}
	totals := ProjectDealTotals{Currency: base}
	err = tx.QueryRow(ctx, storekit.SQLf(`
		SELECT %s, %s, %s, %s FROM deal d WHERE %s`,
		FinalizeBaseTotalSQL(BaseValueSumSQL(asOfSQL, baseSQL, "d", "d.status = 'open'"), UnpricedCountSQL(asOfSQL, baseSQL, "d", "d.status = 'open'")),
		FinalizeBaseTotalSQL(BaseValueSumSQL(asOfSQL, baseSQL, "d", "d.status = 'won'"), UnpricedCountSQL(asOfSQL, baseSQL, "d", "d.status = 'won'")),
		UnpricedCountSQL(asOfSQL, baseSQL, "d", "d.status = 'open'"),
		UnpricedCountSQL(asOfSQL, baseSQL, "d", "d.status = 'won'"), strings.Join(where, " AND ")),
		args...).Scan(&totals.OpenMinor, &totals.WonMinor, &totals.OpenUnpriced, &totals.WonUnpriced)
	if err != nil {
		return ProjectDealTotals{}, err
	}
	return totals, nil
}
