// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

// What the reader's calendar says past today's meetings lane: whether it feeds
// the lane at all, from capture's connections, and the next customer meeting,
// through the same gated activity list and meeting dials the lane reads.

import (
	"context"

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

func (h attentionMeetingHorizon) Calendar(ctx context.Context) (crmcontracts.ReaderCalendar, error) {
	return readerCalendar(ctx, h.registry)
}

// NextCustomerMeeting is the meetings lane's own read moved past today. It
// reads the reader's meetings by the lane's "mine" dials: readable, linked to a
// customer record they may see, and booked. OpenStatusOnly means no result
// recorded yet, which for a meeting still ahead is booked or unset. Soonest
// first, one row.
func (h attentionMeetingHorizon) NextCustomerMeeting(
	ctx context.Context, window attention.MeetingWindow,
) (crmcontracts.Contact360NextMeeting, bool, error) {
	kind := string(crmcontracts.ActivityKindMeeting)
	soonestFirst := "occurred_at"
	one := 1
	in := activities.ListActivitiesInput{
		Kind: &kind, OccurredAfter: &window.From, OccurredBefore: &window.Until, DayDates: dayDatesOf(window),
		Sort: &soonestFirst, Limit: &one,
		OpenStatusOnly: true, CustomerMeetingsOnly: true, ReadableOnly: true,
	}
	if !applyMeetingScope(ctx, &in, attention.TasksMine, ids.UUID{}) {
		return crmcontracts.Contact360NextMeeting{}, false, nil
	}
	rows, err := listReusingPlans(ctx, h.store, in)
	if err != nil || len(rows) == 0 {
		return crmcontracts.Contact360NextMeeting{}, false, err
	}
	row := rows[0]
	if !meetingReadable(row) {
		return crmcontracts.Contact360NextMeeting{}, false, nil
	}
	// Not found means the meeting was archived between the two reads, which leaves no next meeting to name.
	room, found, err := h.store.MeetingRoom(ctx, ids.UUID(row.Id))
	if err != nil || !found {
		return crmcontracts.Contact360NextMeeting{}, false, err
	}
	meeting := crmcontracts.Contact360NextMeeting{ActivityId: row.Id, StartsAt: row.OccurredAt, OccurredPrecision: row.OccurredPrecision, Subject: row.Subject}
	room.Onto(&meeting)
	return meeting, true, nil
}
