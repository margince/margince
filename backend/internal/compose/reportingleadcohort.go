// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

// The lead cohort, version 1 of lead_cohort_outcomes: the leads leadArrivedSQL
// admits in the interval, credited to the owner each arrived with. A lead's
// outcome is the furthest it reached by the cutoff. That is an accepted
// handoff, then Qualified, then Disqualified, as weekly.LeadMovesSQL counts
// moves. A lead that reached none is still open, never lost.

import (
	"context"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/compose/weekly"
	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/modules/reporting"
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
	scope, withheld, err := leadMetricScope(ctx, tx, frame, reportingLeadCohortOutcomes, &b)
	if err != nil {
		return nil, crmcontracts.ReportingCoverage{}, err
	}
	said := reporting.ExplanationCopy(frame)
	cutoff := b.add(frame.EvaluatedAt)
	arrived := "SELECT l.id FROM lead l WHERE " + leadArrivedSQL(frame, &b, cutoff) + " AND " + scope
	moves := weekly.LeadMovesSQL("arrived", b.add(reportingEarliest(frame)), cutoff)
	label, err := leadLabelSQL(ctx, &b, b.add(said.Say(reportingBreakdownLabels.Lead)))
	if err != nil {
		return nil, crmcontracts.ReportingCoverage{}, err
	}
	outcome, reachedAt := leadReachedSQL(cutoff)
	query := `WITH arrived AS (` + arrived + `), moves AS (` + moves + `)
 SELECT l.id::text,l.id,` + label + `,l.arrival_owner_id,COALESCE(u.display_name,''),` + leadArrivalSQL + `,
 (EXTRACT(EPOCH FROM (` + reachedAt + `-` + leadArrivalSQL + `))/86400)::float8,
 CASE WHEN l.arrival_owner_id IS NULL AND l.owner_id IS NOT NULL THEN '` + leadCohortOwnerUnrecorded + `' ELSE '` + leadCohortProvenance + `' END,
 ` + outcome + `,''
 FROM lead l JOIN arrived ON arrived.id=l.id
 LEFT JOIN app_user u ON u.id=l.arrival_owner_id
 LEFT JOIN LATERAL (SELECT min(m.occurred_at) FILTER (WHERE m.move='` + weekly.LeadMovePromoted + `') AS promoted,
   min(m.occurred_at) FILTER (WHERE m.move='` + weekly.LeadMoveDisqualified + `') AS disqualified
   FROM moves m WHERE m.lead_id=l.id AND m.occurred_at>=` + leadArrivalSQL + `) mv ON true
 LEFT JOIN LATERAL (SELECT min(e.occurred_at) AS at FROM sdr_handoff h JOIN sdr_handoff_event e ON e.handoff_id=h.id
   WHERE h.lead_id=l.id AND e.to_status='accepted' AND e.occurred_at>=` + leadArrivalSQL + ` AND e.occurred_at<` + cutoff + `) acc ON true
 ORDER BY l.id LIMIT ` + b.add(reportingFactLimit+1)
	facts, coverage, err := readGroupedFacts(ctx, tx, frame, reportingLeadCohortOutcomes, string(recordTypeLead), query, b, withheld)
	if err != nil {
		return nil, coverage, err
	}
	unrecorded := false
	for i := range facts {
		facts[i].Outcome = facts[i].GroupKey
		facts[i].GroupLabel = said.Say(cohortOutcomeWords[facts[i].Outcome])
		unrecorded = unrecorded || facts[i].Provenance == leadCohortOwnerUnrecorded
	}
	if unrecorded {
		coverage = reportingGap(reportingPartial, said.Say(cohortWords.ArrivalOwnerUnknown))
		coverage.Withheld = withheld
	}
	return facts, coverage, nil
}
