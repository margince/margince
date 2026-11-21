// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package activities

import (
	"context"
	"errors"
	"slices"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/platform/database/storekit"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/ports/connector"
)

// memberPolicy reads a member's own calendars with the page's buffer, so a
// member's meetings are padded as the host's are.
func memberPolicy(member teamCalendar, page crmcontracts.SchedulingProfile) crmcontracts.SchedulingProfile {
	policy := member.Profile
	policy.BufferMinutes = page.BufferMinutes
	return policy
}

const (
	fieldStart              = "start"
	codeOutsideAvailability = "outside_availability"
)

// The refusals of a team time. Neither names a colleague, because the public
// page shows them to a guest.
var (
	errOutsideTeamHours = &SchedulingArgumentError{
		Field: fieldStart, Code: codeOutsideAvailability,
		Message: "This time is outside the meeting team's hours. Choose another time.",
	}
	errTeamCalendarUnreadable = &SchedulingArgumentError{
		Field: fieldStart, Code: "team_calendar_unavailable",
		Message: "A calendar of the meeting team cannot be read, so this meeting cannot move now. Try again later.",
	}
)

// teamOccupancy answers the busy time and working hours of each member over
// the window. exclude and moved name a meeting being moved, which holds no
// time against itself: exclude is its local hold, and moved its current time.
func (s *Store) teamOccupancy(ctx context.Context, team []teamCalendar, page crmcontracts.SchedulingProfile, from, to time.Time, exclude ids.UUID, moved *slot) ([]slot, []WorkingHours, error) {
	var busy []slot
	hours := make([]WorkingHours, 0, len(team))
	for _, member := range team {
		policy := memberPolicy(member, page)
		// A member's copy of the moved meeting has an event id of its own, so
		// it is known by the time it sits at.
		memberBusy, err := s.providerBusy(ctx, member.User, policy, from, to, func(_ string, interval connector.CalendarInterval) bool {
			return moved != nil && interval.Start.Equal(moved.Start) && interval.End.Equal(moved.End)
		})
		if err != nil {
			return nil, nil, err
		}
		memberBusy, err = s.withLocalBusy(ctx, member.User, policy, from, to, exclude, memberBusy)
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
// over a member's busy time. It asks the question the offered times answer.
func (s *Store) validateTeamTime(ctx context.Context, team []teamCalendar, page crmcontracts.SchedulingProfile, start, end time.Time, exclude ids.UUID, moved *slot) error {
	busy, hours, err := s.teamOccupancy(ctx, team, page, start, end, exclude, moved)
	if err != nil {
		return err
	}
	booked := slot{Start: start, End: end}
	if !allCover(hours, booked) {
		return errOutsideTeamHours
	}
	if overlapsAny(booked, busy) {
		return &SlotTakenError{Start: start}
	}
	return nil
}

// validatePageTime checks a time booked on a standing page for its host and
// the team the page was opened with, before anything is written.
func (s *Store) validatePageTime(ctx context.Context, host ids.UserID, team []teamCalendar, start, end time.Time) error {
	profile, err := s.validateInvitationTime(ctx, host, start, end)
	if err != nil {
		return err
	}
	return s.validateTeamTime(ctx, team, profile, start, end, ids.Nil, nil)
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

// lockInvitationHolders takes the locks of a booked meeting's host and team in
// lockSlotHolders' order. The team is fixed when the meeting is booked, so it is
// read before any lock.
func lockInvitationHolders(ctx context.Context, tx pgx.Tx, host ids.UserID, invitation ids.UUID) error {
	var team []ids.UUID
	args := schedulingArgs{}
	err := tx.QueryRow(ctx, `SELECT team_user_ids FROM meeting_invitation WHERE activity_id=`+args.add(invitation), args...).Scan(&team)
	if err != nil && !errors.Is(err, pgx.ErrNoRows) {
		return err
	}
	return lockSlotHolders(ctx, tx, host, team)
}

// confirmTeamTx takes the host's page lock and reads each member's answer again
// under it. A member who is no longer confirmed refuses the booking, as a
// paused page does.
func confirmTeamTx(ctx context.Context, tx pgx.Tx, host ids.UserID, team []ids.UUID) error {
	if err := storekit.LockWriteIdentity(ctx, tx, "booking_page", host.String()); err != nil {
		return err
	}
	if len(team) == 0 {
		return nil
	}
	var stored *storedSchedulingProfile
	args := schedulingArgs{}
	err := tx.QueryRow(ctx, `SELECT scheduling_policy FROM booking_page WHERE host_user_id=`+args.add(host)+
		` AND revoked_at IS NULL ORDER BY created_at DESC LIMIT 1`, args...).Scan(&stored)
	if errors.Is(err, pgx.ErrNoRows) || (err == nil && stored == nil) {
		return apperrors.ErrNotFound
	}
	if err != nil {
		return err
	}
	confirmed := confirmedMembers(stored.SchedulingProfile)
	for _, member := range team {
		if !confirmed[member] {
			return apperrors.ErrNotFound
		}
	}
	return nil
}

// confirmedMembers are the members of a page whose own answer is confirmed.
func confirmedMembers(profile crmcontracts.SchedulingProfile) map[ids.UUID]bool {
	out := map[ids.UUID]bool{}
	for _, m := range teamMembers(profile) {
		if memberState(m) == crmcontracts.SchedulingTeamMemberStateConfirmed {
			out[memberUser(m).UUID] = true
		}
	}
	return out
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

// teamOf is the members a booked meeting holds time for, as calendars to check
// a move against. Only a member still confirmed on the host's page counts. A
// member who left the link or the workspace is dropped, and the meeting stands.
// A confirmed member whose calendar cannot be read refuses the move, as the
// host's own calendar would.
func (s *Store) teamOf(ctx context.Context, host ids.UserID, members []ids.UUID) ([]teamCalendar, error) {
	if len(members) == 0 {
		return nil, nil
	}
	page, err := s.hostSchedulingProfile(ctx, host)
	if err != nil {
		return nil, err
	}
	confirmed := confirmedMembers(page)
	team := make([]teamCalendar, 0, len(members))
	for _, member := range members {
		if !confirmed[member] {
			continue
		}
		calendar, standing, err := s.memberCalendar(ctx, ids.From[ids.UserKind](member))
		switch {
		case err != nil:
			return nil, err
		case standing == memberUnreadable:
			return nil, errTeamCalendarUnreadable
		case standing == memberReadable:
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
