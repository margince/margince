// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package integration

// Who gets the coaching layer on a meeting brief, and what it may say.
//
// Two properties, and the second is the one worth the suite. WHO is a
// permission rule — a seat that may coach, and a live team shared with somebody
// in the room. WHAT is an honesty rule: the lead and the rep must be looking at
// the same meeting, so every field of the plan except the coaching itself has
// to be identical between the two reads. That one is asserted reflectively,
// because a field added later is a field somebody would forget to add here.

import (
	"context"
	"reflect"
	"testing"

	"github.com/jackc/pgx/v5"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// seatUserInRoom seats one of OUR people in the meeting, as against
// seatInRoom's counterparty.
func seatUserInRoom(t *testing.T, owner *pgx.Conn, activity, user ids.UUID) {
	t.Helper()
	if _, err := owner.Exec(context.Background(),
		`INSERT INTO activity_participant (activity_id, role, user_id)
		 VALUES ($1, 'organizer', $2)`, activity, user); err != nil {
		t.Fatalf("seating a colleague: %v", err)
	}
}

// coachPerms is the manager seat: the rep grid, plus the role key that decides
// whether a seat may coach at all.
func coachPerms() principal.Permissions {
	perms := roomPerms
	perms.RoleKeys = []string{roleManager}
	return perms
}

func repPerms() principal.Permissions {
	perms := roomPerms
	perms.RoleKeys = []string{roleRep}
	return perms
}

// coachingRoom seeds one meeting with a colleague seated in it.
func coachingRoom(t *testing.T, e *Env, seated ids.UUID) ids.UUID {
	t.Helper()
	owner := OwnerConn(t)
	attendee := e.SeedPerson(t, "Ana Roth", &e.Rep1)
	meeting := SeedIDRow(t, owner, `INSERT INTO activity (id, kind, subject, occurred_at, created_at, source, captured_by)
		VALUES ($1, 'meeting', 'Expansion review', $2, $2, 'manual', 'human:x')`, roomTomorrow)
	LinkActivity(t, owner, meeting, "person", attendee)
	seatInRoom(t, owner, e.WS, meeting, attendee)
	seatUserInRoom(t, owner, meeting, seated)
	return meeting
}

func coachingOn(t *testing.T, e *Env, ctx context.Context, meeting ids.UUID) *crmcontracts.MeetingPlanCoaching {
	t.Helper()
	brief, err := meetingBriefService(e).Get(ctx, meeting)
	if err != nil {
		t.Fatalf("reading the brief: %v", err)
	}
	if brief.Plan == nil {
		t.Fatal("the brief carries no plan")
	}
	return brief.Plan.ManagerCoaching
}

// A lead reading their teammate's meeting is the case this exists for.
func TestALeadReadingATeammatesMeetingIsCoached(t *testing.T) {
	e := Setup(t)
	meeting := coachingRoom(t, e, e.Rep2)
	// Rep1 and Rep2 share Team1.
	ctx := e.As(e.Rep1, []ids.UUID{e.Team1}, coachPerms())
	if coachingOn(t, e, ctx, meeting) == nil {
		t.Error("a lead reading their teammate's meeting was not coached")
	}
}

// THE honesty rule. The lead sees one more thing, not different things.
func TestTheLeadAndTheRepReadTheSameMeeting(t *testing.T) {
	e := Setup(t)
	meeting := coachingRoom(t, e, e.Rep2)
	service := meetingBriefService(e)

	lead, err := service.Get(e.As(e.Rep1, []ids.UUID{e.Team1}, coachPerms()), meeting)
	if err != nil {
		t.Fatalf("the lead's read: %v", err)
	}
	rep, err := service.Get(e.As(e.Rep2, []ids.UUID{e.Team1}, repPerms()), meeting)
	if err != nil {
		t.Fatalf("the rep's read: %v", err)
	}
	if lead.Plan == nil || rep.Plan == nil {
		t.Fatal("a read came back with no plan")
	}
	if lead.Plan.ManagerCoaching == nil {
		t.Fatal("the lead was not coached, so this proves nothing about the facts")
	}
	if rep.Plan.ManagerCoaching != nil {
		t.Error("the rep was coached on their own meeting")
	}

	// Every field EXCEPT the coaching, compared by walking the struct — a
	// field added later is covered without anyone remembering to add it here.
	leadPlan, repPlan := *lead.Plan, *rep.Plan
	leadPlan.ManagerCoaching, repPlan.ManagerCoaching = nil, nil
	shape := reflect.TypeOf(leadPlan)
	leadValue, repValue := reflect.ValueOf(leadPlan), reflect.ValueOf(repPlan)
	for i := range shape.NumField() {
		name := shape.Field(i).Name
		if !reflect.DeepEqual(leadValue.Field(i).Interface(), repValue.Field(i).Interface()) {
			t.Errorf("the lead and the rep disagree about %s; coaching must add a reading, never a fact", name)
		}
	}
	if !reflect.DeepEqual(lead.Sections, rep.Sections) {
		t.Error("the lead and the rep were shown different sections")
	}
}

// A rep is excluded by the ROLE, not by the row scope — which is the carve-out
// the tree already makes for raising a coaching notice, and the one my earlier
// row-scope rule would have got wrong.
func TestARepIsNeverCoachedEvenOnATeammatesMeeting(t *testing.T) {
	e := Setup(t)
	meeting := coachingRoom(t, e, e.Rep2)
	ctx := e.As(e.Rep1, []ids.UUID{e.Team1}, repPerms())
	if coachingOn(t, e, ctx, meeting) != nil {
		t.Error("a rep sharing a team with the seated colleague was coached")
	}
}

// A lead in the room coaching their rep through it is the ordinary case.
func TestASeatedLeadIsStillCoached(t *testing.T) {
	e := Setup(t)
	owner := OwnerConn(t)
	meeting := coachingRoom(t, e, e.Rep2)
	seatUserInRoom(t, owner, meeting, e.Rep1)
	ctx := e.As(e.Rep1, []ids.UUID{e.Team1}, coachPerms())
	if coachingOn(t, e, ctx, meeting) == nil {
		t.Error("a lead who is in the room was not coached; being seated is not a disqualifier")
	}
}

// A lead of another team gets the rep's brief, not a refusal. They asked for a
// brief and a brief is what they may have.
func TestALeadOfAnotherTeamReadsTheBriefWithoutCoaching(t *testing.T) {
	e := Setup(t)
	meeting := coachingRoom(t, e, e.Rep2)
	// Rep3 is in Team2; Rep2 is in Team1.
	ctx := e.As(e.Rep3, []ids.UUID{e.Team2}, coachPerms())
	brief, err := meetingBriefService(e).Get(ctx, meeting)
	if err != nil {
		t.Fatalf("a lead of another team was refused the brief entirely: %v", err)
	}
	if brief.Plan != nil && brief.Plan.ManagerCoaching != nil {
		t.Error("a lead was coached about a rep on a team they do not share")
	}
}

// A meeting with nobody but the reader in it has nobody to coach.
func TestALeadAloneInTheRoomIsNotCoached(t *testing.T) {
	e := Setup(t)
	meeting := coachingRoom(t, e, e.Rep1)
	ctx := e.As(e.Rep1, []ids.UUID{e.Team1}, coachPerms())
	if coachingOn(t, e, ctx, meeting) != nil {
		t.Error("a lead alone in the room was coached about themselves")
	}
}

// An agent inherits its human's seat and is refused by RequireCoach, which
// admits only a human. It still gets the brief.
func TestAnAgentActingForALeadIsNotCoached(t *testing.T) {
	e := Setup(t)
	meeting := coachingRoom(t, e, e.Rep2)
	ctx := e.AgentFor(t, e.Rep1, []ids.UUID{e.Team1}, coachPerms())
	brief, err := meetingBriefService(e).Get(ctx, meeting)
	if err != nil {
		t.Fatalf("an agent was refused the brief: %v", err)
	}
	if brief.Plan != nil && brief.Plan.ManagerCoaching != nil {
		t.Error("an agent was handed the coaching layer")
	}
}

// A composition that wired no membership seam projects no coaching. Fail
// closed: the honest answer for a deployment that did not ask for it.
func TestAServiceWithNoTeammatesSeamCoachesNobody(t *testing.T) {
	e := Setup(t)
	meeting := coachingRoom(t, e, e.Rep2)
	ctx := e.As(e.Rep1, []ids.UUID{e.Team1}, coachPerms())
	brief, err := meetingBriefServiceWithoutTeammates(e).Get(ctx, meeting)
	if err != nil {
		t.Fatalf("reading the brief: %v", err)
	}
	if brief.Plan != nil && brief.Plan.ManagerCoaching != nil {
		t.Error("a service with no membership seam projected coaching anyway")
	}
}
