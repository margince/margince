// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package attention

// A day meeting has a date and no hour. It belongs to the meetings lane for its
// whole date and is never about to start. It asks for an outcome once that date is over.

import (
	"context"
	"time"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/shared/kernel/occurredday"
)

func isDayPrecision(p crmcontracts.OccurredPrecision) bool {
	return p == crmcontracts.OccurredPrecisionOccurredPrecisionDay
}

// meetingsAhead is the meetings lane: what starts between asOf and the day's end.
// A day meeting dated today stays on it after its stored noon. That noon is one
// instant, so the second read asks for that instant alone.
func (s *Service) meetingsAhead(ctx context.Context, asOf, until time.Time) ([]Meeting, error) {
	timed, err := s.meetings.Today(ctx, asOf, until, plannedCap, s.taskScope, s.taskOwner)
	if err != nil {
		return nil, err
	}
	loc, err := s.location(ctx)
	if err != nil {
		return nil, err
	}
	anchor := occurredday.Anchor(occurredday.Date(asOf, loc), loc)
	if !anchor.Before(asOf) {
		return timed, nil
	}
	onAnchor, err := s.meetings.Today(ctx, anchor, anchor.Add(time.Second), plannedCap, s.taskScope, s.taskOwner)
	if err != nil {
		return nil, err
	}
	return withTodaysDayMeetings(timed, onAnchor), nil
}

// withTodaysDayMeetings puts today's day meetings ahead of the timed ones, as a
// calendar draws its all-day row, within the lane's cap. An hour that merely
// starts at the anchor has passed, so only a day is kept from that read.
func withTodaysDayMeetings(timed, onAnchor []Meeting) []Meeting {
	out := make([]Meeting, 0, len(timed)+len(onAnchor))
	for _, meeting := range onAnchor {
		if isDayPrecision(meeting.Precision) {
			out = append(out, meeting)
		}
	}
	out = append(out, timed...)
	if len(out) > plannedCap {
		out = out[:plannedCap]
	}
	return out
}

// meetingsOwingAnOutcome is the outcome lane: meetings since unansweredSince
// that started before asOf, less a day meeting whose date runs on.
func (s *Service) meetingsOwingAnOutcome(ctx context.Context, asOf time.Time) ([]MeetingAwaitingOutcome, error) {
	began, err := s.unansweredSince(ctx, asOf)
	if err != nil {
		return nil, err
	}
	over, err := s.meetingsAwaitingOutcome.Since(ctx, began, asOf, plannedCap, s.taskScope, s.taskOwner)
	if err != nil {
		return nil, err
	}
	loc, err := s.location(ctx)
	return stillOwingAnOutcome(over, asOf, loc), err
}

// stillOwingAnOutcome drops a day meeting whose date has not ended. The rows
// come oldest first, so the ones dropped are the newest and hide no older row.
func stillOwingAnOutcome(over []MeetingAwaitingOutcome, asOf time.Time, loc *time.Location) []MeetingAwaitingOutcome {
	out := make([]MeetingAwaitingOutcome, 0, len(over))
	for _, meeting := range over {
		if isDayPrecision(meeting.Precision) && !occurredday.Ended(meeting.StartedAt, asOf, loc) {
			continue
		}
		out = append(out, meeting)
	}
	return out
}
