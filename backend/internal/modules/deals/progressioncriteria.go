// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package deals

import (
	"context"
	"errors"

	"github.com/jackc/pgx/v5"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/platform/database/storekit"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// ValidateProgressionCriteriaTx holds the checklist against its writers until the move commits.
func (s *Store) ValidateProgressionCriteriaTx(ctx context.Context, tx pgx.Tx, change StageProgressionChange) error {
	if err := auth.Require(ctx, "pipeline", principal.ActionRead); err != nil {
		return err
	}
	var stageID ids.StageID
	// Criteria writers take this stage FOR UPDATE; concurrent decisions may share the read lock.
	err := tx.QueryRow(ctx, `SELECT id FROM stage WHERE id = $1 AND archived_at IS NULL FOR SHARE`, change.FromStageID).Scan(&stageID)
	if errors.Is(err, pgx.ErrNoRows) {
		return apperrors.ErrNotFound
	}
	if err != nil {
		return err
	}
	current, err := readCriteria(ctx, tx, stageID, storekit.LiveOnly)
	if err != nil {
		return err
	}
	return matchProgressionCriteria(change.Criteria, current)
}

func matchProgressionCriteria(proposed []ProposedCriterion, current []crmcontracts.StageExitCriterion) error {
	if len(proposed) == 0 {
		return apperrors.ErrInvalidArgument
	}
	if len(proposed) != len(current) {
		return apperrors.ErrVersionSkew
	}
	byID := make(map[ids.UUID]crmcontracts.StageExitCriterion, len(current))
	for _, criterion := range current {
		byID[ids.UUID(criterion.Id)] = criterion
	}
	for _, criterion := range proposed {
		if criterion.Version <= 0 {
			return apperrors.ErrInvalidArgument
		}
		live, ok := byID[criterion.CriterionID.UUID]
		if !ok || live.Version == nil || *live.Version != criterion.Version || live.Key != criterion.Key || live.Label != criterion.Label || live.Required != criterion.Required {
			return apperrors.ErrVersionSkew
		}
		delete(byID, criterion.CriterionID.UUID)
	}
	return nil
}
