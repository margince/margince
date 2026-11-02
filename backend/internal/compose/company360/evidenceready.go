// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package company360

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/modules/activities"
	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

func requireCompanyEvidenceReady(ctx context.Context, tx pgx.Tx, companyID ids.CompanyID, opts AssembleOptions) error {
	var args []any
	arg := func(value any) int { args = append(args, value); return len(args) }
	company := arg(companyID)
	pending, err := auth.ActivityRepairPendingClause(ctx, "a", arg)
	if err != nil {
		return err
	}
	visible, err := auth.ActivityDiscoverClause(ctx, "a", arg)
	if err != nil {
		return err
	}
	sourcePending, err := auth.ActivityRepairPendingClause(ctx, "repair_source", arg)
	if err != nil {
		return err
	}
	var incomplete bool
	query := `SELECT EXISTS (SELECT 1 FROM activity a WHERE ` + activities.CompanyLinkedActivityExists(company) + opts.projectScope(arg) +
		` AND ((` + pending + `) OR ((` + visible + `) AND EXISTS (SELECT 1 FROM activity repair_source
 WHERE repair_source.id=a.source_activity_id AND (` + sourcePending + `)))))`
	if err := tx.QueryRow(ctx, query, args...).Scan(&incomplete); err != nil {
		return fmt.Errorf("reading account evidence readiness: %w", err)
	}
	if incomplete {
		return apperrors.ErrConflict
	}
	return nil
}
