// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

// The two views of the lead entry cohort besides the leads themselves: the
// companies those leads belong to, and the sources they came from. Both read
// the cohort leadEntryCohortSQL admits, so one lead is in all three or none.

import (
	"cmp"
	"context"
	"slices"
	"strings"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/compose/analyticsquery"
	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/modules/reporting"
	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/shared/kernel/langcopy"
)

const (
	reportingAccountCohort      = "account_cohort"
	reportingSourceQualityBlock = "source_quality"

	sourceGroupPrefix = "source:"
	sourceLinked      = "opportunity_linked"
	sourceNotLinked   = "not_linked"
)

// leadEntryCohortMetrics narrow their scope by the owner a lead arrived with.
var leadEntryCohortMetrics = []crmcontracts.ReportingMetricID{reportingLeadCohortOutcomes, reportingAccountProgression, reportingSourceQuality}

var entryCohortWords = struct{ Companies, CompanyAccess, Linked, NotLinked langcopy.Phrase }{
	Companies:     langcopy.Phrase{En: "Companies that entered", De: "Eingegangene Unternehmen", Vi: "Công ty đã vào"},
	CompanyAccess: langcopy.Phrase{En: "Companies are counted only for readers who may see companies", De: "Unternehmen werden nur gezählt, wenn du Unternehmen sehen darfst.", Vi: "Chỉ đếm công ty khi bạn được phép xem công ty."},
	Linked:        langcopy.Phrase{En: "Linked to a deal", De: "Mit einem Deal verknüpft", Vi: "Đã gắn với một deal"},
	NotLinked:     langcopy.Phrase{En: "Not linked yet", De: "Noch nicht verknüpft", Vi: "Chưa gắn với một deal"},
}

// readReportingAccountProgression reads each company of the entry cohort's leads
// once, under the furthest outcome any of those leads reached. The order is the
// one the lead cohort lists its outcomes in. A company counts only for a reader
// who may read it and its name.
func readReportingAccountProgression(ctx context.Context, tx pgx.Tx, frame crmcontracts.ReportingContext, _ crmcontracts.ReportingFramework, _ metricEvaluator) ([]reporting.Fact, crmcontracts.ReportingCoverage, error) {
	if frame.PipelineId != nil {
		return nil, noPipelineGap(frame), nil
	}
	said := reporting.ExplanationCopy(frame)
	if !auth.ReadGranted(ctx, string(recordTypeCompany)) {
		gap := reportingGap(reportingUnavailable, said.Say(entryCohortWords.CompanyAccess))
		gap.Withheld = true
		return nil, gap, nil
	}
	var b reportingBindings
	ctes, cutoff, leadWithheld, err := leadEntryCohortSQL(ctx, tx, frame, reportingAccountProgression, &b)
	if err != nil {
		return nil, crmcontracts.ReportingCoverage{}, err
	}
	rowScope, err := auth.ScopeClauseFor(ctx, string(recordTypeCompany), "c", b.arg)
	if err != nil {
		return nil, crmcontracts.ReportingCoverage{}, err
	}
	companyFields, companyWithheld, err := reportingFieldScope(ctx, string(recordTypeCompany), "c", []string{companyDisplayName}, &b)
	if err != nil {
		return nil, crmcontracts.ReportingCoverage{}, err
	}
	outcome, reachedAt := leadReachedSQL(cutoff)
	rank := "array_position(ARRAY['" + strings.Join(leadCohortOutcomes, "','") + "'], " + outcome + ")"
	query := ctes + `, reached AS (
 SELECT l.company_id,l.arrival_owner_id,l.id AS lead_id,` + leadCohortProvenanceSQL + ` AS provenance,` + outcome + ` AS outcome,
 ` + rank + ` AS rank,` + leadArrivalSQL + ` AS arrived_at,` + reachedAt + ` AS reached_at
 FROM lead l JOIN arrived ON arrived.id=l.id ` + leadOutcomeJoinsSQL(cutoff) + `
 WHERE l.company_id IS NOT NULL),
 best AS (SELECT DISTINCT ON (company_id) * FROM reached ORDER BY company_id,rank,reached_at,lead_id)
 SELECT c.id::text,c.id,c.display_name,best.arrival_owner_id,COALESCE(u.display_name,''),best.arrived_at,
 (EXTRACT(EPOCH FROM (best.reached_at-best.arrived_at))/86400)::float8,best.provenance,best.outcome,'',best.lead_id
 FROM best JOIN company c ON c.id=best.company_id LEFT JOIN app_user u ON u.id=best.arrival_owner_id
 WHERE c.archived_at IS NULL AND ` + orUnnarrowed(rowScope) + ` AND ` + companyFields + `
 ORDER BY c.id LIMIT ` + b.add(reportingFactLimit+1)
	withheld := leadWithheld || companyWithheld
	facts, coverage, err := readGroupedFactsVia(ctx, tx, frame, reportingAccountProgression, string(recordTypeCompany), query, b, withheld, withLeadVia)
	if err != nil {
		return nil, coverage, err
	}
	facts, coverage = finishCohortFacts(facts, coverage, said, withheld)
	return facts, coverage, nil
}

// readReportingSourceQuality reads each lead of the entry cohort under its
// origin, valued 1 once it is linked to a deal by the evaluation instant.
func readReportingSourceQuality(ctx context.Context, tx pgx.Tx, frame crmcontracts.ReportingContext, _ crmcontracts.ReportingFramework, _ metricEvaluator) ([]reporting.Fact, crmcontracts.ReportingCoverage, error) {
	if frame.PipelineId != nil {
		return nil, noPipelineGap(frame), nil
	}
	var b reportingBindings
	said := reporting.ExplanationCopy(frame)
	ctes, cutoff, withheld, err := leadEntryCohortSQL(ctx, tx, frame, reportingSourceQuality, &b)
	if err != nil {
		return nil, crmcontracts.ReportingCoverage{}, err
	}
	label, err := leadLabelSQL(ctx, &b, b.add(said.Say(reportingBreakdownLabels.Lead)))
	if err != nil {
		return nil, crmcontracts.ReportingCoverage{}, err
	}
	linked := leadQualifiedBySQL(cutoff)
	query := ctes + `
 SELECT l.id::text,l.id,` + label + `,l.arrival_owner_id,COALESCE(u.display_name,''),` + leadArrivalSQL + `,
 CASE WHEN ` + linked + ` THEN 1 ELSE 0 END::float8,` + leadCohortProvenanceSQL + `,
 '` + sourceGroupPrefix + `'||replace(replace(COALESCE(l.origin,''),'%','%25'),':','%3A')||CASE WHEN ` + linked + ` THEN ':` + sourceLinked + `' ELSE ':` + sourceNotLinked + `' END,
 COALESCE(lo.label,` + b.add(said.Say(reportingBreakdownLabels.NoOrigin)) + `)
 FROM lead l JOIN arrived ON arrived.id=l.id LEFT JOIN lead_origin lo ON lo.key=l.origin LEFT JOIN app_user u ON u.id=l.arrival_owner_id
 ORDER BY l.id LIMIT ` + b.add(reportingFactLimit+1)
	facts, coverage, err := readGroupedFacts(ctx, tx, frame, reportingSourceQuality, string(recordTypeLead), query, b, withheld)
	if err != nil {
		return nil, coverage, err
	}
	for _, fact := range facts {
		if fact.Provenance == leadCohortOwnerUnrecorded {
			coverage = reportingGap(reportingPartial, said.Say(cohortWords.ArrivalOwnerUnknown))
			coverage.Withheld = withheld
			break
		}
	}
	return facts, coverage, nil
}

// reportingAccountCohortChart draws the entry cohort's companies as one point.
func reportingAccountCohortChart(out *reporting.Evaluation, chart crmcontracts.ReportingChart) crmcontracts.ReportingChart {
	frame := out.Result.Context
	chart.ContextId = reportingInterval
	chart.Interval = &frame.Interval
	facts := metricFacts(out.Facts, chart.Metric, chart.ContextId)
	point := cohortPoint("companies", reporting.ExplanationCopy(frame).Say(entryCohortWords.Companies), facts)
	point.Evidence = &crmcontracts.ReportingEvidenceRef{Metric: chart.Metric, ContextId: chart.ContextId}
	outcomes := cohortCounts(frame, chart, facts, leadCohortOutcomes, "")
	point.Outcomes = &outcomes
	chart.Points = []crmcontracts.ReportingPoint{point}
	return chart
}

// reportingSourceQualityChart draws one point per origin: its entrants, and how
// many of them are linked to an opportunity. The point's rate is that share.
func reportingSourceQualityChart(out *reporting.Evaluation, chart crmcontracts.ReportingChart) crmcontracts.ReportingChart {
	frame := out.Result.Context
	said := reporting.ExplanationCopy(frame)
	chart.ContextId = reportingInterval
	chart.Interval = &frame.Interval
	// The metric counts linked leads; each point's value is a share of entrants.
	chart.Unit = "percent"
	sources := map[string][]reporting.Fact{}
	for _, fact := range metricFacts(out.Facts, chart.Metric, chart.ContextId) {
		source, _ := sourceOfGroup(fact.GroupKey)
		sources[source] = append(sources[source], fact)
	}
	for source, facts := range sources {
		entrants := int64(len(facts))
		linkedCount := int64(0)
		for _, fact := range facts {
			if _, linked := sourceOfGroup(fact.GroupKey); linked {
				linkedCount++
			}
		}
		count := func(key string, n int64, label langcopy.Phrase) crmcontracts.ReportingCohortCount {
			group := source + ":" + key
			return crmcontracts.ReportingCohortCount{
				Key: key, Label: said.Say(label), Count: n,
				Evidence: &crmcontracts.ReportingEvidenceRef{Metric: chart.Metric, ContextId: chart.ContextId, GroupKey: &group},
			}
		}
		outcomes := []crmcontracts.ReportingCohortCount{
			count(sourceLinked, linkedCount, entryCohortWords.Linked),
			count(sourceNotLinked, entrants-linkedCount, entryCohortWords.NotLinked),
		}
		point := crmcontracts.ReportingPoint{
			Key: source, Label: facts[0].GroupLabel, Observations: &entrants, Outcomes: &outcomes,
			Status:   crmcontracts.ReportingStatus("ok"),
			Evidence: &crmcontracts.ReportingEvidenceRef{Metric: chart.Metric, ContextId: chart.ContextId, GroupKey: &source},
		}
		if entrants >= analyticsquery.PercentileSampleFloor {
			rate := float64(linkedCount) / float64(entrants) * 100
			point.Value = &rate
		} else {
			point.Status = reportingInsufficientSample
		}
		chart.Points = append(chart.Points, point)
	}
	slices.SortFunc(chart.Points, func(a, b crmcontracts.ReportingPoint) int {
		return cmp.Or(cmp.Compare(*b.Observations, *a.Observations), cmp.Compare(a.Key, b.Key))
	})
	return chart
}

// sourceOfGroup splits a source quality fact's group into the evidence group of
// its origin and whether the lead is linked to an opportunity.
func sourceOfGroup(group string) (source string, linked bool) {
	if source, linked = strings.CutSuffix(group, ":"+sourceLinked); linked {
		return source, true
	}
	source, _ = strings.CutSuffix(group, ":"+sourceNotLinked)
	return source, false
}
