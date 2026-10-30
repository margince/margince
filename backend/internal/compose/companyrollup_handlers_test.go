// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

import (
	"errors"
	"testing"
	"time"

	openapi_types "github.com/oapi-codegen/runtime/types"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// TestCompanyRollupToWireMapsRestrictedNodes proves the one branch the HTTP
// integration suite cannot reach without inventing a second bounded
// session (the contract has no user-invitation endpoint): a non-empty
// RestrictedExcluded must map into the wire's anonymous item shape by
// value, id and name each carried over exactly. companyRollupToWire is a pure
// function, so this needs no database or HTTP harness at all.
func TestCompanyRollupToWireMapsRestrictedNodes(t *testing.T) {
	restrictedID := ids.NewV7()
	result := CompanyRollupResult{
		RootID:                 ids.NewV7(),
		Scope:                  companyRollupScopeTree,
		WeightedPipelineMinor:  12_345,
		ClosedWonMinor:         new(int64(6_789)),
		BaseCurrency:           "EUR",
		ActivityCount30d:       3,
		AggregatedAccountCount: 2,
		RestrictedExcluded:     []restrictedNode{{ID: restrictedID, DisplayName: "Hidden Child"}},
		ComputedAt:             time.Date(2026, time.July, 11, 12, 0, 0, 0, time.UTC),
	}

	wire := companyRollupToWire(result)

	if wire.RootId != openapi_types.UUID(result.RootID) || wire.Scope != crmcontracts.CompanyHierarchyRollupScopeTree {
		t.Errorf("envelope = {root %v scope %v}, want {%v tree}", wire.RootId, wire.Scope, result.RootID)
	}
	if *wire.WeightedPipeline.AmountMinor != 12_345 || *wire.WeightedPipeline.Currency != "EUR" {
		t.Errorf("weighted_pipeline = %+v, want {12345 EUR}", wire.WeightedPipeline)
	}
	if *wire.ClosedWon.AmountMinor != 6_789 || *wire.ClosedWon.Currency != "EUR" {
		t.Errorf("closed_won = %+v, want {6789 EUR}", wire.ClosedWon)
	}
	if len(wire.RestrictedExcluded) != 1 {
		t.Fatalf("restricted_excluded = %+v, want exactly one item", wire.RestrictedExcluded)
	}
	if got := wire.RestrictedExcluded[0]; got.Id != openapi_types.UUID(restrictedID) || got.DisplayName != "Hidden Child" {
		t.Errorf("restricted_excluded[0] = %+v, want {%v Hidden Child}", got, restrictedID)
	}
	if !wire.ComputedAt.Equal(result.ComputedAt) {
		t.Errorf("computed_at = %v, want %v", wire.ComputedAt, result.ComputedAt)
	}
}

// TestCompanyRollupToWireEmptyRestrictedIsNeverNil proves the make-with-length
// path also produces a real (non-nil) empty slice when nothing is
// restricted — encoding/json only omits the array shape for a nil slice,
// and the contract requires restricted_excluded to always be present.
func TestCompanyRollupToWireEmptyRestrictedIsNeverNil(t *testing.T) {
	result := CompanyRollupResult{RootID: ids.NewV7(), Scope: companyRollupScopeSelf, BaseCurrency: "EUR", ComputedAt: time.Now().UTC()}

	wire := companyRollupToWire(result)

	if wire.RestrictedExcluded == nil {
		t.Error("restricted_excluded is nil, want a real empty slice")
	}
	if len(wire.RestrictedExcluded) != 0 {
		t.Errorf("restricted_excluded = %+v, want empty", wire.RestrictedExcluded)
	}
}

// The wire states each measure's own date: the pipeline at the read, closed-won
// over its period with that period's neighbours, activity over its 30 days.
func TestCompanyRollupToWireStatesEachMeasuresDate(t *testing.T) {
	read := time.Date(2026, time.October, 30, 9, 0, 0, 0, time.UTC)
	next := "2026-Q3"
	result := CompanyRollupResult{
		RootID: ids.NewV7(), Scope: companyRollupScopeTree, BaseCurrency: "EUR", ComputedAt: read, PipelineStateAt: read,
		ActivityWindowStart: read.AddDate(0, 0, -30),
		ClosedWonPeriod: RollupWonPeriod{
			Grain: "quarter", Label: "2026-Q2", Chosen: true, Previous: "2026-Q1", Next: &next,
			Start: time.Date(2026, time.April, 1, 0, 0, 0, 0, time.UTC), End: time.Date(2026, time.July, 1, 0, 0, 0, 0, time.UTC),
		},
		CurrentPeriods: map[string]string{fieldPeriodYear: "2026", fieldPeriodQuarter: "2026-Q4", fieldPeriodMonth: "2026-10"},
	}

	wire := companyRollupToWire(result)

	if !wire.PipelineStateAt.Equal(read) || !wire.ActivityWindow.EndAt.Equal(read) || !wire.ActivityWindow.StartAt.Equal(result.ActivityWindowStart) {
		t.Errorf("pipeline at %s, activity over [%s, %s), want both read at %s", wire.PipelineStateAt, wire.ActivityWindow.StartAt, wire.ActivityWindow.EndAt, read)
	}
	won := wire.ClosedWonPeriod
	if won.Grain != crmcontracts.CompanyRollupWonPeriodGrainQuarter || won.Label != "2026-Q2" || !won.Chosen ||
		won.Anchor != crmcontracts.CompanyRollupWonPeriodAnchorClosedAt || won.Previous != "2026-Q1" || won.Next == nil || *won.Next != next ||
		!won.StartAt.Equal(result.ClosedWonPeriod.Start) || !won.EndAt.Equal(result.ClosedWonPeriod.End) {
		t.Errorf("closed_won_period = %+v, want the chosen quarter 2026-Q2 with its neighbours", won)
	}
	if wire.CurrentPeriods.PeriodYear != "2026" || wire.CurrentPeriods.PeriodQuarter != "2026-Q4" || wire.CurrentPeriods.PeriodMonth != "2026-10" {
		t.Errorf("current_periods = %+v", wire.CurrentPeriods)
	}
}

// One period parameter names the window; none keeps the default; several are
// refused naming one of them.
func TestTheRollupTakesAtMostOnePeriodParameter(t *testing.T) {
	label := "2026"
	if period, err := NewCompanyRollupPeriod(nil, nil, nil); err != nil || period != (CompanyRollupPeriod{}) {
		t.Errorf("no parameter = %+v %v, want the default", period, err)
	}
	if period, err := NewCompanyRollupPeriod(&label, nil, nil); err != nil || period.Field != fieldPeriodYear || period.Label != label {
		t.Errorf("period_year = %+v %v", period, err)
	}
	_, err := NewCompanyRollupPeriod(&label, nil, &label)
	refusal, ok := errors.AsType[*rollupPeriodRefusal](err)
	if !ok || !errors.Is(err, apperrors.ErrInvalidArgument) {
		t.Fatalf("two parameters = %v, want a refusal", err)
	}
	if field, _, _ := refusal.FieldFault(); field != fieldPeriodMonth {
		t.Errorf("the refusal names %s, want period_month", field)
	}
}
