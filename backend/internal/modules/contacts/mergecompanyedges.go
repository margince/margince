// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package contacts

import (
	"context"
	"fmt"
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

// liveEmployeesOf lists the contacts holding a live employment edge at the
// company, in id order. A company merge reads it before the edges move, because
// afterwards the source no longer names them.
func liveEmployeesOf(ctx context.Context, tx pgx.Tx, company ids.CompanyID) ([]ids.ContactID, error) {
	rows, err := tx.Query(ctx, `SELECT DISTINCT contact_id FROM relationship
 WHERE kind=$1 AND company_id=$2 AND contact_id IS NOT NULL AND archived_at IS NULL ORDER BY contact_id`,
		employmentKind, company)
	if err != nil {
		return nil, fmt.Errorf("list the employees of the merged-away company: %w", err)
	}
	return pgx.CollectRows(rows, pgx.RowTo[ids.ContactID])
}

// fileMergedEmployerTwins files the same-employer twins the moved edges
// created: two similar-name contacts who worked at different companies now
// share one.
func fileMergedEmployerTwins(ctx context.Context, tx pgx.Tx, employees []ids.ContactID, by string) error {
	for _, contact := range employees {
		if err := fileEmployerTwins(ctx, tx, contact, by); err != nil {
			return err
		}
	}
	return nil
}
