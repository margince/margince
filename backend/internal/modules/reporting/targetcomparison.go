// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package reporting

import (
	"context"
	"slices"
	"strings"

	openapi_types "github.com/oapi-codegen/runtime/types"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// comparedMetric is the target a forecast landing stands against: sales won
// in the quarter the landing projects.
const comparedMetric crmcontracts.ReportingMetricID = "bookings_won"

const forecastLanding crmcontracts.ReportingMetricID = "forecast_landing"

// ForecastOperands are the figures of one forecast reading that a target
// comparison rests on, in base-currency minor units. Won, landing and open
// come from the same population, so no deal is in two of them.
type ForecastOperands struct {
	// Quarter is the fiscal quarter the landing projects, half-open.
	Quarter      crmcontracts.ReportingWindow
	Measure      crmcontracts.ReportingTargetComparisonBasis
	LandingMinor int64
	WonMinor     int64
	OpenMinor    int64
	// QualifiedOpenMinor is nil while the qualified forecast is switched off.
	QualifiedOpenMinor *int64
	// FxMissing marks totals that a deal with no exchange rate left short.
	FxMissing bool
	// CallOverPipeline marks a manager-call installation read for one
	// pipeline: the call covers every pipeline, so it has no landing here.
	CallOverPipeline bool
	Operands         crmcontracts.ReportingComparisonOperands
}

// compareForecast places the forecast landing against the target held for
// it. held carries the scope's target for this pipeline and, when a pipeline
// is selected, the scope's whole-pipeline target under whole.
func compareForecast(ctx context.Context, evaluation crmcontracts.ReportingEvaluation, metric crmcontracts.ReportingMetric, ops *ForecastOperands, held heldTargets) *crmcontracts.ReportingTargetComparison {
	frame := evaluation.Context
	out := &crmcontracts.ReportingTargetComparison{TargetMetric: comparedMetric}
	if ops == nil {
		// The forecast reads no figure over several teams at once.
		if frame.Scope.Kind != crmcontracts.ReportingScopeKindManagedTeams {
			return nil
		}
		return incompatible(out, crmcontracts.ReportingTargetComparisonIncompatibilityScope, oneTeam(ctx, evaluation.Selection))
	}
	out.Basis, out.Operands = &ops.Measure, &ops.Operands
	whole, wholeHeld := held[wholePipelineKey(frame.Scope, comparedMetric)]
	target, targetHeld := held[keyFor(frame.Scope, comparedMetric)]
	switch {
	case frame.TargetInterval == nil || !sameWindow(*frame.TargetInterval, ops.Quarter):
		quarter := evaluation.Selection
		quarter.Period, quarter.TargetBasis, quarter.Interval = reportingThisQuarter, reportingFiscalQuarter, nil
		return incompatible(out, crmcontracts.ReportingTargetComparisonIncompatibilityPeriod, &quarter)
	case ops.CallOverPipeline:
		return incompatible(out, crmcontracts.ReportingTargetComparisonIncompatibilityManagerCallPipeline, withoutPipeline(evaluation.Selection))
	case metric.Coverage.Withheld:
		out.Status = crmcontracts.ReportingTargetComparisonStatusPartialVisibility
		return out
	case ops.FxMissing:
		out.Status = crmcontracts.ReportingTargetComparisonStatusFxMissing
		return out
	case !targetHeld && wholeHeld && whole.Unit == frame.Currency:
		return incompatible(out, crmcontracts.ReportingTargetComparisonIncompatibilityPipeline, withoutPipeline(evaluation.Selection))
	case !targetHeld:
		out.Status = crmcontracts.ReportingTargetComparisonStatusNoTarget
		return out
	case target.Unit != frame.Currency:
		// No selection changes a target's currency; it is set again.
		return incompatible(out, crmcontracts.ReportingTargetComparisonIncompatibilityCurrency, nil)
	}
	measure(out, *ops, target)
	return out
}

// measure fills the figures: the gap to the target, what is left to win and
// how far the open pipeline covers it. Nothing left to win is the target
// reached, never an unbounded coverage.
func measure(out *crmcontracts.ReportingTargetComparison, ops ForecastOperands, target crmcontracts.ReportingTarget) {
	value := target.Definition.Value
	remaining := max(value-ops.WonMinor, 0)
	out.TargetId, out.TargetRevision = &target.Id, &target.Revision
	out.Target, out.Landing, out.Won = money(value), money(ops.LandingMinor), money(ops.WonMinor)
	out.Gap, out.Remaining, out.OpenPipeline = money(value-ops.LandingMinor), money(remaining), money(ops.OpenMinor)
	qualified := crmcontracts.ReportingQualifiedCoverage{Status: crmcontracts.ReportingQualifiedCoverageStatusNotEnforced}
	if ops.QualifiedOpenMinor != nil {
		qualified.OpenPipeline = money(*ops.QualifiedOpenMinor)
	}
	out.Status = crmcontracts.ReportingTargetComparisonStatusTargetReached
	if remaining > 0 {
		out.Status = crmcontracts.ReportingTargetComparisonStatusCompared
		out.Coverage = ratio(ops.OpenMinor, remaining)
	}
	if ops.QualifiedOpenMinor != nil {
		qualified.Status = crmcontracts.ReportingQualifiedCoverageStatusTargetReached
		if remaining > 0 {
			qualified.Status = crmcontracts.ReportingQualifiedCoverageStatusCompared
			qualified.Coverage = ratio(*ops.QualifiedOpenMinor, remaining)
		}
	}
	out.Qualified = &qualified
}

func incompatible(out *crmcontracts.ReportingTargetComparison, why crmcontracts.ReportingTargetComparisonIncompatibility, supported *crmcontracts.ReportingSelection) *crmcontracts.ReportingTargetComparison {
	out.Status, out.Incompatibility, out.SupportedSelection = crmcontracts.ReportingTargetComparisonStatusIncompatible, &why, supported
	return out
}

func withoutPipeline(selection crmcontracts.ReportingSelection) *crmcontracts.ReportingSelection {
	selection.PipelineId = nil
	return &selection
}

// oneTeam is the selection over the first team the reader manages, which the
// forecast reads as one population.
func oneTeam(ctx context.Context, selection crmcontracts.ReportingSelection) *crmcontracts.ReportingSelection {
	actor, ok := principal.Actor(ctx)
	if !ok || len(actor.TeamIDs) == 0 {
		return nil
	}
	team := openapi_types.UUID(slices.MinFunc(actor.TeamIDs, func(a, b ids.UUID) int { return strings.Compare(a.String(), b.String()) }))
	selection.Scope = crmcontracts.ReportingScope{Kind: reportingTeam, Id: &team}
	return &selection
}

func sameWindow(a, b crmcontracts.ReportingWindow) bool {
	return a.StartAt.Equal(b.StartAt) && a.EndAt.Equal(b.EndAt)
}

func money(minor int64) *float64 {
	value := float64(minor)
	return &value
}

func ratio(open, remaining int64) *float64 {
	value := float64(open) / float64(remaining)
	return &value
}
