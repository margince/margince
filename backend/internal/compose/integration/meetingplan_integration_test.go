// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package integration

import (
	"context"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// seedMail writes one captured email the way the capture sink writes it: the
// activity, then the link that files it under a person. `created_at` is pinned
// alongside `occurred_at` rather than left to the column default, so an
// assertion cannot depend on when the suite ran.
func seedMail(
	t *testing.T, owner *pgx.Conn, person ids.UUID, at time.Time, subject, body, direction string,
) ids.UUID {
	t.Helper()
	id := SeedIDRow(t, owner, `INSERT INTO activity
		(id, kind, subject, body, occurred_at, created_at, direction, source, captured_by)
		VALUES ($1, 'email', $2, $3, $4, $4, $5, 'manual', 'human:x')`,
		subject, body, at, direction)
	LinkActivity(t, owner, id, "person", person)
	return id
}

// planFor reads the brief and returns its plan, failing when there is none.
func planFor(
	t *testing.T, ctx context.Context, e *Env, meeting ids.UUID,
) crmcontracts.MeetingPlan {
	t.Helper()
	brief, err := meetingBriefService(e).Get(ctx, meeting)
	if err != nil {
		t.Fatalf("reading the brief: %v", err)
	}
	if brief.Plan == nil {
		t.Fatal("the brief carries no plan")
	}
	return *brief.Plan
}

// THE test that says this feature shipped.
//
// Every other test here proves a cap, an order or an id. None of them can fail
// when the assembly reads a rich account and produces a brief that says
// nothing, which is the defect this whole change exists to fix — so this one
// seeds a relationship with a shape, through the real writers, and asks whether
// the plan RECOGNISED it.
//
// The fixture is an anonymised equivalent of the account this concept was
// designed against: a wish list nobody answered, repeated asks to get started,
// a plan proposing workstreams, an ownership handover, and a pile of newer
// unrelated traffic that recency alone would have shown instead.
func TestTheMeetingPlanRecognisesWhatTheAccountAskedFor(t *testing.T) {
	e := Setup(t)
	owner := OwnerConn(t)
	ctx := e.As(e.Rep1, nil, roomPerms)
	attendee := e.SeedPerson(t, "Ana Roth", &e.Rep1)

	// Three weeks of one argument, in one thread.
	wishList := seedMail(t, owner, attendee, roomFixedNow.AddDate(0, -2, 0),
		"CRM requirements",
		"We need issue tracking, quote tracking, multi-channel capture and relationship intelligence.",
		"inbound")
	seedMail(t, owner, attendee, roomFixedNow.AddDate(0, -2, 0).Add(48*time.Hour),
		"Re: CRM requirements", "How do we actually get this started?", "inbound")
	seedMail(t, owner, attendee, roomFixedNow.AddDate(0, -2, 0).Add(96*time.Hour),
		"AW: Re: CRM requirements", "Asking again — how do we start?", "inbound")

	// A month later, a separate moment: the handover.
	handover := seedMail(t, owner, attendee, roomFixedNow.AddDate(0, -1, 0),
		"Workstream ownership",
		"Rainer takes the requirements from here so Nabil can stay on operations.",
		"inbound")

	// And a wall of recent noise. Recency alone would show this and nothing else.
	for i := range 30 {
		seedMail(t, owner, attendee, roomFixedNow.Add(-time.Duration(i+1)*time.Hour),
			fmt.Sprintf("Newsletter %d", i), "Nothing to see here.", "inbound")
	}

	meeting := SeedIDRow(t, owner, `INSERT INTO activity (id, kind, subject, occurred_at, created_at, source, captured_by)
		VALUES ($1, 'meeting', 'Coffee', $2, $2, 'manual', 'human:x')`, roomTomorrow)
	LinkActivity(t, owner, meeting, "person", attendee)
	seatInRoom(t, owner, e.WS, meeting, attendee)

	plan := planFor(t, ctx, e, meeting)

	// The arc found the two moments that matter and did not spend its five
	// slots on the newsletters.
	cited := map[ids.UUID]bool{}
	var titles []string
	for _, moment := range plan.AccountArc {
		titles = append(titles, moment.Title)
		for _, evidence := range moment.Summary.Evidence {
			cited[ids.UUID(evidence.EntityId)] = true
		}
	}
	if !cited[wishList] {
		t.Errorf("the arc never cites the requirements thread; it cites %v with titles %v",
			cited, titles)
	}
	if !cited[handover] {
		t.Errorf("the arc never cites the ownership handover; titles %v", titles)
	}

	// An informal subject over a commercial history: the plan must not read the
	// coffee as a negotiation, and must say the intent is not captured.
	if plan.MeetingType.Value != crmcontracts.MeetingPlanTypeRelationship {
		t.Errorf("meeting kind = %q, want relationship for a meeting called Coffee",
			plan.MeetingType.Value)
	}
	if len(plan.Advance.Minimum.Text) == 0 {
		t.Error("the plan offers no minimum advance")
	}
}

// The old reader stopped at the ten newest rows. On any account with traffic
// that is the last week, and the last week is rarely what the meeting is about.
func TestTheArcReadsPastTheNewestPage(t *testing.T) {
	e := Setup(t)
	owner := OwnerConn(t)
	ctx := e.As(e.Rep1, nil, roomPerms)
	attendee := e.SeedPerson(t, "Ana Roth", &e.Rep1)

	old := seedMail(t, owner, attendee, roomFixedNow.AddDate(0, -6, 0),
		"The decision we made", "Agreed: we start with the depot rollout.", "inbound")
	for i := range 25 {
		seedMail(t, owner, attendee, roomFixedNow.Add(-time.Duration(i+1)*time.Hour),
			fmt.Sprintf("Chatter %d", i), "Nothing much.", "outbound")
	}

	meeting := SeedIDRow(t, owner, `INSERT INTO activity (id, kind, subject, occurred_at, created_at, source, captured_by)
		VALUES ($1, 'meeting', 'Review', $2, $2, 'manual', 'human:x')`, roomTomorrow)
	LinkActivity(t, owner, meeting, "person", attendee)
	seatInRoom(t, owner, e.WS, meeting, attendee)

	plan := planFor(t, ctx, e, meeting)
	for _, moment := range plan.AccountArc {
		for _, evidence := range moment.Summary.Evidence {
			if ids.UUID(evidence.EntityId) == old {
				return
			}
		}
	}
	t.Errorf("the arc never reached the six-month-old decision; %d moments", len(plan.AccountArc))
}

// A conversation this caller may not read must not reach them through the arc,
// and the brief must SAY that something was kept out — a thin arc nobody
// explains reads exactly like a quiet account.
func TestTheArcNamesNoConversationTheCallerMayNotRead(t *testing.T) {
	e := Setup(t)
	owner := OwnerConn(t)
	attendee := e.SeedPerson(t, "Ana Roth", &e.Rep1)

	secret := seedMail(t, owner, attendee, roomFixedNow.AddDate(0, -1, 0),
		"Private negotiation", "The number we will not go below is in here.", "inbound")
	if _, err := owner.Exec(context.Background(),
		`UPDATE activity SET audience = 'participants' WHERE id = $1`, secret); err != nil {
		t.Fatalf("narrowing the activity: %v", err)
	}
	seedMail(t, owner, attendee, roomFixedNow.AddDate(0, -1, 0).Add(time.Hour),
		"Ordinary thread", "Nothing sensitive.", "inbound")

	meeting := SeedIDRow(t, owner, `INSERT INTO activity (id, kind, subject, occurred_at, created_at, source, captured_by)
		VALUES ($1, 'meeting', 'Review', $2, $2, 'manual', 'human:x')`, roomTomorrow)
	LinkActivity(t, owner, meeting, "person", attendee)
	seatInRoom(t, owner, e.WS, meeting, attendee)

	// Rep2 shares no seat on the narrowed conversation.
	ctx := e.As(e.Rep2, nil, roomPerms)
	brief, err := meetingBriefService(e).Get(ctx, meeting)
	if err != nil {
		t.Fatalf("reading the brief: %v", err)
	}
	if brief.Plan == nil {
		t.Fatal("no plan")
	}
	for _, moment := range brief.Plan.AccountArc {
		if strings.Contains(moment.Title, "Private negotiation") {
			t.Errorf("a narrowed conversation named itself in the arc: %q", moment.Title)
		}
		for _, evidence := range moment.Summary.Evidence {
			if ids.UUID(evidence.EntityId) == secret {
				t.Error("the arc cites a conversation this caller may not read")
			}
		}
	}
	named := false
	for _, omission := range valueOf(brief.Omitted) {
		if omission.Source == "activity_history" {
			named = true
		}
	}
	if !named {
		t.Error("a conversation was withheld and the brief did not say so")
	}
}

func valueOf[T any](p *[]T) []T {
	if p == nil {
		return nil
	}
	return *p
}

// A gap is a fact about the record. It must vanish when the record answers.
func TestThePlanNamesTheAbsenceOfADeal(t *testing.T) {
	e := Setup(t)
	owner := OwnerConn(t)
	ctx := e.As(e.Rep1, nil, roomPerms)
	attendee := e.SeedPerson(t, "Ana Roth", &e.Rep1)
	meeting := SeedIDRow(t, owner, `INSERT INTO activity (id, kind, subject, occurred_at, created_at, source, captured_by)
		VALUES ($1, 'meeting', 'Review', $2, $2, 'manual', 'human:x')`, roomTomorrow)
	LinkActivity(t, owner, meeting, "person", attendee)
	seatInRoom(t, owner, e.WS, meeting, attendee)

	plan := planFor(t, ctx, e, meeting)
	found := false
	for _, unknown := range plan.Unknowns {
		if unknown.Kind == crmcontracts.MeetingPlanUnknownNoOpenDeal {
			found = true
		}
	}
	if !found {
		t.Error("a meeting with no deal did not report no_open_deal")
	}
}
