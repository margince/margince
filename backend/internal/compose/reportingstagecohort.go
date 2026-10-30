// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

// The stage cohort, version 1 of stage_conversion. A member is a deal's
// earliest entry into a stage in the interval, by weekly.StageEntriesSQL. It is
// credited to the owner and pipeline the move recorded. Its outcome is its next
// move before the cutoff, read in the stage order that move recorded, never
// today's order. A deal with no next move is unresolved, never lost.

import (
	"context"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/compose/weekly"
	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/modules/reporting"
	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/langcopy"
)

const (
	reportingStageConversion = "stage_conversion"
	reportingStageCohort     = "stage_cohort"

	cohortAdvanced       = "advanced"
	cohortWon            = "won"
	cohortLost           = "lost"
	cohortRegressed      = "regressed"
	cohortLeftPipeline   = "left_pipeline"
	cohortUnresolved     = "unresolved"
	cohortHistoryPartial = "history_partial"

	cohortEntryFirst    = "first"
	cohortEntryRepeat   = "repeat"
	cohortEntryReopened = "reopened"

	stageCohortProvenance = "stage_history"
)

// stageCohortOutcomes is the order a stage point lists its outcomes in.
var stageCohortOutcomes = []string{cohortAdvanced, cohortWon, cohortLost, cohortRegressed, cohortLeftPipeline, cohortUnresolved, cohortHistoryPartial}

// stageExitSQL names the outcome of an entry from its next move, nx. Won and
// lost need only the meaning the move recorded; a direction needs both of its
// recorded positions, and without them the outcome is history_partial.
const stageExitSQL = `CASE WHEN nx.changed_at IS NULL THEN '` + cohortUnresolved + `'
	WHEN nx.semantic_at_change = 'won' THEN '` + cohortWon + `'
	WHEN nx.semantic_at_change = 'lost' THEN '` + cohortLost + `'
	WHEN nx.pipeline_id_at_change IS DISTINCT FROM e.pipeline_id THEN '` + cohortLeftPipeline + `'
	WHEN nx.to_position_at_change > nx.from_position_at_change THEN '` + cohortAdvanced + `'
	WHEN nx.to_position_at_change < nx.from_position_at_change THEN '` + cohortRegressed + `'
	ELSE '` + cohortHistoryPartial + `' END`

// cohortWindow is where a cohort's members are read: the interval, up to the
// cutoff. A deal counts once per stage, so the window cannot also span the
// previous period without moving an entry out of the interval's cohort.
func cohortWindow(frame crmcontracts.ReportingContext) (time.Time, time.Time) {
	end := frame.Interval.EndAt
	if frame.EvaluatedAt.Before(end) {
		end = frame.EvaluatedAt
	}
	return frame.Interval.StartAt, end
}

// stageMoveOrder is the order one deal's moves were written in. Two moves can
// share an instant, and the write sequence orders them; rows written before it
// was kept fall back to their id. alias is a compile-time literal.
func stageMoveOrder(alias string) string {
	return "(" + alias + ".changed_at,COALESCE(" + alias + ".write_seq,0)," + alias + ".id)"
}

// cohortOutcomeGroup is the evidence group of one outcome's members: within a
// stage for the stage cohort, or of the whole cohort when stage is empty. The
// fact and the chart's count both take their key from here.
func cohortOutcomeGroup(stage, outcome string) string {
	if stage == "" {
		return "outcome:" + outcome
	}
	return "stage:" + stage + ":outcome:" + outcome
}

func readReportingStageConversion(ctx context.Context, tx pgx.Tx, frame crmcontracts.ReportingContext, _ crmcontracts.ReportingFramework, _ metricEvaluator) ([]reporting.Fact, crmcontracts.ReportingCoverage, error) {
	// Stages belong to one pipeline. Without one the reading is refused before
	// any history is read.
	if frame.PipelineId == nil {
		return nil, reportingGap(crmcontracts.ReportingStatusNotConfigured, reporting.ExplanationCopy(frame).Say(reportingCoverageWords.ChoosePipeline)), nil
	}
	var b reportingBindings
	_, population, err := analyticsPopulationExpression(ctx, tx, reportingRequested(frame.Scope), "h.owner_id_at_change", b.arg, unownedIsExcluded)
	if err != nil {
		return nil, crmcontracts.ReportingCoverage{}, err
	}
	rowScope, err := auth.ScopeClauseFor(ctx, string(recordTypeDeal), "d", b.arg)
	if err != nil {
		return nil, crmcontracts.ReportingCoverage{}, err
	}
	narrowing := orUnnarrowed(population) + " AND " + orUnnarrowed(rowScope) + " AND d.archived_at IS NULL AND h.pipeline_id_at_change = " + b.add(*frame.PipelineId)
	fieldScope, withheld, err := reportingFieldScope(ctx, string(recordTypeDeal), "t", reportingDealFields(reportingStageConversion), &b)
	if err != nil {
		return nil, crmcontracts.ReportingCoverage{}, err
	}
	start, end := cohortWindow(frame)
	cutoff := b.add(frame.EvaluatedAt)
	said := reporting.ExplanationCopy(frame)
	query := `SELECT e.id::text,t.id,t.name,e.owner_id,COALESCE(u.display_name,` + b.add(said.Say(reportingLabelWords.Unassigned)) + `),e.changed_at,
 (EXTRACT(EPOCH FROM (COALESCE(nx.changed_at,` + cutoff + `::timestamptz)-e.changed_at))/86400)::float8,
 e.stage_id::text,COALESCE(s.name,` + b.add(said.Say(reportingLabelWords.Unmapped)) + `),` + stageExitSQL + `,'` + stageCohortProvenance + `',COALESCE(s."position",0),
 '','',
 EXISTS(SELECT 1 FROM deal_stage_history r WHERE r.deal_id=e.deal_id AND r.to_stage_id=e.stage_id AND ` + stageMoveOrder("r") + `<` + stageMoveOrder("eh") + `),
 COALESCE(pv.semantic_at_change IN ('won','lost'),false)
 FROM (` + weekly.StageEntriesSQL(b.add(start), b.add(end), narrowing) + `) e
 JOIN deal_stage_history eh ON eh.id=e.id
 JOIN deal t ON t.id=e.deal_id
 LEFT JOIN stage s ON s.id=e.stage_id
 LEFT JOIN app_user u ON u.id=e.owner_id
 LEFT JOIN LATERAL (SELECT x.changed_at,x.semantic_at_change,x.pipeline_id_at_change,x.from_position_at_change,x.to_position_at_change
   FROM deal_stage_history x WHERE x.deal_id=e.deal_id AND ` + stageMoveOrder("x") + `>` + stageMoveOrder("eh") + ` AND x.changed_at<` + cutoff + `
   ORDER BY x.changed_at,COALESCE(x.write_seq,0),x.id LIMIT 1) nx ON true
 LEFT JOIN LATERAL (SELECT p.semantic_at_change FROM deal_stage_history p
   WHERE p.deal_id=e.deal_id AND ` + stageMoveOrder("p") + `<` + stageMoveOrder("eh") + ` ORDER BY p.changed_at DESC,COALESCE(p.write_seq,0) DESC,p.id DESC LIMIT 1) pv ON true
 WHERE ` + fieldScope + ` ORDER BY e.id LIMIT ` + b.add(reportingFactLimit+1)
	facts, err := scanStageCohortFacts(ctx, tx, query, b, said)
	if err != nil {
		return nil, crmcontracts.ReportingCoverage{}, err
	}
	coverage := crmcontracts.ReportingCoverage{Status: "ok", Withheld: withheld}
	for _, fact := range facts {
		if fact.Outcome == cohortHistoryPartial {
			coverage = reportingGap(reportingPartial, said.Say(cohortWords.StageOrderUnknown))
			coverage.Withheld = withheld
			break
		}
	}
	return reportingCohorts(frame, facts), coverage, nil
}

func scanStageCohortFacts(ctx context.Context, tx pgx.Tx, query string, b reportingBindings, said langcopy.Spoken) ([]reporting.Fact, error) {
	rows, err := tx.Query(ctx, query, b.values...)
	if err != nil {
		return nil, err
	}
	facts, err := pgx.CollectRows(rows, func(row pgx.CollectableRow) (reporting.Fact, error) {
		var repeat, reopened bool
		fact, err := scanDealMetricFact(row, reportingStageConversion, false, &repeat, &reopened)
		fact.Entry, fact.Reopened = cohortEntryFirst, reopened
		if repeat {
			fact.Entry = cohortEntryRepeat
		}
		fact.GroupKey = cohortOutcomeGroup(fact.StageID, fact.Outcome)
		fact.GroupLabel = said.Say(cohortOutcomeWords[fact.Outcome])
		return fact, err
	})
	if err != nil {
		return nil, err
	}
	if len(facts) > reportingFactLimit {
		return nil, fmt.Errorf("narrow the reporting population before evaluating: %w", apperrors.ErrInvalidArgument)
	}
	return facts, nil
}
