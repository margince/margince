// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package companyscan

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/platform/database"
	"github.com/margince/margince/backend/internal/platform/database/storekit"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// BudgetDeferredScans identifies the saved scans whose retry clock recovery owns.
const BudgetDeferredScans = "status = 'queued' AND degrade_reason = 'budget_deferred' AND next_attempt_at IS NOT NULL"

// BudgetScanResume preserves the original queued request while advancing its job clock.
type BudgetScanResume func(context.Context, pgx.Tx, ids.UUID, ids.UUID, ids.UUID) (bool, error)

// ResumeBudgetScans makes eligible requests due without changing their owner or attempt.
func ResumeBudgetScans(ctx context.Context, db *database.DB, resume BudgetScanResume) error {
	if err := auth.Require(ctx, "ai_budget", principal.ActionUpdate); err != nil {
		return err
	}
	return storekit.RecoverPages(ctx, db.Tx, func(tx pgx.Tx, after ids.UUID) ([]budgetScan, error) {
		args := []any{after}
		query := fmt.Sprintf(`SELECT `+`id,user_id,company_id`+` FROM company_scan WHERE `+BudgetDeferredScans+` AND next_attempt_at>now() AND id>$%d ORDER BY id LIMIT 100 FOR UPDATE SKIP LOCKED`, len(args))
		rows, err := tx.Query(ctx, query, args...)
		if err != nil {
			return nil, err
		}
		return pgx.CollectRows(rows, pgx.RowToStructByPos[budgetScan])
	}, func(row budgetScan) ids.UUID { return row.ID }, func(tx pgx.Tx, row budgetScan) error {
		// Bound the recovery caller here; the callback also resolves the original
		// requester, whose authority may have changed since the scan was queued.
		if err := auth.EnsureVisible(ctx, tx, "company", row.CompanyID); err != nil {
			return err
		}
		ok, err := resume(ctx, tx, row.ID, row.Requester, row.CompanyID)
		if err != nil || !ok {
			return err
		}
		args := []any{row.ID}
		query := fmt.Sprintf(`UPDATE company_scan SET next_attempt_at=now() WHERE id=$%d RETURNING `+rowColumns, len(args))
		scan, err := scanRow(tx.QueryRow(ctx, query, args...))
		if err != nil {
			return err
		}
		return announce(ctx, tx, scan)
	})
}

type budgetScan struct {
	ID        ids.UUID
	Requester ids.UUID
	CompanyID ids.UUID
}
