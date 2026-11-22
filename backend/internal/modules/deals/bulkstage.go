// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package deals

// The single-record stage move a bulk change drives, on the caller's
// transaction. It is AdvanceDealTx plus two answers the undo needs: the stage
// the deal stood on, and the history row the move wrote. With them the undo
// moves the deal back and names the row it reverses.

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// BulkStageRefusal maps to 422: a bulk stage move leaves this deal alone.
type BulkStageRefusal struct {
	code    string
	message string
}

func (e *BulkStageRefusal) Error() string { return e.message }

// FieldFault names the stage the change asked for.
func (e *BulkStageRefusal) FieldFault() (field, code, message string) {
	return "stage_id", e.code, e.message
}

var (
	errBulkStageClosedDeal = &BulkStageRefusal{"deal_closed",
		"A won or lost deal is reopened on its own, where its close day and exchange rate are asked for."}
	errBulkStageNotOpen = &BulkStageRefusal{"stage_not_open",
		"A bulk stage move lands on an open stage. Closing a deal asks for its own reason, one deal at a time."}
)

// BulkStageMove is what one bulk stage move did to a deal. Moved is false when
// the deal already stood on the stage.
type BulkStageMove struct {
	Moved     bool
	From      ids.UUID
	HistoryID ids.UUID
}

// CheckBulkStage refuses a stage a bulk move cannot land on: one nobody has,
// and one that closes a deal. Which pipeline it belongs to is each deal's own
// question, answered when that deal moves.
func (s *Store) CheckBulkStage(ctx context.Context, stage ids.StageID) error {
	if err := auth.Require(ctx, "pipeline", principal.ActionRead); err != nil {
		return err
	}
	return s.Tx(ctx, func(tx pgx.Tx) error {
		var semantic string
		err := tx.QueryRow(ctx, `SELECT semantic FROM stage WHERE id = $1 AND archived_at IS NULL`, stage).Scan(&semantic)
		if errors.Is(err, pgx.ErrNoRows) {
			return auth.NameMissingReference("stage_id", &StageGoneError{StageID: stage})
		}
		if err != nil {
			return fmt.Errorf("read the stage a bulk move names: %w", err)
		}
		if StageSemantic(semantic) != SemanticOpen {
			return errBulkStageNotOpen
		}
		return nil
	})
}

// MoveStageForBulkTx moves one deal to the open stage to, conditioned on
// version, through the advance every other stage move goes through. A move
// that undoes an earlier one names its history row in reversalOf.
func (s *Store) MoveStageForBulkTx(
	ctx context.Context, tx pgx.Tx, id ids.DealID, to ids.StageID, version int64, reversalOf *ids.UUID,
) (BulkStageMove, error) {
	if err := auth.Require(ctx, "deal", principal.ActionUpdate); err != nil {
		return BulkStageMove{}, err
	}
	var from ids.UUID
	var status string
	err := tx.QueryRow(ctx, `SELECT stage_id, status FROM deal WHERE id = $1 AND archived_at IS NULL`, id).Scan(&from, &status)
	if errors.Is(err, pgx.ErrNoRows) {
		return BulkStageMove{}, apperrors.ErrNotFound
	}
	if err != nil {
		return BulkStageMove{}, fmt.Errorf("read the stage of deal %s: %w", id, err)
	}
	switch {
	case DealStatus(status) != DealOpen:
		return BulkStageMove{}, errBulkStageClosedDeal
	case from == to.UUID:
		return BulkStageMove{From: from}, nil
	}
	moved, err := s.advanceOnTx(ctx, tx, id, AdvanceDealInput{
		ToStageID: to, IfVersion: &version, ReversalOf: reversalOf,
	}, nil)
	if err != nil {
		return BulkStageMove{}, err
	}
	return BulkStageMove{Moved: true, From: from, HistoryID: moved.HistoryID}, nil
}
