// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package privacy

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/platform/database/storekit"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/provenance"
)

const heldCommitmentStages = 1

// A legal hold postpones the accepted erasure; lifting it must finish that work.
func (s *RetentionService) evaluateHeldCommitments(ctx context.Context) error {
	var tasks []ids.UUID
	err := s.db.Tx(ctx, func(tx pgx.Tx) error {
		rows, err := tx.Query(ctx, `SELECT a.id FROM activity a
 JOIN activity_commitment_admission d ON d.source_activity_id = a.source_activity_id AND d.locator = a.source_id
 WHERE a.source_system = @task_origin AND d.outcome = @outcome AND a.restricted_at IS NULL
 `+notHeldThroughAnyLink("a.id")+` ORDER BY a.id LIMIT @limit`, pgx.StrictNamedArgs{
			"task_origin": provenance.CommitmentTaskSource, "outcome": auth.CommitmentEvidenceUnavailable, "limit": retentionBatch,
		})
		if err != nil {
			return err
		}
		tasks, err = pgx.CollectRows(rows, pgx.RowTo[ids.UUID])
		return err
	})
	if err != nil {
		return fmt.Errorf("retention: finding commitments awaiting erasure: %w", err)
	}
	for _, task := range tasks {
		if err := s.finishHeldCommitment(ctx, task); err != nil {
			return err
		}
	}
	return nil
}

func (s *RetentionService) finishHeldCommitment(ctx context.Context, task ids.UUID) error {
	return s.db.Tx(ctx, func(tx pgx.Tx) error {
		if _, err := storekit.LockRow(ctx, tx, "activity", task, storekit.IncludeArchived); err != nil {
			return err
		}
		item, err := unavailableCommitmentForTask(ctx, tx, task)
		if errors.Is(err, pgx.ErrNoRows) {
			return nil
		}
		if err != nil {
			return err
		}
		return s.eraser.eraseCommitmentDerivative(ctx, tx, item, aLegalHoldEnded)
	})
}
