// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package commissions

import (
	"context"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/platform/database/storekit"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// Accrual is one ledger entry as a report counts it. The amount is in the
// deal's currency, with the rate to base the entry kept when it accrued.
type Accrual struct {
	ID     ids.UUID
	DealID ids.UUID
	// PartnerCompanyID is nil when the caller may not see the partner company.
	PartnerCompanyID *ids.UUID
	AmountMinor      int64
	Currency         string
	FxRateToBase     pgtype.Numeric
	AccruedAt        time.Time
}

// AccrualsTx lists the entries accrued in [from, to) that still stand, under
// the same readable predicate as Summary. A void entry was taken back, so it
// is left out. withheld reports that a mask left some entries out. More than
// limit entries is refused rather than cut short, so no total is silently low.
func AccrualsTx(ctx context.Context, tx pgx.Tx, from, to time.Time, limit int) ([]Accrual, bool, error) {
	if err := auth.Require(ctx, commissionObject, principal.ActionRead); err != nil {
		return nil, false, err
	}
	var args []any
	arg := func(v any) int { args = append(args, v); return len(args) }
	where, withheld, err := readableWhere(ctx, arg)
	if err != nil {
		return nil, false, err
	}
	partner, err := visiblePartnerSQL(ctx, arg)
	if err != nil {
		return nil, false, err
	}
	query := storekit.SQLf(
		`SELECT id, deal_id, %s, amount_minor, currency, fx_rate_to_base, created_at
		   FROM commission_entry
		  WHERE status <> 'void' AND created_at >= $%d AND created_at < $%d AND %s
		  ORDER BY created_at, id LIMIT $%d`, partner, arg(from), arg(to), where, arg(limit+1))
	rows, err := tx.Query(ctx, query, args...)
	if err != nil {
		return nil, false, fmt.Errorf("read commission accruals: %w", err)
	}
	out, err := pgx.CollectRows(rows, func(row pgx.CollectableRow) (Accrual, error) {
		var a Accrual
		err := row.Scan(&a.ID, &a.DealID, &a.PartnerCompanyID, &a.AmountMinor, &a.Currency, &a.FxRateToBase, &a.AccruedAt)
		return a, err
	})
	if err != nil {
		return nil, false, fmt.Errorf("scan commission accruals: %w", err)
	}
	if len(out) > limit {
		return nil, false, fmt.Errorf("narrow the reporting population before evaluating: %w", apperrors.ErrInvalidArgument)
	}
	return out, withheld, nil
}

// visiblePartnerSQL renders the partner company column as the caller may see
// it: the id when they may read that company, NULL when they may not. The
// entry still counts, because its visibility is its deal's.
func visiblePartnerSQL(ctx context.Context, arg func(any) int) (string, error) {
	if !auth.ReadGranted(ctx, "company") {
		return "NULL::uuid", nil
	}
	scope, err := auth.ScopeClauseFor(ctx, "company", "pc", arg)
	if err != nil {
		return "", err
	}
	if scope == "" {
		return "partner_company_id", nil
	}
	return "CASE WHEN EXISTS (SELECT 1 FROM company pc WHERE pc.id = partner_company_id AND " + scope + ") THEN partner_company_id END", nil
}
