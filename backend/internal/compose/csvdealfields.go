// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

// A deal row as the deals store's inputs. The cell check has already read the
// pipeline, stage, amount, currency and close date into typed; this only
// places them.

import (
	"time"

	openapi_types "github.com/oapi-codegen/runtime/types"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/modules/deals"
	"github.com/margince/margince/backend/internal/modules/migration"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// The columns of deals.csv beside ref, owner_email, tags, description,
// company_ref and the deal's custom fields.
const (
	dealTargetName       = "name"
	dealTargetPipeline   = "pipeline"
	dealTargetStage      = "stage"
	dealTargetAmount     = "amount"
	dealTargetCurrency   = "currency"
	dealTargetCloseDate  = "close_date"
	dealTargetLostReason = "lost_reason"
	dealTargetContactRef = "contact_ref"
)

// dealWriterPendingReason skips a deal row at commit: the test run checks
// deals, and this build writes none yet.
var dealWriterPendingReason = migration.Why(migration.ReasonDealNotYetWritten)

// dealStoredColumns names the stored deal's JSON field for each target the
// file spells differently, so an unedited cell compares equal to it.
//
//nolint:goconst // the deal wire's own keys; another surface's constant of one spelling is another vocabulary
var dealStoredColumns = map[string]string{
	dealTargetPipeline:  "pipeline_id",
	dealTargetStage:     "stage_id",
	dealTargetAmount:    "amount_minor",
	dealTargetCloseDate: "expected_close_date",
}

// importedWinReason answers why an imported win carries no contract: its
// agreement lives in the system the file came from. The store writes it only on a win.
var importedWinReason = string(crmcontracts.DealWonWithoutContractReasonImported)

// dealCreate is a deal row's create input and its lost reason. The create
// input has no member for the reason, which the import writes on a lost stage.
type dealCreate struct {
	Deal       deals.CreateDealInput
	LostReason *string
}

// dealPatch is a deal row's update. A new stage is an advance, never a field
// write, so it travels apart from the fields.
type dealPatch struct {
	Deal deals.UpdateDealInput
	Move *deals.AdvanceDealInput
}

// dealCreateFrom builds the create input for one checked deal row. Only mapped
// cells are set.
func dealCreateFrom(fields map[string]string, typed map[string]any, sourceSystem, source string) dealCreate {
	in := deals.CreateDealInput{
		SourceSystem:  &sourceSystem,
		Source:        source,
		AmountMinor:   dealAmount(fields, typed),
		Currency:      checkedString(fields, typed, dealTargetCurrency),
		ExpectedClose: dealCloseDate(fields, typed),
		Description:   importString(fields, descriptionField),
		OwnerID:       ownerFrom(fields, typed),
		CustomFields:  customFieldsFrom(fields, typed),
	}
	if name := importString(fields, dealTargetName); name != nil {
		in.Name = *name
	}
	if pipeline, ok := typed[dealTargetPipeline].(ids.PipelineID); ok {
		in.PipelineID = pipeline
	}
	if stage, ok := typed[dealTargetStage].(ids.StageID); ok {
		in.StageID = stage
	}
	return dealCreate{Deal: in, LostReason: importString(fields, dealTargetLostReason)}
}

// dealUpdateFrom builds the update for the cells that differ from the stored
// deal. A changed pipeline or stage moves the deal to the stage the row names.
func dealUpdateFrom(changed map[string]string, typed map[string]any) dealPatch {
	patch := dealPatch{Deal: deals.UpdateDealInput{
		Name:          importString(changed, dealTargetName),
		AmountMinor:   dealAmount(changed, typed),
		Currency:      checkedString(changed, typed, dealTargetCurrency),
		ExpectedClose: dealCloseDate(changed, typed),
		Description:   importString(changed, descriptionField),
		OwnerID:       ownerFrom(changed, typed),
		CustomFields:  customFieldsFrom(changed, typed),
	}}
	_, newStage := changed[dealTargetStage]
	_, newPipeline := changed[dealTargetPipeline]
	if stage, ok := typed[dealTargetStage].(ids.StageID); ok && (newStage || newPipeline) {
		patch.Move = &deals.AdvanceDealInput{ToStageID: stage, WonWithoutContractReason: &importedWinReason}
	}
	return patch
}

// dealAmount is the checked amount in minor units, nil when the row maps none.
func dealAmount(fields map[string]string, typed map[string]any) *int64 {
	if _, given := fields[dealTargetAmount]; !given {
		return nil
	}
	if minor, ok := typed[dealTargetAmount].(int64); ok {
		return &minor
	}
	return nil
}

// dealCloseDate is the checked close date, nil when the row maps none.
func dealCloseDate(fields map[string]string, typed map[string]any) *time.Time {
	if _, given := fields[dealTargetCloseDate]; !given {
		return nil
	}
	if date, ok := typed[dealTargetCloseDate].(openapi_types.Date); ok {
		return &date.Time
	}
	return nil
}
