// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package deals

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/platform/database/storekit"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// evidenceAuthorshipBatch bounds one reconciliation transaction.
const evidenceAuthorshipBatch = 200

// ReconcileEvidenceAuthorship judges again who wrote each activity behind
// live stage evidence, against the current own domains. The side was judged
// at the write, so an own domain added or removed since leaves it wrong.
// A changed side is corrected through the audited writer, and the stage
// policy reads it again (SettlesBuyerMilestone).
//
// Refuted rows stay as a member left them. Each batch commits on its own, so
// a repeated pass finds nothing left. It answers how many rows it corrected.
func (s *Store) ReconcileEvidenceAuthorship(ctx context.Context, own OwnDomainReader) (int, error) {
	if err := auth.RequireSystem(ctx); err != nil {
		return 0, err
	}
	corrected := 0
	var after ids.UUID
	for {
		var scanned int
		err := s.Tx(ctx, func(tx pgx.Tx) error {
			var err error
			var changed int
			scanned, changed, after, err = reconcileEvidenceAuthorshipTx(ctx, tx, own, after)
			corrected += changed
			return err
		})
		if err != nil {
			return corrected, err
		}
		if scanned < evidenceAuthorshipBatch {
			return corrected, nil
		}
	}
}

type activityEvidence struct {
	ID       ids.UUID
	DealID   ids.UUID
	SourceID ids.UUID
	Side     string
}

// reconcileEvidenceAuthorshipTx corrects one page of activity-backed evidence
// after the cursor. It answers the rows it inspected, the rows it changed and
// the next cursor.
func reconcileEvidenceAuthorshipTx(
	ctx context.Context, tx pgx.Tx, own OwnDomainReader, after ids.UUID,
) (int, int, ids.UUID, error) {
	rows, err := tx.Query(ctx, `
		SELECT id, deal_id, source_id, author_side FROM deal_stage_evidence
		 WHERE source_type = $1 AND refuted_at IS NULL AND id > $2
		 ORDER BY id LIMIT $3 FOR UPDATE`, SourceActivity, after, evidenceAuthorshipBatch)
	if err != nil {
		return 0, 0, after, fmt.Errorf("deals: reading evidence to judge its author again: %w", err)
	}
	page, err := pgx.CollectRows(rows, pgx.RowToStructByPos[activityEvidence])
	if err != nil {
		return 0, 0, after, fmt.Errorf("deals: reading evidence to judge its author again: %w", err)
	}
	sides := map[ids.UUID]AuthorSide{}
	changed := 0
	for _, ev := range page {
		after = ev.ID
		side, known := sides[ev.SourceID]
		if !known {
			authorship, err := ReadActivityAuthorship(ctx, tx, ev.SourceID, own)
			if errors.Is(err, apperrors.ErrNotFound) {
				// The activity is archived or reaches no deal now. Nothing
				// reads its side for a stage move, so it is left as written.
				sides[ev.SourceID] = AuthorSide(ev.Side)
				continue
			}
			if err != nil {
				return len(page), changed, after, err
			}
			side = authorship.AuthorSide
			sides[ev.SourceID] = side
		}
		if string(side) == ev.Side {
			continue
		}
		if err := correctEvidenceAuthor(ctx, tx, ev, side); err != nil {
			return len(page), changed, after, err
		}
		changed++
	}
	return len(page), changed, after, nil
}

// correctEvidenceAuthor writes the side the current own domains give, with
// the audit row and the deal event every evidence change carries.
func correctEvidenceAuthor(ctx context.Context, tx pgx.Tx, ev activityEvidence, side AuthorSide) error {
	lock, err := storekit.LockRow(ctx, tx, evidenceEntity, ev.ID, storekit.IncludeArchived)
	if err != nil {
		return err
	}
	patch := storekit.NewPatch()
	patch.Set("author_side", ev.Side, string(side))
	if err := patch.ApplyLocked(ctx, tx, lock); err != nil {
		return fmt.Errorf("deals: correcting an evidence author: %w", err)
	}
	auditID, err := storekit.AuditWithEvidence(ctx, tx, "update", evidenceEntity, ev.ID,
		patch.Before(), patch.After(), map[string]any{"rejudged_against": "own_domains"})
	if err != nil {
		return fmt.Errorf("deals: auditing a corrected evidence author: %w", err)
	}
	return emitEvidenceChanged(ctx, tx, auditID, ids.From[ids.DealKind](ev.DealID))
}
