// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package compose

// The briefing lane against the real brief engine.
//
// Two of its rules cannot be proven with a fake reader, because both are about
// what the ENGINE answers rather than what the lane does with it: a rep with no
// run for today gets ErrNotFound, which must read as an empty morning and never
// as a withheld one, and an item the rep has answered must leave the lane so
// the worklist can be finished.
//
// A third needs the real DEAL store beside the engine: the lane keeps an entry
// only while its deal is still resolvable, and what makes one unresolvable is
// the archive stamp the product's own delete verb writes.

import (
	"context"
	"testing"
	"time"

	"github.com/margince/margince/backend/internal/compose/briefs"
	"github.com/margince/margince/backend/internal/compose/integration"
	"github.com/margince/margince/backend/internal/modules/deals"
	"github.com/margince/margince/backend/internal/modules/people"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// briefingLaneEnv is one workspace with the seam's own reader over the real
// engine — the same construction newAttentionHandlers makes.
type briefingLaneEnv struct {
	*integration.Env
	engine  *briefs.BriefEngine
	deals   *deals.Store
	reader  attentionBriefing
	repCtx  context.Context
	nowFunc func() time.Time
	now     time.Time
}

func setupBriefingLane(t *testing.T) *briefingLaneEnv {
	t.Helper()
	e := integration.Setup(t)
	b := &briefingLaneEnv{
		Env:    e,
		engine: briefs.NewBriefEngine(e.Pool, people.NewStore(InstallationDB(e.Pool))),
		now:    time.Date(2026, 6, 4, 8, 0, 0, 0, time.UTC),
	}
	b.nowFunc = func() time.Time { return b.now }
	b.deals = deals.NewStore(InstallationDB(e.Pool), DealsInstallation())
	b.reader = attentionBriefing{
		engine:  b.engine,
		figures: attentionDealFacts{store: b.deals},
		now:     b.nowFunc,
	}
	b.repCtx = e.As(e.Rep1, []ids.UUID{e.Team1}, integration.AdminPerms)
	return b
}

func TestAMorningWithNoRunReadsAsAnEmptyLaneNotARefusal(t *testing.T) {
	b := setupBriefingLane(t)

	// The rep has never had a brief. The engine answers not-found, and this
	// lane must turn that into "nothing this morning" — reporting it as a
	// refusal would tell her something was hidden when nothing was.
	entries, ran, err := b.reader.Queue(b.repCtx)
	if err != nil {
		t.Fatalf("a rep with no run got an error rather than an empty morning: %v", err)
	}
	if len(entries) != 0 {
		t.Fatalf("the lane carries %d entries for a rep with no run, want none", len(entries))
	}
	if ran {
		t.Fatal("ran = true for a rep with no run — the feed would tick a morning that never happened")
	}
}

func TestAnAnsweredBriefingItemLeavesTheLane(t *testing.T) {
	b := setupBriefingLane(t)
	seedBriefingLaneDeals(t, b)

	run, err := b.engine.SnapshotRun(b.repCtx, b.now)
	if err != nil {
		t.Fatal(err)
	}
	if len(run.Items) < 2 {
		t.Fatalf("the fixture queued %d items, and this test needs 2 to tell removal from emptiness", len(run.Items))
	}

	before, ran, err := b.reader.Queue(b.repCtx)
	if err != nil {
		t.Fatal(err)
	}
	if !ran {
		t.Fatal("ran = false over a snapshotted run — the feed would report a morning that plainly exists as never produced")
	}
	if len(before) != len(run.Items) {
		t.Fatalf("the lane carries %d of the run's %d items before anything is answered", len(before), len(run.Items))
	}

	answered := run.Items[0]
	if _, err := b.engine.MarkActed(b.repCtx, answered.ID, b.now.Add(time.Minute)); err != nil {
		t.Fatal(err)
	}

	after, _, err := b.reader.Queue(b.repCtx)
	if err != nil {
		t.Fatal(err)
	}
	if len(after) != len(before)-1 {
		t.Fatalf("the lane carries %d entries after one was answered, want %d — a worklist that cannot shrink cannot be finished",
			len(after), len(before)-1)
	}
	for _, entry := range after {
		if entry.ID == answered.ID {
			t.Fatal("the answered item is still in the lane")
		}
	}
}

func TestASetAsideBriefingItemComesBackWhenItsWindowPasses(t *testing.T) {
	b := setupBriefingLane(t)
	seedBriefingLaneDeals(t, b)

	run, err := b.engine.SnapshotRun(b.repCtx, b.now)
	if err != nil {
		t.Fatal(err)
	}
	item := run.Items[0]
	until := b.now.Add(3 * time.Hour)
	if _, err := b.engine.MarkSnoozed(b.repCtx, item.ID, until, b.now.Add(time.Minute)); err != nil {
		t.Fatal(err)
	}

	// While the window runs, the item is out of the lane.
	b.now = b.now.Add(time.Hour)
	during, _, err := b.reader.Queue(b.repCtx)
	if err != nil {
		t.Fatal(err)
	}
	for _, entry := range during {
		if entry.ID == item.ID {
			t.Fatal("a set-aside item is still in the lane inside its own window")
		}
	}

	// Once it passes, it is back — and nothing in this lane knows that rule.
	// The engine's own read resurfaces it, which is why the lane asks the
	// engine rather than deciding what a state means for itself.
	b.now = until.Add(time.Minute)
	after, _, err := b.reader.Queue(b.repCtx)
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for _, entry := range after {
		found = found || entry.ID == item.ID
	}
	if !found {
		t.Fatal("a set-aside item never came back after its window passed")
	}
}

// A deal the rep deletes in the morning takes its ranked row with it.
//
// DELETE /v1/deals/{id} is an archive: no row is removed, so the run keeps the
// entry and the lane used to serve it. Everything that would have furnished
// that row excludes an archived deal, which left the rep a row carrying no
// amount, no close date and no reason, still offering three verbs over a deal
// they had just deleted — and counting toward the day's total for the rest of
// the local day, because a brief is one run per rep per day.
func TestADeletedDealTakesItsBriefingRowWithIt(t *testing.T) {
	b := setupBriefingLane(t)
	seedBriefingLaneDeals(t, b)

	run, err := b.engine.SnapshotRun(b.repCtx, b.now)
	if err != nil {
		t.Fatal(err)
	}
	if len(run.Items) < 2 {
		t.Fatalf("the fixture queued %d items, and this test needs 2 to tell removal from emptiness", len(run.Items))
	}

	before, ran, err := b.reader.Queue(b.repCtx)
	if err != nil {
		t.Fatal(err)
	}
	if !ran || len(before) != len(run.Items) {
		t.Fatalf("the lane carries %d of the run's %d items (ran=%v) before anything is deleted",
			len(before), len(run.Items), ran)
	}

	// Through the product's own delete verb, not an UPDATE of its own: what
	// this test is about is that ArchiveDeal leaves the ranked row behind.
	deleted := run.Items[0]
	if _, err := b.deals.ArchiveDeal(b.repCtx, ids.From[ids.DealKind](deleted.DealID), nil); err != nil {
		t.Fatalf("deleting the deal: %v", err)
	}

	after, ran, err := b.reader.Queue(b.repCtx)
	if err != nil {
		t.Fatal(err)
	}
	if !ran {
		t.Fatal("ran = false after a deal was deleted — the morning still happened")
	}
	for _, entry := range after {
		if entry.DealID == deleted.DealID {
			t.Fatal("the deleted deal's row is still in the lane, where it can state nothing about itself")
		}
	}
	if len(after) != len(before)-1 {
		t.Fatalf("the lane carries %d entries after one deal was deleted, want %d — the others must survive",
			len(after), len(before)-1)
	}
}

// seedBriefingLaneDeals gives the ranking something to rank, through the real
// deal writer.
func seedBriefingLaneDeals(t *testing.T, b *briefingLaneEnv) {
	t.Helper()
	ctx := context.Background()
	owner := integration.OwnerConn(t)
	pipeline, _, _ := integration.DealFixture(t, b.Env)
	var stage ids.UUID
	if err := owner.QueryRow(ctx,
		`SELECT id FROM stage WHERE pipeline_id = $1 AND semantic = 'open' ORDER BY position LIMIT 1`,
		pipeline).Scan(&stage); err != nil {
		t.Fatalf("reading the fixture stage: %v", err)
	}
	if _, err := owner.Exec(ctx, `UPDATE stage SET win_probability = 80 WHERE id = $1`, stage); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"Briefing Deal A", "Briefing Deal B"} {
		if _, err := owner.Exec(ctx, `
			INSERT INTO deal (pipeline_id, stage_id, name, owner_id, status, source, captured_by,
			                  amount_minor, currency, expected_close_date)
			VALUES ($1, $2, $3, $4, 'open', 'manual', 'test', 6000000, 'EUR', $5)`,
			pipeline, stage, name, b.Rep1, b.now.AddDate(0, 0, 5)); err != nil {
			t.Fatalf("seeding %s: %v", name, err)
		}
	}
}
