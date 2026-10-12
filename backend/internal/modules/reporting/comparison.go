// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package reporting

import (
	"math"
	"slices"
	"time"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/shared/kernel/reportperiod"
)

func comparisonReason(left, right crmcontracts.ReportingEdition) string {
	if left.Withheld || right.Withheld || left.Redacted || right.Redacted {
		return "Historical evidence is restricted or redacted"
	}
	if left.ReportId != right.ReportId {
		return "Choose two editions of the same report"
	}
	a, b := left.Evaluation.Context, right.Evaluation.Context
	if a.PopulationFingerprint != b.PopulationFingerprint {
		return "Team membership or population changed"
	}
	if differentComparisonContext(a, b) {
		return "Reporting definitions or context differ"
	}
	sa, sb := left.Evaluation.Selection, right.Evaluation.Selection
	if sa.Scope.Kind != sb.Scope.Kind || !sameID(sa.Scope.Id, sb.Scope.Id) || sa.TargetBasis != sb.TargetBasis || sa.CloseWindow != sb.CloseWindow || !slices.Equal(slices.Sorted(slices.Values(sa.Metrics)), slices.Sorted(slices.Values(sb.Metrics))) {
		return "Saved report selections differ"
	}
	return comparisonPeriodsAndMetrics(left, right)
}

func comparisonMetric(metrics []crmcontracts.ReportingMetric, id crmcontracts.ReportingMetricID) (crmcontracts.ReportingMetric, bool) {
	for _, metric := range metrics {
		if metric.Id == id {
			return metric, true
		}
	}
	return crmcontracts.ReportingMetric{}, false
}

func comparisonDeltas(left, right crmcontracts.ReportingEdition) []crmcontracts.ReportingDelta {
	deltas := []crmcontracts.ReportingDelta{}
	for _, before := range left.Evaluation.Metrics {
		switch before.Id {
		case "open_pipeline", reportingStageAge, "forecast_landing":
			continue
		}
		after, found := comparisonMetric(right.Evaluation.Metrics, before.Id)
		if !found || before.Value == nil || after.Value == nil || before.Coverage.Status != "ok" || after.Coverage.Status != "ok" || before.Coverage.Withheld || after.Coverage.Withheld {
			continue
		}
		absolute := *after.Value - *before.Value
		if math.Abs(absolute) > 9007199254740991 {
			continue
		}
		delta := crmcontracts.ReportingDelta{Metric: before.Id, Absolute: absolute}
		if *before.Value > 0 && before.Unit != "percent" {
			percentage := absolute / *before.Value * 100
			delta.Percentage = &percentage
		}
		deltas = append(deltas, delta)
	}
	return deltas
}

func comparisonPeriodsAndMetrics(left, right crmcontracts.ReportingEdition) string {
	a, b := left.Evaluation.Context, right.Evaluation.Context
	zone, err := time.LoadLocation(a.Timezone)
	if err != nil {
		return "Reporting timezone is unavailable"
	}
	leftPeriod := reportperiod.Context{Start: a.Interval.StartAt, End: a.Interval.EndAt, Timezone: zone.String(), Currency: a.Currency, Version: a.DefinitionVersion, Complete: true}
	rightPeriod := reportperiod.Context{Start: b.Interval.StartAt, End: b.Interval.EndAt, Timezone: b.Timezone, Currency: b.Currency, Version: b.DefinitionVersion, Complete: true}
	if !reportperiod.Comparable(leftPeriod, rightPeriod) {
		return "Compare adjacent complete weeks, months or quarters"
	}
	for _, before := range left.Evaluation.Metrics {
		after, found := comparisonMetric(right.Evaluation.Metrics, before.Id)
		if !found || before.Version != after.Version || before.Unit != after.Unit {
			return "Metric versions differ"
		}
	}
	return ""
}

func differentComparisonContext(a, b crmcontracts.ReportingContext) bool {
	return a.Timezone != b.Timezone || a.Currency != b.Currency || a.DefinitionVersion != b.DefinitionVersion || a.FrameworkRevision != b.FrameworkRevision || !sameID(a.PipelineId, b.PipelineId)
}
