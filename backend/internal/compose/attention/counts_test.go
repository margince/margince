// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package attention

// What the page says about the work it is NOT showing.
//
// The queue is a cut at 25 rows, and before these figures existed it said
// nothing about the rest — so a full first page made a real backlog read as
// zero, and a rep narrowing to one kind of work found rows they had no way to
// know were there.

import (
	"testing"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
)

// countFor finds one category's figures, failing loudly rather than returning a
// zero value: an absent category and a category with nothing in it are
// different answers, and a helper that conflated them would let a test pass on
// the first while claiming the second.
func countFor(t *testing.T, out crmcontracts.Worklist, category string) crmcontracts.WorklistCount {
	t.Helper()
	for _, count := range out.Counts {
		if string(count.Category) == category {
			return count
		}
	}
	t.Fatalf("the page reported no figures at all for %q", category)
	return crmcontracts.WorklistCount{}
}

// The page states how much of each kind of work it holds and how much it is
// showing. Without this the reader cannot tell a finished queue from a cut one.
func TestThePageCountsEachKindOfWork(t *testing.T) {
	tasks := []crmcontracts.AttentionItem{}
	for i := 0; i < 4; i++ {
		tasks = append(tasks, item("t"+string(rune('a'+i)), "task", withDue(rankInstant)))
	}
	day := crmcontracts.Attention{
		AsOf:    rankInstant,
		Planned: tasks,
		AtRisk:  lane(item("deal", "deal_at_risk", withDeal(50_000_00))),
	}

	got := (&Service{}).worklistFrom(t.Context(), day, scopeAll, "", 25, nil)

	if count := countFor(t, got, "tasks"); count.Considered != 4 || count.Shown != 4 {
		t.Fatalf("tasks: considered %d shown %d, wanted four of each", count.Considered, count.Shown)
	}
	if count := countFor(t, got, "deals_at_risk"); count.Considered != 1 {
		t.Fatalf("deals_at_risk considered %d, wanted the one deal", count.Considered)
	}
}

// Work that did not fit the page is COUNTED, which is the whole point: the cut
// is what the reader could not see, and a page that reported only what it drew
// would call a backlog empty.
func TestWorkBelowTheCutIsStillCounted(t *testing.T) {
	tasks := []crmcontracts.AttentionItem{}
	for i := 0; i < 9; i++ {
		tasks = append(tasks, item("t"+string(rune('a'+i)), "task", withDue(rankInstant)))
	}
	day := crmcontracts.Attention{AsOf: rankInstant, Planned: tasks}

	got := (&Service{}).worklistFrom(t.Context(), day, scopeAll, "", 3, nil)

	count := countFor(t, got, "tasks")
	if count.Shown != 3 {
		t.Fatalf("shown = %d, and the page carries three", count.Shown)
	}
	if count.Considered != 9 {
		t.Fatalf("considered = %d, and nine were read — the six past the cut are what this figure exists to report",
			count.Considered)
	}
}

// A narrowed page still says what the OTHER kinds held. Counting after the
// narrowing would answer "no tasks" on a page filtered to deals, when the
// honest answer is "tasks, not shown".
func TestANarrowedPageStillCountsTheKindsItIsNotDrawing(t *testing.T) {
	day := crmcontracts.Attention{
		AsOf:    rankInstant,
		Planned: []crmcontracts.AttentionItem{item("task", "task", withDue(rankInstant))},
		AtRisk:  lane(item("deal", "deal_at_risk", withDeal(50_000_00))),
	}

	got := (&Service{}).worklistFrom(t.Context(), day, scopeAll, "deals_at_risk", 25, nil)

	tasks := countFor(t, got, "tasks")
	if tasks.Considered != 1 {
		t.Fatalf("a page filtered to deals reported %d tasks considered, and there was one",
			tasks.Considered)
	}
	if tasks.Shown != 0 {
		t.Fatalf("a page filtered to deals reported %d tasks shown", tasks.Shown)
	}
}

// A folded group is one row standing for many, and the count says how many. A
// group counted as a single row would report a category as barely present on
// the page where it is the dominant thing.
func TestAFoldedGroupCountsTheItemsItStandsFor(t *testing.T) {
	failures := []crmcontracts.AttentionItem{}
	for i := 0; i < 6; i++ {
		row := item("f"+string(rune('a'+i)), "automation_run")
		cause := "automation_run:one-rule"
		row.CauseRef = &cause
		failures = append(failures, row)
	}
	day := crmcontracts.Attention{AsOf: rankInstant, AutomationHealth: &failures}

	got := (&Service{}).worklistFrom(t.Context(), day, scopeAll, "", 50, nil)

	count := countFor(t, got, "system")
	if count.Shown != 6 {
		t.Fatalf("shown = %d, and the folded row stands for six", count.Shown)
	}
}

// A category inherits its sources' honesty. Reporting a flat number over a pile
// the read never finished counting is the failure the flag exists to prevent.
func TestACategoryWhoseSourceHitItsBoundSaysThereMayBeMore(t *testing.T) {
	decisions := []crmcontracts.AttentionItem{}
	for i := 0; i < batchScanDepth; i++ {
		decisions = append(decisions, item("d"+string(rune(i)), "approval", withKind("send_email")))
	}
	day := crmcontracts.Attention{AsOf: rankInstant, NeedsYou: decisions}

	got := (&Service{}).worklistFrom(t.Context(), day, scopeAll, "", 25, nil)

	if count := countFor(t, got, "decisions"); !count.MoreAvailable {
		t.Fatal("a category read to its bound reported a flat count, so a client would draw it as a total")
	}
}

// Two reads of one unchanged day produce the same bytes. Map order is not an
// order, and a client diffing the payload would see a change that is not one.
func TestCountsAreOrderedTheSameWayTwice(t *testing.T) {
	day := crmcontracts.Attention{
		AsOf:     rankInstant,
		Planned:  []crmcontracts.AttentionItem{item("task", "task", withDue(rankInstant))},
		AtRisk:   lane(item("deal", "deal_at_risk", withDeal(50_000_00))),
		NeedsYou: []crmcontracts.AttentionItem{item("decision", "approval", withKind("send_email"))},
	}

	first := (&Service{}).worklistFrom(t.Context(), day, scopeAll, "", 25, nil)
	second := (&Service{}).worklistFrom(t.Context(), day, scopeAll, "", 25, nil)

	if len(first.Counts) != len(second.Counts) {
		t.Fatalf("two reads counted %d and %d categories", len(first.Counts), len(second.Counts))
	}
	for i := range first.Counts {
		if first.Counts[i].Category != second.Counts[i].Category {
			t.Fatalf("position %d was %q then %q", i, first.Counts[i].Category, second.Counts[i].Category)
		}
	}
}
