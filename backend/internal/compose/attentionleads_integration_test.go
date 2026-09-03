// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package compose

// The lead-response lane, against a real database and through the real
// composition.
//
// The lane's own tests (attention/leadlane_test.go) answer it against fixture
// rows: a stub returns an owed lead because the test said so, which proves the
// classifier and nothing about the store that has to find one. Three paths are
// invisible to them, and each is a defect a rep would see —
//
//   the DEDUPE, which stops one late lead becoming three rows (the lead, the
//   escalation's task about the lead, and its notice about the same thing);
//   the leadLead=8 crowding cut, which needs nine real breached leads; and
//   whether the lane reaches the page AT ALL under a real scope resolution.
//
// All three need a database. What they defend is the difference between a lane
// that ranks correctly and a lane a rep actually sees.

import (
	"context"
	"encoding/json"
	"fmt"
	"testing"
	"time"

	openapi_types "github.com/oapi-codegen/runtime/types"

	"github.com/margince/margince/backend/internal/compose/integration"
	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/modules/activities"
	"github.com/margince/margince/backend/internal/modules/approvals"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
	"github.com/margince/margince/backend/internal/shared/ports/datasource"
	"github.com/margince/margince/backend/internal/shared/ports/workflow"
)

// leadRepPerms is a rep who may read leads, held to their own rows.
//
// Its own fixture rather than integration.RepPerms, which grants no `lead` at
// all — a caller without it is refused inside the seam, and the seam turns that
// refusal into "this installation measures nothing", so the page shows an ABSENT
// lane that looks exactly like a correct one. Rather than widening the shared
// fixture, which several suites read as a rep who cannot see certain objects,
// this mirrors the seeded rep row for the objects under test.
//
// RowScopeOwn, so the `mine` narrowing is a real question. AdminPerms would
// answer it with RowScopeAll and prove nothing about whose queue this is.
var leadRepPerms = principal.Permissions{
	RoleKeys: []string{"rep"},
	Objects: map[string]principal.ObjectGrant{
		"lead":     {Create: true, Read: true, Update: true},
		"person":   {Create: true, Read: true, Update: true},
		"deal":     {Create: true, Read: true, Update: true},
		"activity": {Create: true, Read: true, Update: true},
		// A read resolves the basis it reports money in; every seeded role holds it.
		"installation_settings": {Read: true},
	},
	RowScope: principal.RowScopeOwn,
}

// leadLeadPerms is the same vocabulary at TEAM row scope — a Team Lead who may
// open a named rep's queue.
var leadLeadPerms = func() principal.Permissions {
	p := leadRepPerms
	p.RoleKeys = []string{"manager"}
	p.RowScope = principal.RowScopeTeam
	return p
}()

// measureFirstResponse turns the installation's first-response target on.
//
// Off by default, deliberately — a fresh installation should not open on a list
// where every lead reads "overdue" — so a lane test that skipped this would
// assert against a lane the store correctly refuses to fill, and pass for the
// wrong reason.
func measureFirstResponse(t *testing.T, e *integration.Env) {
	t.Helper()
	// The floor the setting allows, so every seeded wait below is comfortably
	// past it and no case turns on minutes.
	e.WsExec(t, `INSERT INTO setting (key, value) VALUES
		('people.first_response_enabled', 'true'::jsonb),
		('people.first_response_target_minutes', to_jsonb(15::int))
	 ON CONFLICT (key) DO UPDATE SET value = EXCLUDED.value`)
}

// seedOwedLead plants an inbound lead that has been waiting, unanswered, since
// `waitingFor` ago — so its state is DERIVED from the clock and the target the
// way a real one is, rather than stamped onto the row.
//
// created_at carries the wait because the SLA clock starts there when nothing
// routed the lead (formulas §18.1), and first_response_at stays null because a
// lead that has been answered owes nothing.
func seedOwedLead(t *testing.T, e *integration.Env, name string, owner *ids.UUID, waitingFor time.Duration) ids.UUID {
	t.Helper()
	id := ids.NewV7()
	e.WsExec(t, `INSERT INTO lead (id, full_name, status, source, captured_by, owner_id, created_at)
		VALUES ($1, $2, 'new', 'inbound', 'human:x', $3, now() - $4::interval)`,
		id, name, owner, fmt.Sprintf("%d seconds", int(waitingFor.Seconds())))
	return id
}

// ownQueue reads one caller's OWN queue through the production wiring. The
// named-owner read has its own call site, because its whole point is the
// arguments this helper pins.
func ownQueue(ctx context.Context, t *testing.T, e *integration.Env) crmcontracts.Worklist {
	t.Helper()
	svc := newAttentionService(e.Pool, approvals.NewService(e.DB()), failClosedOverlayMeter(), time.Now)
	page, err := svc.Worklist(ctx, "mine", "", ids.Nil, 50, "")
	if err != nil {
		t.Fatalf("reading the worklist: %v", err)
	}
	return page
}

// leadRows names the titles the lead-response lane contributed.
func leadRows(page crmcontracts.Worklist) []string {
	var out []string
	for _, item := range page.Queue {
		if string(item.Source) == "lead_response" && item.Title != nil {
			out = append(out, *item.Title)
		}
	}
	return out
}

// An owed lead reaches its owner's queue, and only its owner's.
//
// The admit half is what makes the refusal meaningful: a lane bound to nothing,
// or a scope clause that dropped lead_response, would give an empty queue to
// both reps and satisfy a refusal-only test.
func TestAnOwedLeadReachesItsOwnersQueueAndNobodyElses(t *testing.T) {
	e := integration.Setup(t)
	measureFirstResponse(t, e)
	seedOwedLead(t, e, "Waiting On Rep1", &e.Rep1, 2*time.Hour)

	mine := ownQueue(e.As(e.Rep1, []ids.UUID{e.Team1}, leadRepPerms), t, e)
	if got := leadRows(mine); len(got) != 1 || got[0] != "Waiting On Rep1" {
		t.Fatalf("the owner's queue carries %v, want the one owed lead — a lane the "+
			"composition never bound looks exactly like this", got)
	}

	theirs := ownQueue(e.As(e.Rep2, []ids.UUID{e.Team1}, leadRepPerms), t, e)
	if got := leadRows(theirs); len(got) != 0 {
		t.Errorf("a colleague's `mine` queue carries %v, want none", got)
	}
}

// The lane claims nothing when the installation measures no first response.
//
// Not merely "no rows": an absent source must publish no reach row either, or
// the page reports a bound on a source it never consulted.
func TestWithTheTargetOffTheLaneIsAbsentFromThePage(t *testing.T) {
	e := integration.Setup(t)
	// Deliberately NOT calling measureFirstResponse: this is the default.
	seedOwedLead(t, e, "Nobody Is Counting", &e.Rep1, 48*time.Hour)

	page := ownQueue(e.As(e.Rep1, []ids.UUID{e.Team1}, leadRepPerms), t, e)
	if got := leadRows(page); len(got) != 0 {
		t.Errorf("the queue carries %v with the target switched off", got)
	}
	for _, reach := range page.Reach {
		if string(reach.Source) == "lead_response" {
			t.Errorf("the page publishes a reach row for a source it never read: %+v", reach)
		}
	}
}

// One late lead is one row, not three.
//
// The SLA escalation writes a task AND a notice when a lead breaches, and both
// reached the page before this lane existed. This drives the REAL escalation
// handler rather than inserting a task by hand: a hand-written row would prove
// the dedupe matches whatever this test typed, while the shipped escalation
// could file its task differently and the queue would show two rows in front of
// a rep with every test green.
func TestTheEscalationsTaskFoldsIntoTheLeadItIsAbout(t *testing.T) {
	e := integration.Setup(t)
	measureFirstResponse(t, e)
	lead := seedOwedLead(t, e, "Late And Escalated", &e.Rep1, 4*time.Hour)

	raiseSLAEscalation(t, e, lead)

	// The escalation really did file a task against this lead — asserted, not
	// assumed, because a dedupe test whose second row never existed passes
	// while proving nothing.
	if tasks := tasksLinkedToLead(t, e, lead); tasks != 1 {
		t.Fatalf("the escalation filed %d task(s) on the lead, want 1 — with none "+
			"there is no duplicate for the fold to remove", tasks)
	}

	page := ownQueue(e.As(e.Rep1, []ids.UUID{e.Team1}, leadRepPerms), t, e)
	if got := leadRows(page); len(got) != 1 {
		t.Fatalf("lead rows = %v, want the one lead", got)
	}
	for _, item := range page.Queue {
		if string(item.Source) == "task" && item.Subject != nil &&
			string(item.Subject.Type) == "lead" && ids.UUID(item.Subject.Id) == lead {
			t.Error("the escalation's task is still on the page beside the lead it is " +
				"about — one late reply reads as two things to do")
		}
	}
}

// Past the lane's lead, a further overdue lead sinks below other work without
// leaving the page.
//
// leadLead = 8, so nine breached leads is the smallest fixture that can tell the
// crowding rule from its absence. What it asserts is NOT a cap: the constant's
// own comment says "a cap on how much of ONE kind a reader meets before they see
// the others, rather than a cap on the source: the rest stay ranked and
// reachable". My first version of this test asserted at-most-eight and failed
// against correct code — nine rows on a page holding only leads is right.
//
// So the fixture gives the rep one task as well, and the claim is about ORDER:
// the ninth lead must sort below that task, while the first eight sort above it.
// Without a competing row there is nothing for a crowded row to sink beneath and
// the case cannot fail.
func TestPastTheLanesLeadAFurtherLeadSinksBelowOtherWork(t *testing.T) {
	e := integration.Setup(t)
	measureFirstResponse(t, e)
	for i := range 9 {
		seedOwedLead(t, e, fmt.Sprintf("Breached %d", i), &e.Rep1, time.Duration(i+2)*time.Hour)
	}
	// One ordinary overdue task, as the other kind of work the crowded lead has
	// to sink beneath. Logged AS the rep, not as the admin: a task takes its
	// author as assignee, so the shared logTask helper would file it to somebody
	// this RowScopeOwn reader cannot see, and the page would carry no competing
	// row at all.
	rep := e.As(e.Rep1, []ids.UUID{e.Team1}, leadRepPerms)
	due := time.Now().UTC().Add(-2 * time.Hour)
	subject := "An ordinary overdue task"
	if _, _, err := e.Activities.LogActivity(rep, activities.LogActivityInput{
		Kind: "task", Subject: &subject, DueAt: &due, Source: "manual",
	}); err != nil {
		t.Fatalf("logging the competing task: %v", err)
	}

	page := ownQueue(rep, t, e)

	leadsAt := map[string]int{}
	taskAt := -1
	for i, item := range page.Queue {
		switch {
		case string(item.Source) == "lead_response" && item.Title != nil:
			leadsAt[*item.Title] = i
		case string(item.Source) == "task" && item.Title != nil && *item.Title == "An ordinary overdue task":
			taskAt = i
		}
	}
	if len(leadsAt) != 9 {
		t.Fatalf("the page carries %d lead rows, want all 9 — crowding sorts a row "+
			"down, it does not drop it", len(leadsAt))
	}
	if taskAt < 0 {
		t.Fatal("the competing task never reached the page, so nothing here could " +
			"tell a sunk row from a kept one")
	}
	// Which one is the ninth: the lane sorts by DEADLINE ASCENDING, and the
	// fixture's wait grows with i, so a larger i means an EARLIER deadline and
	// sorts FIRST. "Breached 0" waited least, so it is the ninth of its kind and
	// the one the cut marks. (The obvious reading is the wrong way round, and
	// asserting on "Breached 8" fails against correct code — it sits at 0.)
	const ninth = "Breached 0"
	if leadsAt[ninth] < taskAt {
		t.Errorf("the ninth lead sits at %d, above the task at %d — past the lane's "+
			"lead a further lead must sink below other work, or one kind fills the page",
			leadsAt[ninth], taskAt)
	}
	// And the other direction: the first eight are NOT crowded, so an
	// implementation that marked every lead would fail here rather than pass
	// the assertion above for the wrong reason.
	above := 0
	for name, at := range leadsAt {
		if name != ninth && at < taskAt {
			above++
		}
	}
	if above == 0 {
		t.Error("no lead sorts above the task, so every lead was crowded — the cut " +
			"is marking its whole lane rather than what exceeds its lead")
	}
}

// A lead survives a NAMED OWNER read, which is the path the page-level filter
// can get wrong.
//
// This is the case `mine` cannot reach. Under `mine` the store narrows in its
// own query, so the page-level filter has nothing left to decide and a broken
// one still looks right. Under `owner=<rep>` the rows arrive already narrowed by
// the store, and keepOwnedBy has to KEEP them on that evidence
// (narrowedByItsOwnLane) rather than re-deciding from an owner id — a lead's
// owner column and the queue it answers are not the same question, and comparing
// them drops the row whenever they disagree.
//
// What this does NOT prove, stated because the obvious reading of it is wrong:
// dropping sourceLeadResponse from narrowedByItsOwnLane leaves this test green
// too. Both of that helper's callers pair it with a fallback that already keeps
// a lead row — keepReadersOwn ORs it with ownedByReader, which returns true for
// any row it cannot attribute, and keepOwnedBy falls through to answersTo, which
// agrees with the owner for an owned lead. So the lead arm is a second answer to
// a question something else already answers, and no fixture reaches it. It is
// worth keeping for the reason its own comment gives — re-judging a row the lane
// already narrowed asks the wrong question — but a test claiming to hold it
// would be claiming more than it does.
func TestALeadSurvivesTheNamedOwnerRead(t *testing.T) {
	e := integration.Setup(t)
	measureFirstResponse(t, e)
	seedOwedLead(t, e, "Owed To The Rep", &e.Rep1, 3*time.Hour)

	// The lead opens their teammate's queue by name.
	lead := e.As(e.Rep2, []ids.UUID{e.Team1}, leadLeadPerms)
	svc := newAttentionService(e.Pool, approvals.NewService(e.DB()), failClosedOverlayMeter(), time.Now)
	page, err := svc.Worklist(lead, "team", "", e.Rep1, 50, "")
	if err != nil {
		t.Fatalf("opening the rep's queue: %v", err)
	}

	if got := leadRows(page); len(got) != 1 || got[0] != "Owed To The Rep" {
		t.Errorf("the rep's own queue carries %v, want their owed lead — the page-level "+
			"filter re-decided an answer the store had already narrowed", got)
	}
}

// raiseSLAEscalation runs the real breach handler, the way the bus does.
func raiseSLAEscalation(t *testing.T, e *integration.Env, lead ids.UUID) {
	t.Helper()
	deadline := time.Now().UTC().Add(-time.Hour)
	target := openapi_types.UUID(e.Rep1)
	payload, err := json.Marshal(crmcontracts.PublicEventLeadSlaBreached{
		Deadline: deadline, OwnerId: &target, EscalationTarget: &target,
	})
	if err != nil {
		t.Fatal(err)
	}
	ctx := principal.WithWorkspaceID(context.Background(), e.WS)
	ctx = principal.WithCorrelationID(ctx, ids.NewV7())
	ctx = principal.WithActor(ctx, principal.Principal{Type: principal.PrincipalSystem, ID: "system:test"})

	ev := workflow.Event{
		ID: ids.NewV7(), Type: "lead.sla_breached", WorkspaceID: e.WS, OccurredAt: deadline,
		Entity:  datasource.EntityRef{Type: datasource.EntityLead, ID: lead},
		Payload: payload,
	}
	h := newLeadSLAEscalation(e.DB(), func() time.Time { return deadline.Add(time.Minute) })
	eff, err := h.Plan(ctx, ev)
	if err != nil {
		t.Fatalf("planning the escalation: %v", err)
	}
	if _, err := h.Apply(ctx, ev, eff, nil); err != nil {
		t.Fatalf("applying the escalation: %v", err)
	}
}

// tasksLinkedToLead counts the open tasks filed against one lead.
func tasksLinkedToLead(t *testing.T, e *integration.Env, lead ids.UUID) int {
	t.Helper()
	return e.WsCount(t, `SELECT count(*) FROM activity a
		 JOIN activity_link l ON l.activity_id = a.id
		WHERE a.kind = 'task' AND NOT a.is_done AND l.lead_id = $1`, lead)
}
