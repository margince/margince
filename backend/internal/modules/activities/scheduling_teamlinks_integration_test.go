// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package activities

import (
	"errors"
	"net/http"
	"slices"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	openapi_types "github.com/oapi-codegen/runtime/types"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/platform/datatask"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
	"github.com/margince/margince/backend/internal/shared/ports/connector"
)

// resolve reads a page the way the public door does, by its address.
func (f *teamFixture) resolve(t *testing.T, slug string) BookingPage {
	t.Helper()
	page, err := f.store.ResolveBookingPage(f.door(), slug)
	if err != nil {
		t.Fatalf("resolving %q: %v", slug, err)
	}
	return page
}

func (f *teamFixture) personalSlug(t *testing.T, host ids.UserID) string {
	t.Helper()
	profile, err := f.store.SchedulingProfile(f.as(host))
	if err != nil || profile.Slug == nil {
		t.Fatalf("the personal page of %s: %+v %v", host, profile.Slug, err)
	}
	return *profile.Slug
}

// slotsOn is the times page offers a guest on Monday morning.
func (f *teamFixture) slotsOn(t *testing.T, page BookingPage) []time.Time {
	t.Helper()
	_, open := f.store.openForBooking(f.door(), page)
	if open.err != nil || open.paused {
		t.Fatalf("the page is not open: paused=%v err=%v", open.paused, open.err)
	}
	free, _, err := f.store.PageAvailability(f.door(), page.HostUserID, open.team, monday(9), monday(13), 30*time.Minute)
	if err != nil {
		t.Fatal(err)
	}
	return starts(free)
}

func TestThePersonalLinkOffersItsHostsTimeAloneBesideATeamLink(t *testing.T) {
	f := newTeamFixture(t)
	member := f.colleague(t, "Busy")
	f.calendar.busyOf[member] = []connector.CalendarInterval{{Start: monday(10), End: monday(11)}}
	f.name(t, true, member)

	personal := f.resolve(t, f.personalSlug(t, f.host))
	team := f.resolve(t, f.teamLink(t).Slug)
	if personal.TeamLink || !team.TeamLink || personal.ID == team.ID {
		t.Fatalf("the two addresses resolve to %+v and %+v", personal, team)
	}
	if solo := f.slotsOn(t, personal); !slices.ContainsFunc(solo, monday(10).Equal) {
		t.Fatalf("the personal link withholds a time only the colleague holds: %v", solo)
	}
	joint := f.slotsOn(t, team)
	if slices.ContainsFunc(joint, monday(10).Equal) || !slices.ContainsFunc(joint, monday(9).Equal) {
		t.Fatalf("the team link offers %v, want only times both are free", joint)
	}
	code, meeting := f.bookOn(t, personal, monday(10))
	if code != http.StatusCreated {
		t.Fatalf("a booking on the personal link answered %d", code)
	}
	if row := f.invitationRow(t, ids.UUID(meeting.Id)); len(row.Team) != 0 {
		t.Fatalf("a booking on the personal link holds the colleague's time: %v", row.Team)
	}
}

func TestAColleaguesOwnLinkChangesOnlyByRealBookings(t *testing.T) {
	f := newTeamFixture(t)
	member := f.colleague(t, "Member")
	profile, err := f.store.SchedulingProfile(f.as(member))
	if err != nil {
		t.Fatal(err)
	}
	profile.Enabled = true
	if _, err := f.store.SaveSchedulingProfile(f.as(member), profile); err != nil {
		t.Fatal(err)
	}
	own := f.resolve(t, f.personalSlug(t, member))
	before := f.slotsOn(t, own)

	f.name(t, true, member)
	if named := f.slotsOn(t, own); !slices.Equal(named, before) {
		t.Fatalf("being named on a team link changed the colleague's own times: %v, was %v", named, before)
	}
	profile, err = f.store.SchedulingProfile(f.as(member))
	if err != nil || profile.TeamMembers != nil || *profile.Blocker != crmcontracts.SchedulingProfileBlockerNone {
		t.Fatalf("the colleague's own page reads %+v %v", profile, err)
	}
	if code, _ := f.book(t, monday(11)); code != http.StatusCreated {
		t.Fatalf("a booking on the team link answered %d", code)
	}
	booked := f.slotsOn(t, own)
	if slices.ContainsFunc(booked, monday(11).Equal) || !slices.ContainsFunc(booked, monday(9).Equal) {
		t.Fatalf("after the team booking the colleague's own page offers %v", booked)
	}
	for _, start := range booked {
		if !slices.ContainsFunc(before, start.Equal) {
			t.Fatalf("the colleague's own page gained %s", start)
		}
	}
}

func TestDeletingATeamLinkLeavesThePersonalLinkAsItWas(t *testing.T) {
	f := newTeamFixture(t)
	before, err := f.store.SchedulingProfile(f.ctx)
	if err != nil {
		t.Fatal(err)
	}
	personal := f.resolve(t, *before.Slug)
	solo := f.slotsOn(t, personal)
	member := f.colleague(t, "Member")
	f.calendar.busyOf[member] = []connector.CalendarInterval{{Start: monday(10), End: monday(11)}}
	f.name(t, true, member)
	slug := f.teamLink(t).Slug

	if err := f.store.DeleteSchedulingTeamLink(f.ctx, f.link); err != nil {
		t.Fatal(err)
	}
	after, err := f.store.SchedulingProfile(f.ctx)
	if err != nil {
		t.Fatal(err)
	}
	if *after.Slug != *before.Slug || after.Enabled != before.Enabled || after.Title != before.Title || *after.Blocker != *before.Blocker {
		t.Fatalf("deleting the team link changed the personal page: %+v, was %+v", after, before)
	}
	if now := f.slotsOn(t, personal); !slices.Equal(now, solo) {
		t.Fatalf("deleting the team link changed the personal times: %v, was %v", now, solo)
	}
	if _, err := f.store.ResolveBookingPage(f.door(), slug); !errors.Is(err, apperrors.ErrNotFound) {
		t.Fatalf("a deleted team link still resolves: %v", err)
	}
	if !slices.Contains(f.requests.settled, member) {
		t.Fatalf("the deleted link's request is not settled: %v", f.requests.settled)
	}
	links, err := f.store.SchedulingTeamLinks(f.ctx)
	if err != nil || len(links) != 0 {
		t.Fatalf("the host still lists %+v %v", links, err)
	}
}

func TestTwoTeamLinksAskTheColleagueOnEach(t *testing.T) {
	f := newTeamFixture(t)
	member := f.colleague(t, "Shared")
	first, err := f.store.CreateSchedulingTeamLink(f.ctx, teamLinkRequest(member))
	if err != nil {
		t.Fatal(err)
	}
	second, err := f.store.CreateSchedulingTeamLink(f.ctx, teamLinkRequest(member))
	if err != nil {
		t.Fatal(err)
	}
	asked := f.requests.askedOf(member)
	if len(asked) != 2 || asked[0].Page != ids.UUID(first.Id) || asked[1].Page != ids.UUID(second.Id) || first.Slug == second.Slug {
		t.Fatalf("two team links asked %+v", asked)
	}
	requests, err := f.store.SchedulingTeamRequests(f.as(member))
	if err != nil || len(requests) != 2 {
		t.Fatalf("the colleague sees %+v %v, want a request for each link", requests, err)
	}
}

// legacyTeam puts members with their own answers on the host's personal page.
// That is how a team was stored before team links had pages of their own.
func (f *teamFixture) legacyTeam(t *testing.T, members []crmcontracts.SchedulingTeamMember) {
	t.Helper()
	tag, err := f.env.owner.Exec(f.ctx, `UPDATE booking_page SET scheduling_policy = jsonb_set(scheduling_policy, '{team_members}', to_jsonb($1::json))
 WHERE host_user_id=$2 AND revoked_at IS NULL AND NOT team_link`, members, f.host.UUID)
	if err != nil || tag.RowsAffected() != 1 {
		t.Fatalf("storing the personal team: %v rows, %v", tag.RowsAffected(), err)
	}
}

func (f *teamFixture) moveTeams(t *testing.T) datatask.BatchOut {
	t.Helper()
	system := principal.SystemActing(f.ctx, "system:datatask")
	var out datatask.BatchOut
	err := f.store.tx(system, func(tx pgx.Tx) error {
		var err error
		out, err = f.store.MoveTeamsToTeamLinksTx(system, tx, datatask.BatchIn{Limit: 10}, monday(6))
		return err
	})
	if err != nil {
		t.Fatal(err)
	}
	return out
}

func TestTheDataTaskMovesAPersonalTeamOntoATeamLink(t *testing.T) {
	f := newTeamFixture(t)
	confirmed, pending := f.colleague(t, "Confirmed"), f.colleague(t, "Pending")
	f.calendar.busyOf[confirmed] = []connector.CalendarInterval{{Start: monday(10), End: monday(11)}}
	yes, waiting := crmcontracts.SchedulingTeamMemberStateConfirmed, crmcontracts.SchedulingTeamMemberStatePending
	f.legacyTeam(t, []crmcontracts.SchedulingTeamMember{
		{UserId: openapi_types.UUID(confirmed.UUID), State: &yes}, {UserId: openapi_types.UUID(pending.UUID), State: &waiting},
	})
	slug := f.personalSlug(t, f.host)

	if out := f.moveTeams(t); out.Changed != 1 {
		t.Fatalf("the task moved %d teams, want 1", out.Changed)
	}
	links, err := f.store.SchedulingTeamLinks(f.ctx)
	if err != nil || len(links) != 1 {
		t.Fatalf("the host's team links: %+v %v", links, err)
	}
	if states := memberStates(links[0].TeamMembers); !slices.Equal(states, []string{"confirmed", "pending"}) || links[0].Slug == slug {
		t.Fatalf("the moved team link reads %+v", links[0])
	}
	if asked := f.requests.askedOf(pending); len(asked) != 1 || asked[0].Page != ids.UUID(links[0].Id) || len(f.requests.askedOf(confirmed)) != 0 {
		t.Fatalf("the move asked %+v, want the pending member asked once on the new link", f.requests.asked)
	}
	personal := f.resolve(t, slug)
	if personal.TeamLink || f.personalSlug(t, f.host) != slug {
		t.Fatalf("the old address no longer names the personal page: %+v", personal)
	}
	if solo := f.slotsOn(t, personal); !slices.ContainsFunc(solo, monday(10).Equal) {
		t.Fatalf("the personal page still reads the moved team: %v", solo)
	}
	if out := f.moveTeams(t); out.Changed != 0 {
		t.Fatalf("a second run moved %d teams again", out.Changed)
	}
}

func (f *teamFixture) bookings(t *testing.T) int {
	t.Helper()
	var count int
	if err := f.env.owner.QueryRow(f.ctx, `SELECT count(*) FROM meeting_invitation WHERE host_user_id=$1`, f.host.UUID).Scan(&count); err != nil {
		t.Fatal(err)
	}
	return count
}

// A guest books the team the page offered when they opened it. The booking is
// refused when, before it commits, the host switched the link off or named a
// colleague who has not confirmed.
func TestABookingRefusesALinkThatChangedAfterItOpened(t *testing.T) {
	for name, change := range map[string]func(f *teamFixture, member ids.UserID){
		"switched off": func(f *teamFixture, member ids.UserID) {
			request := teamLinkRequest(member)
			request.Enabled = new(false)
			if _, err := f.store.UpdateSchedulingTeamLink(f.ctx, f.link, request); err != nil {
				t.Fatal(err)
			}
		},
		"a pending member added": func(f *teamFixture, member ids.UserID) {
			f.name(t, false, member, f.colleague(t, "Newcomer"))
		},
	} {
		t.Run(name, func(t *testing.T) {
			f := newTeamFixture(t)
			member := f.colleague(t, "Member")
			f.name(t, true, member)
			team := f.openTeam(t)
			change(f, member)
			request := f.request
			request.Start, request.End = monday(11), monday(11).Add(30*time.Minute)
			_, err := f.store.reserveAndQueueInvitation(f.door(), f.host, request, invitationIntent{DescriptionFromBooker: true, Team: team, Page: f.link})
			if !errors.Is(err, apperrors.ErrNotFound) {
				t.Fatalf("a booking on a link that changed after it opened: %v", err)
			}
			if got := f.bookings(t); got != 0 {
				t.Fatalf("%d meetings were booked, want none", got)
			}
		})
	}
}

// A meeting reads the calendars of the link that booked it. A colleague who
// left that link is not read for a move, though another link still names them.
func TestAMoveReadsOnlyTheLinkThatBookedTheMeeting(t *testing.T) {
	f := newTeamFixture(t)
	alice := f.colleague(t, "Alice")
	f.name(t, true, alice)
	sales := f.link
	support, err := f.store.CreateSchedulingTeamLink(f.ctx, teamLinkRequest(alice))
	if err != nil {
		t.Fatal(err)
	}
	if err := f.store.AnswerSchedulingTeamRequest(f.as(alice), ids.UUID(support.Id), crmcontracts.SchedulingTeamAnswerStateConfirmed); err != nil {
		t.Fatal(err)
	}
	meeting := f.bookAndDeliver(t, monday(11))
	if row := f.invitationRow(t, ids.UUID(meeting.Id)); row.Page == nil || *row.Page != sales {
		t.Fatalf("the meeting names page %v, want the link that booked it", row.Page)
	}
	f.calendar.busyOf[alice] = []connector.CalendarInterval{{Start: monday(13), End: monday(14)}}
	if err := f.store.AnswerSchedulingTeamRequest(f.as(alice), sales, crmcontracts.SchedulingTeamAnswerStateLeft); err != nil {
		t.Fatal(err)
	}
	free, _, err := f.store.invitationAvailability(f.ctx, f.host, ids.UUID(meeting.Id), monday(12), monday(15))
	if err != nil {
		t.Fatal(err)
	}
	if !slices.ContainsFunc(starts(free), monday(13).Equal) {
		t.Fatalf("a colleague who left the booking link still holds the move back: %v", starts(free))
	}
}

// Taking an omitted switch as on would turn a link the host switched off back on.
func TestAnUpdateWithoutTheSwitchKeepsItOff(t *testing.T) {
	f := newTeamFixture(t)
	member := f.colleague(t, "Member")
	f.name(t, true, member)
	off := teamLinkRequest(member)
	off.Enabled = new(false)
	if _, err := f.store.UpdateSchedulingTeamLink(f.ctx, f.link, off); err != nil {
		t.Fatal(err)
	}
	renamed := teamLinkRequest(member)
	renamed.Title = "Renamed"
	link, err := f.store.UpdateSchedulingTeamLink(f.ctx, f.link, renamed)
	if err != nil || link.Enabled || link.Title != "Renamed" {
		t.Fatalf("an update that left out the switch reads %+v, %v; want it still off", link, err)
	}
}

// A pending member's line follows the link's name; a confirmed member's does not.
func TestARenamedLinkRewordsOnlyThePendingRequests(t *testing.T) {
	f := newTeamFixture(t)
	confirmed, pending := f.colleague(t, "Confirmed"), f.colleague(t, "Pending")
	f.name(t, false, confirmed, pending)
	if err := f.store.AnswerSchedulingTeamRequest(f.as(confirmed), f.link, crmcontracts.SchedulingTeamAnswerStateConfirmed); err != nil {
		t.Fatal(err)
	}
	renamed := teamLinkRequest(confirmed, pending)
	renamed.Title = "Renewal call"
	if _, err := f.store.UpdateSchedulingTeamLink(f.ctx, f.link, renamed); err != nil {
		t.Fatal(err)
	}
	if got := f.requests.retitled; len(got) != 1 || got[0].Member != pending || got[0].Title != "Renewal call" || got[0].Page != f.link {
		t.Fatalf("the rename reworded %+v, want only the pending member's line", got)
	}
	if _, err := f.store.UpdateSchedulingTeamLink(f.ctx, f.link, renamed); err != nil {
		t.Fatal(err)
	}
	if len(f.requests.retitled) != 1 {
		t.Fatalf("a save that kept the name reworded again: %+v", f.requests.retitled)
	}
}
