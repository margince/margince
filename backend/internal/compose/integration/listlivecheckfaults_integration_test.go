// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package integration

// How a Live List check behaves when something goes wrong around it: an
// erasure landing between the filter and the write, a stored filter that no
// longer compiles, and a pass long enough that its lists are checked at
// different times.

import (
	"context"
	"testing"
	"time"

	"github.com/margince/margince/backend/internal/compose"
	"github.com/margince/margince/backend/internal/modules/collections"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

type checkResult struct {
	checks []collections.LiveCheck
	err    error
}

// checkWhileHeld runs one pass while another transaction holds an
// uncommitted change, commits that change once the pass is waiting on its
// lock (or has already finished), and answers what the pass did.
func (f *liveListFixture) checkWhileHeld(t *testing.T, statement string, args ...any) checkResult {
	t.Helper()
	ctx := context.Background()
	held, err := f.e.Pool.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = held.Rollback(ctx) }() //craft:ignore swallowed-errors the rollback after a commit has nothing left to undo
	if _, err := held.Exec(ctx, statement, args...); err != nil {
		t.Fatal(err)
	}
	f.clock = f.clock.Add(time.Minute)
	done := make(chan checkResult, 1)
	go func() {
		checks, err := compose.CheckLiveLists(ctx, f.e.Pool, f.e.WS, func() time.Time { return f.clock })
		done <- checkResult{checks, err}
	}()
	tick := time.NewTicker(10 * time.Millisecond)
	defer tick.Stop()
	deadline := time.After(20 * time.Second)
	for waiting := false; !waiting; {
		select {
		case result := <-done:
			if err := held.Commit(ctx); err != nil {
				t.Fatal(err)
			}
			return result
		case <-deadline:
			t.Fatal("the check neither finished nor waited on a lock")
		case <-tick.C:
			var n int
			if err := f.e.Pool.QueryRow(ctx, `SELECT count(*) FROM pg_stat_activity
				WHERE datname = current_database() AND wait_event_type = 'Lock'`).Scan(&n); err != nil {
				t.Fatal(err)
			}
			waiting = n > 0
		}
	}
	if err := held.Commit(ctx); err != nil {
		t.Fatal(err)
	}
	return <-done
}

func TestAnErasureBetweenTheFilterAndTheWriteLeavesNoTrace(t *testing.T) {
	f := newLiveListFixture(t)
	f.check(t)
	subject := f.contactTitled(t, "Racing Buyer", "Buyer")
	// The erasure's first write to the record, held open while the check runs:
	// the check's filter still sees the record as it was.
	result := f.checkWhileHeld(t, `UPDATE contact SET title = NULL, archived_at = now() WHERE id = $1`, subject)
	if result.err != nil {
		t.Fatal(result.err)
	}
	if len(result.checks) != 1 || result.checks[0].Outcome != collections.CheckFailed {
		t.Fatalf("the racing check = %+v, want it to fail rather than write", result.checks)
	}
	if n := f.e.WsCount(t, `SELECT count(*) FROM list_live_member WHERE entity_id = $1`, subject); n != 0 {
		t.Fatalf("the erased record was written back into the last check (%d rows)", n)
	}
	if n := f.e.WsCount(t, `SELECT count(*) FROM list_member_event WHERE entity_id = $1`, subject); n != 0 {
		t.Fatalf("the erased record was recorded as joining (%d events)", n)
	}
	f.check(t)
	if n := f.e.WsCount(t, `SELECT count(*) FROM list_member_event WHERE entity_id = $1`, subject); n != 0 {
		t.Fatalf("the retry recorded %d events about the erased record", n)
	}
}

func TestAFilterThatNoLongerCompilesIsCheckedInvalidAndTheRestStillRun(t *testing.T) {
	f := newLiveListFixture(t)
	broken, err := f.store.CreateList(f.e.Admin(), collections.CreateListInput{
		Name: "Once valid", EntityType: "contact", ListType: "dynamic", Sharing: "workspace", Definition: titleIs("Seller"),
	})
	if err != nil {
		t.Fatal(err)
	}
	// A field the vocabulary has since lost, as a retired custom field leaves it.
	f.e.WsExec(t, `UPDATE list SET definition = '{"field": "cf_gone", "op": "eq", "value": "x"}' WHERE id = $1`, broken.ID)
	f.contactTitled(t, "Still Checked", "Buyer")
	outcomes := map[ids.ListID]string{}
	for _, check := range f.check(t) {
		outcomes[check.ListID] = check.Outcome
	}
	if outcomes[broken.ID] != collections.CheckInvalid || outcomes[f.list] != collections.CheckComplete {
		t.Fatalf("the pass answered %v, want the broken list invalid and the other complete", outcomes)
	}
	view, err := f.store.ListView(f.e.Admin(), broken.ID)
	if err != nil || view.LastCheck == nil || string(view.LastCheck.Outcome) != collections.CheckInvalid {
		t.Fatalf("the broken list's last check = %+v (%v), want invalid", view.LastCheck, err)
	}
}

func TestEachListIsStampedWithItsOwnCheckTime(t *testing.T) {
	f := newLiveListFixture(t)
	other, err := f.store.CreateList(f.e.Admin(), collections.CreateListInput{
		Name: "Sellers", EntityType: "contact", ListType: "dynamic", Sharing: "workspace", Definition: titleIs("Seller"),
	})
	if err != nil {
		t.Fatal(err)
	}
	start := f.clock
	reads := 0
	checks, err := compose.CheckLiveLists(context.Background(), f.e.Pool, f.e.WS, func() time.Time {
		reads++
		return start.Add(time.Duration(reads) * time.Minute)
	})
	if err != nil || len(checks) != 2 {
		t.Fatalf("the pass = %+v (%v)", checks, err)
	}
	stamps := map[time.Time]bool{}
	for _, id := range []ids.ListID{f.list, other.ID} {
		view, err := f.store.ListView(f.e.Admin(), id)
		if err != nil || view.LastCheck == nil {
			t.Fatalf("list %s: %+v (%v)", id, view.LastCheck, err)
		}
		stamps[view.LastCheck.CheckedAt.UTC()] = true
	}
	if len(stamps) != 2 {
		t.Fatalf("the two lists share one check time %v, want each its own", stamps)
	}
}
