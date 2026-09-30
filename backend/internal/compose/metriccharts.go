// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

import (
	"cmp"
	"slices"
	"time"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/modules/reporting"
)

func reportingGroups(out *reporting.Evaluation, chart crmcontracts.ReportingChart, owners bool) crmcontracts.ReportingChart {
	frame := out.Result.Context
	chart.ContextId = reportingState
	chart.StateAt = &frame.StateAt
	if owners {
		chart.ContextId = reportingTarget
		chart.Interval = frame.TargetInterval
		chart.StateAt = nil
	}
	if !owners && frame.PipelineId == nil {
		chart.Coverage.Status = "not_configured"
		reason := "Choose one pipeline to compare stages"
		chart.Coverage.Reason = &reason
		return chart
	}
	groups := map[string][]reporting.Fact{}
	labels := map[string]string{}
	for _, fact := range metricFacts(out.Facts, chart.Metric, chart.ContextId) {
		key, label := fact.StageID, fact.StageLabel
		if owners {
			key, label = fact.OwnerID.String(), fact.OwnerLabel
			if fact.OwnerID.IsZero() {
				label = "Unassigned"
			}
		}
		groups[key] = append(groups[key], fact)
		labels[key] = label
	}
	keys := make([]string, 0, len(groups))
	for key := range groups {
		keys = append(keys, key)
	}
	slices.SortFunc(keys, func(a, b string) int {
		if !owners {
			if order := cmp.Compare(groups[a][0].StagePosition, groups[b][0].StagePosition); order != 0 {
				return order
			}
		}
		if order := cmp.Compare(labels[a], labels[b]); order != 0 {
			return order
		}
		return cmp.Compare(a, b)
	})
	for _, key := range keys {
		facts := groups[key]
		value, upper, count, status := reportingGroupValue(facts, chart.Kind)
		group := "stage:" + key
		if owners {
			group = "owner:" + key
		}
		chart.Points = append(chart.Points, crmcontracts.ReportingPoint{Key: key, Label: labels[key], Value: value, Upper: upper, Observations: &count, Status: status, Evidence: &crmcontracts.ReportingEvidenceRef{Metric: chart.Metric, ContextId: chart.ContextId, GroupKey: &group}})
	}
	return chart
}

func reportingTrend(out *reporting.Evaluation, chart crmcontracts.ReportingChart) crmcontracts.ReportingChart {
	frame := out.Result.Context
	chart.ContextId = reportingInterval
	chart.Interval = &frame.Interval
	previous := reportingPrevious(frame)
	if !previous.StartAt.IsZero() {
		chart.ComparisonInterval = &previous
	}
	currentFacts := metricFacts(out.Facts, chart.Metric, reportingInterval)
	previousFacts := metricFacts(out.Facts, chart.Metric, "previous")
	start := frame.Interval.StartAt
	previousDay := previous.StartAt
	for day := start; day.Before(frame.Interval.EndAt); day = day.AddDate(0, 0, 1) {
		end := day.AddDate(0, 0, 1)
		if end.After(frame.Interval.EndAt) {
			end = frame.Interval.EndAt
		}
		value := sumFactsBefore(currentFacts, end)
		var comparison *float64
		if previousDay.Before(previous.EndAt) {
			priorEnd := previousDay.AddDate(0, 0, 1)
			if priorEnd.After(previous.EndAt) {
				priorEnd = previous.EndAt
			}
			comparison = sumFactsBefore(previousFacts, priorEnd)
		}
		chart.Points = append(chart.Points, crmcontracts.ReportingPoint{Key: day.Format(time.DateOnly), Label: day.Format("2 Jan"), At: &day, Value: value, Comparison: comparison, Status: chart.Coverage.Status, Evidence: &crmcontracts.ReportingEvidenceRef{Metric: chart.Metric, ContextId: reportingInterval, Through: &end}})
		previousDay = previousDay.AddDate(0, 0, 1)
	}
	return chart
}

func sumFactsBefore(facts []reporting.Fact, end time.Time) *float64 {
	selected := []reporting.Fact{}
	for _, fact := range facts {
		if fact.Row.OccurredAt != nil && fact.Row.OccurredAt.Before(end) {
			selected = append(selected, fact)
		}
	}
	sum, _, _ := metricSum(selected)
	return sum
}

func reportingWeekly(out *reporting.Evaluation, chart crmcontracts.ReportingChart) crmcontracts.ReportingChart {
	frame := out.Result.Context
	chart.ContextId = reportingMonthContext
	month := reportingMonth(frame)
	chart.Interval = &month
	facts := metricFacts(out.Facts, chart.Metric, reportingMonthContext)
	for start := month.StartAt; start.Before(month.EndAt) && start.Before(frame.EvaluatedAt); {
		days := 7 - (int(start.Weekday())+6)%7
		end := start.AddDate(0, 0, days)
		if end.After(month.EndAt) {
			end = month.EndAt
		}
		if end.After(frame.EvaluatedAt) {
			end = frame.EvaluatedAt
		}
		selected := []reporting.Fact{}
		for _, fact := range facts {
			if fact.Row.OccurredAt != nil && !fact.Row.OccurredAt.Before(start) && fact.Row.OccurredAt.Before(end) {
				selected = append(selected, fact)
			}
		}
		value, _, _ := metricSum(selected)
		key := start.Format(time.DateOnly)
		label := start.Format("2 Jan") + "–" + end.Add(-time.Nanosecond).Format("2 Jan")
		if days < 7 || end.Sub(start) < 6*24*time.Hour {
			label += " (partial)"
		}
		group := "week:" + key
		chart.Points = append(chart.Points, crmcontracts.ReportingPoint{Key: key, Label: label, At: &start, Value: value, Status: chart.Coverage.Status, Evidence: &crmcontracts.ReportingEvidenceRef{Metric: chart.Metric, ContextId: reportingMonthContext, GroupKey: &group}})
		start = end
	}
	return chart
}

func reportingGroupValue(facts []reporting.Fact, kind crmcontracts.ReportingBlockKind) (*float64, *float64, int64, crmcontracts.ReportingStatus) {
	value, count, exact := metricSum(facts)
	status := crmcontracts.ReportingStatus("ok")
	var upper *float64
	if !exact {
		status = reportingUnavailable
	}
	if kind == reportingStageAge {
		value = metricPercentile(facts, .5)
		upper = metricPercentile(facts, .75)
		count = int64(len(facts))
		if value == nil {
			status = reportingInsufficientSample
		}
	}
	return value, upper, count, status
}
