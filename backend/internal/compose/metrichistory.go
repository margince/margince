// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

import (
	"context"
	"time"

	"github.com/jackc/pgx/v5"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/platform/auth"
)

func reportingHistoryCoverage(ctx context.Context, tx pgx.Tx, frame crmcontracts.ReportingContext, metric crmcontracts.ReportingMetricID, coverage crmcontracts.ReportingCoverage) (crmcontracts.ReportingCoverage, error) {
	if coverage.Status != "ok" && coverage.Status != "no_data" {
		return coverage, nil
	}
	coverage, stop, err := qualificationCoverage(ctx, tx, frame, metric, coverage)
	if err != nil || stop {
		return coverage, err
	}
	var b reportingBindings
	object, alias, from, owner, missing := string(recordTypeDeal), "t", "deal t", "t.owner_id", ""
	switch metric {
	case reportingBookingsWon, reportingClosedWinRate, reportingQualifiedPipelineCreated:
		missing = "EXISTS(SELECT 1 FROM deal_stage_history h WHERE h.deal_id=t.id AND h.changed_at>=" + b.add(reportingEarliest(frame)) + " AND h.changed_at<" + b.add(frame.EvaluatedAt) + " AND (h.owner_id_at_change IS NULL OR h.pipeline_id_at_change IS NULL))"
	case reportingMeetingsHeld:
		object, alias, from, owner = string(recordTypeActivity), "t", "activity t", "t.host_user_id"
		missing = "t.kind='meeting' AND EXISTS(SELECT 1 FROM activity_meeting_history h WHERE h.activity_id=t.id AND h.scheduled_start>=" + b.add(reportingEarliest(frame)) + " AND h.scheduled_start<" + b.add(frame.EvaluatedAt) + " AND (h.host_id_at_change IS NULL OR h.customer_eligible_at_change IS NULL OR h.partial_pre_history))"
	case reportingAcceptedOpportunities:
		object, alias, from, owner = reportingCreditObject, "t", "sdr_handoff t", "t.submitted_by"
		missing = "EXISTS(SELECT 1 FROM sdr_handoff_event h WHERE h.handoff_id=t.id AND h.to_status='accepted' AND h.occurred_at>=" + b.add(reportingEarliest(frame)) + " AND h.occurred_at<" + b.add(frame.EvaluatedAt) + " AND (h.submitter_id_at_change IS NULL OR h.deal_id_at_change IS NULL))"
	default:
		return coverage, nil
	}
	_, population, err := analyticsPopulationExpression(ctx, tx, reportingRequested(frame.Scope), owner, b.arg, unownedIsExcluded)
	if err != nil {
		return coverage, err
	}
	if population != "" {
		missing += " AND " + population
	}
	clauses, err := reportingHistoryScope(ctx, object, alias, &b)
	if err != nil {
		return coverage, err
	}
	missing += clauses
	var incomplete bool
	if err := tx.QueryRow(ctx, "SELECT EXISTS(SELECT 1 FROM "+from+" WHERE "+missing+")", b.values...).Scan(&incomplete); err != nil {
		return coverage, err
	}
	if incomplete {
		withheld := coverage.Withheld
		coverage = reportingGap(reportingPartial, "Some source history predates reliable attribution. Only known contributions are shown; this period is not a complete comparison baseline.")
		coverage.Withheld = withheld
	}
	return coverage, nil
}

func reportingHasQualification(frame crmcontracts.ReportingContext, framework crmcontracts.ReportingFramework) bool {
	for _, mapping := range framework.Definition.Qualification {
		if frame.PipelineId == nil || mapping.PipelineId == *frame.PipelineId {
			return len(mapping.StageIds) > 0
		}
	}
	return false
}

func qualificationHistoryStart(ctx context.Context, tx pgx.Tx, frame crmcontracts.ReportingContext) (*time.Time, error) {
	var b reportingBindings
	query := "SELECT max(start_at) FROM (SELECT q->>'pipeline_id',min(f.effective_at) AS start_at FROM reporting_framework_revision f CROSS JOIN LATERAL jsonb_array_elements(f.definition->'qualification') q WHERE f.revision<=" + b.add(frame.FrameworkRevision) + " AND jsonb_array_length(q->'stage_ids')>0"
	if frame.PipelineId != nil {
		query += " AND q->>'pipeline_id'=" + b.add(frame.PipelineId.String())
	}
	query += " GROUP BY q->>'pipeline_id') starts"

	var start *time.Time
	err := tx.QueryRow(ctx, query, b.values...).Scan(&start)
	return start, err
}

func reportingHistoryScope(ctx context.Context, object, alias string, b *reportingBindings) (string, error) {
	clauses := ""
	if object == string(recordTypeDeal) {
		clause, err := auth.ScopeClauseFor(ctx, object, alias, b.arg)
		if err != nil {
			return "", err
		}
		if clause != "" {
			clauses += " AND " + clause
		}
	}
	if object == string(recordTypeActivity) {
		clause, err := auth.ActivityContentClause(ctx, alias, b.arg)
		if err != nil {
			return "", err
		}
		if clause != "" {
			clauses += " AND " + clause
		}
	}
	return clauses, nil
}

func qualificationCoverage(ctx context.Context, tx pgx.Tx, frame crmcontracts.ReportingContext, metric crmcontracts.ReportingMetricID, coverage crmcontracts.ReportingCoverage) (crmcontracts.ReportingCoverage, bool, error) {
	if metric == reportingQualifiedPipelineCreated {
		known, err := qualificationHistoryStart(ctx, tx, frame)
		if err != nil {
			return coverage, true, err
		}
		if known == nil || reportingEarliest(frame).Before(*known) {
			coverage.Status = reportingPartial
			reason := "Qualification tracking began after part of this period. Only transitions recorded under a configured stage framework are included."
			coverage.Reason = &reason
			return coverage, true, nil
		}
	}
	return coverage, false, nil
}
