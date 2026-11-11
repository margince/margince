// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package privacy

import (
	"context"
	"errors"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// ApprovalHistoryReview is the review source kind for the audit history of one
// emptied approval that no erasure can be proved for.
const ApprovalHistoryReview = "approval_history"

const approvalEntity = "approval"

// UnlinkedApprovalHistoryPageTx names the emptied, decided approvals whose old
// audit copies no erasure proves whom they concerned, for a reviewer to check.
// A link proved since the correction step ran gets that step's correction here.
func UnlinkedApprovalHistoryPageTx(ctx context.Context, tx pgx.Tx, after, until ids.UUID, limit int, country CountryReader) (HistoricalAuditPage, error) {
	if err := auth.RequireSystem(ctx); err != nil {
		return HistoricalAuditPage{}, err
	}
	floor, err := ResolveFloorPolicy(ctx, tx, country)
	if err != nil {
		return HistoricalAuditPage{}, err
	}
	entries, err := legacyAuditEntries(ctx, tx, after, until, limit)
	if err != nil {
		return HistoricalAuditPage{}, err
	}
	page := HistoricalAuditPage{}
	named := map[ids.UUID]bool{}
	for _, entry := range entries {
		page.Scanned = append(page.Scanned, entry.id)
		if !entry.needed || entry.kind != approvalEntity {
			continue
		}
		match, linked, err := legacyAuditSubject(ctx, tx, entry)
		if err != nil {
			return HistoricalAuditPage{}, err
		}
		if linked {
			changed, err := correctLinkedHistoricalEntry(ctx, tx, entry, match, floor)
			if err != nil {
				return HistoricalAuditPage{}, err
			}
			if changed {
				page.Changed++
			}
			continue
		}
		if named[entry.entity] {
			continue
		}
		review, err := unlinkedApprovalNeedsReview(ctx, tx, entry, floor)
		if err != nil {
			return HistoricalAuditPage{}, err
		}
		if review {
			named[entry.entity] = true
			page.Review = append(page.Review, ErasureReviewSource{Kind: ApprovalHistoryReview, ID: entry.entity})
		}
	}
	return page, nil
}

// unlinkedApprovalNeedsReview reports an emptied, decided approval with no
// proved link. A hold on its target keeps it out of review as out of correction.
func unlinkedApprovalNeedsReview(ctx context.Context, tx pgx.Tx, entry legacyAuditEntry, floor FloorPolicy) (bool, error) {
	var targetType *string
	var targetID *ids.UUID
	err := tx.QueryRow(ctx, `SELECT record.target_entity_type,record.target_entity_id FROM approval record
 WHERE record.id=@entity AND `+blankDecidedApproval, pgx.StrictNamedArgs{"entity": entry.entity}).Scan(&targetType, &targetID)
	if errors.Is(err, pgx.ErrNoRows) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	if targetType == nil || targetID == nil {
		return true, nil
	}
	held, err := historicalAuditUnderHold(ctx, tx, entry.id, *targetType, *targetID, floor)
	if err != nil {
		return false, err
	}
	return !held, nil
}
