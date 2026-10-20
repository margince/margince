// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package deals

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// resolveAdvanceTarget reads the target stage's semantic and win probability
// and enforces that it belongs to the deal's own pipeline: a stage from
// another pipeline is a 422, a missing or archived stage a 404.
func resolveAdvanceTarget(ctx context.Context, tx pgx.Tx, toStage ids.StageID, current crmcontracts.Deal) (semantic string, winProbability int, err error) {
	stage, err := resolveLiveStage(ctx, tx, toStage, ids.PipelineID{UUID: ids.UUID(*current.PipelineId)}, "")
	if err != nil {
		return "", 0, err
	}
	return stage.semantic, stage.winProbability, nil
}

// StageGoneError is a stage nobody has or that is archived. It unwraps to
// apperrors.ErrNotFound, and names the cause for a caller holding a stage it
// read earlier, such as an import run. A request that typed the stage answers
// 422 naming the field instead (auth.NameMissingReference).
type StageGoneError struct{ StageID ids.StageID }

func (e *StageGoneError) Error() string {
	return "stage " + e.StageID.String() + " is not a live stage"
}

// Unwrap lets errors.Is match apperrors.ErrNotFound.
func (e *StageGoneError) Unwrap() error { return apperrors.ErrNotFound }

// liveStage is the part of a stage row a move or a birth decides on.
type liveStage struct {
	semantic       string
	winProbability int
}

// resolveLiveStage is the one answer to "may a deal in this pipeline stand in
// this stage": StageGoneError for a stage nobody has or that is archived, and
// 422 naming field for a live stage of another pipeline. Both the advance and
// the create door ask it. A door whose caller typed the stage wraps the first
// in auth.ReferenceNotFoundError, so one mistake earns one refusal.
func resolveLiveStage(ctx context.Context, tx pgx.Tx, stageID ids.StageID, pipelineID ids.PipelineID, field string) (liveStage, error) {
	var stage liveStage
	var stagePipeline ids.PipelineID
	err := tx.QueryRow(ctx,
		`SELECT semantic, pipeline_id, win_probability FROM stage WHERE id = $1 AND archived_at IS NULL`+
			lockLiveStageTarget,
		stageID).Scan(&stage.semantic, &stagePipeline, &stage.winProbability)
	if errors.Is(err, pgx.ErrNoRows) {
		return liveStage{}, &StageGoneError{StageID: stageID}
	}
	if err != nil {
		return liveStage{}, fmt.Errorf("resolve target stage: %w", err)
	}
	if stagePipeline.UUID != pipelineID.UUID {
		return liveStage{}, &StagePipelineMismatchError{StageID: stageID, Field: field}
	}
	return stage, nil
}
