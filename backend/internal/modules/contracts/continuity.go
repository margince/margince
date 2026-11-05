// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package contracts

import (
	"context"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/platform/database/storekit"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// CompanyContinuityTx cannot prove an ending from an incomplete account read.
func CompanyContinuityTx(ctx context.Context, tx pgx.Tx, company ids.CompanyID, day time.Time, lock bool) (active, complete bool, err error) {
	if err := auth.EnsureLinkTarget(ctx, tx, companyTable, company.UUID); err != nil {
		return false, false, err
	}
	if !auth.ReadGranted(ctx, contractObject) {
		return false, false, nil
	}
	var args []any
	arg := func(v any) int { args = append(args, v); return len(args) }
	companyPos, dayPos := arg(company), arg(day)
	scope, err := VisibleClause(ctx, "", arg)
	if err != nil {
		return false, false, err
	}
	scope = trueWhenEmpty(scope)
	from := "contract"
	if lock {
		// Name the scope and continuity columns so custom-field DDL cannot change the locked row shape.
		from = storekit.SQLf("(SELECT id, company_id, deal_id, status, starts_on, ends_on, cancellation_effective_on, archived_at FROM contract WHERE company_id = $%d ORDER BY id FOR UPDATE)", companyPos)
	}
	query := storekit.SQLf(`SELECT coalesce(bool_or(%s AND %s), false), coalesce(bool_and(%s), true)
 FROM %s AS contract WHERE company_id = $%d AND archived_at IS NULL`, UnderContractSQL(dayPos), scope, scope, from, companyPos)
	if err := tx.QueryRow(ctx, query, args...).Scan(&active, &complete); err != nil {
		return false, false, fmt.Errorf("read account contract continuity: %w", err)
	}
	return active, complete, nil
}
