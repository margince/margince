// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package activities

import (
	"context"
	"time"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/occurredday"
	"github.com/margince/margince/backend/internal/shared/kernel/provenance"
	"github.com/margince/margince/backend/internal/shared/kernel/values"
)

// CustomerMeetingSQL shares the customer-link rule between capture and reporting.
// The activity alias must be a compile-time identifier.
func CustomerMeetingSQL(activityAlias string) string {
	return "EXISTS(SELECT 1 FROM activity_link al WHERE al.activity_id=" + activityAlias + ".id AND " + customerLinkSQL + ")"
}

// customerLinkSQL is what makes a link under alias al a customer link.
const customerLinkSQL = "(al.contact_id IS NOT NULL OR al.lead_id IS NOT NULL OR al.company_id IS NOT NULL)"

// OpenMeetingStatusSQL is a meeting under alias a whose result nobody has recorded. NULL counts:
// capture writes calendar events with no status, and dropping them would empty
// the question on a connected calendar.
const OpenMeetingStatusSQL = "(a.meeting_status IS NULL OR a.meeting_status = 'booked')"

// appendCustomerMeetingClause narrows a list to customer meetings, counting
// only a customer link whose record this reader may see. A link to a record
// they cannot see would otherwise decide whether the meeting appears, which
// tells them the link is there.
func appendCustomerMeetingClause(ctx context.Context, in ListActivitiesInput, arg func(any) int, where []string) ([]string, error) {
	if !in.CustomerMeetingsOnly {
		return where, nil
	}
	customer, err := customerMeetingClause(ctx, arg)
	if err != nil {
		return nil, err
	}
	return append(where, customer), nil
}

func customerMeetingClause(ctx context.Context, arg func(any) int) (string, error) {
	visible, err := auth.LinkTargetReadableClause(ctx, "al", arg)
	if err != nil {
		return "", err
	}
	if visible == "" {
		visible = scopeUnbounded
	}
	return `a.kind = 'meeting' AND EXISTS (SELECT 1 FROM activity_link al WHERE al.activity_id = a.id
	   AND ` + customerLinkSQL + ` AND (` + visible + `))`, nil
}

// appendOutsidePartyClause keeps the meetings with somebody from outside on
// them, when the read asks for WithOutsideParty.
func appendOutsidePartyClause(in ListActivitiesInput, arg func(any) int, where []string) ([]string, error) {
	if in.outsideParty == nil {
		return where, nil
	}
	outside, err := in.outsideParty(arg)
	if err != nil {
		return nil, err
	}
	return append(where, outside), nil
}

func appendAwaitingOutcomeClause(ctx context.Context, in ListActivitiesInput, arg func(any) int, where []string) ([]string, error) {
	if in.AwaitingOutcomeAsOf == nil {
		return where, nil
	}
	owed, err := awaitingOutcomeClause(ctx, *in.AwaitingOutcomeAsOf, arg)
	if err != nil {
		return nil, err
	}
	return append(where, owed), nil
}

// awaitingOutcomeClause is a live customer meeting with no result recorded that
// took place by asOf. The outcome lane filters on it and every Activity read
// stamps it (stampAwaitingOutcome).
func awaitingOutcomeClause(ctx context.Context, asOf time.Time, arg func(any) int) (string, error) {
	open, tookPlace, err := openCustomerMeeting(ctx, asOf, arg)
	return "(a.archived_at IS NULL AND " + open + " AND " + tookPlace + ")", err
}

// UpcomingMeetingClause is a live customer meeting under alias a with no
// result recorded that has not taken place by asOf. It and
// awaitingOutcomeClause split the open meetings between them.
func UpcomingMeetingClause(ctx context.Context, asOf time.Time, arg func(any) int) (string, error) {
	open, tookPlace, err := openCustomerMeeting(ctx, asOf, arg)
	return "(a.archived_at IS NULL AND " + open + " AND NOT " + tookPlace + ")", err
}

func openCustomerMeeting(ctx context.Context, asOf time.Time, arg func(any) int) (string, string, error) {
	customer, err := customerMeetingClause(ctx, arg)
	if err != nil {
		return "", "", err
	}
	return customer + " AND " + OpenMeetingStatusSQL, occurredday.MeetingTookPlaceSQL("a", sprintf("$%d::timestamptz", arg(asOf))), nil
}

// ReadyForRecap checks the meeting as the authorizer can see it.
func (s *Store) ReadyForRecap(ctx context.Context, id ids.ActivityID, asOf time.Time) (bool, error) {
	rows, _, err := s.ListActivities(ctx, ListActivitiesInput{
		IDs:                  []ids.ActivityID{id},
		CustomerMeetingsOnly: true,
		ReadableOnly:         true,
		WithOutsideParty:     true,
		ConversationAsOf:     &asOf,
	})
	if err != nil || len(rows) == 0 {
		return false, err
	}
	meeting := rows[0]
	if meeting.SourceSystem != nil && provenance.ImporterNamespace(*meeting.SourceSystem) {
		return false, nil
	}
	return meetingReadyForRecap(meeting), nil
}

func meetingReadyForRecap(meeting crmcontracts.Activity) bool {
	return meeting.Kind == crmcontracts.ActivityKindMeeting && meeting.MeetingStatus != nil &&
		*meeting.MeetingStatus == crmcontracts.ActivityMeetingStatusHeld && meeting.Body != nil && values.HasVisibleText(*meeting.Body)
}
