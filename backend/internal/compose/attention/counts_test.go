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

	got := (&Service{}).worklistFrom(t.Context(), day, scopeAll, "", 25, waitingRead{}, leadRead{})

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

	got := (&Service{}).worklistFrom(t.Context(), day, scopeAll, "", 3, waitingRead{}, leadRead{})

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

	got := (&Service{}).worklistFrom(t.Context(), day, scopeAll, "deals_at_risk", 25, waitingRead{}, leadRead{})

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

	got := (&Service{}).worklistFrom(t.Context(), day, scopeAll, "", 50, waitingRead{}, leadRead{})

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

	got := (&Service{}).worklistFrom(t.Context(), day, scopeAll, "", 25, waitingRead{}, leadRead{})

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

	first := (&Service{}).worklistFrom(t.Context(), day, scopeAll, "", 25, waitingRead{}, leadRead{})
	second := (&Service{}).worklistFrom(t.Context(), day, scopeAll, "", 25, waitingRead{}, leadRead{})

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
	for _, l := range everyLane() {
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

// EVERY SOURCE THE CONTRACT DECLARES REACHES THE GUARDRAILS BELOW.
//
// everyLane is a hand-written list, and a hand-written census is one that can
// fail short: it reads a smaller tree, reports PASS, and no assertion notices.
// Eleven of the contract's twenty sources were in it when this was written, and
// the nine missing ones included customer_waiting and dedupe_candidate — the
// two categories a reader is most likely to be lied to about.
//
// So the LIST is checked against the contract's own enum. A source added there
// and not here fails this, and the two guardrails below then cover it by
// arriving in everyLane at all.
func TestEverySourceTheContractDeclaresHasALane(t *testing.T) {
	covered := map[crmcontracts.WorklistItemSource]bool{}
	for _, l := range everyLane() {
		covered[crmcontracts.WorklistItemSource(l.source)] = true
	}
	for _, source := range everyWorklistSource() {
		if notADayLane[source] {
			continue
		}
		if !covered[source] {
			t.Errorf("source %q has no lane, so no guardrail below reaches it — "+
				"add one to everyLane", source)
		}
	}
}

// notADayLane are the sources classifyDay cannot produce, and why.
//
// Each is covered somewhere else or is not a producer at all. Listed rather
// than skipped silently, so a source added to this set is a decision somebody
// wrote down.
var notADayLane = map[crmcontracts.WorklistItemSource]bool{
	// A GROUP of rows rather than a producer: foldRoutineDecisions makes one
	// from rows that already have their own lanes, and its category is its
	// members'.
	crmcontracts.WorklistItemSourceBatch: true,
	// Read from its own store rather than from the assembled day: worklist.go
	// classifies the waiting lane separately, so no Attention field carries
	// it. Its own suite (waitinglane_integration_test.go) drives it end to end.
	crmcontracts.WorklistItemSourceCustomerWaiting: true,
}

// everyWorklistSource is the contract's own source list, derived from the
// generated Valid() rather than retyped: a list here would be the third copy of
// a vocabulary that already exists twice.
func everyWorklistSource() []crmcontracts.WorklistItemSource {
	all := []crmcontracts.WorklistItemSource{
		"approval", "dedupe_candidate", "task", "brief_item", "conversation_claim",
		"customer_waiting", "deal_at_risk", "meeting", "relationship_decay",
		"failed_approval", "dsr", "sync_health", "capture_health", "ai_work_health",
		"bounce", "undelivered", "automation_run", "notice", "introduction_request",
		"batch",
	}
	// Held against the generated enum: a value the contract drops fails here
	// rather than leaving a lane nothing produces.
	for _, source := range all {
		if !source.Valid() {
			panic("worklist source " + string(source) + " is not in the contract")
		}
	}
	return all
}

// THE HIDDEN-BACKLOG GUARDRAIL: no category holds work while reporting none.
//
// The concept names this as a metric whose target is zero incidents, and a
// metric with a target of zero is a test rather than a number somebody reads
// once a quarter. It is also the failure this whole accounting exists to
// prevent: a rep told "no decisions" over a pile of them stops believing the
// page, and nothing on the screen says the figure was short.
//
// Over the same lane corpus the agreement test uses, so a source added
// tomorrow is covered by being added THERE — one fixture list, two obligations.
func TestNoCategoryHoldsWorkWhileReportingNone(t *testing.T) {
	for _, l := range everyLane() {
		rows := classifyDay(l.day, rankInstant)
		counts := countsOf(rows, rows, map[crmcontracts.WorklistItemSource]bool{})

		category := rows[0].item.Category
		for _, count := range counts {
			if crmcontracts.WorklistItemCategory(count.Category) != category {
				continue
			}
			if count.Considered == 0 {
				t.Fatalf("%s: category %q holds work and reports none — the page says it is empty",
					l.source, category)
			}
			goto next
		}
		t.Fatalf("%s: category %q holds work and is absent from counts entirely",
			l.source, category)
	next:
	}
}

// A CATEGORY CUT FROM THE PAGE STILL REPORTS WHAT IT HELD.
//
// The cut is the ordinary case — twenty-five rows of a larger day — and a
// category that lost every row to it is exactly when a reader most needs to be
// told the work is there. Reporting nothing then is the hidden backlog by
// another route: not a wrong number, an absent one.
func TestACategoryCutFromThePageStillReportsWhatItHeld(t *testing.T) {
	for _, l := range everyLane() {
		rows := classifyDay(l.day, rankInstant)
		// Considered everything, shown nothing.
		counts := countsOf(rows, nil, map[crmcontracts.WorklistItemSource]bool{})

		if len(counts) == 0 {
			t.Fatalf("%s: a page that drew nothing reported no categories at all", l.source)
		}
		for _, count := range counts {
			if count.Considered == 0 {
				t.Fatalf("%s: category %q reports nothing considered over rows it held",
					l.source, count.Category)
			}
			if count.Shown != 0 {
				t.Fatalf("%s: category %q claims %d rows on a page that drew none",
					l.source, count.Category, count.Shown)
			}
		}
	}
}

// everyLane is one day per source the queue can produce, each holding exactly
// one row. Shared by every test that needs to reach all of them, so a new
// source is added once and inherits every obligation.
func everyLane() []struct {
	source crmcontracts.AttentionItemSource
	day    crmcontracts.Attention
} {
	return []struct {
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
		{"dedupe_candidate", crmcontracts.Attention{AsOf: rankInstant, NeedsYou: []crmcontracts.AttentionItem{
			item("x", "dedupe_candidate"),
		}}},
		{"brief_item", crmcontracts.Attention{AsOf: rankInstant, ThisMorning: []crmcontracts.AttentionItem{
			item("x", "brief_item"),
		}}},
		{"sync_health", crmcontracts.Attention{AsOf: rankInstant, SyncHealth: lane(
			item("x", "sync_health"),
		)}},
		{"capture_health", crmcontracts.Attention{AsOf: rankInstant, CaptureHealth: lane(
			item("x", "capture_health"),
		)}},
		{"ai_work_health", crmcontracts.Attention{AsOf: rankInstant, AiWorkHealth: lane(
			item("x", "ai_work_health"),
		)}},
		{"undelivered", crmcontracts.Attention{AsOf: rankInstant, Undelivered: lane(
			item("x", "undelivered"),
		)}},
		{"introduction_request", crmcontracts.Attention{AsOf: rankInstant, Introductions: lane(
			item("x", "introduction_request"),
		)}},
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
	out := (&Service{}).worklistFrom(t.Context(), day, scopeUnassigned, "", 25, waitingRead{}, leadRead{})

	count := countFor(t, out, "deals_at_risk")
	if count.Considered != 0 {
		t.Fatalf("considered = %d, and every row was filtered out", count.Considered)
	}
	if !count.MoreAvailable {
		t.Fatal("a lane read to its bound reported a finished read, so the page would say nothing was there")
	}
}
