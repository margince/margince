// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

import (
	"context"
	"errors"

	"github.com/jackc/pgx/v5"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/modules/assurance"
	"github.com/margince/margince/backend/internal/modules/forecasting"
	"github.com/margince/margince/backend/internal/modules/reporting"
	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/platform/config"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

func readReportingForecast(ctx context.Context, tx pgx.Tx, frame crmcontracts.ReportingContext, _ crmcontracts.ReportingFramework, e metricEvaluator) ([]reporting.Fact, crmcontracts.ReportingCoverage, error) {
	if frame.Scope.Kind == ScopeKindManagedTeams {
		return nil, reportingGap("unsupported", reporting.ExplanationCopy(frame).Say(reportingCoverageWords.SingleTeam)), nil
	}
	period, base, err := ForecastPeriodAt(ctx, tx, forecasting.PeriodQuarter, frame.EvaluatedAt)
	if err != nil {
		return nil, crmcontracts.ReportingCoverage{}, err
	}
	scope := forecasting.Scope{Kind: string(frame.Scope.Kind), ID: (*ids.UUID)(frame.Scope.Id)}
	assembly, err := assembleForecast(ctx, tx, period, scope, frame.EvaluatedAt, base, (*ids.UUID)(frame.PipelineId))
	if errors.As(err, new(auth.IndividualPerformanceOffError)) {
		// The landing alone is withheld; the evaluation's other figures stand.
		if e.compared != nil {
			e.compared.operands = &reporting.ForecastOperands{Quarter: crmcontracts.ReportingWindow{StartAt: period.Start, EndAt: period.End}, ColleagueWithheld: true}
		}
		return []reporting.Fact{}, loneColleagueCoverage(frame), nil
	}
	if err != nil {
		return nil, crmcontracts.ReportingCoverage{}, err
	}
	readings, withheld := assembly.Readings, assembly.Limited
	measure, err := ForecastForwardMeasure(ctx, tx)
	if err != nil {
		return nil, crmcontracts.ReportingCoverage{}, err
	}
	var called *int64
	if frame.PipelineId == nil {
		call, err := e.forecast.CurrentCallTx(ctx, tx, period, scope)
		if err == nil {
			called = &call.AmountMinor
		} else if !forecasting.IsNoStandingCall(err) {
			return nil, crmcontracts.ReportingCoverage{}, err
		}
	}
	landing, err := forecasting.ProjectLanding(readings, measure, called)
	if err != nil {
		return nil, crmcontracts.ReportingCoverage{}, err
	}
	facts, err := forecastMetricFacts(frame, readings, landing, called)
	if err != nil {
		return nil, crmcontracts.ReportingCoverage{}, err
	}
	qualifiedMinor, enforced, err := forecastQualifiedOpen(ctx, tx, readings)
	if err != nil {
		return nil, crmcontracts.ReportingCoverage{}, err
	}
	var qualifiedOpen *int64
	if enforced {
		qualifiedOpen = &qualifiedMinor
	}
	facts, err = forecastOpenFacts(frame, facts, readings, qualifiedOpen)
	if err != nil {
		return nil, crmcontracts.ReportingCoverage{}, err
	}
	facts, err = forecastRangeFacts(frame, facts, readings.Range)
	if err != nil {
		return nil, crmcontracts.ReportingCoverage{}, err
	}
	// A reader with no capture, such as the weekly digest, compares nothing.
	if e.compared != nil {
		operands := forecastOperands(period, readings, landing, qualifiedOpen, frame.PipelineId != nil && measure == forecasting.MeasureManagerCall)
		if err := guardForecastOperands(ctx, tx, period, scope, frame, readings, operands); err != nil {
			return nil, crmcontracts.ReportingCoverage{}, err
		}
		e.compared.operands = operands
	}
	for i := range facts {
		if facts[i].SourceType == reportingForecastProjection {
			scope := frame.Scope
			facts[i].Scope = &scope
		}
	}
	return facts, reportingForecastCoverage(frame, readings, landing, withheld), nil
}

func forecastMetricFacts(frame crmcontracts.ReportingContext, readings forecasting.Readings, landing forecasting.Landing, called *int64) ([]reporting.Fact, error) {
	out := []reporting.Fact{}
	for _, contribution := range readings.Contributions {
		id, err := ids.Parse(contribution.DealID)
		if err != nil {
			return nil, err
		}
		owner, err := ids.Parse(contribution.Owner)
		if err != nil && contribution.Owner != "" {
			return nil, err
		}
		contexts := []string{}
		if contribution.InWon {
			contexts = append(contexts, "forecast_won")
		}
		if contribution.InEvidence {
			contexts = append(contexts, "forecast_supported")
		}
		if contribution.InBestCase && !contribution.InEvidence {
			contexts = append(contexts, "forecast_upside")
		}
		for _, contextID := range contexts {
			fact := reporting.Fact{Metric: reportingForecastLanding, ContextID: contextID, SourceType: string(recordTypeDeal), SourceID: id, OwnerID: owner, Money: contribution.BaseMinor}
			fact.Row = crmcontracts.ReportingEvidenceRow{Key: contribution.DealID, Label: reporting.ExplanationCopy(frame).Say(reportingLabelWords.ForecastContribution), SourceId: ptrUUID(id), SourceType: &fact.SourceType, OwnerId: ptrUUID(owner)}
			if contribution.BaseMinor != nil {
				if *contribution.BaseMinor > reportingExactInteger || *contribution.BaseMinor < -reportingExactInteger {
					return nil, errors.New("forecast exceeds exact reporting range")
				}
				number := float64(*contribution.BaseMinor)
				fact.Row.Value = &number
			}
			out = append(out, fact)
		}
	}
	return forecastAuthoredFacts(frame, out, landing, called)
}

func reportingForecastChart(out *reporting.Evaluation, chart crmcontracts.ReportingChart) crmcontracts.ReportingChart {
	chart.ContextId = objectForecast
	chart.StateAt = &out.Result.Context.StateAt
	chart.Interval = out.Result.Context.CloseInterval
	for _, part := range []string{"forecast_won", "forecast_supported", "forecast_upside"} {
		facts := metricFacts(out.Facts, chart.Metric, part)
		value, _, _ := metricSum(facts)
		chart.Points = append(chart.Points, crmcontracts.ReportingPoint{Key: part, Label: reporting.ExplanationCopy(out.Result.Context).Say(reportingForecastLabels[part]), Value: value, Status: chart.Coverage.Status, Evidence: &crmcontracts.ReportingEvidenceRef{Metric: chart.Metric, ContextId: part}})
	}
	calls := metricFacts(out.Facts, chart.Metric, "forecast_call")
	if len(calls) == 1 {
		chart.Marker = calls[0].Row.Value
	}
	return chart
}

func reportingForecastCoverage(frame crmcontracts.ReportingContext, readings forecasting.Readings, landing forecasting.Landing, withheld bool) crmcontracts.ReportingCoverage {
	coverage := crmcontracts.ReportingCoverage{Status: "ok", Withheld: withheld}
	if readings.PricedCount < readings.EligibleCount || readings.FxMissingCount > 0 || landing.Caveat != "" {
		coverage.Status = reportingPartial
		reason := reportingForecastReason(frame, landing.Caveat)
		coverage.Reason = &reason
	}
	return coverage
}

func forecastAuthoredFacts(frame crmcontracts.ReportingContext, out []reporting.Fact, landing forecasting.Landing, called *int64) ([]reporting.Fact, error) {
	// Authored totals cannot be redistributed into per-deal credit. Projection
	// retains them only while the complete frozen population remains visible.
	for _, reading := range []struct {
		context string
		amount  *int64
	}{{reportingForecastLanding, &landing.AmountMinor}, {"forecast_call", called}} {
		if reading.amount == nil {
			continue
		}
		if *reading.amount > reportingExactInteger || *reading.amount < -reportingExactInteger {
			return nil, errors.New("forecast exceeds exact reporting range")
		}
		value := float64(*reading.amount)
		out = append(out, reporting.Fact{Metric: reportingForecastLanding, ContextID: reading.context, SourceType: reportingForecastProjection, Provenance: string(landing.Measure), Money: reading.amount, Row: crmcontracts.ReportingEvidenceRow{Key: reading.context, Label: reporting.ExplanationCopy(frame).Say(reportingForecastLabels[reading.context]), Value: &value}})
	}
	return out, nil
}

const (
	forecastOpenContext      = "forecast_open"
	forecastQualifiedContext = "forecast_open_qualified"
	forecastRangeLowContext  = "forecast_range_low"
	forecastRangeHighContext = "forecast_range_high"
)

// forecastRangeFacts adds the two ends of the forecast's range as authored
// totals beside the landing. The metric and its evidence then state the range
// GET /forecast states.
func forecastRangeFacts(frame crmcontracts.ReportingContext, out []reporting.Fact, drawn *forecasting.Range) ([]reporting.Fact, error) {
	if drawn == nil {
		return out, nil
	}
	for _, end := range []struct {
		context string
		amount  int64
	}{{forecastRangeLowContext, drawn.LowMinor}, {forecastRangeHighContext, drawn.HighMinor}} {
		if end.amount > reportingExactInteger {
			return nil, errors.New("forecast exceeds exact reporting range")
		}
		value := float64(end.amount)
		out = append(out, reporting.Fact{
			Metric: reportingForecastLanding, ContextID: end.context, SourceType: reportingForecastProjection,
			Money: new(end.amount),
			Row: crmcontracts.ReportingEvidenceRow{
				Key: end.context, Label: reporting.ExplanationCopy(frame).Say(reportingForecastLabels[end.context]), Value: &value,
			},
		})
	}
	return out, nil
}

// forecastQualifiedOpen is the open money the qualified forecast keeps, and
// false while it is switched off. With enforcement on, every deal read carries
// a qualification, so only a quarter with no deal needs the setting itself.
func forecastQualifiedOpen(ctx context.Context, tx pgx.Tx, readings forecasting.Readings) (int64, bool, error) {
	if readings.Qualified != nil {
		return readings.Qualified.OpenMinor, true, nil
	}
	if len(readings.Contributions) > 0 {
		return 0, false, nil
	}
	enforced, err := config.StoredApplyTx(ctx, tx, config.Bound(), assurance.QualifiedForecastEnforced)
	return 0, enforced, err
}

// forecastOpenFacts adds the open totals a target comparison divides, as
// authored totals beside the landing. A deal with no exchange rate leaves the
// total unknown rather than short.
func forecastOpenFacts(frame crmcontracts.ReportingContext, out []reporting.Fact, readings forecasting.Readings, qualifiedOpen *int64) ([]reporting.Fact, error) {
	totals := map[string]*int64{forecastOpenContext: &readings.OpenMinor}
	if qualifiedOpen != nil {
		totals[forecastQualifiedContext] = qualifiedOpen
	}
	for _, context := range []string{forecastOpenContext, forecastQualifiedContext} {
		amount, ok := totals[context]
		if !ok {
			continue
		}
		if *amount > reportingExactInteger {
			return nil, errors.New("forecast exceeds exact reporting range")
		}
		fact := reporting.Fact{Metric: reportingForecastLanding, ContextID: context, SourceType: reportingForecastProjection, Row: crmcontracts.ReportingEvidenceRow{Key: context, Label: reporting.ExplanationCopy(frame).Say(reportingForecastLabels[context])}}
		if readings.FxMissingCount == 0 {
			value := float64(*amount)
			fact.Money, fact.Row.Value = amount, &value
		}
		out = append(out, fact)
	}
	return out, nil
}

// forecastOperands are the figures a target comparison rests on, read from
// the same readings as the landing.
func forecastOperands(period forecasting.Period, readings forecasting.Readings, landing forecasting.Landing, qualifiedOpen *int64, callOverPipeline bool) *reporting.ForecastOperands {
	ref := func(context string) crmcontracts.ReportingEvidenceRef {
		return crmcontracts.ReportingEvidenceRef{Metric: reportingForecastLanding, ContextId: context}
	}
	out := &reporting.ForecastOperands{
		Quarter: crmcontracts.ReportingWindow{StartAt: period.Start, EndAt: period.End},
		Measure: crmcontracts.ReportingTargetComparisonBasis(landing.Measure), LandingMinor: landing.AmountMinor,
		WonMinor: readings.WonMinor, OpenMinor: readings.OpenMinor, QualifiedOpenMinor: qualifiedOpen,
		FxMissing: readings.FxMissingCount > 0, CallOverPipeline: callOverPipeline,
		CallBelowWon: landing.Caveat == forecasting.CaveatCallBelowActual,
		Operands:     crmcontracts.ReportingComparisonOperands{Won: ref("forecast_won"), Landing: ref(reportingForecastLanding), OpenPipeline: ref(forecastOpenContext)},
	}
	if qualifiedOpen != nil {
		qualified := ref(forecastQualifiedContext)
		out.Operands.QualifiedOpenPipeline = &qualified
	}
	return out
}

// guardForecastOperands marks what the comparison may not show. One is a
// population the reader cannot fully see. The other is a total one colleague
// besides the reader makes up, while individual performance is off. The
// totals are open, won, and every deal read, which the landing rests on.
func guardForecastOperands(ctx context.Context, tx pgx.Tx, period forecasting.Period, scope forecasting.Scope, frame crmcontracts.ReportingContext, readings forecasting.Readings, operands *reporting.ForecastOperands) error {
	narrowed, err := forecastPopulationNarrowed(ctx, tx, period, scope, (*ids.UUID)(frame.PipelineId))
	if err != nil {
		return err
	}
	operands.Narrowed = narrowed
	var open, won, read []ids.UUID
	for _, contribution := range readings.Contributions {
		if contribution.Owner == "" {
			continue
		}
		owner, err := ids.Parse(contribution.Owner)
		if err != nil {
			return err
		}
		read = append(read, owner)
		if contribution.InOpen {
			open = append(open, owner)
		}
		if contribution.InWon {
			won = append(won, owner)
		}
	}
	err = requireNoLoneColleague(ctx, tx, [][]ids.UUID{open, won, read})
	if errors.As(err, new(auth.IndividualPerformanceOffError)) {
		operands.ColleagueWithheld = true
		return nil
	}
	return err
}
