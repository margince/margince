// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package deals

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/platform/database/storekit"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
	"github.com/margince/margince/backend/internal/shared/kernel/workforce"
)

// Why Margince leaves an open deal alone for want of an owner. A departed
// owner left; an inactive one is invited, suspended or archived.
const (
	OwnerlessNoOwner       = "no_owner"
	OwnerlessDepartedOwner = "departed_owner"
	OwnerlessInactiveOwner = "inactive_owner"
)

// NoLiveOwnerSQL is a deal with no owner, or with an owner who is no live
// member. The nightly verdict skips it and the team list shows it.
func NoLiveOwnerSQL(deal string) string {
	return `(` + deal + `.owner_id IS NULL OR NOT EXISTS (SELECT 1 FROM app_user nao WHERE nao.id = ` +
		deal + `.owner_id AND ` + workforce.LiveMemberSQL("nao") + `))`
}

// OwnerlessDeal is an open deal Margince leaves alone until somebody owns it.
type OwnerlessDeal struct {
	DealID   ids.DealID
	DealName string
	Reason   string
	OwnerID  *ids.UUID
	// ValueBaseMinor is nil when the deal has no amount, no rate, or the
	// reader's mask withholds it.
	ValueBaseMinor *int64
}

// ownerlessCap bounds the list; the extra row only says it was cut.
const ownerlessCap = 200

// OwnerlessDeals reads every open deal with no live owner under the reader's
// own row scope, the largest value first.
func (s *Store) OwnerlessDeals(ctx context.Context) ([]OwnerlessDeal, bool, error) {
	if err := auth.Require(ctx, "deal", principal.ActionRead); err != nil {
		return nil, false, err
	}
	var out []OwnerlessDeal
	err := s.Tx(ctx, func(tx pgx.Tx) error {
		base, err := s.installation.BaseCurrencyApplied(ctx, tx)
		if err != nil {
			return err
		}
		args := []any{}
		arg := func(v any) int { args = append(args, v); return len(args) }
		scope, err := auth.ScopeClauseFor(ctx, dealTable, "d", arg)
		if err != nil {
			return err
		}
		where := ""
		if scope != "" {
			where = " AND " + scope
		}
		value, err := auth.MaskedExpressionSQL(ctx, maskObject, "amount_minor", "d",
			BaseValueSQL("CURRENT_DATE", fmt.Sprintf("$%d", arg(base)), "d"), arg)
		if err != nil {
			return err
		}
		query := storekit.SQLf(`
			SELECT d.id, d.name,
			       CASE WHEN d.owner_id IS NULL THEN $%d
			            WHEN EXISTS (SELECT 1 FROM app_user du WHERE du.id = d.owner_id AND %s) THEN $%d
			            ELSE $%d END,
			       d.owner_id, %s
			  FROM deal d
			 WHERE d.status = 'open' AND d.archived_at IS NULL AND %s%s
			 ORDER BY 5 DESC NULLS LAST, d.id
			 LIMIT $%d`, arg(OwnerlessNoOwner), workforce.DepartedMemberSQL("du"), arg(OwnerlessDepartedOwner),
			arg(OwnerlessInactiveOwner), value, NoLiveOwnerSQL("d"), where, arg(ownerlessCap+1))
		rows, err := tx.Query(ctx, query, args...)
		if err != nil {
			return fmt.Errorf("deals: reading the deals nobody can act for: %w", err)
		}
		out, err = pgx.CollectRows(rows, func(row pgx.CollectableRow) (OwnerlessDeal, error) {
			var d OwnerlessDeal
			return d, row.Scan(&d.DealID, &d.DealName, &d.Reason, &d.OwnerID, &d.ValueBaseMinor)
		})
		return err
	})
	if err != nil {
		return nil, false, err
	}
	if len(out) > ownerlessCap {
		return out[:ownerlessCap], true, nil
	}
	return out, false, nil
}
