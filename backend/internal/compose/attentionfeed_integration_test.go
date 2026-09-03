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

	"github.com/margince/margince/backend/internal/compose/integration"
	"github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/modules/activities"
	"github.com/margince/margince/backend/internal/modules/approvals"
	"github.com/margince/margince/backend/internal/modules/people"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// assembleFeed reads the whole feed at a chosen instant, through the wiring the
// route itself serves.
//
// newAttentionService is the production binding: arranging these seams here
// instead would let a test keep passing while the shipped feed lost a lane,
// which is exactly the gap the feed's stub-driven unit tests leave.
func assembleFeed(ctx context.Context, t *testing.T, e *integration.Env, now time.Time) crmcontracts.Attention {
	t.Helper()
	feed := newAttentionService(e.Pool, approvals.NewService(e.DB()), failClosedOverlayMeter(), func() time.Time { return now })
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
	// The decider leaves. Deleting the app_user is what a real departure does,
	// and approval_decided_by_fkey's ON DELETE SET NULL empties decided_by on
	// every approval they decided — the state the old predicate read as a
	// receipt. Driven through the deletion rather than by writing the NULL, so
	// the test still means something if that foreign key is ever changed.
	e.WsExec(t, `DELETE FROM app_user WHERE id = $1`, e.AdminUser)
	var decider *ids.UUID
	if err := e.Pool.QueryRow(e.Admin(), `SELECT decided_by FROM approval WHERE id = $1`, id).Scan(&decider); err != nil {
		t.Fatalf("reading back the decider: %v", err)
	}
	if decider != nil {
		t.Fatalf("deleting the decider left decided_by set: this test no longer reproduces a departure")
	}

	day := assembleFeed(e.Admin(), t, e, time.Now().UTC())
	if got := sourcesOn(day.DoneForYou); len(got) != 0 {
		t.Fatalf("the done-for-you lane = %v, want nothing: a person decided this", got)
	}
}

// The lane's bound is the end of the day, and a task due exactly at it belongs
// to tomorrow. Both sides of that boundary are pinned because it moved once: a
// clause written `<=` put a promise due at tomorrow 00:00 on today's list, which
// reports work late a day before it is.
func TestATaskDueExactlyAtTheBoundaryBelongsToTomorrow(t *testing.T) {
	e := integration.Setup(t)
	now := time.Now().UTC()
	endOfDay := now.Truncate(24 * time.Hour).Add(24 * time.Hour)
	logTask(t, e, "Due a moment before midnight", endOfDay.Add(-time.Second), false)
	logTask(t, e, "Due exactly at midnight", endOfDay, false)

	day := assembleFeed(e.Admin(), t, e, now)
	got := titlesOn(day.Planned)
	if len(got) != 1 || got[0] != "Due a moment before midnight" {
		t.Fatalf("the planned lane = %v, want only the task due before the day ends", got)
	}
}

// A receipt decided minutes ago must not be hidden by approvals staged since.
//
// The page is ordered by when an approval was STAGED, while the lane's window
// asks when it was DECIDED. Those disagree: a proposal staged last week and
// decided this morning sorts below one staged this morning. Applying the window
// after the page therefore discards rows the page spent its limit on and reports
// a quiet night over work that just ran. The window is asked in SQL for exactly
// this reason, and this seeds the shape that would otherwise starve.
func TestARecentReceiptIsNotBuriedByNewerStagings(t *testing.T) {
	e := integration.Setup(t)
	now := time.Now().UTC()
	person, err := e.People.CreatePerson(e.Admin(), people.CreatePersonInput{FullName: "Anna Weber"})
	if err != nil {
		t.Fatalf("creating the target: %v", err)
	}
	svc := approvals.NewService(e.DB())
	// The receipt: staged first, so every later staging sorts above it, and
	// marked as the system's own act decided just now.
	old := stageFor(t, e, svc, person, "Filed a message under Riverty")
	e.WsExec(t, `UPDATE approval
		    SET status = 'approved', decided_by_system = true, decided_at = now(),
		        created_at = now() - interval '7 days'
		  WHERE id = $1`, old)
	// Enough newer stagings, decided outside the window, to fill any page the
	// lane would ask for.
	for i := 0; i < doneLaneWidth+4; i++ {
		id := stageFor(t, e, svc, person, fmt.Sprintf("An older act %d", i))
		e.WsExec(t, `UPDATE approval
			    SET status = 'approved', decided_by_system = true,
			        decided_at = now() - interval '30 days'
			  WHERE id = $1`, id)
	}

	day := assembleFeed(e.Admin(), t, e, now)
	if got := titlesOn(day.DoneForYou); len(got) != 1 || got[0] != "Filed a message under Riverty" {
		t.Fatalf("the done-for-you lane = %v, want this morning's act", got)
	}
}

// stageFor stages one proposal against a person through the real service.
func stageFor(t *testing.T, e *integration.Env, svc *approvals.Service,
	person crmcontracts.Person, summary string,
) ids.ApprovalID {
	t.Helper()
	id, err := svc.Stage(e.Admin(), approvals.StageInput{
		Kind:           "send_email",
		ProposedChange: json.RawMessage(`{"body":"the follow-up"}`),
		DiffHash:       "receipt-" + ids.NewV7().String(),
		Summary:        summary,
		TargetType:     "person",
		TargetID:       ids.UUID(person.Id),
	})
	if err != nil {
		t.Fatalf("staging %q: %v", summary, err)
	}
	return id
}

// THE DAY ENDS AT THE INSTALLATION'S MIDNIGHT, through the shipped wiring.
//
// The boundary used to be UTC's wherever the installation was: a seat seven
// hours east saw work due through 07:00 tomorrow local, which is work they are
// not owed yet, and a seat west lost the evening's. The unit tests pin the
// arithmetic; this pins that the composition actually asks.
func TestTheWorklistsDayEndsAtTheInstallationsMidnight(t *testing.T) {
	e := integration.Setup(t)
	e.WsExec(t, `UPDATE setting SET value = '"Asia/Ho_Chi_Minh"'::jsonb WHERE key = 'installation.timezone'`)

	// 15:00 UTC is 22:00 in Ho Chi Minh City, so the local day ends at 17:00
	// UTC — two hours ahead — while UTC's own midnight is nine hours out.
	now := time.Date(2026, 6, 15, 15, 0, 0, 0, time.UTC)
	logTask(t, e, "Due late tonight, local", time.Date(2026, 6, 15, 16, 30, 0, 0, time.UTC), false)
	logTask(t, e, "Due tomorrow morning, local", time.Date(2026, 6, 15, 18, 0, 0, 0, time.UTC), false)

	day := assembleFeed(e.Admin(), t, e, now)
	got := titlesOn(day.Planned)
	if len(got) != 1 || got[0] != "Due late tonight, local" {
		t.Fatalf("the planned lane = %v, want only tonight's work: 18:00 UTC is 01:00 tomorrow "+
			"where this installation is, and UTC's midnight is not its day", got)
	}
}

// THE PLANNED BADGE COUNTS PAST THE LANE'S CAP, through the shipped wiring.
//
// The lane shows a dozen; a rep with thirteen used to see twelve cards and a
// badge of twelve, on a lane with no second page to reach the thirteenth by.
func TestThePlannedBadgeCountsEveryTaskDueNotJustThePage(t *testing.T) {
	e := integration.Setup(t)
	// A FIXED midday, not the clock: due-two-hours-from-now falls past the end
	// of the day for the last two hours of it, and every task would drop off the
	// lane — a test that passes for twenty-two hours a day and fails the merge
	// queue at random.
	now := time.Date(2026, 8, 4, 12, 0, 0, 0, time.UTC)
	due := now.Add(2 * time.Hour)
	// One past the cap, which is the case the badge used to misreport.
	for i := range 13 {
		logTask(t, e, fmt.Sprintf("Owed %d", i), due, false)
	}

	day := assembleFeed(e.Admin(), t, e, now)
	if len(day.Planned) != 12 {
		t.Fatalf("the lane carries %d cards, want the cap of twelve", len(day.Planned))
	}
	if day.Counts.Planned != 13 {
		t.Errorf("counts.planned = %d, want 13 — the badge is how many they have", day.Counts.Planned)
	}
}

// logOpenTaskLoggedAt writes an open task with its due date and its logged
// date under independent control — margince#3287 needs a promise that is far
// more overdue than its neighbours while having been FILED before them, which
// logTask cannot express since it always logs at the write instant.
func logOpenTaskLoggedAt(t *testing.T, e *integration.Env, subject string, due, occurred time.Time) {
	t.Helper()
	if _, _, err := e.Activities.LogActivity(e.Admin(), activities.LogActivityInput{
		Kind: "task", Subject: &subject, DueAt: &due, OccurredAt: &occurred, Source: "manual",
	}); err != nil {
		t.Fatalf("logging task %q: %v", subject, err)
	}
}

// The Planned lane's cap must keep the tasks nearest their deadline, not the
// tasks most recently logged. Ordered by occurred_at DESC, a dozen tasks filed
// after a stale promise fill the cap and the promise never reaches the page —
// the exact failure a "what do I owe today" surface exists to prevent.
func TestThePlannedLaneCapKeepsTheMostOverdueNotTheNewestLogged(t *testing.T) {
	e := integration.Setup(t)
	now := time.Date(2026, 8, 4, 12, 0, 0, 0, time.UTC)

	// Logged a month ago, due three weeks ago: the one thing on this page the
	// cap must not be allowed to drop.
	stale := "The promise that slipped three weeks ago"
	logOpenTaskLoggedAt(t, e, stale, now.Add(-21*24*time.Hour), now.Add(-30*24*time.Hour))

	// Twelve tasks logged an hour ago, due later today. Under occurred_at DESC
	// these are individually "newer" than the stale promise and fill the whole
	// cap on their own.
	for i := range 12 {
		logOpenTaskLoggedAt(t, e, fmt.Sprintf("Filed this morning %d", i), now.Add(time.Hour), now.Add(-time.Hour))
	}

	day := assembleFeed(e.Admin(), t, e, now)
	if len(day.Planned) != 12 {
		t.Fatalf("the lane carries %d cards, want the cap of twelve", len(day.Planned))
	}
	got := titlesOn(day.Planned)
	for _, title := range got {
		if title == stale {
			return
		}
	}
	t.Fatalf("the lane dropped the most overdue promise behind twelve tasks filed after it: %v", got)
}
