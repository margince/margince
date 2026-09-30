// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

import (
	"fmt"
	"math"
	"math/big"
	"slices"

	"github.com/margince/margince/backend/internal/compose/analyticsquery"
	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/modules/reporting"
	"github.com/margince/margince/backend/internal/shared/apperrors"
)

func metricFacts(facts []reporting.Fact, metric crmcontracts.ReportingMetricID, contextID string) []reporting.Fact {
	out := []reporting.Fact{}
	for _, fact := range facts {
		if fact.Metric == metric && fact.ContextID == contextID {
			out = append(out, fact)
		}
	}
	return out
}

func metricSum(facts []reporting.Fact) (*float64, int64, bool) {
	exact := new(big.Int)
	sum := 0.0
	priced := int64(0)
	for _, fact := range facts {
		if fact.Row.Value == nil {
			continue
		}
		priced++
		if fact.Money != nil {
			exact.Add(exact, big.NewInt(*fact.Money))
		} else {
			sum += *fact.Row.Value
		}
	}
	if !exact.IsInt64() || exact.Cmp(big.NewInt(reportingExactInteger)) > 0 || exact.Cmp(big.NewInt(-reportingExactInteger)) < 0 {
		return nil, priced, false
	}
	sum += float64(exact.Int64())
	if math.Abs(sum) > float64(reportingExactInteger) || math.IsNaN(sum) || math.IsInf(sum, 0) {
		return nil, priced, false
	}
	return &sum, priced, true
}

func projectReportingMetric(def crmcontracts.ReportingMetricDefinition, frame crmcontracts.ReportingContext, facts []reporting.Fact, coverage crmcontracts.ReportingCoverage) crmcontracts.ReportingMetric {
	contextID := reportingInterval
	if def.TemporalBasis == reportingStateAt {
		contextID = reportingState
	}
	out := crmcontracts.ReportingMetric{Id: def.Id, Version: def.Version, Unit: def.Unit, Coverage: coverage, Evidence: crmcontracts.ReportingEvidenceRef{Metric: def.Id, ContextId: contextID}}
	if out.Unit == reportingMoney {
		out.Unit = frame.Currency
	}
	if coverage.Status != "ok" && coverage.Status != reportingPartial {
		return out
	}
	cohort := metricFacts(facts, def.Id, contextID)
	count := int64(len(cohort))
	out.Coverage.EligibleCount = &count
	sum, priced, exact := metricSum(cohort)
	out.Coverage.PricedCount = &priced
	if !exact {
		out.Coverage.Status = reportingUnavailable
		reason := "The total exceeds the exact display range"
		out.Coverage.Reason = &reason
		return out
	}
	out.Value = sum
	if priced < count {
		out.Coverage.Status = reportingPartial
		reason := "Some contributions have no available value"
		out.Coverage.Reason = &reason
	}
	if count == 0 && coverage.Status == "ok" {
		out.Coverage.Status = "no_data"
	}
	switch def.Id {
	case reportingForecastLanding:
		landing := metricFacts(facts, def.Id, reportingForecastLanding)
		if len(landing) == 1 {
			out.Value = landing[0].Row.Value
		}
		out.Coverage.Status = coverage.Status
	case reportingClosedWinRate:
		projectClosedRate(&out, cohort)
	case reportingStageAge:
		out.Value = metricPercentile(cohort, .5)
		if out.Value == nil {
			out.Coverage.Status = reportingInsufficientSample
		}
	}
	if def.SupportsTarget {
		out.TargetActual, _, _ = metricSum(metricFacts(facts, def.Id, reportingTarget))
	}
	return out
}

func projectClosedRate(out *crmcontracts.ReportingMetric, facts []reporting.Fact) {
	if len(facts) < analyticsquery.PercentileSampleFloor || out.Coverage.Withheld {
		out.Value = nil
		out.Coverage.Status = reportingInsufficientSample
		return
	}
	won := 0.0
	for _, fact := range facts {
		if fact.Outcome == "won" {
			won++
		}
	}
	count := float64(len(facts))
	out.Numerator = &won
	out.Denominator = &count
	ratio := won / count * 100
	out.Value = &ratio
}

func metricPercentile(facts []reporting.Fact, fraction float64) *float64 {
	values := []float64{}
	for _, fact := range facts {
		if fact.Row.Value != nil {
			values = append(values, *fact.Row.Value)
		}
	}
	if len(values) < analyticsquery.PercentileSampleFloor {
		return nil
	}
	slices.Sort(values)
	index := float64(len(values)-1) * fraction
	low := int(math.Floor(index))
	high := int(math.Ceil(index))
	value := values[low] + (values[high]-values[low])*(index-float64(low))
	return &value
}

func projectReportingCharts(out *reporting.Evaluation) error {
	for _, block := range out.Result.Selection.Blocks {
		for _, metric := range out.Result.Metrics {
			if block == reportingOwnerAttainment && metric.Id != reportingBookingsWon && slices.Contains(out.Result.Selection.Metrics, crmcontracts.ReportingMetricID(reportingBookingsWon)) {
				continue
			}
			spec := slices.IndexFunc(reportingMetrics, func(spec metricSpec) bool {
				return spec.definition.Id == metric.Id && slices.Contains(spec.definition.Blocks, block)
			})
			if spec < 0 || block == reportingMetricReading {
				continue
			}
			chart := crmcontracts.ReportingChart{Kind: block, Metric: metric.Id, Unit: metric.Unit, Coverage: metric.Coverage, ContextId: metric.Evidence.ContextId, Points: []crmcontracts.ReportingPoint{}}
			if metric.Coverage.Status == reportingUnavailable || metric.Coverage.Status == "unsupported" || metric.Coverage.Status == "not_configured" {
				out.Result.Charts = append(out.Result.Charts, chart)
				continue
			}
			chart = populateReportingChart(out, chart, metric)
			for _, point := range chart.Points {
				if point.Value != nil && math.Abs(*point.Value) > float64(reportingExactInteger) {
					return fmt.Errorf("chart total exceeds the exact display range: %w", apperrors.ErrInvalidArgument)
				}
			}
			out.Result.Charts = append(out.Result.Charts, chart)
		}
	}
	return nil
}

func populateReportingChart(out *reporting.Evaluation, chart crmcontracts.ReportingChart, metric crmcontracts.ReportingMetric) crmcontracts.ReportingChart {
	switch chart.Kind {
	case "bookings_trend":
		chart = reportingTrend(out, chart)
	case reportingOwnerAttainment:
		chart = reportingGroups(out, chart, true)
	case "stage_distribution", reportingStageAge:
		chart = reportingGroups(out, chart, false)
	case reportingTargetProgress:
		chart.ContextId = reportingTarget
		chart.Interval = out.Result.Context.TargetInterval
		chart.Points = []crmcontracts.ReportingPoint{{Key: string(metric.Id), Label: string(metric.Id), Value: metric.TargetActual, Status: metric.Coverage.Status, Evidence: &crmcontracts.ReportingEvidenceRef{Metric: metric.Id, ContextId: reportingTarget}}}
	case reportingSdrOutcomes:
		chart = reportingWeekly(out, chart)
	case "forecast_support":
		chart = reportingForecastChart(out, chart)
	case reportingPipelineMovement:
		chart.Coverage = crmcontracts.ReportingCoverage{Status: reportingUnavailable}
		reason := "Collecting movement history"
		chart.Coverage.Reason = &reason
	}
	return chart
}
