// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package activities

import (
	"context"
	"time"

	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/shared/kernel/occurredday"
)

// CustomerMeetingSQL shares the customer-link rule between capture and reporting.
// The activity alias must be a compile-time identifier.
func CustomerMeetingSQL(activityAlias string) string {
	return "EXISTS(SELECT 1 FROM activity_link al WHERE al.activity_id=" + activityAlias + ".id AND " + customerLinkSQL + ")"
}

// customerLinkSQL is what makes a link under alias al a customer link.
const customerLinkSQL = "(al.contact_id IS NOT NULL OR al.lead_id IS NOT NULL OR al.company_id IS NOT NULL)"

// openMeetingStatusSQL is a meeting whose result nobody has recorded. NULL counts:
// capture writes calendar events with no status, and dropping them would empty
// the question on a connected calendar.
const openMeetingStatusSQL = "(a.meeting_status IS NULL OR a.meeting_status = 'booked')"

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
	customer, err := customerMeetingClause(ctx, arg)
	if err != nil {
		return "", err
	}
	tookPlace := occurredday.MeetingTookPlaceSQL("a", sprintf("$%d::timestamptz", arg(asOf)))
	return "(a.archived_at IS NULL AND " + customer + " AND " + openMeetingStatusSQL + " AND " + tookPlace + ")", nil
}
