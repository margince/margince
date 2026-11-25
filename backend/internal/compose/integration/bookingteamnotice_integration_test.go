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

// nameOnLink saves the host's booking link with members as its team.
func nameOnLink(t *testing.T, host *apptest.AppEnv, members ...string) {
	t.Helper()
	team := []AnyMap{}
	for _, member := range members {
		team = append(team, AnyMap{"user_id": member})
	}
	mustCall(t, host, http.MethodPut, "/v1/scheduling/profile", AnyMap{
		"enabled": true, "provider": "gcal", "calendar_id": "work", "duration_minutes": 30,
		"notice_minutes": 0, "horizon_days": 30, "buffer_minutes": 0, "title": "Intro call", "location": "",
		"team_members": team,
	}, http.StatusOK, nil)
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

// A colleague named on a booking link is told once, in the app and by mail,
// and nobody else is. A save that keeps them on the list tells them nothing
// new, and their answer takes the line back.
func TestAColleagueNamedOnABookingLinkIsToldInTheAppAndByMail(t *testing.T) {
	e := apptest.SetupBookingApp(t)
	e.DescribeCompany(t)
	const email, password = "member@team-notice.test", "team-notice-member-password"
	member := createdRecord(t, e, "/v1/users", AnyMap{"email": email, "display_name": "Mia Member", "role": "rep"})
	apptest.SetPasswordByLink(t, e, member, password)
	colleague := e.AsSecondSeat(t, email, password)
	apptest.EnableBooking(t, colleague)

	nameOnLink(t, e, member)
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

	nameOnLink(t, e, member)
	if all, _ := teamRequestLines(t, e, member); all != 1 || teamRequestMails(t, e) != 1 {
		t.Fatalf("a save that kept the pending colleague wrote again: %d notices, %d mails", all, teamRequestMails(t, e))
	}

	var requests []struct {
		HostUserID string `json:"host_user_id"`
	}
	mustCall(t, colleague, http.MethodGet, "/v1/scheduling/team-requests", nil, http.StatusOK, &requests)
	if len(requests) != 1 {
		t.Fatalf("the colleague sees %d requests, want 1", len(requests))
	}
	mustCall(t, colleague, http.MethodPut, "/v1/scheduling/team-requests/"+requests[0].HostUserID, AnyMap{"state": "confirmed"}, http.StatusNoContent, nil)
	if _, live := teamRequestLines(t, e, member); live != 0 {
		t.Fatalf("a confirmed request still interrupts the colleague: %d live notices", live)
	}

	nameOnLink(t, e, member)
	if all, live := teamRequestLines(t, e, member); all != 1 || live != 0 || teamRequestMails(t, e) != 1 {
		t.Fatalf("a save that kept the confirmed colleague told them again: %d notices, %d live, %d mails", all, live, teamRequestMails(t, e))
	}
}

// A colleague taken off the link loses the line, and named again gets it back
// unread rather than a second one.
func TestAColleagueTakenOffALinkLosesTheRequestUntilNamedAgain(t *testing.T) {
	e := apptest.SetupBookingApp(t)
	e.DescribeCompany(t)
	const email, password = "removed@team-notice.test", "team-notice-removed-password"
	member := createdRecord(t, e, "/v1/users", AnyMap{"email": email, "display_name": "Remy Removed", "role": "rep"})
	apptest.SetPasswordByLink(t, e, member, password)
	apptest.EnableBooking(t, e.AsSecondSeat(t, email, password))

	nameOnLink(t, e, member)
	nameOnLink(t, e)
	if all, live := teamRequestLines(t, e, member); all != 1 || live != 0 {
		t.Fatalf("a colleague taken off the link holds %d notices, %d live; want 1 and 0", all, live)
	}
	nameOnLink(t, e, member)
	if all, live := teamRequestLines(t, e, member); all != 1 || live != 1 {
		t.Fatalf("a colleague named again holds %d notices, %d live; want 1 and 1", all, live)
	}
}
