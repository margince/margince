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

// meetingsAhead is the meetings lane: what starts between asOf and the day's
// end, and the day meetings dated today, which the zone says.
func (s *Service) meetingsAhead(ctx context.Context, asOf, until time.Time) ([]Meeting, error) {
	loc, err := s.location(ctx)
	if err != nil {
		return nil, err
	}
	today := occurredday.AnchorIn(asOf, loc)
	window := MeetingWindow{From: asOf, Until: until, DaysFrom: today, DaysUntil: today.AddDate(0, 0, 1)}
	return s.meetings.Today(ctx, window, plannedCap, s.taskScope, s.taskOwner)
}

// meetingsOwingAnOutcome is the outcome lane: meetings since unansweredSince
// that started before asOf, and the day meetings whose date is over.
func (s *Service) meetingsOwingAnOutcome(ctx context.Context, asOf time.Time) ([]MeetingAwaitingOutcome, error) {
	began, err := s.unansweredSince(ctx, asOf)
	if err != nil {
		return nil, err
	}
	loc, err := s.location(ctx)
	if err != nil {
		return nil, err
	}
	window := MeetingWindow{From: began, Until: asOf, DaysFrom: occurredday.AnchorIn(began, loc), DaysUntil: occurredday.AnchorIn(asOf, loc)}
	return s.meetingsAwaitingOutcome.Since(ctx, window, plannedCap, s.taskScope, s.taskOwner)
}
