// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package privacy

// The quotations a deal's stage evidence holds of a message whose text is
// destroyed. Its own file because both destructive engines reach it: the
// Art. 17 cascade for the subject's timeline, and the per-activity content
// purge every retention and release arm runs.

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// redactQuotationsOf withdraws the proposals and clears the stage evidence
// that quote the activities an Art. 17 request emptied or holds. Held rows are
// included because a held record reads as gone, and a deal would go on showing
// its words.
func redactQuotationsOf(ctx context.Context, tx pgx.Tx, activities []ids.UUID, reason string) error {
	if err := redactApprovalsCitingActivities(ctx, tx, activities, ErasedSourceWithdrawal); err != nil {
		return err
	}
	return redactEvidenceQuoting(ctx, tx, activities, reason, causeContactErasure)
}

// redactEvidenceQuoting clears the snippet of every stage-evidence row read
// out of one of these activities, and tombstones each row it cleared.
//
// The snippet goes and the row stays. The ledger keeps a claim even after it
// is refuted, because a stage move rests on it. No cascade reaches the
// column: source_id is polymorphic, and the activity is emptied in place.
func redactEvidenceQuoting(ctx context.Context, tx pgx.Tx, activities []ids.UUID, reason, cause string) error {
	if len(activities) == 0 {
		return nil
	}
	rows, err := tx.Query(ctx, `
		UPDATE deal_stage_evidence SET snippet = NULL
		 WHERE source_type = 'activity' AND source_id = ANY($1) AND snippet IS NOT NULL
		RETURNING id`, activities)
	if err != nil {
		return fmt.Errorf("privacy: clearing stage evidence quoting an erased message: %w", err)
	}
	cleared, err := pgx.CollectRows(rows, pgx.RowTo[ids.UUID])
	if err != nil {
		return fmt.Errorf("privacy: clearing stage evidence quoting an erased message: %w", err)
	}
	return tombstoneCollateralScrubs(ctx, tx, "deal_stage_evidence", cleared, reason, cause)
}
