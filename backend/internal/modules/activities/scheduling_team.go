// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package activities

// A team booking link is a host's standing booking page that names colleagues
// who attend every meeting booked through it. The members live in the page's
// scheduling_policy, each with their own answer. A page takes bookings only
// while every member has confirmed and Margince can read their calendar.

import (
	"context"
	"errors"
	"slices"

	"github.com/jackc/pgx/v5"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/platform/database/storekit"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/workforce"
)

const (
	maxTeamMembers   = 5
	fieldTeamMembers = "team_members"
	fieldTeamAnswer  = "state"
)

// teamCalendar is a member whose calendar a page may read: their invited
// address and their own booking settings, which name the calendars to read.
type teamCalendar struct {
	User    ids.UserID
	Address string
	Profile crmcontracts.SchedulingProfile
}

func teamMembers(p crmcontracts.SchedulingProfile) []crmcontracts.SchedulingTeamMember {
	if p.TeamMembers == nil {
		return nil
	}
	return *p.TeamMembers
}

func memberState(m crmcontracts.SchedulingTeamMember) crmcontracts.SchedulingTeamMemberState {
	if m.State == nil {
		return crmcontracts.SchedulingTeamMemberStatePending
	}
	return *m.State
}

func memberUser(m crmcontracts.SchedulingTeamMember) ids.UserID {
	return ids.From[ids.UserKind](ids.UUID(m.UserId))
}

// memberCalendar answers false when the colleague left the workspace or their
// own settings name no calendar connection Margince can read. A read-only
// grant is enough, since a member's calendar is only read.
func (s *Store) memberCalendar(ctx context.Context, member ids.UserID) (teamCalendar, bool, error) {
	var address string
	err := s.tx(ctx, func(tx pgx.Tx) error {
		args := schedulingArgs{}
		return tx.QueryRow(ctx, `SELECT u.email FROM app_user u WHERE u.id=`+args.add(member)+` AND `+workforce.LiveHumanSQL("u"), args...).Scan(&address)
	})
	if errors.Is(err, pgx.ErrNoRows) {
		return teamCalendar{}, false, nil
	}
	if err != nil {
		return teamCalendar{}, false, err
	}
	profile, err := s.hostSchedulingProfile(ctx, member)
	if err != nil || profile.Provider == "" || s.calendar == nil {
		return teamCalendar{}, false, err
	}
	blocker, err := s.calendar.Blocker(ctx, member, string(profile.Provider))
	if err != nil {
		return teamCalendar{}, false, err
	}
	readable := blocker == crmcontracts.SchedulingProfileBlockerNone || blocker == crmcontracts.SchedulingProfileBlockerCalendarReadOnly
	return teamCalendar{User: member, Address: address, Profile: profile}, readable, nil
}

// teamCalendars answers every member's calendar, and false when a member has
// not confirmed or cannot be read. A page without members is ready.
func (s *Store) teamCalendars(ctx context.Context, profile crmcontracts.SchedulingProfile) ([]teamCalendar, bool, error) {
	members := teamMembers(profile)
	team := make([]teamCalendar, 0, len(members))
	for _, m := range members {
		if memberState(m) != crmcontracts.SchedulingTeamMemberStateConfirmed {
			return nil, false, nil
		}
		calendar, ok, err := s.memberCalendar(ctx, memberUser(m))
		if err != nil || !ok {
			return nil, false, err
		}
		team = append(team, calendar)
	}
	return team, true, nil
}

// pageTeam is the team of a page about to take a booking. A team that stopped
// being ready since the page was opened answers as a paused page does.
func (s *Store) pageTeam(ctx context.Context, profile crmcontracts.SchedulingProfile) ([]teamCalendar, error) {
	team, ready, err := s.teamCalendars(ctx, profile)
	if err != nil {
		return nil, err
	}
	if !ready {
		return nil, apperrors.ErrNotFound
	}
	return team, nil
}

// pageBlocker is why a standing page takes no bookings: its host's calendar
// first, then its team.
func (s *Store) pageBlocker(ctx context.Context, host ids.UserID, profile crmcontracts.SchedulingProfile) (crmcontracts.SchedulingProfileBlocker, error) {
	blocker, err := s.calendarBlocker(ctx, host, profile)
	if err != nil || blocker != crmcontracts.SchedulingProfileBlockerNone {
		return blocker, err
	}
	if _, ready, err := s.teamCalendars(ctx, profile); err != nil || !ready {
		return crmcontracts.SchedulingProfileBlockerTeamNotReady, err
	}
	return crmcontracts.SchedulingProfileBlockerNone, nil
}

// describeTeam gives the owner each member's name and the state to act on. A
// member who cannot be read shows as unavailable, unless they left.
func (s *Store) describeTeam(ctx context.Context, profile *crmcontracts.SchedulingProfile) error {
	members := slices.Clone(teamMembers(*profile))
	for i, m := range members {
		name, err := s.memberName(ctx, memberUser(m))
		if err != nil {
			return err
		}
		state := memberState(m)
		if state != crmcontracts.SchedulingTeamMemberStateLeft {
			if _, ok, err := s.memberCalendar(ctx, memberUser(m)); err != nil {
				return err
			} else if !ok {
				state = crmcontracts.SchedulingTeamMemberStateUnavailable
			}
		}
		members[i].Name, members[i].State = &name, &state
	}
	if profile.TeamMembers != nil {
		profile.TeamMembers = &members
	}
	return nil
}

// memberName reads the colleague's name whatever their status, so a member
// who left the workspace is still named to the owner.
func (s *Store) memberName(ctx context.Context, member ids.UserID) (string, error) {
	var name string
	err := s.tx(ctx, func(tx pgx.Tx) error {
		var err error
		name, err = schedulingAccountName(ctx, tx, member)
		return err
	})
	return name, err
}

// admitTeam refuses a member list the owner may not save. Only a member new to
// the list is checked for a readable calendar: one already on it keeps its
// place and shows its own state.
func (s *Store) admitTeam(ctx context.Context, host ids.UserID, stored, next crmcontracts.SchedulingProfile) error {
	if next.TeamMembers == nil {
		return nil
	}
	if len(*next.TeamMembers) > maxTeamMembers {
		return &SchedulingArgumentError{Field: fieldTeamMembers, Code: faultTooMany, Max: maxTeamMembers, Message: "Name at most five colleagues on one booking link"}
	}
	seen := map[ids.UUID]bool{}
	for _, m := range *next.TeamMembers {
		user := memberUser(m)
		if user == host || seen[user.UUID] {
			return &SchedulingArgumentError{Field: fieldTeamMembers, Code: faultInvalid, Message: "Name each colleague once, and not yourself"}
		}
		seen[user.UUID] = true
		if slices.ContainsFunc(teamMembers(stored), func(was crmcontracts.SchedulingTeamMember) bool { return was.UserId == m.UserId }) {
			continue
		}
		if _, ok, err := s.memberCalendar(ctx, user); err != nil {
			return err
		} else if !ok {
			return &SchedulingArgumentError{Field: fieldTeamMembers, Code: "unavailable", Message: "Name only colleagues whose calendar Margince can read"}
		}
	}
	return nil
}

// mergeTeam keeps each kept member's own answer; a new member starts pending.
// The owner never writes a member's answer, and an absent list keeps the old.
func mergeTeam(before *storedSchedulingProfile, next *[]crmcontracts.SchedulingTeamMember) *[]crmcontracts.SchedulingTeamMember {
	var was []crmcontracts.SchedulingTeamMember
	if before != nil {
		was = teamMembers(before.SchedulingProfile)
	}
	if next == nil {
		if was == nil {
			return nil
		}
		return &was
	}
	merged := make([]crmcontracts.SchedulingTeamMember, 0, len(*next))
	for _, m := range *next {
		state := crmcontracts.SchedulingTeamMemberStatePending
		if i := slices.IndexFunc(was, func(old crmcontracts.SchedulingTeamMember) bool { return old.UserId == m.UserId }); i >= 0 {
			state = memberState(was[i])
		}
		merged = append(merged, crmcontracts.SchedulingTeamMember{UserId: m.UserId, State: &state})
	}
	return &merged
}

// teamAudit is the member list as the History tab shows it.
type teamAudit struct {
	TeamMembers []crmcontracts.SchedulingTeamMember `json:"team_members"`
}

// AnswerSchedulingTeamRequest records the acting colleague's answer on the
// host's page that names them. Confirming needs a calendar Margince can read.
func (s *Store) AnswerSchedulingTeamRequest(ctx context.Context, host ids.UserID, answer crmcontracts.SchedulingTeamAnswerState) error {
	me, err := schedulingHost(ctx)
	if err != nil {
		return err
	}
	if !answer.Valid() {
		return &SchedulingArgumentError{Field: fieldTeamAnswer, Code: faultInvalid, Message: "Answer confirmed or left"}
	}
	if answer == crmcontracts.SchedulingTeamAnswerStateConfirmed {
		if _, ok, err := s.memberCalendar(ctx, me); err != nil {
			return err
		} else if !ok {
			return errCalendarNotConnected()
		}
	}
	return s.tx(ctx, func(tx pgx.Tx) error {
		if err := storekit.LockWriteIdentity(ctx, tx, "booking_page", host.String()); err != nil {
			return err
		}
		var page ids.UUID
		var stored *storedSchedulingProfile
		args := schedulingArgs{}
		err := tx.QueryRow(ctx, `SELECT id, scheduling_policy FROM booking_page WHERE host_user_id=`+args.add(host)+
			` AND revoked_at IS NULL ORDER BY created_at DESC LIMIT 1 FOR UPDATE`, args...).Scan(&page, &stored)
		if errors.Is(err, pgx.ErrNoRows) || (err == nil && stored == nil) {
			return apperrors.ErrNotFound
		}
		if err != nil {
			return err
		}
		before := slices.Clone(teamMembers(stored.SchedulingProfile))
		i := slices.IndexFunc(before, func(m crmcontracts.SchedulingTeamMember) bool { return memberUser(m) == me })
		if i < 0 {
			return apperrors.ErrNotFound
		}
		state := crmcontracts.SchedulingTeamMemberState(answer)
		if memberState(before[i]) == state {
			return nil
		}
		after := slices.Clone(before)
		after[i].State = &state
		stored.TeamMembers = &after
		args = schedulingArgs{}
		if _, err := tx.Exec(ctx, `UPDATE booking_page SET scheduling_policy=`+args.add(stored)+` WHERE id=`+args.add(page), args...); err != nil {
			return err
		}
		audit, err := storekit.Audit(ctx, tx, "update", "booking_page", page, teamAudit{before}, teamAudit{after})
		if err != nil {
			return err
		}
		return storekit.EmitEvent(ctx, tx, audit, page, crmcontracts.InternalEventBookingPageUpdated{Enabled: stored.Enabled})
	})
}
