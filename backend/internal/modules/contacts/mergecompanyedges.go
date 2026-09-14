// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package contacts

import (
	"context"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/platform/database/storekit"
	"github.com/margince/margince/backend/internal/shared/kernel/employment"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// relinkCompanyEdges moves A's relationship edges to B across both company
// columns. Order matters: edges that would degenerate (A↔B partner
// edges, duplicates of what B already has) archive first, then the
// survivors relink.
func relinkCompanyEdges(ctx context.Context, tx pgx.Tx, sourceID, targetID ids.CompanyID) error {
	if err := relinkEmploymentSupport(ctx, tx, sourceID.UUID, targetID.UUID, false); err != nil {
		return err
	}
	args := []any{sourceID}
	sourcePos := len(args)
	args = append(args, targetID)
	if _, err := tx.Exec(ctx, storekit.SQLf(`UPDATE provider_employment_resolution SET company_id=$%d WHERE company_id=$%d`, len(args), sourcePos), args...); err != nil {
		return err
	}
	now := time.Now().UTC()
	// An edge between the two merging companies would become a self-edge.
	if _, err := tx.Exec(ctx, `
		UPDATE relationship SET archived_at = $3
		WHERE archived_at IS NULL
		  AND ((company_id = $1 AND counterparty_company_id = $2)
		    OR (company_id = $2 AND counterparty_company_id = $1))`,
		sourceID, targetID, now); err != nil {
		return err
	}
	// Duplicates of edges the survivor already has, on either column.
	if _, err := tx.Exec(ctx, `
		UPDATE relationship a SET archived_at = $3
		WHERE a.archived_at IS NULL
		  AND (a.company_id = $1 OR a.counterparty_company_id = $1)
		  AND EXISTS (
		    SELECT 1 FROM relationship b
		    WHERE b.kind = a.kind AND b.archived_at IS NULL AND b.id <> a.id
		      AND b.contact_id IS NOT DISTINCT FROM a.contact_id
		      AND b.deal_id IS NOT DISTINCT FROM a.deal_id
		      -- The project too, or a project_company edge on project A is
		      -- read as a duplicate of one on project B merely because both
		      -- name the survivor — and the company silently leaves project A.
		      AND b.project_id IS NOT DISTINCT FROM a.project_id
		      AND b.company_id IS NOT DISTINCT FROM
		            (CASE WHEN a.company_id = $1 THEN $2::uuid ELSE a.company_id END)
		      AND b.counterparty_company_id IS NOT DISTINCT FROM
		            (CASE WHEN a.counterparty_company_id = $1 THEN $2::uuid ELSE a.counterparty_company_id END)
		      AND `+roleKeyedDuplicateSQL+`)`,
		sourceID, targetID, now); err != nil {
		return err
	}
	// Relinked employment edges keep ≤1 current-primary per contact.
	if _, err := tx.Exec(ctx, `
		UPDATE relationship a SET company_id = $2,
		  is_current_primary = a.is_current_primary AND NOT EXISTS (
		    SELECT 1 FROM relationship b
		    WHERE b.contact_id = a.contact_id AND b.id <> a.id
		      AND `+employment.CurrentPrimarySlotSQL("b")+`)
		WHERE a.company_id = $1 AND a.archived_at IS NULL`, sourceID, targetID); err != nil {
		return err
	}
	_, err := tx.Exec(ctx,
		`UPDATE relationship SET counterparty_company_id = $2
		 WHERE counterparty_company_id = $1 AND archived_at IS NULL`, sourceID, targetID)
	return err
}
