// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

// The lead cohort, version 1 of lead_cohort_outcomes: the leads
// leadArrivedBetweenSQL admits in the interval, credited to the owner each arrived with. A lead's
// outcome is the furthest it reached by the cutoff. That is an accepted
// handoff, then Qualified, then Disqualified, as weekly.LeadMovesSQL counts
// moves. A lead that reached none is still open, never lost.

import (
	"context"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/compose/weekly"
	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/modules/reporting"
	"github.com/margince/margince/backend/internal/shared/kernel/langcopy"
)

const (
	reportingLeadCohortOutcomes = "lead_cohort_outcomes"
	reportingLeadCohort         = "lead_cohort"

	cohortAccepted     = "accepted"
	cohortQualified    = "qualified"
	cohortDisqualified = "disqualified"

	leadArrivalOwnerColumn = "l.arrival_owner_id"

	leadCohortProvenance      = "lead_arrival"
	leadCohortOwnerUnrecorded = "lead_arrival_owner_unrecorded"

	leadCohortProvenanceSQL = "CASE WHEN l.arrival_owner_id IS NULL AND l.owner_id IS NOT NULL THEN '" + leadCohortOwnerUnrecorded + "' ELSE '" + leadCohortProvenance + "' END"
)

// leadCohortOutcomes is the order the lead cohort point lists its outcomes in.
var leadCohortOutcomes = []string{cohortAccepted, cohortQualified, cohortDisqualified, cohortUnresolved}

// leadReachedSQL is the furthest outcome a lead reached, with the instant it
// reached it as reachedAt, from the accepted handoff (acc) and the moves (mv).
func leadReachedSQL(cutoff string) (outcome, reachedAt string) {
	outcome = `CASE WHEN acc.at IS NOT NULL THEN '` + cohortAccepted + `'
	WHEN mv.promoted IS NOT NULL THEN '` + cohortQualified + `'
	WHEN mv.disqualified IS NOT NULL THEN '` + cohortDisqualified + `'
	ELSE '` + cohortUnresolved + `' END`
	reachedAt = `CASE WHEN acc.at IS NOT NULL THEN acc.at WHEN mv.promoted IS NOT NULL THEN mv.promoted
	WHEN mv.disqualified IS NOT NULL THEN mv.disqualified ELSE ` + cutoff + `::timestamptz END`
	return outcome, reachedAt
}

func readReportingLeadCohort(ctx context.Context, tx pgx.Tx, frame crmcontracts.ReportingContext, _ crmcontracts.ReportingFramework, _ metricEvaluator) ([]reporting.Fact, crmcontracts.ReportingCoverage, error) {
	if frame.PipelineId != nil {
		return nil, noPipelineGap(frame), nil
	}
	var b reportingBindings
	said := reporting.ExplanationCopy(frame)
	ctes, cutoff, withheld, err := leadEntryCohortSQL(ctx, tx, frame, reportingLeadCohortOutcomes, &b)
	if err != nil {
		return nil, crmcontracts.ReportingCoverage{}, err
	}
	label, err := leadLabelSQL(ctx, &b, b.add(said.Say(reportingBreakdownLabels.Lead)))
	if err != nil {
		return nil, crmcontracts.ReportingCoverage{}, err
	}
	outcome, reachedAt := leadReachedSQL(cutoff)
	query := ctes + `
 SELECT l.id::text,l.id,` + label + `,l.arrival_owner_id,COALESCE(u.display_name,''),` + leadArrivalSQL + `,
 (EXTRACT(EPOCH FROM (` + reachedAt + `-` + leadArrivalSQL + `))/86400)::float8,
 ` + leadCohortProvenanceSQL + `,
 ` + outcome + `,''
 FROM lead l JOIN arrived ON arrived.id=l.id
 LEFT JOIN app_user u ON u.id=l.arrival_owner_id
 ` + leadOutcomeJoinsSQL(cutoff) + `
 ORDER BY l.id LIMIT ` + b.add(reportingFactLimit+1)
	facts, coverage, err := readGroupedFacts(ctx, tx, frame, reportingLeadCohortOutcomes, string(recordTypeLead), query, b, withheld)
	if err != nil {
		return nil, coverage, err
	}
	facts, coverage = finishCohortFacts(facts, coverage, said, withheld)
	return facts, coverage, nil
}

// leadEntryCohortSQL is the lead entry cohort every cohort reader follows. It is
// the leads that arrived in the interval, narrowed by the reader's scope, as the
// arrived CTE, with the moves CTE over them. Outcomes are followed to the
// evaluation instant, bound as cutoff.
func leadEntryCohortSQL(ctx context.Context, tx pgx.Tx, frame crmcontracts.ReportingContext, metric crmcontracts.ReportingMetricID, b *reportingBindings) (ctes, cutoff string, withheld bool, err error) {
	scope, withheld, err := leadMetricScope(ctx, tx, frame, metric, b)
	if err != nil {
		return "", "", false, err
	}
	start, end := cohortWindow(frame)
	from := b.add(start)
	cutoff = b.add(frame.EvaluatedAt)
	arrived := "SELECT l.id FROM lead l WHERE " + leadArrivedBetweenSQL(from, b.add(end)) + " AND " + scope
	return "WITH arrived AS (" + arrived + "), moves AS (" + weekly.LeadMovesSQL("arrived", from, cutoff) + ")", cutoff, withheld, nil
}

// finishCohortFacts moves each fact's outcome out of its group column and into
// its own evidence group. A lead that arrived before the arrival owner was kept
// makes the coverage partial.
func finishCohortFacts(facts []reporting.Fact, coverage crmcontracts.ReportingCoverage, said langcopy.Spoken, withheld bool) ([]reporting.Fact, crmcontracts.ReportingCoverage) {
	unrecorded := false
	for i := range facts {
		facts[i].Outcome = facts[i].GroupKey
		facts[i].GroupKey = cohortOutcomeGroup("", facts[i].Outcome)
		facts[i].GroupLabel = said.Say(cohortOutcomeWords[facts[i].Outcome])
		unrecorded = unrecorded || facts[i].Provenance == leadCohortOwnerUnrecorded
	}
	if unrecorded {
		coverage = reportingGap(reportingPartial, said.Say(cohortWords.ArrivalOwnerUnknown))
		coverage.Withheld = withheld
	}
	return facts, coverage
}

// leadOutcomeJoinsSQL joins what a lead reached by the cutoff. That is its first
// qualification or disqualification (mv) and its accepted handoff (acc), over the
// moves CTE and the lead alias l.
func leadOutcomeJoinsSQL(cutoff string) string {
	return `LEFT JOIN LATERAL (SELECT min(m.occurred_at) FILTER (WHERE m.move='` + weekly.LeadMovePromoted + `') AS promoted,
   min(m.occurred_at) FILTER (WHERE m.move='` + weekly.LeadMoveDisqualified + `') AS disqualified
   FROM moves m WHERE m.lead_id=l.id AND m.occurred_at>=` + leadArrivalSQL + `) mv ON true
 LEFT JOIN LATERAL (SELECT min(e.occurred_at) AS at FROM sdr_handoff h JOIN sdr_handoff_event e ON e.handoff_id=h.id
   WHERE h.lead_id=l.id AND e.to_status='accepted' AND e.occurred_at>=` + leadArrivalSQL + ` AND e.occurred_at<` + cutoff + `) acc ON true`
}
