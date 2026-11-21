// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package activities

import (
	"context"
	"slices"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/platform/database/storekit"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// memberPolicy reads a member's own calendars with the page's buffer, so a
// member's meetings are padded as the host's are.
func memberPolicy(member teamCalendar, page crmcontracts.SchedulingProfile) crmcontracts.SchedulingProfile {
	policy := member.Profile
	policy.BufferMinutes = page.BufferMinutes
	return policy
}

// teamOccupancy answers the busy time and working hours of each member over the window.
// exclude and event name the meeting being moved, which holds no time against itself.
func (s *Store) teamOccupancy(ctx context.Context, team []teamCalendar, page crmcontracts.SchedulingProfile, from, to time.Time, exclude ids.UUID, event string) ([]slot, []WorkingHours, error) {
	var busy []slot
	hours := make([]WorkingHours, 0, len(team))
	for _, member := range team {
		memberBusy, err := s.reliableBusy(ctx, member.User, memberPolicy(member, page), from, to, exclude, event)
		if err != nil {
			return nil, nil, err
		}
		memberHours, err := s.strictHours(ctx, member.User)
		if err != nil {
			return nil, nil, err
		}
		busy = append(busy, memberBusy...)
		hours = append(hours, memberHours)
	}
	return busy, hours, nil
}

// validateTeamTime refuses a booked time outside a member's working hours or
// over a meeting on a member's calendar, before anything is written.
func (s *Store) validateTeamTime(ctx context.Context, team []teamCalendar, page crmcontracts.SchedulingProfile, start, end time.Time, event string) error {
	for _, member := range team {
		hours, err := s.strictHours(ctx, member.User)
		if err != nil {
			return err
		}
		if !hours.covers(start, end) {
			return &SchedulingArgumentError{Field: "start", Code: "outside_availability", Message: "Choose a future time within the host's bookable hours and notice period"}
		}
		busy, err := s.calendarBusy(ctx, member.User, memberPolicy(member, page), start, end, event)
		if err != nil {
			return err
		}
		if overlapsAny(slot{Start: start, End: end}, busy) {
			return &SlotTakenError{Start: start}
		}
	}
	return nil
}

// validatePageTime checks a time booked on a standing page for its host and
// every member, before anything is written.
func (s *Store) validatePageTime(ctx context.Context, host ids.UserID, page crmcontracts.SchedulingProfile, start, end time.Time) error {
	profile, err := s.validateInvitationTime(ctx, host, start, end)
	if err != nil {
		return err
	}
	team, err := s.pageTeam(ctx, page)
	if err != nil {
		return err
	}
	return s.validateTeamTime(ctx, team, profile, start, end, "")
}

// slotHolders are the host and each member whose time a booking holds. They
// come in one order, so two bookings that share a colleague lock alike.
func slotHolders(host ids.UserID, team []ids.UUID) []ids.UserID {
	holders := []ids.UserID{host}
	for _, member := range team {
		if member != host.UUID {
			holders = append(holders, ids.From[ids.UserKind](member))
		}
	}
	slices.SortFunc(holders, func(a, b ids.UserID) int { return strings.Compare(a.String(), b.String()) })
	return slices.Compact(holders)
}

// lockSlotHolders takes the write identity of the host and every member.
func lockSlotHolders(ctx context.Context, tx pgx.Tx, host ids.UserID, team []ids.UUID) error {
	for _, holder := range slotHolders(host, team) {
		if err := storekit.LockWriteIdentity(ctx, tx, "meeting_host", holder.String()); err != nil {
			return err
		}
	}
	return nil
}

// reserveSlotHolders refuses a time the host or a member already holds, under
// lockSlotHolders. A member's time is held by their own meetings and by every
// other team booking that names them.
func reserveSlotHolders(ctx context.Context, tx pgx.Tx, host ids.UserID, team []ids.UUID, start, end time.Time, buffer int, exclude ids.UUID) error {
	for _, holder := range slotHolders(host, team) {
		if err := reserveInvitation(ctx, tx, holder, start, end, buffer, exclude); err != nil {
			return err
		}
	}
	return nil
}

// teamOf is the members a booked appointment holds time for, as calendars the
// page can still read. A member who can no longer be read is left out: the
// meeting stands, and they are still invited to it.
func (s *Store) teamOf(ctx context.Context, members []ids.UUID) ([]teamCalendar, error) {
	team := make([]teamCalendar, 0, len(members))
	for _, member := range members {
		calendar, ok, err := s.memberCalendar(ctx, ids.From[ids.UserKind](member))
		if err != nil {
			return nil, err
		}
		if ok {
			team = append(team, calendar)
		}
	}
	return team, nil
}

func teamIDs(team []teamCalendar) []ids.UUID {
	out := make([]ids.UUID, 0, len(team))
	for _, member := range team {
		out = append(out, member.User.UUID)
	}
	return out
}
