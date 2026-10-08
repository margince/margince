// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package attention

// What an empty meetings lane means. A zero is a free day only when the
// reader's calendar feeds the lane; under a missing or broken one it is no
// reading at all. And a free day still says when the next customer
// conversation is, so the reader is not left to open the calendar to find out.

import (
	"context"
	"errors"
	"log/slog"
	"time"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/shared/apperrors"
)

// nextMeetingHorizon is how far past today the next customer meeting is looked for.
const nextMeetingHorizon = 30 * 24 * time.Hour

// The two side reads, named on the page when they fail. Each speaks for the
// meetings reading, so categoryOfSource files both under meetings.
const (
	sourceCalendar    = "calendar"
	sourceNextMeeting = "next_meeting"
)

// MeetingHorizon answers for the reader's OWN day only: a wider scope has no
// single calendar, and its next meeting would be somebody else's.
type MeetingHorizon interface {
	// Calendar is whether the reader's own calendar connections feed the lane.
	Calendar(ctx context.Context) (crmcontracts.WorklistCalendar, error)
	// NextCustomerMeeting is the reader's soonest booked customer meeting starting
	// in [after, before); found is false when none is.
	NextCustomerMeeting(ctx context.Context, after, before time.Time) (meeting crmcontracts.Contact360NextMeeting, found bool, err error)
}

// WithMeetingHorizon binds the horizon. Unbound, the page carries neither field,
// which a client reads as "unknown" rather than as connected or as nothing booked.
func (s *Service) WithMeetingHorizon(h MeetingHorizon) *Service {
	s.horizon = h
	return s
}

// horizonRead is what the horizon answered, and the side reads that failed.
type horizonRead struct {
	calendar *crmcontracts.WorklistCalendar
	next     *crmcontracts.Contact360NextMeeting
	failed   []*crmcontracts.WorklistSourceUnavailable
}

// meetingHorizon reads both for a reader's own day. The next meeting is asked
// only when the lane answered and holds nothing left today: a withheld lane's
// emptiness is not a free day, and a day with a meeting left needs no pointer past it.
func (s *Service) meetingHorizon(ctx context.Context, day crmcontracts.Attention, until time.Time) horizonRead {
	var out horizonRead
	if s.horizon == nil || s.taskScope != TasksMine {
		return out
	}
	if failed := s.horizonSide(ctx, sourceCalendar, func(ctx context.Context) error {
		calendar, err := s.horizon.Calendar(ctx)
		if err == nil {
			out.calendar = &calendar
		}
		return err
	}); failed != nil {
		out.failed = append(out.failed, failed)
	}
	if day.Meetings == nil || len(*day.Meetings) > 0 {
		return out
	}
	if failed := s.horizonSide(ctx, sourceNextMeeting, func(ctx context.Context) error {
		next, found, err := s.horizon.NextCustomerMeeting(ctx, until, until.Add(nextMeetingHorizon))
		if found && err == nil {
			out.next = &next
		}
		return err
	}); failed != nil {
		out.failed = append(out.failed, failed)
	}
	return out
}

// horizonSide runs one side read through degradable. A refusal leaves its field
// absent; a failure leaves it absent AND names it, so absent never claims a
// connected calendar or an empty month that nobody read.
func (s *Service) horizonSide(
	ctx context.Context, source string, read func(context.Context) error,
) *crmcontracts.WorklistSourceUnavailable {
	err := s.degradable(ctx, laneBudget, read)
	if err == nil || errors.Is(err, apperrors.ErrPermissionDenied) {
		return nil
	}
	slog.ErrorContext(ctx, "a meeting-horizon read failed", "source", source, "error", err)
	return &crmcontracts.WorklistSourceUnavailable{
		Source: source, Reason: crmcontracts.WorklistSourceUnavailableReasonFailed,
	}
}
