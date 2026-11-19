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
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// movePipelineField is the request field that names the target pipeline.
const movePipelineField = "pipeline_id"

// MoveDealPipelineInput is one move of a deal to another pipeline.
type MoveDealPipelineInput struct {
	PipelineID ids.PipelineID
	// StageID is an open stage of the target pipeline. Nil lands the deal on the
	// pipeline's first open stage; an Undo names the stage the deal left.
	StageID   *ids.StageID
	IfVersion *int64
}

// MoveDealToPipeline moves an open deal to another live pipeline.
func (s *Store) MoveDealToPipeline(ctx context.Context, id ids.DealID, in MoveDealPipelineInput) (AdvancedDeal, error) {
	// The landing stage is open, so the move claims no win.
	advance := AdvanceDealInput{toPipeline: &in.PipelineID, IfVersion: in.IfVersion, WonWithoutContractReason: nil}
	if in.StageID != nil {
		advance.ToStageID = *in.StageID
	}
	return s.AdvanceDeal(ctx, id, advance)
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
)

// resolveMoveTarget answers the semantic and win probability of the stage a
// move lands on. A cross-pipeline move first checks the deal and the target
// pipeline and settles which stage it lands on, writing it into in.
func resolveMoveTarget(ctx context.Context, tx pgx.Tx, in *AdvanceDealInput, current crmcontracts.Deal) (string, int, error) {
	pipeline := ids.PipelineID{UUID: ids.UUID(*current.PipelineId)}
	field := ""
	if in.toPipeline != nil {
		if err := chooseLandingStage(ctx, tx, in, current); err != nil {
			return "", 0, err
		}
		pipeline, field = *in.toPipeline, "stage_id"
	}
	stage, err := resolveLiveStage(ctx, tx, in.ToStageID, pipeline, field)
	if err != nil {
		return "", 0, err
	}
	if in.toPipeline != nil && stage.semantic != string(SemanticOpen) {
		return "", 0, errMoveNotOpen
	}
	return stage.semantic, stage.winProbability, nil
}

// chooseLandingStage refuses a deal or pipeline that cannot take the move.
// It picks the first open stage when none was named.
func chooseLandingStage(ctx context.Context, tx pgx.Tx, in *AdvanceDealInput, current crmcontracts.Deal) error {
	if DealStatus(current.Status) != DealOpen {
		return errMoveClosedDeal
	}
	if ids.UUID(*current.PipelineId) == in.toPipeline.UUID {
		return errMoveSamePipe
	}
	var retired bool
	err := tx.QueryRow(ctx, `SELECT archived_at IS NOT NULL FROM pipeline WHERE id = $1`, *in.toPipeline).Scan(&retired)
	if err != nil {
		return auth.NameMissingReference(movePipelineField, notFoundOr(err, "read the target pipeline"))
	}
	if retired {
		return &PipelineRetiredError{Field: movePipelineField}
	}
	if !in.ToStageID.IsZero() {
		return nil
	}
	err = tx.QueryRow(ctx, `
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
