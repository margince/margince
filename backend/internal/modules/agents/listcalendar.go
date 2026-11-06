// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package agents

import (
	"context"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/shared/ports/datasource"
)

// ActivityListCalendar is what an activity list owes about the reader's own
// calendar, and whether it owes anything. compose injects the function GET
// /activities reads, so the tool and the page answer alike.
type ActivityListCalendar func(ctx context.Context, kind string, windowed bool) (calendar crmcontracts.ReaderCalendar, owed bool, err error)

// WithActivityListCalendar binds it. Unbound, list_records says nothing about
// the calendar, which reads as unknown.
func WithActivityListCalendar(calendar ActivityListCalendar) RegistryOption {
	return func(r *Registry) { r.listCalendar = calendar }
}

// activityListCalendar answers for one list_records call, and raises the
// caveat a list that is not the diary owes. Only `connected` buys silence.
func (t listRecords) activityListCalendar(ctx context.Context, recordType string, filters map[string]string) (calendar crmcontracts.ReaderCalendar, owed bool, err error) {
	if t.calendar == nil || recordType != string(datasource.EntityActivity) {
		return "", false, nil
	}
	windowed := filters["occurred_after"] != "" || filters["occurred_before"] != ""
	calendar, owed, err = t.calendar(ctx, filters["kind"], windowed)
	if err != nil || !owed {
		return "", false, err
	}
	switch calendar {
	case crmcontracts.ReaderCalendarConnected:
		return calendar, true, nil
	case crmcontracts.ReaderCalendarUnreadable:
		noteWarning(ctx, warningCalendarUnreadable, meetingListCalendarUnreadableMessage)
	default:
		noteWarning(ctx, warningNoCalendarConnected, meetingListNoCalendarMessage)
	}
	noteAnswerLacksItsSource(ctx)
	return calendar, true, nil
}
