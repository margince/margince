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
	"github.com/margince/margince/backend/internal/platform/database/storekit"
	"github.com/margince/margince/backend/internal/shared/kernel/occurredday"
)

func readReportingMeetings(ctx context.Context, tx pgx.Tx, frame crmcontracts.ReportingContext, _ crmcontracts.ReportingFramework, _ metricEvaluator) ([]reporting.Fact, crmcontracts.ReportingCoverage, error) {
	if frame.PipelineId != nil {
		return nil, reportingGap("unsupported", reporting.ExplanationCopy(frame).Say(reportingCoverageWords.MeetingPipeline)), nil
	}
	var b reportingBindings
	legacy := "(held.id IS NULL OR held.customer_eligible_at_change IS NULL OR held.partial_pre_history)"
	eligible := "CASE WHEN " + legacy + " THEN " + activities.CustomerMeetingSQL("a") + " ELSE held.customer_eligible_at_change END"
	// A transition says whether its start was a time or a date, so a day
	// meeting moved later stays on the day it was held. A row recorded before
	// it said so reads as the meeting's precision only while it is the
	// meeting's time; otherwise as an instant.
	start := "COALESCE(held.scheduled_start,a.occurred_at)"
	precision := "COALESCE(held.scheduled_precision,CASE WHEN held.scheduled_start IS NULL OR held.scheduled_start = a.occurred_at THEN a.occurred_precision ELSE 'instant' END)"
	placed := occurredday.StartAtSQL(start, precision)
	// Only a noon UTC start can be a date's anchor. On a row with no recorded
	// precision that reading is a guess, and the coverage says so.
	unknownDay := "(held.scheduled_precision IS NULL AND held.scheduled_start IS NOT NULL AND " + occurredday.IsAnchorSQL("held.scheduled_start") + ")"
	// Every colleague the attendance evidence credits counts the meeting in
	// their own figures; a team counts it once, because it is one row here.
	// The row is listed under the first credited seat in the population. A
	// recorded transition keeps the host it recorded.
	seats := storekit.MeetingCreditedSeatsAtChangeSQL("a", legacy, "held.host_id_at_change")
	_, credited, err := analyticsPopulationExpression(ctx, tx, reportingRequested(frame.Scope), "credit.id", b.arg, unownedIsExcluded)
	if err != nil {
		return nil, crmcontracts.ReportingCoverage{}, err
	}
	// The whole workspace counts every held meeting, credited or not.
	population := sqlUnnarrowed
	if credited == "" {
		credited = sqlUnnarrowed
	} else {
		population = "EXISTS (SELECT 1 FROM " + seats + " WHERE " + credited + ")"
	}
	owner := "(SELECT credit.id FROM " + seats + " WHERE " + credited +
		" ORDER BY credit.id IS NOT DISTINCT FROM " + storekit.MeetingCreditedSeatAtChangeSQL("a", legacy, "held.host_id_at_change") + " DESC, credit.id LIMIT 1)"
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
	query := `SELECT COALESCE(held.id,a.id)::text,a.id,COALESCE(a.subject,` + b.add(reporting.ExplanationCopy(frame).Say(reportingLabelWords.Meeting)) + `),` + owner + `,COALESCE(u.display_name,''),` + start + `,` + precision + `,` + placed + `,
 CASE WHEN ` + legacy + ` THEN 'current_meeting' WHEN ` + unknownDay + ` THEN 'held_transition_day_unknown' ELSE 'held_transition' END
 FROM activity a LEFT JOIN LATERAL(SELECT h.* FROM activity_meeting_history h WHERE h.activity_id=a.id AND h.effective_at<=` + b.add(frame.EvaluatedAt) + ` ORDER BY h.effective_at DESC,h.id DESC LIMIT 1) held ON true
 LEFT JOIN app_user u ON u.id=(` + owner + `)
 WHERE a.kind='meeting' AND a.archived_at IS NULL
 AND (held.id IS NOT NULL OR NOT EXISTS(SELECT 1 FROM activity_meeting_history future WHERE future.activity_id=a.id))
 AND COALESCE(held.status,a.meeting_status)='held' AND (` + eligible + `)
 AND ` + occurredday.WithinAtSQL(start, precision, b.add(reportingEarliest(frame)), b.add(frame.EvaluatedAt)) + " AND " + population + " AND " + visible + " ORDER BY COALESCE(held.id,a.id) LIMIT " + b.add(reportingFactLimit+1)
	facts, coverage, err := readOutcomeFacts(ctx, tx, frame, reportingMeetingsHeld, string(recordTypeActivity), query, b)
	coverage = meetingHistoryCoverage(frame, facts, coverage)
	coverage.Withheld = withheld
	return facts, coverage, err
}

// meetingHistoryCoverage marks the reading partial when an older meeting is
// attributed from today's record. It also does so when a day rests on a start
// with no recorded precision. Attribution is named first, because it moves a
// meeting to another colleague, and the day only to its neighbour.
func meetingHistoryCoverage(frame crmcontracts.ReportingContext, facts []reporting.Fact, coverage crmcontracts.ReportingCoverage) crmcontracts.ReportingCoverage {
	var legacy, unknownDay bool
	for _, fact := range facts {
		legacy = legacy || fact.Provenance == "current_meeting"
		unknownDay = unknownDay || fact.Provenance == "held_transition_day_unknown"
	}
	switch {
	case legacy:
		return reportingGap(reportingPartial, reporting.ExplanationCopy(frame).Say(reportingCoverageWords.LegacyMeetings))
	case unknownDay:
		return reportingGap(reportingPartial, reporting.ExplanationCopy(frame).Say(reportingCoverageWords.LegacyMeetingDay))
	}
	return coverage
}
