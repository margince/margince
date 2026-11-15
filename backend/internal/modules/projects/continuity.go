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

// CompanyDeliveryTx treats delivery as work owed, excluding opportunities alone.
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
	// The company's projects are the ones it is on now, by a live edge: the
	// stored company_id keeps naming a company taken off the project. Whether
	// an account still has delivery is a fact about the account, so the edges
	// are read whole, whatever this caller may read of them.
	onProject := storekit.SQLf("p.id IN (SELECT c.project_id FROM relationship c WHERE c.kind = '%s'"+
		" AND c.company_id = $%d AND c.archived_at IS NULL)", auth.ProjectCompanyKind, companyPos)
	from := "project"
	if lock {
		// Removing a company takes its project's edges FOR UPDATE, so sharing
		// them here holds the delivery fact still. A new edge to the company
		// waits on the company row the decision has already locked.
		if _, err := tx.Exec(ctx, "SELECT 1 FROM relationship c WHERE c.kind = $1 AND c.company_id = $2"+
			" AND c.archived_at IS NULL FOR SHARE", auth.ProjectCompanyKind, company); err != nil {
			return false, false, fmt.Errorf("lock account delivery edges: %w", err)
		}
		// Name the scope and continuity columns so custom-field DDL cannot change the locked row shape.
		from = "(SELECT id, company_id, owner_id, phase, archived_at FROM project p WHERE " + onProject + " ORDER BY id FOR UPDATE)"
	}
	query := storekit.SQLf(`SELECT coalesce(bool_or(p.phase = $%d AND %s), false), coalesce(bool_and(%s), true)
 FROM %s p WHERE %s AND p.archived_at IS NULL`, phasePos, scope, scope, from, onProject)
	if err := tx.QueryRow(ctx, query, args...).Scan(&active, &complete); err != nil {
		return false, false, fmt.Errorf("read account delivery continuity: %w", err)
	}
	return active, complete, nil
}
