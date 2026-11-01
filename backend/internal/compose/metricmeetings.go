// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

import (
	"context"
	"fmt"
	"time"

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
	_, credited, err := analyticsPopulationExpression(ctx, tx, reportingRequested(frame.Scope), "credit.id", b.arg, unownedIsExcluded)
	if err != nil {
		return nil, crmcontracts.ReportingCoverage{}, err
	}
	held, withheld, err := heldMeetingSource(ctx, &b, frame.EvaluatedAt, reportingEarliest(frame), frame.EvaluatedAt)
	if err != nil {
		return nil, crmcontracts.ReportingCoverage{}, err
	}
	// The whole workspace counts every held meeting, credited or not.
	population := sqlUnnarrowed
	if credited == "" {
		credited = sqlUnnarrowed
	} else {
		population = "EXISTS (SELECT 1 FROM " + held.seats + " WHERE " + credited + ")"
	}
	// The row is listed under the first credited seat in the population.
	owner := "(SELECT credit.id FROM " + held.seats + " WHERE " + credited +
		" ORDER BY credit.id IS NOT DISTINCT FROM " + held.seat + " DESC, credit.id LIMIT 1)"
	query := `SELECT COALESCE(held.id,a.id)::text,a.id,COALESCE(a.subject,` + b.add(reporting.ExplanationCopy(frame).Say(reportingLabelWords.Meeting)) + `),` + owner + `,COALESCE(u.display_name,''),` + held.start + `,` + held.precision + `,` + held.placed + `,` + held.provenance + `
 FROM ` + held.from + `
 LEFT JOIN app_user u ON u.id=(` + owner + `)
 WHERE ` + held.where + " AND " + population + " ORDER BY COALESCE(held.id,a.id) LIMIT " + b.add(reportingFactLimit+1)
	facts, err := scanOutcomeFacts(ctx, tx, reportingMeetingsHeld, string(recordTypeActivity), query, b)
	if err != nil {
		return nil, crmcontracts.ReportingCoverage{}, err
	}
	coverage, err := meetingHistoryCoverage(frame, facts)
	coverage.Withheld = withheld
	return reportingCohorts(frame, facts), coverage, err
}

// heldMeeting is the held-meetings reading's row source, for every reader that
// counts held meetings: one meeting per row of from under where, aliased a.
type heldMeeting struct {
	from, where string
	// seats is a row source of the seats the meeting credits, as credit(id).
	// seat is the one seat a figure grouped by colleague lists it under.
	seats, seat              string
	start, precision, placed string
	// provenance names how the row's credit and day were read; legacy is
	// whether they come from the meeting as it is today.
	provenance, legacy string
}

// heldMeetingSource renders the held meetings this seat may read that took
// place in [from, to), as their history stood at at. withheld says a field
// mask narrowed them.
func heldMeetingSource(ctx context.Context, b *reportingBindings, at, from, to time.Time) (heldMeeting, bool, error) {
	// The reader Weekly counts its own held meetings through, so the two place
	// a meeting on the same day.
	held := storekit.MeetingHeldAsOfSQL("a", b.add(at))
	legacy, start, precision, unknownDay := held.Legacy, held.Start, held.Precision, held.UnknownDay
	eligible := "CASE WHEN " + legacy + " THEN " + activities.CustomerMeetingSQL("a") + " ELSE held.customer_eligible_at_change END"
	visible, err := auth.ActivityContentClause(ctx, "a", b.arg)
	if err != nil {
		return heldMeeting{}, false, err
	}
	fieldScope, withheld, err := reportingFieldScope(ctx, string(recordTypeActivity), "a", reportingMeetingFields(), b)
	if err != nil {
		return heldMeeting{}, false, err
	}
	lower, upper := b.add(from), b.add(to)
	within := "(" + occurredday.WithinAtSQL(start, precision, lower, upper) + " OR (" + unknownDay + " AND (" +
		occurredday.WithinAtSQL(start, "'instant'", lower, upper) + " OR " + occurredday.WithinAtSQL(start, "'day'", lower, upper) + ")))"
	// Every colleague the attendance evidence credits counts the meeting in
	// their own figures; a team counts it once, because it is one row here.
	// A recorded transition keeps the host it recorded.
	return heldMeeting{
		from: "activity a " + held.Join,
		where: `a.kind='meeting' AND a.archived_at IS NULL
 AND ` + held.Recorded + `
 AND COALESCE(held.status,a.meeting_status)='held' AND (` + eligible + `)
 AND ` + within + " AND " + orUnnarrowed(visible) + " AND " + fieldScope,
		seats:      storekit.MeetingCreditedSeatsAtChangeSQL("a", legacy, "held.host_id_at_change"),
		seat:       storekit.MeetingCreditedSeatAtChangeSQL("a", legacy, "held.host_id_at_change"),
		start:      start,
		precision:  precision,
		placed:     occurredday.StartAtSQL(start, precision),
		provenance: "CASE WHEN " + legacy + " THEN 'current_meeting' WHEN " + unknownDay + " THEN 'held_transition_day_unknown' ELSE 'held_transition' END",
		legacy:     legacy,
	}, withheld, nil
}

// meetingHistoryCoverage marks the reading partial when an older meeting is
// attributed from today's record. It also does so when a row with no recorded
// precision could fall on a day in one of the reading's periods. That holds
// whichever day its guess chose. Attribution is named first: it moves a
// meeting to another colleague, the day only to its neighbour.
func meetingHistoryCoverage(frame crmcontracts.ReportingContext, facts []reporting.Fact) (crmcontracts.ReportingCoverage, error) {
	zone, err := time.LoadLocation(frame.Timezone)
	if err != nil {
		return crmcontracts.ReportingCoverage{}, fmt.Errorf("reading the reporting timezone: %w", err)
	}
	var legacy, unknownDay bool
	for _, fact := range facts {
		legacy = legacy || fact.Provenance == "current_meeting"
		unknownDay = unknownDay || fact.Provenance == "held_transition_day_unknown" && eitherDayInAPeriod(frame, fact, zone)
	}
	switch {
	case legacy:
		return reportingGap(reportingPartial, reporting.ExplanationCopy(frame).Say(reportingCoverageWords.LegacyMeetings)), nil
	case unknownDay:
		return reportingGap(reportingPartial, reporting.ExplanationCopy(frame).Say(reportingCoverageWords.LegacyMeetingDay)), nil
	}
	return crmcontracts.ReportingCoverage{Status: "ok"}, nil
}

// eitherDayInAPeriod reports whether the stored start falls in the interval,
// its comparison or its target window. It reads the start as an instant and
// as its date's local start, which is StartAtSQL's day branch.
func eitherDayInAPeriod(frame crmcontracts.ReportingContext, fact reporting.Fact, zone *time.Location) bool {
	if fact.Row.OccurredAt == nil {
		return false
	}
	windows := []crmcontracts.ReportingWindow{frame.Interval, reportingPrevious(frame)}
	if frame.TargetInterval != nil {
		windows = append(windows, *frame.TargetInterval)
	}
	at := *fact.Row.OccurredAt
	for _, candidate := range []time.Time{at, occurredday.Begins(at, zone)} {
		for _, window := range windows {
			if reportingInWindow(candidate, window) {
				return true
			}
		}
	}
	return false
}
