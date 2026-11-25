// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package integration

import (
	"context"
	"net/http"
	"testing"

	"github.com/margince/margince/backend/internal/compose/integration/apptest"
)

// teamLink is a team link as the API answers it.
type teamLink struct {
	ID          string `json:"id"`
	Slug        string `json:"slug"`
	TeamMembers []struct {
		UserID string `json:"user_id"`
		State  string `json:"state"`
	} `json:"team_members"`
}

// nameOnLink names members on the host's team link and answers its id. An
// empty link makes a new team link.
func nameOnLink(t *testing.T, host *apptest.AppEnv, link string, members ...string) string {
	t.Helper()
	team := []AnyMap{}
	for _, member := range members {
		team = append(team, AnyMap{"user_id": member})
	}
	body := AnyMap{"title": "Meet the team", "team_members": team}
	var out teamLink
	if link == "" {
		mustCall(t, host, http.MethodPost, "/v1/scheduling/team-links", body, http.StatusCreated, &out)
	} else {
		mustCall(t, host, http.MethodPut, "/v1/scheduling/team-links/"+link, body, http.StatusOK, &out)
	}
	return out.ID
}

// bookingColleague adds a colleague whose personal page reads a calendar, and
// answers their id and their signed-in seat.
func bookingColleague(t *testing.T, e *apptest.AppEnv, email, name string) (string, *apptest.AppEnv) {
	t.Helper()
	password := "team-notice-" + name + "-password"
	member := createdRecord(t, e, "/v1/users", AnyMap{"email": email, "display_name": name, "role": "rep"})
	apptest.SetPasswordByLink(t, e, member, password)
	seat := e.AsSecondSeat(t, email, password)
	apptest.EnableBooking(t, seat)
	return member, seat
}

// teamRequestLines counts the member's team request notices, and how many of
// them still interrupt the member.
func teamRequestLines(t *testing.T, e *apptest.AppEnv, member string) (all, live int) {
	t.Helper()
	if err := e.Owner.QueryRow(context.Background(), `SELECT count(*), count(*) FILTER (WHERE retracted_at IS NULL AND read_at IS NULL)
		FROM notice WHERE recipient_user_id = $1 AND kind = 'meeting_team_request'`, member).Scan(&all, &live); err != nil {
		t.Fatal(err)
	}
	return all, live
}

func teamRequestMails(t *testing.T, e *apptest.AppEnv) int {
	t.Helper()
	var staged int
	if err := e.Owner.QueryRow(context.Background(), `SELECT count(*) FROM river_job j JOIN notice n ON n.id::text = j.args->>'notice_id'
		WHERE j.kind = 'notification_email' AND n.kind = 'meeting_team_request'`).Scan(&staged); err != nil {
		t.Fatal(err)
	}
	return staged
}

// A colleague named on a team link is told once, in the app and by mail, and
// nobody else is. A save that keeps them on the list tells them nothing new,
// and their answer, given by the link's id, takes the line back.
func TestAColleagueNamedOnATeamLinkIsToldInTheAppAndByMail(t *testing.T) {
	e := apptest.SetupBookingApp(t)
	e.DescribeCompany(t)
	member, colleague := bookingColleague(t, e, "member@team-notice.test", "Mia")

	link := nameOnLink(t, e, "", member)
	if all, live := teamRequestLines(t, e, member); all != 1 || live != 1 {
		t.Fatalf("the named colleague holds %d team request notices, %d unread; want 1 and 1", all, live)
	}
	if staged := teamRequestMails(t, e); staged != 1 {
		t.Fatalf("%d team request mails are staged, want 1", staged)
	}
	var others int
	if err := e.Owner.QueryRow(context.Background(), `SELECT count(*) FROM notice WHERE kind = 'meeting_team_request' AND recipient_user_id <> $1`, member).Scan(&others); err != nil || others != 0 {
		t.Fatalf("%d team request notices reached someone else (%v)", others, err)
	}

	nameOnLink(t, e, link, member)
	if all, _ := teamRequestLines(t, e, member); all != 1 || teamRequestMails(t, e) != 1 {
		t.Fatalf("a save that kept the pending colleague wrote again: %d notices, %d mails", all, teamRequestMails(t, e))
	}

	var requests []struct {
		PageID string `json:"page_id"`
	}
	mustCall(t, colleague, http.MethodGet, "/v1/scheduling/team-requests", nil, http.StatusOK, &requests)
	if len(requests) != 1 || requests[0].PageID != link {
		t.Fatalf("the colleague sees %+v, want one request naming the team link", requests)
	}
	mustCall(t, colleague, http.MethodPut, "/v1/scheduling/team-links/"+link+"/answer", AnyMap{"state": "confirmed"}, http.StatusNoContent, nil)
	if _, live := teamRequestLines(t, e, member); live != 0 {
		t.Fatalf("a confirmed request still interrupts the colleague: %d live notices", live)
	}

	nameOnLink(t, e, link, member)
	if all, live := teamRequestLines(t, e, member); all != 1 || live != 0 || teamRequestMails(t, e) != 1 {
		t.Fatalf("a save that kept the confirmed colleague told them again: %d notices, %d live, %d mails", all, live, teamRequestMails(t, e))
	}
}

// A colleague taken off a team link loses the line, and named again gets it
// back unread rather than a second one.
func TestAColleagueTakenOffALinkLosesTheRequestUntilNamedAgain(t *testing.T) {
	e := apptest.SetupBookingApp(t)
	e.DescribeCompany(t)
	member, _ := bookingColleague(t, e, "removed@team-notice.test", "Remy")
	kept, _ := bookingColleague(t, e, "kept@team-notice.test", "Kim")

	link := nameOnLink(t, e, "", member, kept)
	nameOnLink(t, e, link, kept)
	if all, live := teamRequestLines(t, e, member); all != 1 || live != 0 {
		t.Fatalf("a colleague taken off the link holds %d notices, %d live; want 1 and 0", all, live)
	}
	nameOnLink(t, e, link, kept, member)
	if all, live := teamRequestLines(t, e, member); all != 1 || live != 1 {
		t.Fatalf("a colleague named again holds %d notices, %d live; want 1 and 1", all, live)
	}
}

// Two team links that name one colleague ask twice, each in its own line, and
// answering one link leaves the other's line standing.
func TestTwoTeamLinksEachAskTheColleague(t *testing.T) {
	e := apptest.SetupBookingApp(t)
	e.DescribeCompany(t)
	member, colleague := bookingColleague(t, e, "twice@team-notice.test", "Tess")

	first := nameOnLink(t, e, "", member)
	nameOnLink(t, e, "", member)
	if all, live := teamRequestLines(t, e, member); all != 2 || live != 2 || teamRequestMails(t, e) != 2 {
		t.Fatalf("two links naming the colleague left %d notices, %d live, %d mails; want 2 of each", all, live, teamRequestMails(t, e))
	}
	mustCall(t, colleague, http.MethodPut, "/v1/scheduling/team-links/"+first+"/answer", AnyMap{"state": "confirmed"}, http.StatusNoContent, nil)
	if all, live := teamRequestLines(t, e, member); all != 2 || live != 1 {
		t.Fatalf("answering one link left %d notices, %d live; want 2 and 1", all, live)
	}
}

// Only the host who made a team link changes or deletes it. A member gets 404,
// and the personal page refuses a team.
func TestOnlyTheOwnerChangesATeamLink(t *testing.T) {
	e := apptest.SetupBookingApp(t)
	e.DescribeCompany(t)
	member, colleague := bookingColleague(t, e, "owner@team-notice.test", "Olli")
	link := nameOnLink(t, e, "", member)

	body := AnyMap{"title": "Taken over", "team_members": []AnyMap{{"user_id": member}}}
	mustCall(t, colleague, http.MethodPut, "/v1/scheduling/team-links/"+link, body, http.StatusNotFound, nil)
	mustCall(t, colleague, http.MethodDelete, "/v1/scheduling/team-links/"+link, nil, http.StatusNotFound, nil)
	mustCall(t, e, http.MethodPut, "/v1/scheduling/profile", AnyMap{
		"enabled": true, "provider": "gcal", "calendar_id": "work", "duration_minutes": 30,
		"notice_minutes": 0, "horizon_days": 30, "buffer_minutes": 0, "title": "Meeting", "location": "",
		"team_members": []AnyMap{{"user_id": member}},
	}, http.StatusUnprocessableEntity, nil)

	var links []teamLink
	mustCall(t, e, http.MethodGet, "/v1/scheduling/team-links", nil, http.StatusOK, &links)
	if len(links) != 1 || links[0].ID != link || len(links[0].TeamMembers) != 1 || links[0].TeamMembers[0].State != "pending" {
		t.Fatalf("the owner's team links read %+v", links)
	}
	mustCall(t, e, http.MethodDelete, "/v1/scheduling/team-links/"+link, nil, http.StatusNoContent, nil)
	mustCall(t, e, http.MethodGet, "/v1/scheduling/team-links", nil, http.StatusOK, &links)
	if len(links) != 0 {
		t.Fatalf("a deleted team link is still listed: %+v", links)
	}
	if _, live := teamRequestLines(t, e, member); live != 0 {
		t.Fatalf("a deleted link's request still interrupts the colleague: %d live", live)
	}
}

// A client that answers by host, as it did when a host had one link, still
// works. Its answer confirms every team link of that host that waits for it, and
// only a caller no link of that host names is not found.
func TestTheAnswerByHostConfirmsEveryTeamLinkOfThatHost(t *testing.T) {
	e := apptest.SetupBookingApp(t)
	e.DescribeCompany(t)
	member, colleague := bookingColleague(t, e, "byhost@team-notice.test", "Hal")
	nameOnLink(t, e, "", member)
	nameOnLink(t, e, "", member)
	var host struct {
		User struct {
			ID string `json:"id"`
		} `json:"user"`
	}
	mustCall(t, e, http.MethodGet, "/v1/me", nil, http.StatusOK, &host)

	mustCall(t, colleague, http.MethodPut, "/v1/scheduling/team-requests/"+host.User.ID, AnyMap{"state": "confirmed"}, http.StatusNoContent, nil)
	var links []teamLink
	mustCall(t, e, http.MethodGet, "/v1/scheduling/team-links", nil, http.StatusOK, &links)
	if len(links) != 2 {
		t.Fatalf("the host has %d team links, want 2", len(links))
	}
	for _, link := range links {
		if len(link.TeamMembers) != 1 || link.TeamMembers[0].State != "confirmed" {
			t.Errorf("team link %s reads %+v after the answer by host, want the colleague confirmed", link.ID, link.TeamMembers)
		}
	}
	if _, live := teamRequestLines(t, e, member); live != 0 {
		t.Fatalf("%d team request notices still interrupt the colleague, want none", live)
	}

	// A repeat changes nothing and answers as the one-link answer does: 204, no audit row.
	pageAudits := func() (n int) {
		if err := e.Owner.QueryRow(context.Background(), `SELECT count(*) FROM audit_log WHERE entity_type = 'booking_page'`).Scan(&n); err != nil {
			t.Fatalf("counting booking page audit rows: %v", err)
		}
		return n
	}
	before := pageAudits()
	mustCall(t, colleague, http.MethodPut, "/v1/scheduling/team-requests/"+host.User.ID, AnyMap{"state": "confirmed"}, http.StatusNoContent, nil)
	if after := pageAudits(); after != before {
		t.Errorf("a repeat answer by host that changes nothing wrote %d audit row(s), want none", after-before)
	}

	_, stranger := bookingColleague(t, e, "unnamed@team-notice.test", "Una")
	mustCall(t, stranger, http.MethodPut, "/v1/scheduling/team-requests/"+host.User.ID, AnyMap{"state": "confirmed"}, http.StatusNotFound, nil)
}
