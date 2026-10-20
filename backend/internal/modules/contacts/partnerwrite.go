// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package contacts

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/platform/database/storekit"
)

// partnerWrite is what one partner upsert did to the partner row: the row as it
// stands, and the audit images narrowed to the columns that moved. A write that
// left the partner as it was is not landed, and is not recorded.
type partnerWrite struct {
	row           partnerRow
	before, after map[string]any
	landed        bool
}

// writePartner lands the promotion-or-update and takes it back when it left the
// partner exactly as it was.
//
// The upsert is one statement whose absent fields coalesce onto their current
// values, so whether it moved anything is only knowable from the row it returns.
// By then the table's version trigger has already run, and a save that changed
// nothing would have moved the version and made every other editor's If-Match
// stale. So the write runs in a savepoint, and a result equal to the standing
// row rolls the savepoint back.
//
// Promotion counts as a change even when the partner fields are identical: a
// retired partner row coming back, or a company that did not yet carry the
// 'partner' relationship type, is a fact about the company.
func writePartner(
	ctx context.Context, tx pgx.Tx, in UpsertPartnerInput, fit partnerFitState, capturedBy string,
	standing map[string]any,
) (partnerWrite, error) {
	var wasLive bool
	if err := tx.QueryRow(ctx,
		`SELECT EXISTS (SELECT 1 FROM partner WHERE company_id = $1 AND archived_at IS NULL)`,
		in.CompanyID).Scan(&wasLive); err != nil {
		return partnerWrite{}, fmt.Errorf("read whether the partner is live: %w", err)
	}
	nested, err := tx.Begin(ctx)
	if err != nil {
		return partnerWrite{}, fmt.Errorf("open the partner write: %w", err)
	}
	staged, promoted, err := stagePartner(ctx, nested, in, fit, capturedBy, standing)
	if err != nil {
		if rbErr := nested.Rollback(ctx); rbErr != nil {
			return partnerWrite{}, errors.Join(err, rbErr)
		}
		return partnerWrite{}, err
	}
	if len(staged.after) > 0 || promoted || !wasLive {
		if err := nested.Commit(ctx); err != nil {
			return partnerWrite{}, fmt.Errorf("commit the partner write: %w", err)
		}
		staged.landed = true
		return staged, nil
	}
	if err := nested.Rollback(ctx); err != nil {
		return partnerWrite{}, fmt.Errorf("withdraw the partner write that changed nothing: %w", err)
	}
	standingRow, err := scanPartner(tx.QueryRow(ctx,
		`SELECT `+partnerColumns+` FROM partner WHERE company_id = $1`, in.CompanyID))
	if err != nil {
		return partnerWrite{}, fmt.Errorf("read the unchanged partner: %w", err)
	}
	return partnerWrite{row: standingRow}, nil
}

// stagePartner runs the upsert and the relationship-type assertion that is the
// other half of the partner invariant, and diffs the result against the row the
// caller read under the company lock.
func stagePartner(
	ctx context.Context, tx pgx.Tx, in UpsertPartnerInput, fit partnerFitState, capturedBy string,
	standing map[string]any,
) (w partnerWrite, promoted bool, err error) {
	row, err := upsertPartnerRow(ctx, tx, in, fit, capturedBy)
	if err != nil {
		return partnerWrite{}, false, err
	}
	// Promotion is a company fact, and it is the invariant's other half: a company
	// IS a partner iff it has this row AND a live 'partner' relationship
	// type (ADR-0079 amending ADR-0032 §Decision 2). Both are written in
	// this one transaction, so the two can never disagree — the
	// classification flip this replaces did the same job for the same
	// reason, against a column that could hold only one answer.
	promoted, err = ensureCompanyRelationshipType(ctx, tx, in.CompanyID,
		relationshipTypePartner, "system", capturedBy)
	if err != nil {
		return partnerWrite{}, false, err
	}
	// The row as it now stands, not the request: an absent field coalesced
	// onto its current value did not change, and an image built from the
	// request would claim ownership of every field the caller left out.
	// Narrowing to what moved is the same rule from the other side — it is
	// what keeps a lifecycle edit from claiming the fit fields.
	before, after := storekit.ChangedColumns(standing, partnerAuditImage(row))
	return partnerWrite{row: row, before: before, after: after}, promoted, nil
}
