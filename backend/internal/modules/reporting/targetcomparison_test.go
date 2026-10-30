// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package reporting

import (
	"context"
	"fmt"
	"testing"
	"time"

	openapi_types "github.com/oapi-codegen/runtime/types"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

var comparisonQuarter = crmcontracts.ReportingWindow{
	StartAt: time.Date(2026, 10, 1, 0, 0, 0, 0, time.UTC),
	EndAt:   time.Date(2027, 1, 1, 0, 0, 0, 0, time.UTC),
}

// sampleQuarter is the review's own example, in cents: a target of 800k,
// 420k won, a landing of 760k and 1.36m open in the quarter.
func sampleQuarter() (crmcontracts.ReportingEvaluation, *ForecastOperands, heldTargets) {
	quarter := comparisonQuarter
	evaluation := crmcontracts.ReportingEvaluation{
		Context:   crmcontracts.ReportingContext{Scope: crmcontracts.ReportingScope{Kind: "workspace"}, Currency: "EUR", Interval: crmcontracts.ReportingWindow{StartAt: quarter.StartAt, EndAt: quarter.StartAt.AddDate(0, 1, 15)}, TargetInterval: &quarter},
		Selection: crmcontracts.ReportingSelection{Scope: crmcontracts.ReportingScope{Kind: "workspace"}, Period: "this_quarter", TargetBasis: "fiscal_quarter", CloseWindow: "all_open", Metrics: []crmcontracts.ReportingMetricID{forecastLanding}},
	}
	ops := &ForecastOperands{Quarter: comparisonQuarter, Measure: "commit_evidence", LandingMinor: 76_000_000, WonMinor: 42_000_000, OpenMinor: 136_000_000}
	held := heldTargets{keyFor(evaluation.Context.Scope, comparedMetric): {Id: openapi_types.UUID(ids.NewV7()), Revision: 3, Unit: "EUR", Definition: crmcontracts.ReportingTargetInput{Metric: comparedMetric, Value: 80_000_000}}}
	return evaluation, ops, held
}

func TestTheSampleQuarterIsFortyThousandShortAtThreePointSixCoverage(t *testing.T) {
	evaluation, ops, held := sampleQuarter()
	got := compareForecast(context.Background(), evaluation, ops, held)
	if got.Status != "compared" || *got.Gap != 4_000_000 || *got.Remaining != 38_000_000 || *got.OpenPipeline != 136_000_000 {
		t.Fatalf("comparison %+v, want 40k short with 380k left to win", got)
	}
	if rounded := fmt.Sprintf("%.1f", *got.Coverage); rounded != "3.6" {
		t.Fatalf("coverage %v rounds to %s, want 3.6: 1.36m open over 380k left to win", *got.Coverage, rounded)
	}
	if *got.TargetRevision != 3 || got.Operands == nil || got.Qualified.Status != "not_enforced" {
		t.Fatalf("comparison %+v, want target revision 3, its operands, and no qualified figure while it is off", got)
	}
}

func TestAManagerCallIsTheWholeQuarterAndIsNotAddedToWhatIsWon(t *testing.T) {
	evaluation, ops, held := sampleQuarter()
	ops.Measure, ops.LandingMinor = "manager_call", 76_000_000
	got := compareForecast(context.Background(), evaluation, ops, held)
	if *got.Landing != 76_000_000 || *got.Gap != 4_000_000 {
		t.Fatalf("landing %v gap %v, want the 760k call as it is and 40k short", *got.Landing, *got.Gap)
	}
}

func TestATargetAlreadyWonReadsReachedRatherThanAnEndlessCoverage(t *testing.T) {
	evaluation, ops, held := sampleQuarter()
	ops.WonMinor, ops.LandingMinor, ops.QualifiedOpenMinor = 90_000_000, 95_000_000, new(int64(1_000_000))
	got := compareForecast(context.Background(), evaluation, ops, held)
	if got.Status != "target_reached" || got.Coverage != nil || *got.Remaining != 0 || *got.Gap != -15_000_000 {
		t.Fatalf("comparison %+v, want the target reached, nothing left and no coverage", got)
	}
	if got.Qualified.Status != "target_reached" || got.Qualified.Coverage != nil {
		t.Fatalf("qualified %+v, want reached with no coverage", got.Qualified)
	}
}

func TestQualifiedCoverageDividesTheQualifiedOpenDealsByTheSameRemainder(t *testing.T) {
	evaluation, ops, held := sampleQuarter()
	ops.QualifiedOpenMinor = new(int64(57_000_000))
	got := compareForecast(context.Background(), evaluation, ops, held)
	if got.Qualified.Status != "compared" || *got.Qualified.OpenPipeline != 57_000_000 || *got.Qualified.Coverage != 1.5 {
		t.Fatalf("qualified %+v, want 570k over the 380k left to win, 1.5", got.Qualified)
	}
	ops.QualifiedOpenMinor = new(int64(0))
	got = compareForecast(context.Background(), evaluation, ops, held)
	if got.Qualified.Status != "compared" || *got.Qualified.Coverage != 0 {
		t.Fatalf("qualified %+v, want a measured zero when no open deal qualifies", got.Qualified)
	}
}

// Every state that is not a comparison says which it is. It carries no gap,
// so no screen can draw a figure the answer did not give.
func TestEachStateThatCannotCompareSaysWhyAndGivesNoFigure(t *testing.T) {
	month := crmcontracts.ReportingWindow{StartAt: comparisonQuarter.StartAt, EndAt: comparisonQuarter.StartAt.AddDate(0, 1, 0)}
	pipeline := openapi_types.UUID(ids.NewV7())
	for _, tc := range []struct {
		name   string
		change func(*crmcontracts.ReportingEvaluation, *ForecastOperands, heldTargets)
		status crmcontracts.ReportingTargetComparisonStatus
		why    crmcontracts.ReportingTargetComparisonIncompatibility
	}{
		{"no target", func(e *crmcontracts.ReportingEvaluation, _ *ForecastOperands, h heldTargets) {
			delete(h, keyFor(e.Context.Scope, comparedMetric))
		}, "no_target", ""},
		{"missing exchange rate", func(_ *crmcontracts.ReportingEvaluation, o *ForecastOperands, _ heldTargets) {
			o.FxMissing = true
		}, "fx_missing", ""},
		{"deals the reader cannot see", func(_ *crmcontracts.ReportingEvaluation, o *ForecastOperands, _ heldTargets) {
			o.Narrowed = true
		}, "partial_visibility", ""},
		{"one colleague's own figure", func(_ *crmcontracts.ReportingEvaluation, o *ForecastOperands, _ heldTargets) {
			o.ColleagueWithheld = true
		}, "colleague_withheld", ""},
		{"a manager call below what is won", func(_ *crmcontracts.ReportingEvaluation, o *ForecastOperands, _ heldTargets) {
			o.WonMinor, o.LandingMinor, o.Measure, o.CallBelowWon = 90_000_000, 70_000_000, "manager_call", true
		}, "call_below_won", ""},
		{"this month against the quarter's target", func(e *crmcontracts.ReportingEvaluation, _ *ForecastOperands, _ heldTargets) {
			e.Selection.Period = "this_month"
		}, "incompatible", "period"},
		{"a custom span against the quarter's target", func(e *crmcontracts.ReportingEvaluation, _ *ForecastOperands, _ heldTargets) {
			e.Selection.Period, e.Context.Interval.StartAt = "custom", e.Context.Interval.StartAt.AddDate(0, 0, 7)
		}, "incompatible", "period"},
		{"a whole-team target in another currency over one pipeline", func(e *crmcontracts.ReportingEvaluation, _ *ForecastOperands, h heldTargets) {
			e.Selection.PipelineId = &pipeline
			key := keyFor(e.Context.Scope, comparedMetric)
			target := h[key]
			target.Unit = "USD"
			h[wholePipelineKey(e.Context.Scope)] = target
			delete(h, key)
		}, "incompatible", "currency"},
		{"a month target", func(e *crmcontracts.ReportingEvaluation, _ *ForecastOperands, _ heldTargets) {
			e.Context.TargetInterval, e.Selection.Period, e.Selection.TargetBasis = &month, "this_month", "month"
		}, "incompatible", "period"},
		{"a manager call for one pipeline", func(e *crmcontracts.ReportingEvaluation, o *ForecastOperands, _ heldTargets) {
			e.Selection.PipelineId, o.CallOverPipeline = &pipeline, true
		}, "incompatible", "manager_call_pipeline"},
		{"a whole-team target over one pipeline", func(e *crmcontracts.ReportingEvaluation, _ *ForecastOperands, h heldTargets) {
			e.Selection.PipelineId = &pipeline
			key := keyFor(e.Context.Scope, comparedMetric)
			h[wholePipelineKey(e.Context.Scope)] = h[key]
			delete(h, key)
		}, "incompatible", "pipeline"},
		{"a target in another currency", func(e *crmcontracts.ReportingEvaluation, _ *ForecastOperands, h heldTargets) {
			key := keyFor(e.Context.Scope, comparedMetric)
			target := h[key]
			target.Unit = "USD"
			h[key] = target
		}, "incompatible", "currency"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			evaluation, ops, held := sampleQuarter()
			tc.change(&evaluation, ops, held)
			got := compareForecast(context.Background(), evaluation, ops, held)
			if got.Status != tc.status || got.Gap != nil || got.Coverage != nil || got.Target != nil {
				t.Fatalf("comparison %+v, want %s with no figure", got, tc.status)
			}
			if (tc.why == "") != (got.Incompatibility == nil) || (tc.why != "" && *got.Incompatibility != tc.why) {
				t.Fatalf("incompatibility %v, want %q", got.Incompatibility, tc.why)
			}
		})
	}
}

func TestAnIncompatibleSelectionComesWithOneTheComparisonSupports(t *testing.T) {
	evaluation, ops, held := sampleQuarter()
	evaluation.Selection.Period, evaluation.Context.Interval.StartAt = "custom", evaluation.Context.Interval.StartAt.AddDate(0, 0, 7)
	got := compareForecast(context.Background(), evaluation, ops, held).SupportedSelection
	if got == nil || got.Period != "this_quarter" || got.Interval != nil {
		t.Fatalf("supported selection %+v, want this quarter in place of the custom span", got)
	}

	evaluation, ops, held = sampleQuarter()
	month := crmcontracts.ReportingWindow{StartAt: comparisonQuarter.StartAt, EndAt: comparisonQuarter.StartAt.AddDate(0, 1, 0)}
	evaluation.Context.TargetInterval, evaluation.Selection.Period, evaluation.Selection.TargetBasis = &month, "this_month", "month"
	got = compareForecast(context.Background(), evaluation, ops, held).SupportedSelection
	if got == nil || got.Period != "this_quarter" || got.TargetBasis != "fiscal_quarter" || len(got.Metrics) != 1 {
		t.Fatalf("supported selection %+v, want the same selection over this fiscal quarter", got)
	}

	evaluation, ops, held = sampleQuarter()
	pipeline := openapi_types.UUID(ids.NewV7())
	evaluation.Selection.PipelineId, ops.CallOverPipeline = &pipeline, true
	got = compareForecast(context.Background(), evaluation, ops, held).SupportedSelection
	if got == nil || got.PipelineId != nil {
		t.Fatalf("supported selection %+v, want the same selection over every pipeline", got)
	}
}

func TestASentEditionsQuarterToDateIsStillThisQuarter(t *testing.T) {
	evaluation, ops, held := sampleQuarter()
	evaluation.Selection.Period = "custom"
	if got := compareForecast(context.Background(), evaluation, ops, held); got.Status != "compared" {
		t.Fatalf("comparison %+v, want the pinned quarter-to-date compared", got)
	}
}

func TestSeveralTeamsAtOnceAreOfferedOneOfTheReadersTeams(t *testing.T) {
	evaluation, _, held := sampleQuarter()
	evaluation.Context.Scope = crmcontracts.ReportingScope{Kind: crmcontracts.ReportingScopeKindManagedTeams}
	first, second := ids.MustParse("00000000-0000-7000-8000-000000000001"), ids.MustParse("00000000-0000-7000-8000-000000000002")
	ctx := principal.WithActor(context.Background(), principal.Principal{TeamIDs: []ids.UUID{second, first}})
	got := compareForecast(ctx, evaluation, nil, held)
	if got.Status != "incompatible" || *got.Incompatibility != "scope" || got.SupportedSelection == nil ||
		got.SupportedSelection.Scope.Kind != "team" || *got.SupportedSelection.Scope.Id != openapi_types.UUID(first) {
		t.Fatalf("comparison %+v, want a scope refusal that offers the first team", got)
	}
	if compareForecast(ctx, crmcontracts.ReportingEvaluation{}, nil, held) != nil {
		t.Fatal("a forecast that read nothing for one population still drew a comparison")
	}
}
