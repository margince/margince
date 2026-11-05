// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

import (
	"cmp"
	"slices"

	"github.com/margince/margince/backend/internal/compose/analyticsquery"
	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/modules/reporting"
)

// cohortProgressed says whether an outcome counts toward a cohort's rate.
func cohortProgressed(outcome string) bool {
	return outcome == cohortAdvanced || outcome == cohortWon || outcome == cohortAccepted || outcome == cohortQualified
}

// cohortResolved says whether a member reached an outcome the rate can judge.
// A member still open, and one whose direction is unknown, cannot be judged.
func cohortResolved(outcome string) bool {
	return outcome != cohortUnresolved && outcome != cohortHistoryPartial
}

// cohortRate is the share of resolved members that progressed, as a percent,
// with its numerator and denominator. The rate is nil below the sample floor.
func cohortRate(facts []reporting.Fact) (rate *float64, progressed, resolved float64) {
	for _, fact := range facts {
		if cohortResolved(fact.Outcome) {
			resolved++
		}
		if cohortProgressed(fact.Outcome) {
			progressed++
		}
	}
	if resolved < analyticsquery.PercentileSampleFloor {
		return nil, progressed, resolved
	}
	value := progressed / resolved * 100
	return &value, progressed, resolved
}

func projectCohortRate(out *crmcontracts.ReportingMetric, facts []reporting.Fact) {
	rate, progressed, resolved := cohortRate(facts)
	out.Value, out.Numerator, out.Denominator = rate, &progressed, &resolved
	if rate == nil && out.Coverage.Status != crmcontracts.ReportingStatusNoData {
		out.Coverage.Status = reportingInsufficientSample
	}
}

// cohortCounts lists one count per outcome, in order, zeros included, so every
// point of a chart has the same columns. Each count's evidence is its members.
func cohortCounts(frame crmcontracts.ReportingContext, chart crmcontracts.ReportingChart, facts []reporting.Fact, outcomes []string, stage string) []crmcontracts.ReportingCohortCount {
	said := reporting.ExplanationCopy(frame)
	out := make([]crmcontracts.ReportingCohortCount, 0, len(outcomes))
	for _, outcome := range outcomes {
		members := slices.DeleteFunc(slices.Clone(facts), func(fact reporting.Fact) bool { return fact.Outcome != outcome })
		basis := crmcontracts.ReportingCohortCountDurationBasisCompleted
		if outcome == cohortUnresolved {
			basis = crmcontracts.ReportingCohortCountDurationBasisCurrentAge
		}
		key := cohortOutcomeGroup(stage, outcome)
		out = append(out, crmcontracts.ReportingCohortCount{
			Key: outcome, Label: said.Say(cohortOutcomeWords[outcome]), Count: int64(len(members)),
			MedianDays: metricPercentile(members, .5), DurationBasis: &basis,
			Evidence: &crmcontracts.ReportingEvidenceRef{Metric: chart.Metric, ContextId: chart.ContextId, GroupKey: &key},
		})
	}
	return out
}

// cohortEntries counts how a stage point's members entered the stage.
func cohortEntries(frame crmcontracts.ReportingContext, facts []reporting.Fact) []crmcontracts.ReportingCohortCount {
	said := reporting.ExplanationCopy(frame)
	counts := map[string]int64{}
	for _, fact := range facts {
		counts[fact.Entry]++
		if fact.Reopened {
			counts[cohortEntryReopened]++
		}
	}
	out := []crmcontracts.ReportingCohortCount{}
	for _, entry := range []string{cohortEntryFirst, cohortEntryRepeat, cohortEntryReopened} {
		out = append(out, crmcontracts.ReportingCohortCount{Key: entry, Label: said.Say(cohortEntryWords[entry]), Count: counts[entry]})
	}
	return out
}

func cohortPoint(key, label string, facts []reporting.Fact) crmcontracts.ReportingPoint {
	rate, _, _ := cohortRate(facts)
	count := int64(len(facts))
	status := crmcontracts.ReportingStatus("ok")
	if rate == nil {
		status = reportingInsufficientSample
	}
	return crmcontracts.ReportingPoint{Key: key, Label: label, Value: rate, Observations: &count, Status: status}
}

// reportingStageCohortChart draws one point per entry stage of the chosen
// pipeline, in the pipeline's current order. The order only places the rows;
// each move's direction was settled from the order at the time of the move.
func reportingStageCohortChart(out *reporting.Evaluation, chart crmcontracts.ReportingChart) crmcontracts.ReportingChart {
	frame := out.Result.Context
	chart.ContextId = reportingInterval
	chart.Interval = &frame.Interval
	if frame.PipelineId == nil {
		chart.Coverage.Status = crmcontracts.ReportingStatusNotConfigured
		reason := reporting.ExplanationCopy(frame).Say(reportingCoverageWords.ChoosePipeline)
		chart.Coverage.Reason = &reason
		return chart
	}
	stages := map[string][]reporting.Fact{}
	for _, fact := range metricFacts(out.Facts, chart.Metric, chart.ContextId) {
		stages[fact.StageID] = append(stages[fact.StageID], fact)
	}
	keys := make([]string, 0, len(stages))
	for key := range stages {
		keys = append(keys, key)
	}
	slices.SortFunc(keys, func(a, b string) int {
		if order := cmp.Compare(stages[a][0].StagePosition, stages[b][0].StagePosition); order != 0 {
			return order
		}
		return cmp.Compare(a, b)
	})
	for _, stage := range keys {
		facts := stages[stage]
		point := cohortPoint(stage, facts[0].StageLabel, facts)
		group := "stage:" + stage
		point.Evidence = &crmcontracts.ReportingEvidenceRef{Metric: chart.Metric, ContextId: chart.ContextId, GroupKey: &group}
		outcomes := cohortCounts(frame, chart, facts, stageCohortOutcomes, stage)
		entries := cohortEntries(frame, facts)
		point.Outcomes, point.Entries = &outcomes, &entries
		chart.Points = append(chart.Points, point)
	}
	return chart
}

// reportingLeadCohortChart draws the interval's arrivals as one point.
func reportingLeadCohortChart(out *reporting.Evaluation, chart crmcontracts.ReportingChart) crmcontracts.ReportingChart {
	frame := out.Result.Context
	chart.ContextId = reportingInterval
	chart.Interval = &frame.Interval
	facts := metricFacts(out.Facts, chart.Metric, chart.ContextId)
	point := cohortPoint("arrivals", reporting.ExplanationCopy(frame).Say(cohortWords.Arrivals), facts)
	point.Evidence = &crmcontracts.ReportingEvidenceRef{Metric: chart.Metric, ContextId: chart.ContextId}
	outcomes := cohortCounts(frame, chart, facts, leadCohortOutcomes, "")
	point.Outcomes = &outcomes
	chart.Points = []crmcontracts.ReportingPoint{point}
	return chart
}
