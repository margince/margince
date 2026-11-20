// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package deals

// Moving an open deal to another pipeline.
//
// The move is a stage advance onto a stage of another pipeline. History, the
// legal review gate, audit and event all come from that path.
// The history row keeps no stage positions, because two pipelines' positions
// do not compare.

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/platform/database/storekit"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// movePipelineField is the request field that names the target pipeline.
const movePipelineField = "pipeline_id"

// MoveDealPipelineInput is one move of a deal to another pipeline. The deal
// always lands on that pipeline's first open stage.
type MoveDealPipelineInput struct {
	PipelineID ids.PipelineID
	IfVersion  *int64
}

// MoveDealToPipeline moves an open deal to another live pipeline.
func (s *Store) MoveDealToPipeline(ctx context.Context, id ids.DealID, in MoveDealPipelineInput) (AdvancedDeal, error) {
	// The landing stage is open, so the move claims no win.
	return s.AdvanceDeal(ctx, id, AdvanceDealInput{
		toPipeline: &in.PipelineID, IfVersion: in.IfVersion, WonWithoutContractReason: nil,
	})
}

// UndoDealPipelineMove puts the deal back on the pipeline and stage its latest
// move left. It is refused when anything has moved the deal since.
func (s *Store) UndoDealPipelineMove(ctx context.Context, id ids.DealID, ifVersion *int64) (AdvancedDeal, error) {
	return s.AdvanceDeal(ctx, id, AdvanceDealInput{
		undoPipelineMove: true, IfVersion: ifVersion, WonWithoutContractReason: nil,
	})
}

// MovePipelineRefusalError maps to 422: the deal cannot move to that pipeline.
type MovePipelineRefusalError struct {
	code    string
	message string
}

func (e *MovePipelineRefusalError) Error() string { return e.message }

// FieldFault names the pipeline the caller asked for.
func (e *MovePipelineRefusalError) FieldFault() (field, code, message string) {
	return movePipelineField, e.code, e.message
}

var (
	errMoveClosedDeal = &MovePipelineRefusalError{"deal_closed", "A won or lost deal stays in its pipeline. Reopen it first."}
	errMoveSamePipe   = &MovePipelineRefusalError{"same_pipeline", "The deal is already in this pipeline."}
	errMoveNoOpen     = &MovePipelineRefusalError{"pipeline_has_no_open_stage", "This pipeline has no open stage to put the deal on."}
	errMoveNotOpen    = &MovePipelineRefusalError{"stage_not_open", "A moved deal lands on an open stage."}
	errMoveNoUndo     = &MovePipelineRefusalError{"move_not_undoable", "The deal moved since, so this move cannot be undone."}
)

func (in *AdvanceDealInput) movesPipeline() bool {
	return in.toPipeline != nil || in.undoPipelineMove
}

// resolveMoveTarget answers the semantic and win probability of the stage a
// move lands on. A move between pipelines first checks the version, the deal
// and the target pipeline, and settles the landing stage into in.
func resolveMoveTarget(ctx context.Context, tx pgx.Tx, in *AdvanceDealInput, current crmcontracts.Deal) (string, int, error) {
	pipeline := ids.PipelineID{UUID: ids.UUID(*current.PipelineId)}
	field := ""
	if in.movesPipeline() {
		// Before any refusal about the deal's state. A stale caller then reads
		// a version conflict, not a refusal about a deal it has not seen.
		if err := storekit.EnsureIfMatch(ctx, tx, dealTable, ids.UUID(current.Id), in.IfVersion); err != nil {
			return "", 0, err
		}
		if err := chooseLandingStage(ctx, tx, in, current); err != nil {
			return "", 0, err
		}
		pipeline, field = *in.toPipeline, "stage_id"
	}
	stage, err := resolveLiveStage(ctx, tx, in.ToStageID, pipeline, field)
	if err != nil {
		return "", 0, err
	}
	if in.movesPipeline() && stage.semantic != string(SemanticOpen) {
		return "", 0, errMoveNotOpen
	}
	return stage.semantic, stage.winProbability, nil
}

// chooseLandingStage refuses a deal or pipeline that cannot take the move.
// It settles the landing stage: the target's first open stage, or for an undo
// the stage the latest move left.
func chooseLandingStage(ctx context.Context, tx pgx.Tx, in *AdvanceDealInput, current crmcontracts.Deal) error {
	if DealStatus(current.Status) != DealOpen {
		return errMoveClosedDeal
	}
	if in.undoPipelineMove {
		if err := deriveUndoTarget(ctx, tx, in, current); err != nil {
			return err
		}
	}
	if ids.UUID(*current.PipelineId) == in.toPipeline.UUID {
		return errMoveSamePipe
	}
	if err := requireLivePipeline(ctx, tx, *in.toPipeline); err != nil {
		return err
	}
	if in.undoPipelineMove {
		return nil
	}
	err := tx.QueryRow(ctx, `
		SELECT id FROM stage WHERE pipeline_id = $1 AND semantic = $2 AND archived_at IS NULL
		 ORDER BY "position", id LIMIT 1`, *in.toPipeline, string(SemanticOpen)).Scan(&in.ToStageID)
	if errors.Is(err, pgx.ErrNoRows) {
		return errMoveNoOpen
	}
	if err != nil {
		return fmt.Errorf("read the target pipeline's first open stage: %w", err)
	}
	return nil
}

// requireLivePipeline locks the pipeline row shared, so a retirement or a
// reorder queues behind this move instead of changing what it chose.
func requireLivePipeline(ctx context.Context, tx pgx.Tx, pipeline ids.PipelineID) error {
	var retired bool
	err := tx.QueryRow(ctx, `SELECT archived_at IS NOT NULL FROM pipeline WHERE id = $1 FOR SHARE`, pipeline).Scan(&retired)
	if err != nil {
		return auth.NameMissingReference(movePipelineField, notFoundOr(err, "read the target pipeline"))
	}
	if retired {
		return &PipelineRetiredError{Field: movePipelineField}
	}
	return nil
}

// deriveUndoTarget reads the deal's latest stage move. It is undoable only when
// it left another pipeline, is not itself an undo, and the deal still stands
// where it landed. The source pipeline and stage come from that history row.
func deriveUndoTarget(ctx context.Context, tx pgx.Tx, in *AdvanceDealInput, current crmcontracts.Deal) error {
	var row ids.UUID
	var to, from *ids.UUID
	var reversal *ids.UUID
	var fromPipeline *ids.UUID
	err := tx.QueryRow(ctx, `
		SELECT h.id, h.to_stage_id, h.from_stage_id, h.reversal_of, fs.pipeline_id
		  FROM deal_stage_history h LEFT JOIN stage fs ON fs.id = h.from_stage_id
		 WHERE h.deal_id = $1 ORDER BY h.changed_at DESC, h.id DESC LIMIT 1`, ids.UUID(current.Id)).
		Scan(&row, &to, &from, &reversal, &fromPipeline)
	if errors.Is(err, pgx.ErrNoRows) {
		return errMoveNoUndo
	}
	if err != nil {
		return fmt.Errorf("read the deal's latest stage move: %w", err)
	}
	if reversal != nil || from == nil || fromPipeline == nil || to == nil ||
		*to != ids.UUID(*current.StageId) || *fromPipeline == ids.UUID(*current.PipelineId) {
		return errMoveNoUndo
	}
	in.toPipeline = &ids.PipelineID{UUID: *fromPipeline}
	in.ToStageID = ids.StageID{UUID: *from}
	in.ReversalOf = &row
	return nil
}
