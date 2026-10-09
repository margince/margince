// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

import (
	"context"
	"errors"

	"github.com/jackc/pgx/v5"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/modules/forecasting"
	"github.com/margince/margince/backend/internal/modules/reporting"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

func readReportingForecast(ctx context.Context, tx pgx.Tx, frame crmcontracts.ReportingContext, _ crmcontracts.ReportingFramework, e metricEvaluator) ([]reporting.Fact, crmcontracts.ReportingCoverage, error) {
	if frame.Scope.Kind == ScopeKindManagedTeams {
		return nil, reportingGap("unsupported", "Choose a single team for its forecast"), nil
	}
	period, base, err := ForecastPeriodAt(ctx, tx, forecasting.PeriodQuarter, frame.EvaluatedAt)
	if err != nil {
		return nil, crmcontracts.ReportingCoverage{}, err
	}
	scope := forecasting.Scope{Kind: string(frame.Scope.Kind), ID: (*ids.UUID)(frame.Scope.Id)}
	deals, _, withheld, err := forecastDealsForPipeline(ctx, tx, period, scope, frame.EvaluatedAt, base, (*ids.UUID)(frame.PipelineId))
	if err != nil {
		return nil, crmcontracts.ReportingCoverage{}, err
	}
	readings, err := forecasting.Compute(period, period.LocalDay(frame.EvaluatedAt), deals)
	if err != nil {
		return nil, crmcontracts.ReportingCoverage{}, err
	}
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
	facts, err := forecastMetricFacts(readings, landing, called)
	if err != nil {
		return nil, crmcontracts.ReportingCoverage{}, err
	}
	for i := range facts {
		if facts[i].SourceType == reportingForecastProjection {
			scope := frame.Scope
			facts[i].Scope = &scope
		}
	}
	return facts, reportingForecastCoverage(readings, landing, withheld), nil
}

func forecastMetricFacts(readings forecasting.Readings, landing forecasting.Landing, called *int64) ([]reporting.Fact, error) {
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
			fact.Row = crmcontracts.ReportingEvidenceRow{Key: contribution.DealID, Label: "Forecast contribution", SourceId: ptrUUID(id), SourceType: &fact.SourceType, OwnerId: ptrUUID(owner)}
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
	return forecastAuthoredFacts(out, landing, called)
}

func reportingForecastChart(out *reporting.Evaluation, chart crmcontracts.ReportingChart) crmcontracts.ReportingChart {
	chart.ContextId = objectForecast
	chart.StateAt = &out.Result.Context.StateAt
	chart.Interval = out.Result.Context.CloseInterval
	for _, part := range []struct{ id, label string }{{"forecast_won", "Won"}, {"forecast_supported", "Supported open"}, {"forecast_upside", "Additional upside"}} {
		facts := metricFacts(out.Facts, chart.Metric, part.id)
		value, _, _ := metricSum(facts)
		chart.Points = append(chart.Points, crmcontracts.ReportingPoint{Key: part.id, Label: part.label, Value: value, Status: chart.Coverage.Status, Evidence: &crmcontracts.ReportingEvidenceRef{Metric: chart.Metric, ContextId: part.id}})
	}
	calls := metricFacts(out.Facts, chart.Metric, "forecast_call")
	if len(calls) == 1 {
		chart.Marker = calls[0].Row.Value
	}
	return chart
}

func reportingForecastCoverage(readings forecasting.Readings, landing forecasting.Landing, withheld bool) crmcontracts.ReportingCoverage {
	coverage := crmcontracts.ReportingCoverage{Status: "ok", Withheld: withheld}
	if readings.PricedCount < readings.EligibleCount || readings.FxMissingCount > 0 || landing.Caveat != "" {
		coverage.Status = reportingPartial
		reason := string(landing.Caveat)
		if reason == "" {
			reason = "Some forecast contributions are unpriced"
		}
		coverage.Reason = &reason
	}
	return coverage
}

func forecastAuthoredFacts(out []reporting.Fact, landing forecasting.Landing, called *int64) ([]reporting.Fact, error) {
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
		out = append(out, reporting.Fact{Metric: reportingForecastLanding, ContextID: reading.context, SourceType: reportingForecastProjection, Provenance: string(landing.Measure), Money: reading.amount, Row: crmcontracts.ReportingEvidenceRow{Key: reading.context, Label: reading.context, Value: &value}})
	}
	return out, nil
}
