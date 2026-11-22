// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

// The move_stage verb. Each deal goes through the single-record stage advance
// (deals.MoveStageForBulkTx). So a bulk move writes the same history row, audit
// row and deal.stage_changed event as a move from the deal's page.
//
// An undo is a move of its own, back to the stage the deal stood on. The first
// move's history row and event stay. The undo writes a second pair, and its
// history row is marked as the reversal of the first. A deal that changed after
// the batch is skipped, so the undo never overwrites later work.

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5"
	openapi_types "github.com/oapi-codegen/runtime/types"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// bulkStageBefore is where one deal stood before a stage move, and the history
// row the move wrote.
type bulkStageBefore struct{ stage, history ids.UUID }

// admitStage refuses a stage the move cannot land on, before any row is tried.
// An undo names no stage: each deal goes back to the one it left.
func admitStage(ctx context.Context, target bulkTarget, change bulkChange) error {
	mover, ok := target.(bulkStageMover)
	if !ok || change.verb != crmcontracts.BulkVerbMoveStage || change.undo != nil {
		return nil
	}
	return mover.checkStage(ctx, *change.stageID)
}

// moverOf answers the record type's stage move. admit refuses move_stage for a
// type without one, so a miss here is a bug rather than a row's fault.
//
//nolint:ireturn // the stage-move share of a target is exactly the thing asked for
func moverOf(target bulkTarget) (bulkStageMover, error) {
	mover, ok := target.(bulkStageMover)
	if !ok {
		return nil, fmt.Errorf("compose: a %T has no stage to move", target)
	}
	return mover, nil
}

// applyStageMove moves one deal to the change's stage.
func applyStageMove(
	ctx context.Context, tx pgx.Tx, target bulkTarget, change bulkChange, item crmcontracts.BulkItem,
) (bulkApplied, crmcontracts.BulkSkip, error) {
	row, skip, err := lockUnmoved(ctx, tx, target, change, item)
	if err != nil || skip.Reason != "" {
		return bulkApplied{}, skip, err
	}
	return moveOneStage(ctx, tx, target, change.recordType, row, item, *change.stageID, nil)
}

// undoStageMove puts one deal back on the stage the change took it from.
func undoStageMove(
	ctx context.Context, tx pgx.Tx, target bulkTarget, change bulkChange, item crmcontracts.BulkItem,
) (bulkApplied, crmcontracts.BulkSkip, error) {
	before, known := change.undo.stageMoves[item.Id]
	if !known {
		return bulkApplied{}, skipped(crmcontracts.BulkSkipReasonNoChange), nil
	}
	row, err := target.lock(ctx, tx, ids.UUID(item.Id))
	if err != nil {
		skip, classifyErr := bulkSkipFor(err)
		return bulkApplied{}, skip, classifyErr
	}
	return moveOneStage(ctx, tx, target, change.recordType, row, item, before.stage, &before.history)
}

// moveOneStage runs the move and answers the row as the change shows it. A
// version that moved is changed_since_preview going forward and changed_since_batch
// on an undo, which undoSkipFor says.
func moveOneStage(
	ctx context.Context, tx pgx.Tx, target bulkTarget, recordType crmcontracts.BulkRecordType,
	row bulkRow, item crmcontracts.BulkItem, to ids.UUID, reversalOf *ids.UUID,
) (bulkApplied, crmcontracts.BulkSkip, error) {
	mover, err := moverOf(target)
	if err != nil {
		return bulkApplied{}, crmcontracts.BulkSkip{}, err
	}
	move, err := mover.moveStage(ctx, tx, ids.UUID(item.Id), to, item.Version, reversalOf)
	if err != nil {
		skipFor := bulkSkipFor
		if reversalOf != nil {
			skipFor = undoSkipFor
		}
		skip, classifyErr := skipFor(err)
		return bulkApplied{}, skip, classifyErr
	}
	if !move.Moved {
		return bulkApplied{}, skipped(crmcontracts.BulkSkipReasonNoChange), nil
	}
	from, toWire := openapi_types.UUID(move.From), openapi_types.UUID(to)
	sample := crmcontracts.BulkSampleRow{
		Id: item.Id, Label: row.label,
		Before: crmcontracts.BulkRecordState{OwnerId: wireOwner(row.ownerID), StageId: &from},
		After:  crmcontracts.BulkRecordState{OwnerId: wireOwner(row.ownerID), StageId: &toWire},
	}
	applied, skip, err := appliedAt(ctx, tx, recordType, sample)
	history := openapi_types.UUID(move.HistoryID)
	applied.outcome.StageBefore, applied.outcome.StageHistoryID = &from, &history
	return applied, skip, err
}
