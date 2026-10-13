// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package integration

// A colleague named on a hand-logged meeting is in the relationship graph the
// same way a colleague on a captured calendar meeting is.

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/modules/activities"
	"github.com/margince/margince/backend/internal/modules/search"
	"github.com/margince/margince/backend/internal/platform/clocktest"
	"github.com/margince/margince/backend/internal/platform/database"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

func (e *Env) logMeetingWithColleagues(t *testing.T, contact ids.UUID, subject string, colleagues ...ids.UUID) (ids.UUID, error) {
	t.Helper()
	held := "held"
	when := clocktest.Now(t).Add(-time.Hour)
	in := activities.LogActivityInput{
		Kind: "meeting", Subject: &subject, OccurredAt: &when, MeetingStatus: &held, Source: "manual",
		Links: []activities.ActivityLinkInput{{EntityType: "contact", EntityID: contact}},
	}
	for _, colleague := range colleagues {
		in.AttendeeUserIDs = append(in.AttendeeUserIDs, ids.From[ids.UserKind](colleague))
	}
	logged, _, err := e.Activities.LogActivity(e.Admin(), in)
	return ids.UUID(logged.Id), err
}

// whoKnows folds the meeting the way the graph-edge consumer does, then asks
// the reader behind "who knows" for the contact's colleagues.
func (e *Env) whoKnows(t *testing.T, activityID, contact ids.UUID) map[ids.UUID]int {
	t.Helper()
	ctx := e.Admin()
	known := map[ids.UUID]int{}
	if err := database.WithWorkspaceTx(ctx, e.Pool, func(tx pgx.Tx) error {
		if err := search.RecomputeEdgesForActivities(ctx, tx, []ids.UUID{activityID}); err != nil {
			return err
		}
		edges, err := search.EdgesForContact(ctx, tx, contact, 10)
		for _, edge := range edges {
			known[edge.UserID] = edge.CountTotal
		}
		return err
	}); err != nil {
		t.Fatalf("reading who knows the contact: %v", err)
	}
	return known
}

func TestALoggedMeetingCountsForTheColleagueWhoAttended(t *testing.T) {
	e := Setup(t)
	contact := e.SeedContact(t, "Dana Attendee", nil)

	meeting, err := e.logMeetingWithColleagues(t, contact, "Quarterly review", e.Rep2)
	if err != nil {
		t.Fatalf("logging the meeting: %v", err)
	}

	role := e.WsScalar(t, `SELECT role FROM activity_participant WHERE activity_id = $1 AND user_id = $2`, meeting, e.Rep2)
	if role != "attendee" {
		t.Fatalf("the colleague who attended is stamped as %q, want attendee", role)
	}
	known := e.whoKnows(t, meeting, contact)
	if known[e.Rep2] != 1 {
		t.Fatalf("who knows the contact = %v, want the attending colleague with one meeting", known)
	}
	if known[e.AdminUser] != 1 {
		t.Fatalf("who knows the contact = %v, want the logger kept beside the colleague", known)
	}
	if _, named := known[e.Rep3]; named {
		t.Fatalf("who knows the contact = %v, names a colleague nobody listed", known)
	}
}

func TestALoggedMeetingNamingTheLoggerAsAttendeeKeepsOneRowForThem(t *testing.T) {
	e := Setup(t)
	contact := e.SeedContact(t, "Ola Attendee", nil)

	meeting, err := e.logMeetingWithColleagues(t, contact, "Kickoff", e.AdminUser, e.Rep1, e.Rep1)
	if err != nil {
		t.Fatalf("logging the meeting: %v", err)
	}
	if n := e.WsCount(t, `SELECT count(*) FROM activity_participant WHERE activity_id = $1 AND user_id = $2`, meeting, e.AdminUser); n != 1 {
		t.Fatalf("the logger has %d participant rows, want 1", n)
	}
	if n := e.WsCount(t, `SELECT count(*) FROM activity_participant WHERE activity_id = $1 AND user_id = $2`, meeting, e.Rep1); n != 1 {
		t.Fatalf("a colleague named twice has %d participant rows, want 1", n)
	}
}

func TestALoggedMeetingRefusesAnAttendeeWhoIsNotAnActiveColleague(t *testing.T) {
	e := Setup(t)
	contact := e.SeedContact(t, "Kim Attendee", nil)
	owner := OwnerConn(t)
	if _, err := owner.Exec(context.Background(), `UPDATE app_user SET archived_at = now() WHERE id = $1`, e.Rep3); err != nil {
		t.Fatalf("archiving a colleague: %v", err)
	}
	agent := ids.NewV7()
	if _, err := owner.Exec(context.Background(),
		`INSERT INTO app_user (id, email, display_name, is_agent) VALUES ($1, 'agent@authz.test', 'Agent', true)`, agent); err != nil {
		t.Fatalf("seeding an agent seat: %v", err)
	}

	for name, attendee := range map[string]ids.UUID{
		"an id that names nobody": ids.NewV7(),
		"an archived colleague":   e.Rep3,
		"an agent seat":           agent,
	} {
		subject := "Refused " + name
		_, err := e.logMeetingWithColleagues(t, contact, subject, e.Rep2, attendee)
		var refusal *activities.AttendeeColleagueError
		if !errors.As(err, &refusal) || refusal.Code != "not_a_colleague" {
			t.Fatalf("%s: err = %v, want a not_a_colleague refusal", name, err)
		}
		if n := e.WsCount(t, `SELECT count(*) FROM activity WHERE subject = $1`, subject); n != 0 {
			t.Fatalf("%s: the refused meeting was written anyway", name)
		}
	}
}
