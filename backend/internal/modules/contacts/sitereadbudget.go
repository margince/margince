// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package contacts

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/platform/database/storekit"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// BudgetDeferredSiteReads shares the status/recovery predicate for saved reads.
const BudgetDeferredSiteReads = "status = 'deferred' AND status_code = 'budget_deferred' AND next_attempt_at IS NOT NULL"

// BudgetReadResume binds queue recovery to the original dossier and requester.
type BudgetReadResume func(context.Context, pgx.Tx, BudgetRead) (bool, error)

// ResumeBudgetReads retains the dossier and the original request's limits.
func (s *Store) ResumeBudgetReads(ctx context.Context, resume BudgetReadResume) error {
	if err := auth.Require(ctx, "ai_budget", principal.ActionUpdate); err != nil {
		return err
	}
	return storekit.RecoverPages(ctx, s.tx, func(tx pgx.Tx, after ids.UUID) ([]BudgetRead, error) {
		args := []any{after}
		query := fmt.Sprintf(`SELECT `+`id,requested_by,company_id,target_kind`+` FROM site_read WHERE `+BudgetDeferredSiteReads+` AND next_attempt_at>now() AND id>$%d ORDER BY id LIMIT 100 FOR UPDATE SKIP LOCKED`, len(args))
		rows, err := tx.Query(ctx, query, args...)
		if err != nil {
			return nil, err
		}
		return pgx.CollectRows(rows, pgx.RowToStructByPos[BudgetRead])
	}, func(row BudgetRead) ids.UUID { return row.ID }, func(tx pgx.Tx, row BudgetRead) error {
		// The recovery caller and the original requester are separate principals;
		// the callback checks the latter before it advances the job.
		if err := RequireSiteReadAuthority(ctx, tx, row.CompanyID, row.TargetKind); err != nil {
			return err
		}
		ok, err := resume(ctx, tx, row)
		if err != nil || !ok {
			return err
		}
		args := []any{row.ID}
		query := fmt.Sprintf(`UPDATE site_read SET next_attempt_at=now(),updated_at=now() WHERE id=$%d RETURNING `+siteReadColumns, len(args))
		read, err := scanSiteRead(tx.QueryRow(ctx, query, args...))
		if err != nil {
			return err
		}
		return logSiteReadActivity(ctx, tx, read, 0)
	})
}

// BudgetRead carries persisted authority, including reads without a company yet.
type BudgetRead struct {
	ID         ids.UUID
	Requester  string
	CompanyID  *ids.UUID
	TargetKind string
}

// RequireSiteReadAuthority applies the commissioner's grant and live-row scope
// both when a read starts and when an earlier request becomes runnable again.
func RequireSiteReadAuthority(ctx context.Context, tx pgx.Tx, company *ids.UUID, targetKind string) error {
	if err := requireSiteReadGrant(ctx, targetKind); err != nil {
		return err
	}
	if company != nil {
		return auth.EnsureWritableLive(ctx, tx, "company", *company)
	}
	return nil
}

// An unbound read decides whether a company should exist; it does not update
// an existing record, so create authority is sufficient for that case.
func requireSiteReadGrant(ctx context.Context, targetKind string) error {
	if err := auth.Require(ctx, "company", principal.ActionUpdate); err != nil {
		if targetKind == TargetKindCompany {
			return err
		}
		return auth.Require(ctx, "company", principal.ActionCreate)
	}
	return nil
}
