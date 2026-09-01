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
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
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

// The source-to-category map agrees with the classifiers.
//
// The classifiers decide a row's category; countsOf needs the same answer for a
// bounded source that produced no surviving row. Two spellings of one fact, so
// this feeds a row from each source through the real classification and
// requires categoryOfSource to have said the same thing. A source that changes
// lane fails here rather than reporting its truncation against the wrong cut.
func TestTheSourceMapAgreesWithTheClassifiers(t *testing.T) {
	lanes := []struct {
		source crmcontracts.AttentionItemSource
		day    crmcontracts.Attention
	}{
		{"task", crmcontracts.Attention{AsOf: rankInstant, Planned: []crmcontracts.AttentionItem{
			item("x", "task", withDue(rankInstant)),
		}}},
		{"deal_at_risk", crmcontracts.Attention{AsOf: rankInstant, AtRisk: lane(
			item("x", "deal_at_risk", withDeal(1000)),
		)}},
		{"meeting", crmcontracts.Attention{AsOf: rankInstant, Meetings: lane(
			item("x", "meeting", withDue(rankInstant)),
		)}},
		{"approval", crmcontracts.Attention{AsOf: rankInstant, NeedsYou: []crmcontracts.AttentionItem{
			item("x", "approval", withKind("send_email")),
		}}},
		{"notice", crmcontracts.Attention{AsOf: rankInstant, Notices: lane(item("x", "notice"))}},
		{"bounce", crmcontracts.Attention{AsOf: rankInstant, Bounces: lane(item("x", "bounce"))}},
		{"relationship_decay", crmcontracts.Attention{AsOf: rankInstant, RelationshipDecay: lane(
			item("x", "relationship_decay"),
		)}},
		{"conversation_claim", crmcontracts.Attention{AsOf: rankInstant, Commitments: lane(
			item("x", "conversation_claim", withDue(rankInstant)),
		)}},
		{"dsr", crmcontracts.Attention{AsOf: rankInstant, Dsr: lane(item("x", "dsr", withDue(rankInstant)))}},
		{"failed_approval", crmcontracts.Attention{AsOf: rankInstant, DidNotRun: lane(
			item("x", "failed_approval"),
		)}},
		{"automation_run", crmcontracts.Attention{AsOf: rankInstant, AutomationHealth: lane(
			item("x", "automation_run"),
		)}},
	}

	for _, l := range lanes {
		rows := classifyDay(l.day, rankInstant)
		if len(rows) != 1 {
			t.Fatalf("%s: the lane classified %d rows, and the fixture holds one", l.source, len(rows))
		}
		classified := rows[0].item.Category
		mapped := categoryOfSource(crmcontracts.WorklistItemSource(l.source))
		if classified != mapped {
			t.Fatalf("%s: the classifier files it under %q and the map says %q — a truncation of this source would be reported against the wrong cut",
				l.source, classified, mapped)
		}
	}
}

// A waiting customer, whose classifier takes a different argument shape.
func TestTheSourceMapAgreesForAWaitingCustomer(t *testing.T) {
	row := classifyWaiting(WaitingCustomer{Since: rankInstant}, rankInstant)

	if got := categoryOfSource(sourceWaiting); row.item.Category != got {
		t.Fatalf("the classifier files a wait under %q and the map says %q", row.item.Category, got)
	}
}

// A bounded source whose every row was filtered out still says the read did
// not finish.
//
// The scope filter runs before this snapshot, so a lane can come back full of a
// colleague's work and leave nothing behind. Counted from rows alone the
// category vanishes entirely and the page reports nothing was there — which is
// the false-empty this accounting exists to prevent, reintroduced by the
// accounting itself.
func TestABoundedSourceFilteredToNothingStillSaysThereMayBeMore(t *testing.T) {
	deals := []crmcontracts.AttentionItem{}
	for i := 0; i < quietDealBound; i++ {
		row := item("d"+string(rune('a'+i%26)), "deal_at_risk", withDeal(1000))
		row.Deal.OwnerId = uuidPtr(ids.MustParse("01a05500-0000-7000-8000-0000000000ee"))
		deals = append(deals, row)
	}
	day := crmcontracts.Attention{AsOf: rankInstant, AtRisk: &deals}

	// Unassigned drops every row that has an owner, so the lane read fifty and
	// kept none.
	out := (&Service{}).worklistFrom(t.Context(), day, scopeUnassigned, "", 25, nil)

	count := countFor(t, out, "deals_at_risk")
	if count.Considered != 0 {
		t.Fatalf("considered = %d, and every row was filtered out", count.Considered)
	}
	if !count.MoreAvailable {
		t.Fatal("a lane read to its bound reported a finished read, so the page would say nothing was there")
	}
}
