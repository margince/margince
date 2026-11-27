// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package deals

import (
	"errors"
	"slices"
	"strings"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/platform/database/storekit"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/ports/fieldcatalog"
)

// CustomFieldNotForPipeline is the code for a custom value sent for a field
// that does not apply to the deal's pipeline.
const CustomFieldNotForPipeline = "field_not_valid_for_pipeline"

// checkPipelineScope is the one check of a created or patched deal's cf_ values
// against the pipeline the deal sits in. A create, an update and an import all
// answer through it. A null always passes, so a value stored before the field
// was narrowed, or before the deal moved, can still be cleared.
//
//craft:ignore naked-any values is a request body's extension map
func checkPipelineScope(pipeline ids.PipelineID, columns []fieldcatalog.Column, values map[string]any) error {
	var refused storekit.CustomFieldErrors
	for _, column := range columns {
		if values[column.Name] != nil && !column.AppliesToPipeline(pipeline.UUID) {
			refused = append(refused, storekit.CustomFieldError{
				Field: column.Name, Code: CustomFieldNotForPipeline,
				Message: "does not apply to deals in this pipeline; send null to clear it",
			})
		}
	}
	if len(refused) == 0 {
		return nil
	}
	slices.SortFunc(refused, func(a, b storekit.CustomFieldError) int { return strings.Compare(a.Field, b.Field) })
	return refused
}

// checkStoredDealPipelineScope is checkPipelineScope for a patch, against the
// pipeline the deal sits in now. A patch never moves the deal, so the stored
// pipeline is the one the values land on.
//
//craft:ignore naked-any values is a request body's extension map
func checkStoredDealPipelineScope(current crmcontracts.Deal, columns []fieldcatalog.Column, values map[string]any) error {
	if current.PipelineId == nil {
		return nil
	}
	return checkPipelineScope(ids.PipelineID{UUID: ids.UUID(*current.PipelineId)}, columns, values)
}

// FieldOutsidePipeline names the first custom field of a refused write that does
// not apply to the deal's pipeline. A row-by-row import says which cell to empty.
func FieldOutsidePipeline(err error) (string, bool) {
	refused, ok := errors.AsType[storekit.CustomFieldErrors](err)
	if !ok {
		return "", false
	}
	for _, refusal := range refused {
		if refusal.Code == CustomFieldNotForPipeline {
			return refusal.Field, true
		}
	}
	return "", false
}

// RefuseOutsidePipeline asks, without writing, whether a patch's custom values
// fit the pipeline the stored deal sits in. A reconciling import runs it before
// writing, so its preview and its commit skip the same rows.
//
//craft:ignore naked-any values is a request body's extension map
func (c CustomColumns) RefuseOutsidePipeline(pipeline ids.UUID, values map[string]any) error {
	return checkPipelineScope(ids.PipelineID{UUID: pipeline}, c.cols, values)
}
