// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package briefs

// What yesterday's queue said, read back on today's.
//
// The three cases are one distinction told three ways, and the distinction is
// the whole point: a deal that was ranked yesterday, a deal that was not, and a
// morning with no yesterday at all. A reader that collapsed the second into the
// third would call a months-old deal new; one that collapsed the third into the
// second would tell a rep on their first morning that nothing had changed.

import (
	"context"
	"testing"
	"time"

	"github.com/margince/margince/backend/internal/compose/integration"
)

// theNextMorning is the run one day on from the fixture's own.
var theNextMorning = briefClock.Add(24 * time.Hour)

// itemFor is the queue entry for one deal, or nil when it did not rank.
func itemFor(run BriefRun, dealID interface{ String() string }) *BriefRunItem {
	for i := range run.Items {
		if run.Items[i].DealID.String() == dealID.String() {
			return &run.Items[i]
		}
	}
	return nil
}

func TestADealRankedYesterdayCarriesWhereItStood(t *testing.T) {
	b := setupBrief(t)
	first, err := b.engine.SnapshotRun(b.repCtx, briefClock)
	if err != nil {
		t.Fatal(err)
	}
	if len(first.Items) == 0 {
		t.Fatal("the fixture ranked nothing, so there is no yesterday to carry")
	}
	stuck := first.Items[0]

	if _, err := b.engine.SnapshotRun(b.repCtx, theNextMorning); err != nil {
		t.Fatal(err)
	}
	today, err := b.engine.LatestRun(b.repCtx, theNextMorning)
	if err != nil {
		t.Fatal(err)
	}

	if today.PreviousDay.IsZero() {
		t.Fatal("the run names no previous day, so nothing on it can be read as a comparison")
	}
	if got, want := today.PreviousDay, first.LocalDay; !got.Equal(want) {
		t.Errorf("the run compares against %s, want the run before it (%s)", got, want)
	}
	item := itemFor(today, stuck.DealID)
	if item == nil {
		t.Fatalf("the deal ranked first yesterday is absent today: %v", today.Items)
	}
	if item.PreviousRank == nil {
		t.Fatal("a deal on the queue two mornings running carries no previous rank — " +
			"a brief that cannot see its own last edition reports a week-old deal as news")
	}
	if *item.PreviousRank != stuck.Rank {
		t.Errorf("previous rank = %d, want %d — the position it actually held", *item.PreviousRank, stuck.Rank)
	}
}

func TestADealThatDidNotRankYesterdayCarriesNoRankAndIsNotCalledNew(t *testing.T) {
	b := setupBrief(t)
	owner := integration.OwnerConn(t)
	if _, err := b.engine.SnapshotRun(b.repCtx, briefClock); err != nil {
		t.Fatal(err)
	}

	// Deal C exists and has existed all along — it simply sat below the bar. An
	// activity overnight lifts it onto today's queue without making it new.
	late := integration.SeedIDRow(t, owner, `INSERT INTO activity (id, kind, subject, occurred_at, source, captured_by)
		VALUES ($1, 'email', 'they wrote back', '2026-06-05T01:00:00Z', 'manual', 'human:x')`)
	integration.LinkActivity(t, owner, late, "deal", b.dealC)
	if _, err := owner.Exec(context.Background(),
		`UPDATE stage SET win_probability = 70 WHERE id = (SELECT stage_id FROM deal WHERE id = $1)`,
		b.dealC); err != nil {
		t.Fatal(err)
	}

	if _, err := b.engine.SnapshotRun(b.repCtx, theNextMorning); err != nil {
		t.Fatal(err)
	}
	today, err := b.engine.LatestRun(b.repCtx, theNextMorning)
	if err != nil {
		t.Fatal(err)
	}
	item := itemFor(today, b.dealC)
	if item == nil {
		t.Fatalf("the fixture did not lift the below-the-bar deal onto today's queue: %v", today.Items)
	}
	if item.PreviousRank != nil {
		t.Errorf("a deal that did not rank yesterday carries previous rank %d", *item.PreviousRank)
	}
	// And the run still names the day it is comparing against, which is what
	// keeps "did not rank" from reading as "there was no yesterday".
	if today.PreviousDay.IsZero() {
		t.Error("the run names no previous day, so an absent rank cannot be told from an absent run")
	}
}

func TestAFirstMorningNamesNoPreviousDay(t *testing.T) {
	b := setupBrief(t)
	if _, err := b.engine.SnapshotRun(b.repCtx, briefClock); err != nil {
		t.Fatal(err)
	}
	first, err := b.engine.LatestRun(b.repCtx, briefClock)
	if err != nil {
		t.Fatal(err)
	}
	if !first.PreviousDay.IsZero() {
		t.Errorf("a first run compares against %s — there is nothing before it, and a rep told "+
			"nothing changed on their first morning has been told something false", first.PreviousDay)
	}
	for _, item := range first.Items {
		if item.PreviousRank != nil {
			t.Errorf("item %s carries a previous rank on a first run", item.ID)
		}
	}
}
