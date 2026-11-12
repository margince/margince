// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package automation

import (
	"context"
	"encoding/json"
	"errors"
	"slices"

	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/ports/workflow"
)

// The params a stage rule reads, and the field its stage refusals name.
const (
	paramPipelineID = "pipeline_id"
	paramStageIDs   = "stage_ids"
	fieldStageIDs   = "params." + paramStageIDs
)

// StageLookup reads a stage's meaning and the pipeline it belongs to.
type StageLookup interface {
	StageSemantic(context.Context, ids.UUID) (string, ids.UUID, error)
}

type stageChangeParams struct {
	PipelineID ids.UUID   `json:"pipeline_id"`
	StageIDs   []ids.UUID `json:"stage_ids"`
	TaskTitle  string     `json:"task_title"`
}

func stageChangeSchema(task bool) map[string]any {
	properties := map[string]any{
		paramPipelineID: map[string]any{schemaKeyType: schemaTypeString, schemaKeyFormat: "pipeline"},
		paramStageIDs: map[string]any{
			schemaKeyType: schemaTypeArray, schemaKeyFormat: "stages",
			schemaKeyItems: map[string]any{schemaKeyType: schemaTypeString, schemaKeyFormat: schemaFormatUUID}, "minItems": 1,
		},
	}
	required := []string{paramPipelineID, paramStageIDs}
	if task {
		properties[paramTaskTitle] = map[string]any{schemaKeyType: schemaTypeString, schemaKeyMinLength: 1, schemaKeyMaxLength: maxTaskTitleCharacters}
		properties[paramDueInDays] = intParamProperty(2, 30, "Days until the next task is due.")
		properties[paramTaskTimezone] = taskTimezoneSchema()
		required = append(required, paramTaskTitle, paramDueInDays)
	}
	return map[string]any{
		schemaKeyType: schemaTypeObject, schemaKeyAdditionalProps: false,
		schemaKeyProperties: properties, schemaKeyRequired: required,
	}
}

func decodeStageParams(params map[string]any) (stageChangeParams, error) {
	raw, err := json.Marshal(params)
	if err != nil {
		return stageChangeParams{}, err
	}
	var scope stageChangeParams
	if err := json.Unmarshal(raw, &scope); err != nil {
		return scope, &ParamError{Field: fieldStageIDs, Reason: "choose a pipeline and valid stages"}
	}
	return scope, nil
}

func validateStageChangeParams(task bool) func(map[string]any) error {
	return func(params map[string]any) error {
		for key := range params {
			switch key {
			case paramPipelineID, paramStageIDs:
			case paramTaskTitle, paramDueInDays, paramTaskTimezone:
				if !task {
					return &ParamError{Field: "params." + key, Reason: errNotAParameter}
				}
			default:
				return &ParamError{Field: "params." + key, Reason: errNotAParameter}
			}
		}
		scope, err := decodeStageParams(params)
		if err != nil {
			return err
		}
		if scope.PipelineID.IsZero() {
			return &ParamError{Field: "params.pipeline_id", Reason: "choose a pipeline"}
		}
		if len(scope.StageIDs) == 0 || slices.Contains(scope.StageIDs, ids.Nil) {
			return &ParamError{Field: fieldStageIDs, Reason: "choose at least one stage"}
		}
		if task {
			if !validTaskTitle(scope.TaskTitle) {
				return &ParamError{Field: "params.task_title", Reason: "enter a task title of up to 200 characters"}
			}
			deadline := map[string]any{paramDueInDays: params[paramDueInDays]}
			if zone, present := params[paramTaskTimezone]; present {
				deadline[paramTaskTimezone] = zone
			}
			return validateDueInDays(deadline)
		}
		return nil
	}
}

func validateStageScope(ctx context.Context, lookup StageLookup, params map[string]any) error {
	scope, err := decodeStageParams(params)
	if err != nil {
		return err
	}
	if scope.PipelineID.IsZero() && len(scope.StageIDs) == 0 {
		return nil
	}
	if lookup == nil {
		return &ParamError{Field: fieldStageIDs, Reason: "stage selection is not available"}
	}
	for _, stage := range scope.StageIDs {
		_, pipeline, err := lookup.StageSemantic(ctx, stage)
		if err != nil && !errors.Is(err, apperrors.ErrNotFound) && !errors.Is(err, apperrors.ErrPermissionDenied) {
			return err
		}
		if err != nil || pipeline != scope.PipelineID {
			return &ParamError{Field: fieldStageIDs, Reason: "choose current stages from the selected pipeline"}
		}
	}
	return nil
}

func stageScopeMatches(ctx context.Context, ex Executors, ev workflow.Event) (bool, error) {
	var scope stageChangeParams
	if len(ev.Params) > 0 {
		if err := json.Unmarshal(ev.Params, &scope); err != nil {
			return false, err
		}
	}
	if scope.PipelineID.IsZero() && len(scope.StageIDs) == 0 {
		return true, nil
	}
	var change struct {
		ToStageID ids.UUID `json:"to_stage_id"`
	}
	if err := json.Unmarshal(ev.Payload, &change); err != nil {
		return false, err
	}
	if !slices.Contains(scope.StageIDs, change.ToStageID) {
		return false, nil
	}
	if ex.Provider == nil {
		return false, errors.New("automation: stage lookup is not configured")
	}
	reader, err := ownerContext(ctx, ex.Authority, ev.WorkspaceID, ev.OwnerID)
	if err != nil {
		return false, err
	}
	_, pipeline, err := ex.Provider.StageSemantic(reader, change.ToStageID)
	return pipeline == scope.PipelineID, err
}
