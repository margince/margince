// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

import (
	"slices"
	"time"

	openapi_types "github.com/oapi-codegen/runtime/types"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/modules/reporting"
)

func (metricEvaluator) ProjectFrozen(before crmcontracts.ReportingEvaluation, facts []reporting.Fact) (crmcontracts.ReportingEvaluation, error) {
	zone, err := time.LoadLocation(before.Context.Timezone)
	if err != nil {
		return crmcontracts.ReportingEvaluation{}, err
	}
	before.Context.Interval.StartAt = before.Context.Interval.StartAt.In(zone)
	before.Context.Interval.EndAt = before.Context.Interval.EndAt.In(zone)
	before.Context.EvaluatedAt = before.Context.EvaluatedAt.In(zone)
	if before.Context.TargetInterval != nil {
		target := *before.Context.TargetInterval
		target.StartAt, target.EndAt = target.StartAt.In(zone), target.EndAt.In(zone)
		before.Context.TargetInterval = &target
	}
	out := reporting.Evaluation{Result: before, Facts: facts}
	out.Result.Metrics = []crmcontracts.ReportingMetric{}
	out.Result.Charts = []crmcontracts.ReportingChart{}
	out.Result.Context.MemberIds = []openapi_types.UUID{}
	for _, fact := range facts {
		owner := openapi_types.UUID(fact.OwnerID)
		if !fact.OwnerID.IsZero() && !slices.Contains(out.Result.Context.MemberIds, owner) {
			out.Result.Context.MemberIds = append(out.Result.Context.MemberIds, owner)
		}
	}
	for _, metric := range before.Metrics {
		index := slices.IndexFunc(reportingMetrics, func(spec metricSpec) bool { return spec.definition.Id == metric.Id })
		if index < 0 {
			continue
		}
		coverage := metric.Coverage
		coverage.Withheld = true
		coverage.EligibleCount = nil
		coverage.PricedCount = nil
		projected := projectReportingMetric(reportingMetrics[index].definition, before.Context, facts, coverage)
		if metric.Id == reportingClosedWinRate {
			projected.Coverage.Status = coverage.Status
			projected.Coverage.Withheld = false
			projectClosedRate(&projected, metricFacts(facts, metric.Id, reportingInterval))
			projected.Coverage.Withheld = true
		}
		projected.Coverage.EligibleCount = nil
		projected.Coverage.PricedCount = nil
		if metric.Id == reportingForecastLanding || metric.Id == reportingStageAge {
			projected.Value = nil
			projected.Numerator = nil
			projected.Denominator = nil
			projected.Coverage = reportingGap(reportingUnavailable, "Some historical evidence is restricted; this reading cannot be compared")
			projected.Coverage.Withheld = true
		}
		out.Result.Metrics = append(out.Result.Metrics, projected)
	}
	if err := projectReportingCharts(&out); err != nil {
		return crmcontracts.ReportingEvaluation{}, err
	}
	restrictFrozenCharts(out.Result.Charts)
	return out.Result, nil
}

func restrictFrozenCharts(charts []crmcontracts.ReportingChart) {
	for index := range charts {
		chart := &charts[index]
		chart.Coverage.Withheld = true
		chart.Coverage.EligibleCount = nil
		chart.Coverage.PricedCount = nil
		if chart.Metric == reportingForecastLanding || chart.Metric == reportingStageAge || chart.Kind == reportingPipelineMovement {
			chart.Points = []crmcontracts.ReportingPoint{}
			chart.Marker = nil
			chart.Opening = nil
			chart.Closing = nil
			chart.Coverage.Status = reportingUnavailable
		}
	}
}
