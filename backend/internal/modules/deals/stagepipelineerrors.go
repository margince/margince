// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package deals

// The two 422 refusals that say a stage is not one this deal may stand in.

import "github.com/margince/margince/backend/internal/shared/kernel/ids"

// advanceTargetField is the request field a move names its target stage in.
const advanceTargetField = "to_stage_id"

// StagePipelineMismatchError maps to 422: the target stage exists but
// belongs to another pipeline.
type StagePipelineMismatchError struct {
	StageID ids.StageID
	// Field is the request field that named the stage; empty means the advance
	// route's to_stage_id.
	Field string
}

func (e *StagePipelineMismatchError) Error() string {
	return "stage " + e.StageID.String() + " does not belong to the deal's pipeline"
}

// stagePipelineMismatchMessage is what a client shows; the stage id stays in
// Error(), which only logs read.
const stagePipelineMismatchMessage = "That stage belongs to another pipeline. Pick a stage of the pipeline the deal is in."

// FieldFault refuses a target stage that belongs to another pipeline.
func (e *StagePipelineMismatchError) FieldFault() (field, code, message string) {
	if e.Field != "" {
		return e.Field, "stage_not_in_pipeline", stagePipelineMismatchMessage
	}
	return advanceTargetField, "stage_not_in_pipeline", stagePipelineMismatchMessage
}

// codePipelineRetired is what a move into, or along the stages of, a retired
// pipeline answers.
const codePipelineRetired = "pipeline_retired"

// PipelineRetiredError maps to 422: the pipeline is retired, so a deal takes no
// stage in it until it is restored.
type PipelineRetiredError struct {
	// Field is the request field that named the stage; empty means the advance
	// route's to_stage_id.
	Field string
}

func (e *PipelineRetiredError) Error() string { return "the deal's pipeline is retired" }

// FieldFault refuses a move in a retired pipeline.
func (e *PipelineRetiredError) FieldFault() (field, code, message string) {
	field = e.Field
	if field == "" {
		field = advanceTargetField
	}
	return field, codePipelineRetired, "This pipeline is retired. Restore it in Settings to move the deal."
}
