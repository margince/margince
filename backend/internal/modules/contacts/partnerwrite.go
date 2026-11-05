// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package contacts

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/platform/database/storekit"
)

// partnerWrite is what one partner upsert did to the partner row. A write that
// left the partner as it was is not landed, and is not recorded.
type partnerWrite struct {
	row           partnerRow
	before, after map[string]any
	landed        bool
}

// writePartner lands the promotion-or-update and takes it back when it left the
// partner as it was.
//
// The upsert coalesces absent fields onto stored values, so only the returned
// row shows whether anything moved. By then the version trigger has run. The
// write runs in a savepoint, rolled back when the result equals the standing row.
//
// A retired partner coming back, or a company that did not yet carry the
// 'partner' relationship type, still counts as a change.
func writePartner(ctx context.Context, tx pgx.Tx, up partnerUpsert, standing map[string]any) (partnerWrite, error) {
	in := up.in
	var wasLive bool
	if err := tx.QueryRow(ctx,
		`SELECT EXISTS (SELECT 1 FROM partner WHERE company_id = $1 AND archived_at IS NULL)`,
		in.CompanyID).Scan(&wasLive); err != nil {
		return partnerWrite{}, fmt.Errorf("read whether the partner is live: %w", err)
	}
	var staged partnerWrite
	landed, err := storekit.Savepoint(ctx, tx, func(nested pgx.Tx) (bool, error) {
		var promoted bool
		var err error
		if staged, promoted, err = stagePartner(ctx, nested, up, standing); err != nil {
			return false, err
		}
		return len(staged.after) > 0 || promoted || !wasLive, nil
	})
	if err != nil {
		return partnerWrite{}, err
	}
	if landed {
		staged.landed = true
		return staged, nil
	}
	standingRow, err := scanPartner(tx.QueryRow(ctx,
		`SELECT `+partnerColumns+` FROM partner WHERE company_id = $1`, in.CompanyID))
	if err != nil {
		return partnerWrite{}, fmt.Errorf("read the unchanged partner: %w", err)
	}
	return partnerWrite{row: standingRow}, nil
}

// stagePartner runs the upsert and the relationship-type assertion, then diffs
// the result against the row the caller read under the company lock.
func stagePartner(ctx context.Context, tx pgx.Tx, up partnerUpsert, standing map[string]any) (w partnerWrite, promoted bool, err error) {
	row, err := upsertPartnerRow(ctx, tx, up)
	if err != nil {
		return partnerWrite{}, false, err
	}
	// A company is a partner when it has this row and a live 'partner'
	// relationship type. Both are written in one transaction so the two cannot
	// disagree.
	promoted, err = ensureCompanyRelationshipType(ctx, tx, up.in.CompanyID,
		relationshipTypePartner, "system", up.capturedBy)
	if err != nil {
		return partnerWrite{}, false, err
	}
	// The images come from the row as it stands, not from the request. A field
	// the caller left out kept its stored value and did not move.
	before, after := storekit.ChangedColumns(standing, partnerAuditImage(row))
	return partnerWrite{row: row, before: before, after: after}, promoted, nil
}
