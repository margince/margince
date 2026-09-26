// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package activities

import (
	"context"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

func (s *Store) strictHours(ctx context.Context, host ids.UserID) (WorkingHours, error) {
	if s.workingHours == nil {
		return WorkingHours{}, apperrors.ErrPermissionDenied
	}
	hours, err := s.workingHours(ctx, host)
	if err != nil {
		return hours, err
	}
	if hours.Location == nil {
		return hours, fmt.Errorf("scheduling: host time zone unavailable")
	}
	return hours, nil
}

func (s *Store) calendarBusy(ctx context.Context, host ids.UserID, p crmcontracts.SchedulingProfile, from, to time.Time, exclude string) ([]slot, error) {
	if s.calendar == nil {
		return nil, apperrors.ErrPermissionDenied
	}
	calendars := []string{p.CalendarId}
	if p.BlockingCalendars != nil {
		calendars = append(calendars, (*p.BlockingCalendars)...)
	}
	seen := map[string]bool{}
	busy := []slot{}
	buffer := time.Duration(p.BufferMinutes) * time.Minute
	for _, calendar := range calendars {
		if seen[calendar] {
			continue
		}
		seen[calendar] = true
		intervals, err := s.calendar.Busy(ctx, host, string(p.Provider), calendar, from.Add(-buffer), to.Add(buffer))
		if err != nil {
			return nil, err
		}
		for _, interval := range intervals {
			if exclude != "" && calendar == p.CalendarId && interval.EventID == exclude {
				continue
			}
			if !interval.End.After(interval.Start) {
				return nil, fmt.Errorf("scheduling: invalid provider interval")
			}
			busy = append(busy, slot{Start: interval.Start.Add(-buffer), End: interval.End.Add(buffer)})
		}
	}
	return busy, nil
}

func (s *Store) reliableBusy(ctx context.Context, host ids.UserID, p crmcontracts.SchedulingProfile, from, to time.Time, exclude ids.UUID, event string) ([]slot, error) {
	busy, err := s.calendarBusy(ctx, host, p, from, to, event)
	if err != nil {
		return nil, err
	}
	buffer := time.Duration(p.BufferMinutes) * time.Minute
	err = s.tx(ctx, func(tx pgx.Tx) error {
		local, err := localMeetingSlots(ctx, tx, host, from.Add(-buffer), to.Add(buffer), exclude)
		if err != nil {
			return err
		}
		for _, interval := range local {
			busy = append(busy, slot{Start: interval.Start.Add(-buffer), End: interval.End.Add(buffer)})
		}
		return nil
	})
	return busy, err
}

// ReliableAvailability combines provider occupancy with pending local reservations.
func (s *Store) ReliableAvailability(ctx context.Context, host ids.UserID, from, to time.Time, duration time.Duration) ([]slot, bool, error) {
	if err := auth.Require(ctx, "activity", principal.ActionRead); err != nil {
		return nil, false, err
	}
	actor, ok := principal.Actor(ctx)
	if !ok || (actor.Type != principal.PrincipalSystem && actor.UserID != host.UUID) {
		return nil, false, apperrors.ErrPermissionDenied
	}
	return s.hostReliableAvailability(ctx, host, from, to, duration)
}

func (s *Store) hostReliableAvailability(ctx context.Context, host ids.UserID, from, to time.Time, duration time.Duration) ([]slot, bool, error) {
	profile, err := s.hostSchedulingProfile(ctx, host)
	if err != nil {
		return nil, false, err
	}
	if profile.Provider == "" || s.calendar == nil {
		return nil, false, apperrors.ErrPermissionDenied
	}
	if err := s.calendar.Check(ctx, host, string(profile.Provider)); err != nil {
		return nil, false, err
	}
	if duration <= 0 {
		duration = time.Duration(profile.DurationMinutes) * time.Minute
	}
	if duration < minSlotDuration || duration > maxSlotDuration {
		return nil, false, errAvailabilityDurationOutOfRange
	}
	if !to.After(from) || to.Sub(from) > maxAvailabilityWindow {
		return nil, false, errAvailabilityWindowTooWide
	}
	now := s.now()
	earliest := now.Add(time.Duration(profile.NoticeMinutes) * time.Minute)
	latest := now.AddDate(0, 0, profile.HorizonDays)
	if from.Before(earliest) {
		from = earliest
	}
	if to.After(latest) {
		to = latest
	}
	if !to.After(from) {
		return []slot{}, false, nil
	}
	hours, err := s.strictHours(ctx, host)
	if err != nil {
		return nil, false, err
	}
	busy, err := s.reliableBusy(ctx, host, profile, from, to, ids.Nil, "")
	if err != nil {
		return nil, false, err
	}
	free, truncated := policySlots(from, to, duration, busy, hours)
	return free, truncated, nil
}

func policySlots(from, to time.Time, duration time.Duration, busy []slot, hours WorkingHours) ([]slot, bool) {
	free := []slot{}
	local := from.In(hours.Location)
	cursor := time.Date(local.Year(), local.Month(), local.Day(), local.Hour(), local.Minute()/15*15, 0, 0, hours.Location)
	if cursor.Before(from) {
		cursor = cursor.Add(15 * time.Minute)
	}
	for ; !cursor.Add(duration).After(to); cursor = cursor.Add(15 * time.Minute) {
		candidate := slot{Start: cursor.UTC(), End: cursor.Add(duration).UTC()}
		if !hours.covers(candidate.Start, candidate.End) || overlapsAny(candidate, busy) {
			continue
		}
		free = append(free, candidate)
		if len(free) > maxProposedSlots {
			return free[:maxProposedSlots], true
		}
	}
	return free, false
}

func (s *Store) validateInvitationTime(ctx context.Context, host ids.UserID, start, end time.Time) (crmcontracts.SchedulingProfile, error) {
	profile, err := s.hostSchedulingProfile(ctx, host)
	if err != nil {
		return profile, err
	}
	if err := s.validateBookingCalendars(ctx, host, &profile); err != nil {
		return profile, err
	}
	if err := s.validatePolicyTime(ctx, host, profile, start, end); err != nil {
		return profile, err
	}

	busy, err := s.calendarBusy(ctx, host, profile, start, end, "")
	if err != nil {
		return profile, err
	}
	if overlapsAny(slot{Start: start, End: end}, busy) {
		return profile, &SlotTakenError{Start: start}
	}
	return profile, nil
}

func (s *Store) validatePolicyTime(ctx context.Context, host ids.UserID, profile crmcontracts.SchedulingProfile, start, end time.Time) error {
	hours, err := s.strictHours(ctx, host)
	if err != nil {
		return err
	}
	duration := end.Sub(start)
	now := s.now()
	if duration < minSlotDuration || duration > maxSlotDuration || start.Before(now.Add(time.Duration(profile.NoticeMinutes)*time.Minute)) ||
		end.After(now.AddDate(0, 0, profile.HorizonDays)) || !hours.covers(start, end) {
		return &SchedulingArgumentError{Field: "start", Code: "outside_availability", Message: "Choose a future time within the host's bookable hours and notice period"}
	}
	if s.calendar == nil {
		return apperrors.ErrPermissionDenied
	}
	if err := s.calendar.Check(ctx, host, string(profile.Provider)); err != nil {
		return err
	}
	return nil
}

// Graph all-day responses can retain midnight wall dates despite a UTC preference.
// Converted instants with a non-midnight boundary already carry their offset.
func validateActivityDuration(seconds *int) error {
	if seconds != nil && (*seconds < 0 || *seconds > 31*24*60*60) {
		return &SchedulingArgumentError{Field: fieldDurationSeconds, Code: "out_of_range", Message: "Duration must be between zero and 31 days"}
	}
	return nil
}
