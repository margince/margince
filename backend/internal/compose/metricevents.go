// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/modules/reporting"
	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

func readReportingQualified(ctx context.Context, tx pgx.Tx, frame crmcontracts.ReportingContext, framework crmcontracts.ReportingFramework, _ metricEvaluator) ([]reporting.Fact, crmcontracts.ReportingCoverage, error) {
	if !reportingHasQualification(frame, framework) {
		return nil, reportingGap("not_configured", "Choose qualifying stages in Reporting setup"), nil
	}
	// Revisions apply prospectively; an explicit reversal voids its event,
	// whereas an ordinary regression leaves the first qualifying entry intact.
	from := `JOIN LATERAL (SELECT h.* FROM deal_stage_history h
 JOIN LATERAL (SELECT definition FROM reporting_framework_revision f WHERE f.revision<=@revision AND f.effective_at<=h.changed_at ORDER BY f.revision DESC LIMIT 1) f ON true
 WHERE h.deal_id=t.id AND h.reversal_of IS NULL
 AND NOT EXISTS(SELECT 1 FROM deal_stage_history reversal WHERE reversal.reversal_of=h.id)
 AND EXISTS(SELECT 1 FROM jsonb_array_elements(f.definition->'qualification') q
 WHERE q->>'pipeline_id'=h.pipeline_id_at_change::text AND (q->'stage_ids') ? h.to_stage_id::text)
 ORDER BY h.changed_at,h.id LIMIT 1) qualified ON true`
	query := metricFactQuery{pipeline: "qualified.pipeline_id_at_change", revision: framework.Revision, from: from, where: "t.archived_at IS NULL", owner: "qualified.owner_id_at_change", at: "qualified.changed_at", key: "qualified.id::text", stage: "qualified.to_stage_id::text", stageLabel: "''", outcome: "'qualified'", provenance: "COALESCE(qualified.valuation_provenance,'legacy_unavailable')", value: "qualified.base_minor_at_change", money: true}
	return readDealMetricFacts(ctx, tx, frame, reportingQualifiedPipelineCreated, query)
}

func reportingGap(status crmcontracts.ReportingStatus, reason string) crmcontracts.ReportingCoverage {
	return crmcontracts.ReportingCoverage{Status: status, Reason: &reason}
}

func readReportingMeetings(ctx context.Context, tx pgx.Tx, frame crmcontracts.ReportingContext, _ crmcontracts.ReportingFramework, _ metricEvaluator) ([]reporting.Fact, crmcontracts.ReportingCoverage, error) {
	if frame.PipelineId != nil {
		return nil, reportingGap("unsupported", "Meetings have no pipeline dimension; choose all pipelines"), nil
	}
	var b reportingBindings
	_, population, err := analyticsPopulationExpression(ctx, tx, reportingRequested(frame.Scope), "held.host_id_at_change", b.arg, unownedIsExcluded)
	if err != nil {
		return nil, crmcontracts.ReportingCoverage{}, err
	}
	if population == "" {
		population = sqlUnnarrowed
	}
	visible, err := auth.ActivityContentClause(ctx, "a", b.arg)
	if err != nil {
		return nil, crmcontracts.ReportingCoverage{}, err
	}
	if visible == "" {
		visible = sqlUnnarrowed
	}
	fieldScope, withheld, err := reportingFieldScope(ctx, string(recordTypeActivity), "a", reportingMeetingFields(), &b)
	if err != nil {
		return nil, crmcontracts.ReportingCoverage{}, err
	}
	visible += " AND " + fieldScope
	query := `SELECT held.id::text,a.id,COALESCE(a.subject,'Meeting'),held.host_id_at_change,COALESCE(u.display_name,''),held.scheduled_start
 FROM activity a JOIN LATERAL(SELECT h.* FROM activity_meeting_history h WHERE h.activity_id=a.id AND h.effective_at<=` + b.add(frame.EvaluatedAt) + ` ORDER BY h.effective_at DESC,h.id DESC LIMIT 1) held ON true
 LEFT JOIN app_user u ON u.id=held.host_id_at_change
 WHERE a.kind='meeting' AND a.archived_at IS NULL AND held.status='held' AND held.customer_eligible_at_change AND NOT held.partial_pre_history
 AND held.scheduled_start >= ` + b.add(reportingEarliest(frame)) + " AND held.scheduled_start < " + b.add(frame.EvaluatedAt) + " AND " + population + " AND " + visible + " ORDER BY held.id LIMIT " + b.add(reportingFactLimit+1)
	facts, coverage, err := readOutcomeFacts(ctx, tx, frame, reportingMeetingsHeld, string(recordTypeActivity), query, b)
	coverage.Withheld = withheld
	return facts, coverage, err
}

func readReportingAccepted(ctx context.Context, tx pgx.Tx, frame crmcontracts.ReportingContext, _ crmcontracts.ReportingFramework, _ metricEvaluator) ([]reporting.Fact, crmcontracts.ReportingCoverage, error) {
	if frame.PipelineId != nil {
		return nil, reportingGap("unsupported", "Acceptance credit has no historical pipeline dimension; choose all pipelines"), nil
	}
	var b reportingBindings
	_, population, err := analyticsPopulationExpression(ctx, tx, reportingRequested(frame.Scope), "credit.submitter_id_at_change", b.arg, unownedIsExcluded)
	if err != nil {
		return nil, crmcontracts.ReportingCoverage{}, err
	}
	if population == "" {
		population = sqlUnnarrowed
	}
	// Primary credit is deduplicated before narrowing the SDR population.
	query := `SELECT credit.id::text,credit.handoff_id,'Accepted opportunity',credit.submitter_id_at_change,COALESCE(u.display_name,''),credit.occurred_at
 FROM (SELECT DISTINCT ON (e.deal_id_at_change) e.* FROM sdr_handoff_event e JOIN sdr_handoff h ON h.id=e.handoff_id
 WHERE e.to_status='accepted' AND e.deal_id_at_change IS NOT NULL ORDER BY e.deal_id_at_change,e.occurred_at,e.id) credit
 LEFT JOIN app_user u ON u.id=credit.submitter_id_at_change
 WHERE credit.occurred_at>=` + b.add(reportingEarliest(frame)) + " AND credit.occurred_at<" + b.add(frame.EvaluatedAt) + " AND " + population + " ORDER BY credit.id LIMIT " + b.add(reportingFactLimit+1)
	return readOutcomeFacts(ctx, tx, frame, reportingAcceptedOpportunities, "sdr_handoff", query, b)
}

func readOutcomeFacts(ctx context.Context, tx pgx.Tx, frame crmcontracts.ReportingContext, metric crmcontracts.ReportingMetricID, source, query string, b reportingBindings) ([]reporting.Fact, crmcontracts.ReportingCoverage, error) {
	rows, err := tx.Query(ctx, query, b.values...)
	if err != nil {
		return nil, crmcontracts.ReportingCoverage{}, err
	}
	facts, err := pgx.CollectRows(rows, func(row pgx.CollectableRow) (reporting.Fact, error) {
		fact := reporting.Fact{Metric: metric, SourceType: source}
		var owner *ids.UUID
		err := row.Scan(&fact.Row.Key, &fact.SourceID, &fact.Row.Label, &owner, &fact.OwnerLabel, &fact.Row.OccurredAt)
		one := 1.0
		fact.Row.Value = &one
		if owner != nil {
			fact.Row.OwnerId = ptrUUID(*owner)
		}
		if owner != nil {
			fact.OwnerID = *owner
		}
		if source == "sdr_handoff" {
			fact.Row.Restricted = true
		} else {
			fact.Row.SourceId = ptrUUID(fact.SourceID)
			fact.Row.SourceType = &fact.SourceType
		}
		return fact, err
	})
	if err != nil {
		return nil, crmcontracts.ReportingCoverage{}, err
	}
	if len(facts) > reportingFactLimit {
		return nil, crmcontracts.ReportingCoverage{}, fmt.Errorf("narrow the reporting population: %w", apperrors.ErrInvalidArgument)
	}
	return reportingCohorts(frame, facts), crmcontracts.ReportingCoverage{Status: "ok"}, nil
}
