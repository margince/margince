// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package projects

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/platform/database/storekit"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// Delivery is work owed; pursuing an opportunity alone proves no active engagement.
func CompanyDeliveryTx(ctx context.Context, tx pgx.Tx, company ids.CompanyID, lock bool) (active, complete bool, err error) {
	if err := auth.EnsureLinkTarget(ctx, tx, "company", company.UUID); err != nil {
		return false, false, err
	}
	if !auth.ReadGranted(ctx, projectObject) {
		return false, false, nil
	}
	var args []any
	arg := func(v any) int { args = append(args, v); return len(args) }
	companyPos, phasePos := arg(company), arg(PhaseDelivering)
	scope, err := auth.ScopeClauseFor(ctx, projectObject, "p", arg)
	if err != nil {
		return false, false, err
	}
	if scope == "" {
		scope = "TRUE"
	}
	from := "project"
	if lock {
		from = storekit.SQLf("(SELECT * FROM project WHERE company_id = $%d ORDER BY id FOR UPDATE)", companyPos)
	}
	query := storekit.SQLf(`SELECT coalesce(bool_or(p.phase = $%d AND %s), false), coalesce(bool_and(%s), true)
 FROM %s p WHERE p.company_id = $%d AND p.archived_at IS NULL`, phasePos, scope, scope, from, companyPos)
	if err := tx.QueryRow(ctx, query, args...).Scan(&active, &complete); err != nil {
		return false, false, fmt.Errorf("read account delivery continuity: %w", err)
	}
	return active, complete, nil
}
