// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package integration_test

// The workspace-wide promise read behind review_commitments, against a real
// database.
//
// The unit lane cannot see what these pin. Which promises the read admits is a
// pair of scope clauses and a join; the order that keeps the LIMIT from
// truncating away the answer is SQL; and the lifecycle filter that drops a
// settled promise touches two columns nothing in Go reads. Each of those reads
// as a working tool against a stub and can be wrong here.

import (
	"testing"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/compose/integration"
	"github.com/margince/margince/backend/internal/modules/activities"
	"github.com/margince/margince/backend/internal/modules/people"
	"github.com/margince/margince/backend/internal/platform/database"
)

var sweepClock = time.Date(2026, 8, 25, 9, 0, 0, 0, time.UTC)

func workspacePromises(t *testing.T, e *integration.Env, limit int) ([]people.OrgCommitment, bool) {
	t.Helper()
	var rows []people.OrgCommitment
	var more bool
	store := people.NewStore(e.DB())
	if err := database.WithWorkspaceTx(e.Admin(), e.Pool, func(tx pgx.Tx) error {
		var err error
		rows, more, err = store.OpenCommitmentsAcrossWorkspace(e.Admin(), tx, limit)
		return err
	}); err != nil {
		t.Fatalf("reading the workspace's open promises: %v", err)
	}
	return rows, more
}

// The read the tool answers from must carry the promise that slipped MOST
// RECENTLY, even when more promises exist than the caller asked for. Ordered
// earliest-first and then cut, the bound keeps the oldest and drops the one
// still worth rescuing — and the tool reports the least recoverable promise on
// the record as the thing to do.
func TestABoundedSweepKeepsThePromiseThatSlippedLast(t *testing.T) {
	e := integration.Setup(t)
	person := e.SeedPerson(t, "Carol Wagner", nil)

	for days := 30; days >= 10; days -= 10 {
		due := sweepClock.AddDate(0, 0, -days)
		seedPromise(t, e, person, "Ancient promise", &due)
	}
	yesterday := sweepClock.AddDate(0, 0, -1)
	seedPromise(t, e, person, "Send yesterday's file", &yesterday)

	rows, more := workspacePromises(t, e, 1)

	if len(rows) != 1 {
		t.Fatalf("asked for 1 promise and got %d", len(rows))
	}
	if rows[0].Body != "Send yesterday's file" {
		t.Errorf("the bounded sweep kept %q; it must keep the promise that slipped most "+
			"recently, which is the one still worth rescuing", rows[0].Body)
	}
	if !more {
		t.Error("cut promises from the sweep and did not say so; a model reports a bounded " +
			"set as everything outstanding unless told otherwise")
	}
}

// A promise somebody settled is done with, and one the extractor flagged as
// contested would state as true the very thing it doubted.
func TestASettledPromiseLeavesTheSweep(t *testing.T) {
	e := integration.Setup(t)
	person := e.SeedPerson(t, "Carol Wagner", nil)
	due := sweepClock.AddDate(0, 0, -2)
	claim := seedPromise(t, e, person, "Send the quote", &due)

	if rows, _ := workspacePromises(t, e, 20); len(rows) != 1 {
		t.Fatalf("the promise is not in the sweep to begin with: %d rows", len(rows))
	}

	// The extractor's own lifecycle column, which has no writer on this path,
	// so the filter is exercised against the state it exists to reject.
	e.WsExec(t, `UPDATE conversation_claim SET status = 'done' WHERE id = $1`, claim)

	if rows, _ := workspacePromises(t, e, 20); len(rows) != 0 {
		t.Errorf("a settled promise is still owed according to the sweep: %d rows", len(rows))
	}
}

// The task read behind review_commitments must keep the promise that slipped
// LAST when the bound cuts, for the same reason the claim read must.
//
// Ordered earliest-first and then truncated — which is what the display queue
// wants and what this read did — a bounded sweep holds the oldest promises and
// drops the one that slipped yesterday. The tool then names the least
// recoverable promise on the record and the read looks like it worked.
func TestABoundedTaskSweepKeepsTheTaskThatSlippedLast(t *testing.T) {
	e := integration.Setup(t)
	person := e.SeedPerson(t, "Carol Wagner", nil)
	seedTask := func(subject string, dueDaysAgo int) {
		t.Helper()
		due := sweepClock.AddDate(0, 0, -dueDaysAgo)
		if _, _, err := e.Activities.LogActivity(e.Admin(), activities.LogActivityInput{
			Kind: "task", Subject: &subject, DueAt: &due, Source: "manual",
			Links: []activities.ActivityLinkInput{{EntityType: "person", EntityID: person}},
		}); err != nil {
			t.Fatalf("logging %q: %v", subject, err)
		}
	}
	seedTask("Chase the ancient NDA", 30)
	seedTask("Chase the older NDA", 20)
	seedTask("Send yesterday's file", 1)

	rows, more, err := activities.NewStore(e.DB()).ListOpenTasks(e.Admin(),
		activities.ListOpenTasksInput{Limit: 1, MostRecentlySlippedFirst: true})
	if err != nil {
		t.Fatalf("reading the open tasks: %v", err)
	}

	if len(rows) != 1 {
		t.Fatalf("asked for 1 task and got %d", len(rows))
	}
	if rows[0].Subject != "Send yesterday's file" {
		t.Errorf("the bounded sweep kept %q; it must keep the task that slipped most recently, "+
			"which is the one still worth rescuing", rows[0].Subject)
	}
	if !more {
		t.Error("cut tasks from the sweep and did not say so")
	}
}

// The queue ordering is unchanged for the callers that DISPLAY a list: they
// want the earliest deadline at the top, and the flag is what separates the
// two questions rather than one order being made to serve both.
func TestTheDisplayQueueStillLeadsWithTheEarliestDeadline(t *testing.T) {
	e := integration.Setup(t)
	person := e.SeedPerson(t, "Carol Wagner", nil)
	for _, row := range []struct {
		subject    string
		dueDaysAgo int
	}{{"Chase the ancient NDA", 30}, {"Send yesterday's file", 1}} {
		due := sweepClock.AddDate(0, 0, -row.dueDaysAgo)
		subject := row.subject
		if _, _, err := e.Activities.LogActivity(e.Admin(), activities.LogActivityInput{
			Kind: "task", Subject: &subject, DueAt: &due, Source: "manual",
			Links: []activities.ActivityLinkInput{{EntityType: "person", EntityID: person}},
		}); err != nil {
			t.Fatalf("logging %q: %v", subject, err)
		}
	}

	rows, _, err := activities.NewStore(e.DB()).ListOpenTasks(e.Admin(),
		activities.ListOpenTasksInput{Limit: 5})
	if err != nil {
		t.Fatalf("reading the open tasks: %v", err)
	}

	if len(rows) == 0 || rows[0].Subject != "Chase the ancient NDA" {
		t.Errorf("the queue leads with %v; a display list is ordered by the earliest deadline",
			rows)
	}
}
