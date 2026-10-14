// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

import (
	"encoding/json"
	"maps"
	"testing"
	"time"

	openapi_types "github.com/oapi-codegen/runtime/types"

	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

func TestAnUnchangedDealRowComparesEqualToTheStoredDeal(t *testing.T) {
	typed := resolvedDealCells()
	stored, err := json.Marshal(map[string]any{
		"name": "Acme renewal", "pipeline_id": typed[dealTargetPipeline], "stage_id": typed[dealTargetStage],
		"amount_minor": 1200050, "currency": "EUR", "expected_close_date": "2026-03-02",
	})
	if err != nil {
		t.Fatal(err)
	}
	fields := map[string]string{
		dealTargetName: "Acme renewal", dealTargetPipeline: "Sales", dealTargetStage: "Qualified",
		dealTargetAmount: "12000.50", dealTargetCurrency: "eur", dealTargetCloseDate: "2026-03-02",
	}
	changed, err := changedFields(stored, fields, typed)
	if err != nil {
		t.Fatal(err)
	}
	if len(changed) != 0 {
		t.Fatalf("changed = %v, want none: every cell names what the deal already holds", changed)
	}
	typed[dealTargetAmount] = int64(1300000)
	if changed, _ = changedFields(stored, fields, typed); len(changed) != 1 || changed[dealTargetAmount] == "" {
		t.Fatalf("changed = %v, want only the amount", changed)
	}
}

func TestADealsNewStageIsAnAdvanceNotAFieldWrite(t *testing.T) {
	typed := resolvedDealCells()
	moved := dealUpdateFrom(map[string]string{dealTargetStage: "Won"}, typed)
	if moved.Move == nil || moved.Move.ToStageID != typed[dealTargetStage] {
		t.Fatalf("move = %+v, want an advance to the row's stage", moved.Move)
	}
	if reason := moved.Move.WonWithoutContractReason; reason == nil || *reason != "imported" {
		t.Fatalf("won-without-contract reason = %v, want imported: the win's paper is in the old CRM", reason)
	}
	if still := dealUpdateFrom(map[string]string{dealTargetName: "Renamed"}, typed); still.Move != nil {
		t.Fatalf("move = %+v, want none when neither pipeline nor stage changed", still.Move)
	}
}

// dealRoundTrip is the deal arm of TestEveryImportTargetRoundTripsThroughCreateAndUpdate:
// which targets reach the create input and which the update.
func dealRoundTrip(fields map[string]string, typed map[string]any) (created, patched map[string]bool) {
	maps.Copy(typed, resolvedDealCells())
	in := dealCreateFrom(fields, typed, importSourceSystem(""), "src")
	up := dealUpdateFrom(fields, typed)
	moved := up.Move != nil && !up.Move.ToStageID.IsZero()
	created = map[string]bool{
		"name":        in.Deal.Name != "",
		"pipeline":    !in.Deal.PipelineID.IsZero(),
		"stage":       !in.Deal.StageID.IsZero(),
		"amount":      in.Deal.AmountMinor != nil,
		"currency":    in.Deal.Currency != nil,
		"close_date":  in.Deal.ExpectedClose != nil,
		"description": in.Deal.Description != nil,
		"lost_reason": in.LostReason != nil,
		"owner_email": in.Deal.OwnerID != nil,
		"cf_tier":     in.Deal.CustomFields["cf_tier"] != nil,
	}
	patched = map[string]bool{
		"name":        up.Deal.Name != nil,
		"pipeline":    moved,
		"stage":       moved,
		"amount":      up.Deal.AmountMinor != nil,
		"currency":    up.Deal.Currency != nil,
		"close_date":  up.Deal.ExpectedClose != nil,
		"description": up.Deal.Description != nil,
		"owner_email": up.Deal.OwnerID != nil,
		"cf_tier":     up.Deal.CustomFields["cf_tier"] != nil,
	}
	return created, patched
}

// resolvedDealCells is what the cell check answers for a deal row whose
// pipeline, stage, amount, currency and close date all read.
func resolvedDealCells() map[string]any {
	return map[string]any{
		dealTargetPipeline:  ids.From[ids.PipelineKind](ids.NewV7()),
		dealTargetStage:     ids.From[ids.StageKind](ids.NewV7()),
		dealTargetAmount:    int64(1200050),
		dealTargetCurrency:  "EUR",
		dealTargetCloseDate: openapi_types.Date{Time: time.Date(2026, 3, 2, 0, 0, 0, 0, time.UTC)},
	}
}
