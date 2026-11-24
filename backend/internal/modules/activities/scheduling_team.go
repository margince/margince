// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package activities

// A team booking link is a booking page of its own beside its host's personal
// page. It names colleagues who attend every meeting booked through it. The
// members live in the page's scheduling_policy, each with their own answer. A
// page takes bookings only while every member has confirmed and Margince can
// read their calendar.

import (
	"context"
	"errors"
	"slices"

	"github.com/jackc/pgx/v5"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/workforce"
)

const (
	maxTeamMembers   = 5
	fieldTeamMembers = "team_members"
	fieldTeamAnswer  = "state"
)

// TeamRequestNotice tells a colleague that a host named them on a team link, in
// the same save. It takes the line back once they answer or leave the list.
// Each link asks on its own, so a colleague on two links hears of both.
type TeamRequestNotice interface {
	TeamRequestTx(ctx context.Context, tx pgx.Tx, request TeamRequest) error
	TeamRequestSettledTx(ctx context.Context, tx pgx.Tx, page ids.UUID, member ids.UserID) error
}

// TeamRequest is a host's request that a colleague join one of their team links.
type TeamRequest struct {
	Page            ids.UUID
	Host, Member    ids.UserID
	HostName, Title string
}

// WithTeamRequestNotice binds the line a colleague gets when named on a link.
func (s *Store) WithTeamRequestNotice(notice TeamRequestNotice) *Store {
	clone := *s
	clone.teamRequests = notice
	return &clone
}

// WithTeamRequestNotice binds the line a colleague gets when named on a link.
func (h Handlers) WithTeamRequestNotice(notice TeamRequestNotice) Handlers {
	h.store = h.store.WithTeamRequestNotice(notice)
	return h
}

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

// memberStanding is whether a page can read a colleague's calendar now.
type memberStanding int

const (
	// memberGone is a colleague who left the workspace.
	memberGone memberStanding = iota
	// memberUnreadable is a colleague whose own settings name no calendar
	// connection Margince can read.
	memberUnreadable
	memberReadable
)

// memberCalendar reads a colleague's calendar settings and whether a page may
// read them. A read-only grant is enough, since a member's calendar is only read.
func (s *Store) memberCalendar(ctx context.Context, member ids.UserID) (teamCalendar, memberStanding, error) {
	var address string
	err := s.tx(ctx, func(tx pgx.Tx) error {
		args := schedulingArgs{}
		return tx.QueryRow(ctx, `SELECT u.email FROM app_user u WHERE u.id=`+args.add(member)+` AND `+workforce.LiveHumanSQL("u"), args...).Scan(&address)
	})
	if errors.Is(err, pgx.ErrNoRows) {
		return teamCalendar{}, memberGone, nil
	}
	if err != nil {
		return teamCalendar{}, memberGone, err
	}
	profile, err := s.hostSchedulingProfile(ctx, member)
	if err != nil || profile.Provider == "" || s.calendar == nil {
		return teamCalendar{}, memberUnreadable, err
	}
	blocker, err := s.calendar.Blocker(ctx, member, string(profile.Provider))
	if err != nil {
		return teamCalendar{}, memberUnreadable, err
	}
	if blocker != crmcontracts.SchedulingProfileBlockerNone && blocker != crmcontracts.SchedulingProfileBlockerCalendarReadOnly {
		return teamCalendar{}, memberUnreadable, nil
	}
	return teamCalendar{User: member, Address: address, Profile: profile}, memberReadable, nil
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
		calendar, standing, err := s.memberCalendar(ctx, memberUser(m))
		if err != nil || standing != memberReadable {
			return nil, false, err
		}
		team = append(team, calendar)
	}
	return team, true, nil
}

// pageReadiness is why a standing page takes no bookings, its host's calendar
// first and then its team. A ready page also answers its team, read once.
func (s *Store) pageReadiness(ctx context.Context, host ids.UserID, profile crmcontracts.SchedulingProfile) (crmcontracts.SchedulingProfileBlocker, []teamCalendar, error) {
	blocker, err := s.calendarBlocker(ctx, host, profile)
	if err != nil || blocker != crmcontracts.SchedulingProfileBlockerNone {
		return blocker, nil, err
	}
	team, ready, err := s.teamCalendars(ctx, profile)
	if err != nil || !ready {
		return crmcontracts.SchedulingProfileBlockerTeamNotReady, nil, err
	}
	return crmcontracts.SchedulingProfileBlockerNone, team, nil
}

func (s *Store) pageBlocker(ctx context.Context, host ids.UserID, profile crmcontracts.SchedulingProfile) (crmcontracts.SchedulingProfileBlocker, error) {
	blocker, _, err := s.pageReadiness(ctx, host, profile)
	return blocker, err
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
			if _, standing, err := s.memberCalendar(ctx, memberUser(m)); err != nil {
				return err
			} else if standing != memberReadable {
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

// admitTeam refuses a member list the owner may not save on a team link. Only a
// member new to the list is checked for a readable calendar: one already on it
// keeps its place and shows its own state.
func (s *Store) admitTeam(ctx context.Context, host ids.UserID, stored, next crmcontracts.SchedulingProfile) error {
	if len(teamMembers(next)) == 0 {
		return &SchedulingArgumentError{Field: fieldTeamMembers, Code: codeRequired, Message: "Name at least one colleague on a team link"}
	}
	if len(*next.TeamMembers) > maxTeamMembers {
		return &SchedulingArgumentError{Field: fieldTeamMembers, Code: faultTooMany, Max: maxTeamMembers, Message: "Name at most five colleagues on one team link"}
	}
	seen := map[ids.UUID]bool{}
	for _, m := range *next.TeamMembers {
		user := memberUser(m)
		if user == host || seen[user.UUID] {
			return &SchedulingArgumentError{Field: fieldTeamMembers, Code: faultInvalid, Message: "Name each colleague once, and not yourself"}
		}
		seen[user.UUID] = true
		if slices.ContainsFunc(teamMembers(stored), sameMember(m)) {
			continue
		}
		if _, standing, err := s.memberCalendar(ctx, user); err != nil {
			return err
		} else if standing != memberReadable {
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
		if i := slices.IndexFunc(was, sameMember(m)); i >= 0 {
			state = memberState(was[i])
		}
		merged = append(merged, crmcontracts.SchedulingTeamMember{UserId: m.UserId, State: &state})
	}
	return &merged
}

// tellTeamChanges asks each colleague new to the list to confirm, and settles
// the request of each one taken off it. A member kept on the list hears nothing.
func (s *Store) tellTeamChanges(ctx context.Context, tx pgx.Tx, host ids.UserID, page ids.UUID, before *storedSchedulingProfile, after crmcontracts.SchedulingProfile) error {
	if s.teamRequests == nil {
		return nil
	}
	var was []crmcontracts.SchedulingTeamMember
	if before != nil {
		was = teamMembers(before.SchedulingProfile)
	}
	named := teamMembers(after)
	for _, m := range was {
		if !slices.ContainsFunc(named, sameMember(m)) {
			if err := s.teamRequests.TeamRequestSettledTx(ctx, tx, page, memberUser(m)); err != nil {
				return err
			}
		}
	}
	hostName := ""
	for _, m := range named {
		if slices.ContainsFunc(was, sameMember(m)) {
			continue
		}
		if hostName == "" {
			name, err := schedulingAccountName(ctx, tx, host)
			if err != nil {
				return err
			}
			hostName = name
		}
		request := TeamRequest{Page: page, Host: host, Member: memberUser(m), HostName: hostName, Title: after.Title}
		if err := s.teamRequests.TeamRequestTx(ctx, tx, request); err != nil {
			return err
		}
	}
	return nil
}

func sameMember(m crmcontracts.SchedulingTeamMember) func(crmcontracts.SchedulingTeamMember) bool {
	return func(other crmcontracts.SchedulingTeamMember) bool { return other.UserId == m.UserId }
}
