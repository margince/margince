// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package contacts

import (
	"context"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/platform/database/storekit"
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
	mergeArgs := pgx.NamedArgs{mergeSourceKey: sourceID, mergeTargetKey: targetID, archiveStateKey: time.Now().UTC()}
	if _, err := mergeEdgeWrite(ctx, tx, `archived_at=@archived`, mergeCompanyEdgeSource+` AND `+mergeCompanyEdgeSelf, mergeArgs); err != nil {
		return err
	}
	if err := foldFirstObservations(ctx, tx, companyFK, sourceID.UUID, targetID.UUID); err != nil {
		return err
	}
	if _, err := mergeEdgeWrite(ctx, tx, `archived_at=@archived`, mergeCompanyEdgeSource+` AND `+mergeCompanyEdgeDuplicate, mergeArgs); err != nil {
		return err
	}
	if _, err := mergeEdgeWrite(ctx, tx, `company_id=@target,is_current_primary=`+mergeCompanyEdgePrimary, mergeCompanyForwardSource, mergeArgs); err != nil {
		return err
	}
	_, err := mergeEdgeWrite(ctx, tx, `counterparty_company_id=@target`, `a.counterparty_company_id=@source AND a.archived_at IS NULL`, mergeArgs)
	return err
}
