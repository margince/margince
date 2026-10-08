// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

// What the reader's calendar says past today's meetings lane: whether it feeds
// the lane at all, from capture's connections, and the next customer meeting,
// through the same gated activity list and meeting dials the lane reads.

import (
	"context"
	"time"

	"github.com/margince/margince/backend/internal/compose/attention"
	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/modules/activities"
	"github.com/margince/margince/backend/internal/modules/capture"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// attentionMeetingHorizon reads the calendar providers calendarBackingResolver
// reads, so the scheduler and the Worklist ask about the same connections.
type attentionMeetingHorizon struct {
	registry *capture.Registry
	store    *activities.Store
}

func (h attentionMeetingHorizon) Calendar(ctx context.Context) (crmcontracts.WorklistCalendar, error) {
	reach, err := h.registry.ReachOn(ctx, calendarProviders)
	if err != nil {
		return "", err
	}
	switch reach {
	case capture.ReachLive:
		return crmcontracts.WorklistCalendarConnected, nil
	case capture.ReachBroken:
		return crmcontracts.WorklistCalendarUnreadable, nil
	case capture.ReachNone:
	}
	return crmcontracts.WorklistCalendarNotConnected, nil
}

// NextCustomerMeeting is the meetings lane's own read moved past today: the
// reader's meetings by the lane's "mine" dials, readable, linked to a customer
// record they may see, and booked — AwaitingOutcome is "no result recorded yet",
// which for a meeting still ahead is exactly booked or unset. Soonest first, one row.
func (h attentionMeetingHorizon) NextCustomerMeeting(
	ctx context.Context, after, before time.Time,
) (crmcontracts.Contact360NextMeeting, bool, error) {
	kind := string(crmcontracts.ActivityKindMeeting)
	soonestFirst := "occurred_at"
	one := 1
	in := activities.ListActivitiesInput{
		Kind: &kind, OccurredAfter: &after, OccurredBefore: &before, Sort: &soonestFirst, Limit: &one,
		AwaitingOutcome: true, CustomerMeetingsOnly: true, ReadableOnly: true,
	}
	if !applyMeetingScope(ctx, &in, attention.TasksMine, ids.UUID{}) {
		return crmcontracts.Contact360NextMeeting{}, false, nil
	}
	rows, _, err := h.store.ListActivities(ctx, in)
	if err != nil || len(rows) == 0 {
		return crmcontracts.Contact360NextMeeting{}, false, err
	}
	row := rows[0]
	room, found, err := h.store.MeetingRoom(ctx, ids.UUID(row.Id))
	if err != nil || !found {
		return crmcontracts.Contact360NextMeeting{}, false, err
	}
	meeting := crmcontracts.Contact360NextMeeting{ActivityId: row.Id, StartsAt: row.OccurredAt, Subject: row.Subject}
	room.Onto(&meeting)
	return meeting, true, nil
}
