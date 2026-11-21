// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package activities

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"slices"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	openapi_types "github.com/oapi-codegen/runtime/types"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
	"github.com/margince/margince/backend/internal/shared/ports/connector"
)

// teamCalendarFake answers each colleague's own busy times and readability.
type teamCalendarFake struct {
	*invitationCalendar
	busyOf     map[ids.UserID][]connector.CalendarInterval
	unreadable map[ids.UserID]bool
}

func (c *teamCalendarFake) Busy(ctx context.Context, user ids.UserID, provider, calendar string, from, to time.Time) ([]connector.CalendarInterval, error) {
	if busy, ok := c.busyOf[user]; ok {
		return busy, nil
	}
	return c.invitationCalendar.Busy(ctx, user, provider, calendar, from, to)
}

func (c *teamCalendarFake) Blocker(ctx context.Context, user ids.UserID, provider string) (crmcontracts.SchedulingProfileBlocker, error) {
	if c.unreadable[user] {
		return crmcontracts.SchedulingProfileBlockerCalendarUnavailable, nil
	}
	return c.invitationCalendar.Blocker(ctx, user, provider)
}

type teamFixture struct {
	*invitationFixture
	calendar *teamCalendarFake
	hours    map[ids.UserID]WorkingHours
	host     ids.UserID
}

func newTeamFixture(t *testing.T) *teamFixture {
	t.Helper()
	f := &teamFixture{invitationFixture: newInvitationFixture(t), hours: map[ids.UserID]WorkingHours{}}
	f.host = ids.From[ids.UserKind](f.env.rep)
	f.calendar = &teamCalendarFake{invitationCalendar: f.invitationFixture.calendar, busyOf: map[ids.UserID][]connector.CalendarInterval{}, unreadable: map[ids.UserID]bool{}}
	f.store = f.store.WithSchedulingCalendar(f.calendar).WithWorkingHours(func(_ context.Context, user ids.UserID) (WorkingHours, error) {
		if hours, ok := f.hours[user]; ok {
			return hours, nil
		}
		return fallbackWorkingHours(), nil
	})
	profile, err := f.store.SchedulingProfile(f.ctx)
	if err != nil {
		t.Fatal(err)
	}
	profile.Enabled = true
	if _, err := f.store.SaveSchedulingProfile(f.ctx, profile); err != nil {
		t.Fatal(err)
	}
	return f
}

// colleague adds a live colleague whose own booking settings name a calendar.
func (f *teamFixture) colleague(t *testing.T, name string) ids.UserID {
	t.Helper()
	user := ids.NewV7()
	if _, err := f.env.owner.Exec(f.ctx, `INSERT INTO app_user (id, email, display_name) VALUES ($1,$2,$3)`, user, "member-"+user.String()+"@send.test", name); err != nil {
		t.Fatal(err)
	}
	member := ids.From[ids.UserKind](user)
	profile := defaultSchedulingProfile()
	profile.Provider = "gcal"
	if _, err := f.store.SaveSchedulingProfile(f.as(member), profile); err != nil {
		t.Fatal(err)
	}
	return member
}

func (f *teamFixture) as(user ids.UserID) context.Context {
	actor, _ := principal.Actor(f.ctx)
	actor.UserID, actor.ID = user.UUID, "human:"+user.String()
	return principal.WithActor(f.ctx, actor)
}

func (f *teamFixture) door() context.Context {
	return principal.SystemActing(f.ctx, "system:public_booking")
}

// name puts members on the host's link; each confirms when confirm says so.
func (f *teamFixture) name(t *testing.T, confirm bool, members ...ids.UserID) {
	t.Helper()
	profile, err := f.store.SchedulingProfile(f.ctx)
	if err != nil {
		t.Fatal(err)
	}
	team := []crmcontracts.SchedulingTeamMember{}
	for _, member := range members {
		team = append(team, crmcontracts.SchedulingTeamMember{UserId: openapi_types.UUID(member.UUID)})
	}
	profile.TeamMembers = &team
	if _, err := f.store.SaveSchedulingProfile(f.ctx, profile); err != nil {
		t.Fatal(err)
	}
	if !confirm {
		return
	}
	for _, member := range members {
		if err := f.store.AnswerSchedulingTeamRequest(f.as(member), f.host, crmcontracts.SchedulingTeamAnswerStateConfirmed); err != nil {
			t.Fatal(err)
		}
	}
}

func (f *teamFixture) slots(t *testing.T, from, to time.Time) []slot {
	t.Helper()
	free, _, err := f.store.PageAvailability(f.door(), f.host, from, to, 30*time.Minute)
	if err != nil {
		t.Fatal(err)
	}
	return free
}

func (f *teamFixture) blocker(t *testing.T) crmcontracts.SchedulingProfileBlocker {
	t.Helper()
	profile, err := f.store.SchedulingProfile(f.ctx)
	if err != nil {
		t.Fatal(err)
	}
	return *profile.Blocker
}

// book books start through the public page and answers the HTTP status.
func (f *teamFixture) book(t *testing.T, start time.Time) (int, crmcontracts.MeetingInvitation) {
	t.Helper()
	var req crmcontracts.BookPublicMeetingJSONRequestBody
	body := fmt.Sprintf(`{"booker":{"name":"Guest","email":"guest@example.test"},"start":%q,"end":%q,"consent":{"policy_version":"1"}}`,
		start.Format(time.RFC3339), start.Add(30*time.Minute).Format(time.RFC3339))
	if err := json.Unmarshal([]byte(body), &req); err != nil {
		t.Fatal(err)
	}
	response := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodPost, "/", nil).WithContext(f.door())
	out, ok := Handlers{store: f.store}.reservePublicInvitation(response, request, BookingPage{HostUserID: f.host}, req, ids.UUID(f.request.ContactId), invitationIntent{DescriptionFromBooker: true}, MarketingNotRequested, nil)
	if ok {
		return http.StatusCreated, out
	}
	return response.Code, out
}

func starts(free []slot) []time.Time {
	out := make([]time.Time, 0, len(free))
	for _, s := range free {
		out = append(out, s.Start)
	}
	return out
}

func TestATeamLinkOffersOnlyTheTimesEveryMemberIsFree(t *testing.T) {
	f := newTeamFixture(t)
	busy := f.colleague(t, "Busy Colleague")
	f.calendar.busyOf[busy] = []connector.CalendarInterval{{Start: monday(10), End: monday(11)}}
	f.name(t, true, busy)

	free := starts(f.slots(t, monday(9), monday(12)))
	for _, taken := range []time.Time{monday(10), monday(10).Add(30 * time.Minute)} {
		if slices.ContainsFunc(free, taken.Equal) {
			t.Fatalf("the page offers %s, when the member is busy: %v", taken, free)
		}
	}
	if !slices.ContainsFunc(free, monday(9).Equal) || !slices.ContainsFunc(free, monday(11).Equal) {
		t.Fatalf("the page withholds times every member is free: %v", free)
	}
	if code, _ := f.book(t, monday(10)); code != http.StatusConflict {
		t.Fatalf("a booking over the member's meeting answered %d, want 409", code)
	}
	code, meeting := f.book(t, monday(11))
	if code != http.StatusCreated {
		t.Fatalf("a free time answered %d", code)
	}
	row := f.invitationRow(t, ids.UUID(meeting.Id))
	if !slices.Equal(row.Appointment.Team, []ids.UUID{busy.UUID}) || !slices.Contains(row.Appointment.Attendees, "member-"+busy.String()+"@send.test") {
		t.Fatalf("the booking does not invite the member: team=%v attendees=%v", row.Appointment.Team, row.Appointment.Attendees)
	}
}

func (f *teamFixture) invitationRow(t *testing.T, id ids.UUID) invitationRow {
	t.Helper()
	var row invitationRow
	if err := f.store.tx(f.ctx, func(tx pgx.Tx) error {
		var err error
		row, err = readInvitation(f.ctx, tx, id, false)
		return err
	}); err != nil {
		t.Fatal(err)
	}
	return row
}

func TestThreeMembersLeaveOnlyTheTimeNoneOfThemHolds(t *testing.T) {
	f := newTeamFixture(t)
	a, b, c := f.colleague(t, "A"), f.colleague(t, "B"), f.colleague(t, "C")
	f.calendar.busyOf[a] = []connector.CalendarInterval{{Start: monday(9), End: monday(10)}}
	f.calendar.busyOf[b] = []connector.CalendarInterval{{Start: monday(10), End: monday(11)}}
	f.calendar.busyOf[c] = []connector.CalendarInterval{{Start: monday(11), End: monday(12)}}
	f.name(t, true, a, b, c)

	free := starts(f.slots(t, monday(9), monday(13)))
	if len(free) == 0 || !free[0].Equal(monday(12)) {
		t.Fatalf("the first free time is not noon, when the three members fill the morning: %v", free)
	}
}

func TestEachMemberIsReadInTheirOwnTimeZone(t *testing.T) {
	f := newTeamFixture(t)
	newYork, err := time.LoadLocation("America/New_York")
	if err != nil {
		t.Fatal(err)
	}
	member := f.colleague(t, "Abroad")
	f.hours[member] = WorkingHours{StartMinute: 9 * 60, EndMinute: 17 * 60, Days: []int{1, 2, 3, 4, 5}, Location: newYork}
	f.name(t, true, member)

	free := starts(f.slots(t, monday(9), monday(17)))
	// 09:00 in New York is 13:00 UTC in August; the host keeps 09:00 to 17:00 UTC.
	if len(free) == 0 || !free[0].Equal(monday(13)) {
		t.Fatalf("the first time is not 09:00 in New York: %v", free)
	}
}

func TestATeamLinkWaitsForEveryMemberToConfirm(t *testing.T) {
	f := newTeamFixture(t)
	first, second := f.colleague(t, "First"), f.colleague(t, "Second")
	f.name(t, false, first, second)
	if got := f.blocker(t); got != crmcontracts.SchedulingProfileBlockerTeamNotReady {
		t.Fatalf("a link no member confirmed reads %q", got)
	}
	if _, _, err := f.store.PageAvailability(f.door(), f.host, monday(9), monday(12), 30*time.Minute); !errors.Is(err, apperrors.ErrNotFound) {
		t.Fatalf("an unconfirmed team link offered times: %v", err)
	}
	if err := f.store.AnswerSchedulingTeamRequest(f.as(first), f.host, crmcontracts.SchedulingTeamAnswerStateConfirmed); err != nil {
		t.Fatal(err)
	}
	if got := f.blocker(t); got != crmcontracts.SchedulingProfileBlockerTeamNotReady {
		t.Fatalf("a link one of two members confirmed reads %q", got)
	}
	if err := f.store.AnswerSchedulingTeamRequest(f.as(second), f.host, crmcontracts.SchedulingTeamAnswerStateConfirmed); err != nil {
		t.Fatal(err)
	}
	if got := f.blocker(t); got != crmcontracts.SchedulingProfileBlockerNone {
		t.Fatalf("a link every member confirmed reads %q", got)
	}
	requests, err := f.store.SchedulingTeamRequests(f.as(first))
	if err != nil || len(requests) != 1 || requests[0].State != crmcontracts.SchedulingTeamRequestStateConfirmed || ids.UUID(requests[0].HostUserId) != f.host.UUID {
		t.Fatalf("the member's requests: %+v %v", requests, err)
	}

	if err := f.store.AnswerSchedulingTeamRequest(f.as(second), f.host, crmcontracts.SchedulingTeamAnswerStateLeft); err != nil {
		t.Fatal(err)
	}
	if got := f.blocker(t); got != crmcontracts.SchedulingProfileBlockerTeamNotReady {
		t.Fatalf("a link a member left reads %q", got)
	}
	profile, err := f.store.SchedulingProfile(f.ctx)
	if err != nil {
		t.Fatal(err)
	}
	if states := memberStates(profile); !slices.Equal(states, []string{"confirmed", "left"}) {
		t.Fatalf("the owner sees %v", states)
	}
	if requests, err := f.store.SchedulingTeamRequests(f.as(second)); err != nil || len(requests) != 0 {
		t.Fatalf("a member who left is still asked: %+v %v", requests, err)
	}
}

func memberStates(profile crmcontracts.SchedulingProfile) []string {
	out := []string{}
	for _, m := range teamMembers(profile) {
		out = append(out, string(memberState(m)))
	}
	return out
}

func TestTheOwnerCannotConfirmForAMember(t *testing.T) {
	f := newTeamFixture(t)
	member := f.colleague(t, "Member")
	f.name(t, false, member)
	profile, err := f.store.SchedulingProfile(f.ctx)
	if err != nil {
		t.Fatal(err)
	}
	confirmed := crmcontracts.SchedulingTeamMemberStateConfirmed
	(*profile.TeamMembers)[0].State = &confirmed
	added := f.colleague(t, "Added")
	*profile.TeamMembers = append(*profile.TeamMembers, crmcontracts.SchedulingTeamMember{UserId: openapi_types.UUID(added.UUID), State: &confirmed})
	if _, err := f.store.SaveSchedulingProfile(f.ctx, profile); err != nil {
		t.Fatal(err)
	}
	saved, err := f.store.SchedulingProfile(f.ctx)
	if err != nil {
		t.Fatal(err)
	}
	if states := memberStates(saved); !slices.Equal(states, []string{"pending", "pending"}) {
		t.Fatalf("the owner's own write answered for its members: %v", states)
	}
	if err := f.store.AnswerSchedulingTeamRequest(f.ctx, f.host, crmcontracts.SchedulingTeamAnswerStateConfirmed); !errors.Is(err, apperrors.ErrNotFound) {
		t.Fatalf("the owner answered as a member: %v", err)
	}
}

func TestAMemberWhoLeftTheWorkspaceStopsTheLink(t *testing.T) {
	f := newTeamFixture(t)
	member := f.colleague(t, "Leaver")
	f.name(t, true, member)
	if _, err := f.env.owner.Exec(f.ctx, `UPDATE app_user SET status='deactivated' WHERE id=$1`, member.UUID); err != nil {
		t.Fatal(err)
	}
	if got := f.blocker(t); got != crmcontracts.SchedulingProfileBlockerTeamNotReady {
		t.Fatalf("a link whose member left the workspace reads %q", got)
	}
	profile, err := f.store.SchedulingProfile(f.ctx)
	if err != nil {
		t.Fatal(err)
	}
	if states := memberStates(profile); !slices.Equal(states, []string{"unavailable"}) || *(*profile.TeamMembers)[0].Name != "Leaver" {
		t.Fatalf("the owner sees %v", *profile.TeamMembers)
	}
	if code, _ := f.book(t, monday(11)); code != http.StatusNotFound {
		t.Fatalf("a booking on the stopped link answered %d", code)
	}
}

func TestOnlyAColleagueWithAReadableCalendarCanBeNamed(t *testing.T) {
	f := newTeamFixture(t)
	readable, unreadable := f.colleague(t, "Readable"), f.colleague(t, "Unreadable")
	f.calendar.unreadable[unreadable] = true
	candidates, err := f.store.SchedulingTeamCandidates(f.ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(candidates) != 1 || ids.UUID(candidates[0].UserId) != readable.UUID {
		t.Fatalf("candidates=%+v", candidates)
	}
	profile, err := f.store.SchedulingProfile(f.ctx)
	if err != nil {
		t.Fatal(err)
	}
	profile.TeamMembers = &[]crmcontracts.SchedulingTeamMember{{UserId: openapi_types.UUID(unreadable.UUID)}}
	_, err = f.store.SaveSchedulingProfile(f.ctx, profile)
	if refusal, ok := errors.AsType[*SchedulingArgumentError](err); !ok || refusal.Field != fieldTeamMembers || refusal.Code != "unavailable" {
		t.Fatalf("naming an unreadable colleague: %v", err)
	}
}

func TestTwoLinksCannotTakeOneMembersTime(t *testing.T) {
	f := newTeamFixture(t)
	member := f.colleague(t, "Shared")
	f.name(t, true, member)
	if code, _ := f.book(t, monday(11)); code != http.StatusCreated {
		t.Fatalf("the first booking answered %d", code)
	}
	free, _, err := f.store.ReliableAvailability(f.door(), member, monday(9), monday(13), 30*time.Minute)
	if err != nil {
		t.Fatal(err)
	}
	if slices.ContainsFunc(starts(free), monday(11).Equal) {
		t.Fatalf("the member's own page still offers the time a team booking holds: %v", starts(free))
	}
	err = f.store.tx(f.ctx, func(tx pgx.Tx) error {
		return reserveInvitation(f.ctx, tx, member, monday(11), monday(11).Add(30*time.Minute), 0, ids.Nil)
	})
	if _, ok := errors.AsType[*SlotTakenError](err); !ok {
		t.Fatalf("a second booking of the member's held time: %v", err)
	}
}

// A time the member's own page took is not on any provider calendar yet. Only
// the hold taken under the member's lock refuses the team booking.
func TestATeamBookingRefusesATimeTheMembersOwnPageHolds(t *testing.T) {
	f := newTeamFixture(t)
	member := f.colleague(t, "Shared")
	f.name(t, true, member)
	own := f.request
	own.Start, own.End = monday(11), monday(11).Add(30*time.Minute)
	if _, err := f.store.CreateInvitation(f.as(member), own); err != nil {
		t.Fatal(err)
	}
	if code, _ := f.book(t, monday(11)); code != http.StatusConflict {
		t.Fatalf("a team booking over the member's own held time answered %d, want 409", code)
	}
}

func TestATeamBookingClosesEachMembersOpenTask(t *testing.T) {
	f := newTeamFixture(t)
	member := f.colleague(t, "Member")
	f.name(t, true, member)
	contact := ids.UUID(f.request.ContactId)
	task, _, err := f.store.LogActivity(f.ctx, LogActivityInput{
		Kind: "task", Subject: new("Find a time with the guest"), Source: "manual", AssigneeID: new(member),
		Links: []ActivityLinkInput{{EntityType: "contact", EntityID: contact}},
	})
	if err != nil {
		t.Fatal(err)
	}
	if code, _ := f.book(t, monday(11)); code != http.StatusCreated {
		t.Fatalf("booking answered %d", code)
	}
	if !f.taskIsDone(t, ids.UUID(task.Id)) {
		t.Fatal("the member's own task on the booker stayed open")
	}
}
