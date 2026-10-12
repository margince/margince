// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

import (
	"context"

	"github.com/jackc/pgx/v5"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/modules/activities"
	"github.com/margince/margince/backend/internal/modules/reporting"
	"github.com/margince/margince/backend/internal/platform/auth"
)

func readReportingMeetings(ctx context.Context, tx pgx.Tx, frame crmcontracts.ReportingContext, _ crmcontracts.ReportingFramework, _ metricEvaluator) ([]reporting.Fact, crmcontracts.ReportingCoverage, error) {
	if frame.PipelineId != nil {
		return nil, reportingGap("unsupported", "Meetings have no pipeline dimension; choose all pipelines"), nil
	}
	var b reportingBindings
	legacy := "(held.id IS NULL OR held.customer_eligible_at_change IS NULL OR held.partial_pre_history)"
	owner := "CASE WHEN " + legacy + " THEN a.host_user_id ELSE held.host_id_at_change END"
	eligible := "CASE WHEN " + legacy + " THEN " + activities.CustomerMeetingSQL("a") + " ELSE held.customer_eligible_at_change END"
	start := "COALESCE(held.scheduled_start,a.occurred_at)"
	_, population, err := analyticsPopulationExpression(ctx, tx, reportingRequested(frame.Scope), owner, b.arg, unownedIsExcluded)
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
	query := `SELECT COALESCE(held.id,a.id)::text,a.id,COALESCE(a.subject,'Meeting'),` + owner + `,COALESCE(u.display_name,''),` + start + `,
 CASE WHEN ` + legacy + ` THEN 'current_meeting' ELSE 'held_transition' END
 FROM activity a LEFT JOIN LATERAL(SELECT h.* FROM activity_meeting_history h WHERE h.activity_id=a.id AND h.effective_at<=` + b.add(frame.EvaluatedAt) + ` ORDER BY h.effective_at DESC,h.id DESC LIMIT 1) held ON true
 LEFT JOIN app_user u ON u.id=(` + owner + `)
 WHERE a.kind='meeting' AND a.archived_at IS NULL
 AND (held.id IS NOT NULL OR NOT EXISTS(SELECT 1 FROM activity_meeting_history future WHERE future.activity_id=a.id))
 AND COALESCE(held.status,a.meeting_status)='held' AND (` + eligible + `)
 AND ` + start + ` >= ` + b.add(reportingEarliest(frame)) + " AND " + start + " < " + b.add(frame.EvaluatedAt) + " AND " + population + " AND " + visible + " ORDER BY COALESCE(held.id,a.id) LIMIT " + b.add(reportingFactLimit+1)
	facts, coverage, err := readOutcomeFacts(ctx, tx, frame, reportingMeetingsHeld, string(recordTypeActivity), query, b)
	for _, fact := range facts {
		if fact.Provenance == "current_meeting" {
			coverage = reportingGap(reportingPartial, "Older meetings use their current host and customer links because historical attribution is unavailable.")
			break
		}
	}
	coverage.Withheld = withheld
	return facts, coverage, err
}
