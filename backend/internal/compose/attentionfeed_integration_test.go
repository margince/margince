// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package compose

// What actually reaches the day's page, against a real database.
//
// The feed's own tests answer every lane against stubs, which prove the
// assembly and nothing about the producers: a stub returns a row because the
// test told it to, so a producer that stopped reaching the surface would leave
// every one of them green. These drive the REAL writers — the staging service,
// the activity store, the person store — and read the whole feed back through
// the same wiring the HTTP handler uses, so a break anywhere between the write
// and the lane fails here.
//
// One lane is deliberately absent: `done_for_you` reads approvals that were
// approved with no decider, and no ordinary writer produces that pair. Seeding
// one by hand would prove only that this file can write a row.

import (
	"context"
	"encoding/json"
	"fmt"
	"testing"
	"time"

	"github.com/margince/margince/backend/internal/compose/attention"
	"github.com/margince/margince/backend/internal/compose/briefs"
	"github.com/margince/margince/backend/internal/compose/integration"
	"github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/modules/activities"
	"github.com/margince/margince/backend/internal/modules/approvals"
	"github.com/margince/margince/backend/internal/modules/people"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// assembleFeed reads the whole feed through the real seams, at a chosen instant.
//
// Built from newAttentionHandlers' own wiring rather than a second arrangement
// of it: a test that assembled its own service would pass while the wiring the
// product ships was broken.
func assembleFeed(ctx context.Context, t *testing.T, e *integration.Env, now time.Time) crmcontracts.Attention {
	t.Helper()
	db := InstallationDB(e.Pool)
	svc := approvals.NewService(e.DB())
	clock := func() time.Time { return now }
	// The four required seams are real. The last three are optional by
	// construction — nil means the installation does not serve that lane — and
	// leaving them out keeps each test's subject to the lane it is about.
	feed := attention.NewService(
		attentionApprovals{svc: svc},
		attentionDuplicates{store: people.NewStore(db)},
		attentionTasks{store: activities.NewStore(db)},
		attentionReceipts{svc: svc},
		attentionBriefing{engine: briefs.NewBriefEngine(e.Pool, people.NewStore(db)), now: clock},
		nil, nil, nil,
		clock,
	)
	day, err := feed.Assemble(ctx)
	if err != nil {
		t.Fatalf("assembling the day: %v", err)
	}
	return day
}

// sourcesOn names what a lane carries, for an assertion that reads as a
// sentence rather than as an index into a slice.
func sourcesOn(items []crmcontracts.AttentionItem) []string {
	out := make([]string, 0, len(items))
	for _, item := range items {
		out = append(out, string(item.Source))
	}
	return out
}

func titlesOn(items []crmcontracts.AttentionItem) []string {
	out := make([]string, 0, len(items))
	for _, item := range items {
		if item.Title != nil {
			out = append(out, *item.Title)
		}
	}
	return out
}

// logTask writes one task through the real activity writer, completing it
// through the real completion path rather than inserting a done row — a task is
// logged open and finished by an update, and a test that wrote the end state
// directly would prove nothing about how a task actually leaves the lane.
func logTask(t *testing.T, e *integration.Env, subject string, due time.Time, done bool) {
	t.Helper()
	row, _, err := e.Activities.LogActivity(e.Admin(), activities.LogActivityInput{
		Kind: "task", Subject: &subject, DueAt: &due, Source: "manual",
	})
	if err != nil {
		t.Fatalf("logging task %q: %v", subject, err)
	}
	if done {
		finished := true
		id := ids.From[ids.ActivityKind](ids.UUID(row.Id))
		if _, err := e.Activities.UpdateActivity(e.Admin(), id, activities.UpdateActivityInput{IsDone: &finished}); err != nil {
			t.Fatalf("completing task %q: %v", subject, err)
		}
	}
}

// A staged proposal is what the decision lane exists for. This drives the real
// staging service and asserts the proposal arrives with the verb that answers
// it — not a link to somewhere else it could be answered.
func TestAStagedProposalReachesTheDecisionLane(t *testing.T) {
	e := integration.Setup(t)
	person, err := e.People.CreatePerson(e.Admin(), people.CreatePersonInput{FullName: "Anna Weber"})
	if err != nil {
		t.Fatalf("creating the target: %v", err)
	}
	svc := approvals.NewService(e.DB())
	if _, err := svc.Stage(e.Admin(), approvals.StageInput{
		Kind:           "send_email",
		ProposedChange: json.RawMessage(`{"body":"the follow-up"}`),
		DiffHash:       "feed-" + ids.NewV7().String(),
		Summary:        "Send the follow-up to Anna Weber",
		TargetType:     "person",
		TargetID:       ids.UUID(person.Id),
	}); err != nil {
		t.Fatalf("staging the proposal: %v", err)
	}

	day := assembleFeed(e.Admin(), t, e, time.Now().UTC())
	if got := sourcesOn(day.NeedsYou); len(got) != 1 || got[0] != "approval" {
		t.Fatalf("the decision lane = %v, want the one staged approval", got)
	}
	item := day.NeedsYou[0]
	if item.Title == nil || *item.Title != "Send the follow-up to Anna Weber" {
		t.Errorf("the card reads %v, want the summary composed at staging", item.Title)
	}
	if !containsAction(item.Actions, "decide") {
		t.Errorf("the card offers %v, want the verb that answers it", item.Actions)
	}
	if day.Counts.NeedsYou != 1 {
		t.Errorf("the lane counts %d, want the one decision it can page to", day.Counts.NeedsYou)
	}
}

// A near-match found by the real dedupe ladder. The pair is what the reader
// compares, so a candidate that arrives without both faces named is a card
// nobody can answer.
func TestADetectedDuplicateReachesTheDecisionLane(t *testing.T) {
	e := integration.Setup(t)
	if _, err := e.People.CreatePerson(e.Admin(), people.CreatePersonInput{FullName: "Lucy Vo"}); err != nil {
		t.Fatalf("creating the incumbent: %v", err)
	}
	if _, err := e.People.CreatePerson(e.Admin(), people.CreatePersonInput{FullName: "LUCY VO"}); err != nil {
		t.Fatalf("creating the near-match: %v", err)
	}

	day := assembleFeed(e.Admin(), t, e, time.Now().UTC())
	if got := sourcesOn(day.NeedsYou); len(got) != 1 || got[0] != "dedupe_candidate" {
		t.Fatalf("the decision lane = %v, want the pair the create detected", got)
	}
	item := day.NeedsYou[0]
	if item.Pair == nil {
		t.Fatal("the pair is absent: a merge card names both records or it cannot be answered")
	}
	if item.Pair.Left.Label == "" || item.Pair.Right.Label == "" {
		t.Errorf("the pair reads %q vs %q, want both records named", item.Pair.Left.Label, item.Pair.Right.Label)
	}
}

// An overdue task is today's agreed work. Both halves of the filter are pinned:
// what is due is carried, and what is done or still ahead is not.
func TestAnOverdueTaskReachesThePlannedLane(t *testing.T) {
	e := integration.Setup(t)
	now := time.Now().UTC()
	logTask(t, e, "Ring the buyer back", now.Add(-2*time.Hour), false)
	logTask(t, e, "Already handled", now.Add(-3*time.Hour), true)
	logTask(t, e, "Next week's prep", now.AddDate(0, 0, 7), false)

	day := assembleFeed(e.Admin(), t, e, now)
	got := titlesOn(day.Planned)
	if len(got) != 1 || got[0] != "Ring the buyer back" {
		t.Fatalf("the planned lane = %v, want only the task actually due", got)
	}
	if day.Planned[0].Overdue == nil || !*day.Planned[0].Overdue {
		t.Error("the task is not marked overdue, so the reader cannot see which work slipped")
	}
}

// The cap-before-filter case, from the reader's side.
//
// The task read is bounded and the "open and due" test is applied afterwards in
// Go, so a pile of finished tasks ahead of one overdue promise can fill the scan
// and leave the lane empty. The day would read clear while the work was still
// there, and nobody reports a quiet day as a bug.
func TestAPileOfFinishedTasksDoesNotHideAnOverdueOne(t *testing.T) {
	e := integration.Setup(t)
	now := time.Now().UTC()
	// The overdue one is written FIRST so every finished task is newer, which
	// is the order the store returns and therefore the order that buries it.
	logTask(t, e, "The promise that slipped", now.Add(-48*time.Hour), false)
	for i := 1; i <= 140; i++ {
		logTask(t, e, fmt.Sprintf("Finished %d", i), now.Add(-time.Duration(i)*time.Minute), true)
	}

	day := assembleFeed(e.Admin(), t, e, now)
	if got := titlesOn(day.Planned); len(got) != 1 || got[0] != "The promise that slipped" {
		t.Fatalf("the planned lane = %v, want the overdue promise under a pile of finished work", got)
	}
}

func containsAction(actions []crmcontracts.AttentionItemActions, want string) bool {
	for _, action := range actions {
		if string(action) == want {
			return true
		}
	}
	return false
}

// A human's decision stays a human's decision, even after that human is gone.
//
// The receipt lane used to read "approved with no decider" as "the system did
// this". Deleting an app_user empties decided_by on every approval they ever
// decided, so their work would reappear on a lane headed "done for you" — the
// product claiming nobody was asked about something somebody was asked about.
// The lane now reads the decision's own marker, which a deletion cannot rewrite.
func TestADepartedColleaguesDecisionIsNotReportedAsTheSystemsWork(t *testing.T) {
	e := integration.Setup(t)
	person, err := e.People.CreatePerson(e.Admin(), people.CreatePersonInput{FullName: "Anna Weber"})
	if err != nil {
		t.Fatalf("creating the target: %v", err)
	}
	svc := approvals.NewService(e.DB())
	id, err := svc.Stage(e.Admin(), approvals.StageInput{
		Kind:           "send_email",
		ProposedChange: json.RawMessage(`{"body":"the follow-up"}`),
		DiffHash:       "receipt-" + ids.NewV7().String(),
		Summary:        "Send the follow-up to Anna Weber",
		TargetType:     "person",
		TargetID:       ids.UUID(person.Id),
	})
	if err != nil {
		t.Fatalf("staging the proposal: %v", err)
	}
	if _, err := svc.Decide(e.Admin(), id, true, nil); err != nil {
		t.Fatalf("approving as a human: %v", err)
	}
	// The decider leaves. The foreign key empties decided_by on the row they
	// decided, which is exactly the state the old predicate read as a receipt.
	e.WsExec(t, `UPDATE approval SET decided_by = NULL WHERE id = $1`, id)

	day := assembleFeed(e.Admin(), t, e, time.Now().UTC())
	if got := sourcesOn(day.DoneForYou); len(got) != 0 {
		t.Fatalf("the done-for-you lane = %v, want nothing: a person decided this", got)
	}
}
