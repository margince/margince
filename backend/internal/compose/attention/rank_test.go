// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package attention

// What the ranked queue PROMISES, and each way it can break that promise.
//
// The queue's whole claim is that a reader can trust the order without reading
// fourteen panels. Every test here is one ordering a rep would call wrong.

import (
	"testing"
	"time"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
)

// rankInstant is the moment every case below is judged against, so a test that
// asserts "overdue" is asserting a fact the clock set rather than the calendar.
var rankInstant = time.Date(2026, 8, 25, 9, 0, 0, 0, time.UTC)

// candidate builds a ranked row without going through a producer, so an
// ordering can be stated in the terms the comparator actually reads.
func candidate(id string, level int, opts ...func(*ranked)) ranked {
	row := ranked{
		item: crmcontracts.WorklistItem{
			Id:      id,
			Level:   level,
			Actions: []crmcontracts.WorklistItemActions{},
			Because: []crmcontracts.WorklistReason{},
		},
		occurredAt: rankInstant,
		asOf:       rankInstant,
	}
	for _, opt := range opts {
		opt(&row)
	}
	return row
}

func due(at time.Time) func(*ranked) {
	return func(r *ranked) {
		r.deadlineAt = at
		r.item.DueAt = &at
		r.overdue = at.Before(rankInstant)
	}
}

func expected(minor int64) func(*ranked) {
	return func(r *ranked) {
		r.expectedBase = minor
		r.hasExpected = true
	}
}

// quiet builds a wait of the given age the way classifyWaiting does: the true
// age for what a reader is shown, the bounded age for the order. Setting only
// one would model a row production never produces, and the ordering tests would
// then pass against a shape that cannot occur.
func quiet(days int) func(*ranked) {
	return func(r *ranked) {
		r.waitingDays = days
		r.waitingRank = min(days, waitingDaysCeiling)
	}
}

func idsOf(items []crmcontracts.WorklistItem) []string {
	out := make([]string, 0, len(items))
	for _, item := range items {
		out = append(out, item.Id)
	}
	return out
}

func assertOrder(t *testing.T, got []crmcontracts.WorklistItem, want ...string) {
	t.Helper()
	ids := idsOf(got)
	if len(ids) != len(want) {
		t.Fatalf("ranked %v, wanted %v", ids, want)
	}
	for i, id := range want {
		if ids[i] != id {
			t.Fatalf("ranked %v, wanted %v", ids, want)
		}
	}
}

// A pile of cheap work must never bury one customer. This is the failure the
// whole ranked queue exists to end: 188 contact decisions above an unanswered
// buyer on a six-figure deal.
func TestAPileOfHygieneNeverOutranksACustomerWaiting(t *testing.T) {
	rows := []ranked{}
	for i := 0; i < 60; i++ {
		rows = append(rows, candidate(string(rune('a'+i%26))+string(rune('0'+i/26)), levelRoutine))
	}
	rows = append(rows, candidate("buyer", levelWaiting))

	got := rankAll(rows)

	if got[0].Id != "buyer" {
		t.Fatalf("the waiting customer ranked %d of %d, behind hygiene", 1+indexOf(got, "buyer"), len(got))
	}
}

func indexOf(items []crmcontracts.WorklistItem, id string) int {
	for i, item := range items {
		if item.Id == id {
			return i
		}
	}
	return -1
}

// Deadline leads inside a level, which is the concept's own model: a date
// somebody agreed to is the fact that expires. The larger deal loses because it
// can still be worked tomorrow.
func TestADealClosingTomorrowBeatsALargerOneClosingInNineMonths(t *testing.T) {
	soon := candidate("closing-tomorrow", levelMaterialRisk,
		due(rankInstant.Add(24*time.Hour)), expected(90_000_00))
	later := candidate("closing-in-nine-months", levelMaterialRisk,
		due(rankInstant.Add(270*24*time.Hour)), expected(95_000_00))

	got := rankAll([]ranked{later, soon})

	assertOrder(t, got, "closing-tomorrow", "closing-in-nine-months")
}

// With the dates tied, money decides — and it decides on the BASE-currency
// figure. Comparing raw minor units would put ten million yen above sixty
// thousand euros, which is the wrong way round.
func TestWhenDatesTieTheBiggerExpectedRevenueLeads(t *testing.T) {
	same := rankInstant.Add(48 * time.Hour)
	yen := candidate("ten-million-yen", levelMaterialRisk, due(same), expected(58_000_00))
	euro := candidate("sixty-thousand-euro", levelMaterialRisk, due(same), expected(60_000_00))

	got := rankAll([]ranked{yen, euro})

	assertOrder(t, got, "sixty-thousand-euro", "ten-million-yen")
}

// An overdue item beats one merely due, whatever the second one is worth.
func TestAnOverdueItemLeadsOneThatIsMerelyDue(t *testing.T) {
	late := candidate("overdue", levelPromise, due(rankInstant.Add(-72*time.Hour)))
	soon := candidate("due-later", levelPromise, due(rankInstant.Add(6*time.Hour)), expected(500_000_00))

	got := rankAll([]ranked{soon, late})

	assertOrder(t, got, "overdue", "due-later")
}

// A deal whose value nobody recorded sorts BELOW one whose value is known.
// Absence of a figure is not a large figure, and the opposite reading puts every
// unpriced record at the head of the queue.
func TestADealWithNoAmountSortsLastNotFirst(t *testing.T) {
	unknown := candidate("no-amount", levelMaterialRisk)
	known := candidate("known-amount", levelMaterialRisk, expected(1_00))

	got := rankAll([]ranked{unknown, known})

	assertOrder(t, got, "known-amount", "no-amount")
}

// A pin is the reader's one override, and it crosses every band: a rep who
// pinned a merge means it, and a queue that quietly kept it at the bottom would
// be refusing an instruction rather than taking one.
func TestAPinnedHygieneItemLeadsTheWholeQueue(t *testing.T) {
	pinned := candidate("pinned-merge", levelPinned)
	waiting := candidate("buyer", levelWaiting)
	promise := candidate("promise", levelPromise)

	got := rankAll([]ranked{waiting, promise, pinned})

	assertOrder(t, got, "pinned-merge", "buyer", "promise")
}

// The order has to be READABLE, not merely correct: every row says which
// tie-break beat the row below it, and the values on both sides.
func TestEachRowNamesTheComparatorThatBeatTheNextOne(t *testing.T) {
	soon := candidate("soon", levelMaterialRisk, due(rankInstant.Add(24*time.Hour)))
	later := candidate("later", levelMaterialRisk, due(rankInstant.Add(240*time.Hour)))

	got := rankAll([]ranked{later, soon})

	if got[0].AboveNext == nil {
		t.Fatal("the leading row explains nothing about why it leads")
	}
	if got[0].AboveNext.Comparator != crmcontracts.WorklistComparisonComparatorDeadline {
		t.Fatalf("the row claims %q decided it, but the dates differ", got[0].AboveNext.Comparator)
	}
	if got[0].AboveNext.Mine == nil || got[0].AboveNext.Theirs == nil {
		t.Fatal("the comparison names no values, so a reader cannot check it")
	}
}

// A row that beat the next one on its BAND says so, rather than reaching for a
// tie-break that did not decide it.
func TestARowThatWonOnItsLevelSaysSoRatherThanNamingATieBreak(t *testing.T) {
	waiting := candidate("buyer", levelWaiting, expected(1_00))
	routine := candidate("merge", levelRoutine, expected(900_000_00))

	got := rankAll([]ranked{routine, waiting})

	if got[0].AboveNext.Comparator != crmcontracts.WorklistComparisonComparatorLevel {
		t.Fatalf("claimed %q, but what beat the next row was the level",
			got[0].AboveNext.Comparator)
	}
}

// The last row has nothing below it. Inventing a comparison there would be a
// sentence about a row that is not on the page.
func TestTheLastRowExplainsNothingBecauseNothingIsBelowIt(t *testing.T) {
	got := rankAll([]ranked{candidate("one", levelWaiting), candidate("two", levelPromise)})

	if got[len(got)-1].AboveNext != nil {
		t.Fatal("the last row compares itself against a row that does not exist")
	}
}

// Two rows alike in every comparator still need a stable order, or the queue
// reshuffles between reads and teaches the reader that the ranking means
// nothing. The ids break it, and the row says the truth: nothing else did.
func TestACompleteTieIsBrokenStablyAndSaysSo(t *testing.T) {
	first := rankAll([]ranked{candidate("b", levelAgreed), candidate("a", levelAgreed)})
	second := rankAll([]ranked{candidate("a", levelAgreed), candidate("b", levelAgreed)})

	assertOrder(t, first, "a", "b")
	assertOrder(t, second, "a", "b")
	if first[0].AboveNext.Comparator != crmcontracts.WorklistComparisonComparatorOrder {
		t.Fatalf("claimed %q decided a complete tie", first[0].AboveNext.Comparator)
	}
}

// With dates and money tied, the longer wait leads.
func TestWithEverythingElseEqualTheLongerWaitLeads(t *testing.T) {
	old := candidate("waiting-83-days", levelAgreed, quiet(83))
	recent := candidate("waiting-3-days", levelAgreed, quiet(3))

	got := rankAll([]ranked{recent, old})

	assertOrder(t, got, "waiting-83-days", "waiting-3-days")
}

// A comparator that "decided" on two equal values decided nothing a reader can
// check. The live page printed "07:30 against 07:30" and asked them to accept
// it as a reason.
func TestARowNeverClaimsADateDecidedWhenTheDatesAreTheSame(t *testing.T) {
	same := rankInstant.Add(24 * time.Hour)
	first := candidate("a", levelWaiting, due(same), quiet(9))
	second := candidate("b", levelWaiting, due(same), quiet(3))

	got := rankAll([]ranked{first, second})

	if got[0].AboveNext.Comparator == crmcontracts.WorklistComparisonComparatorDeadline {
		t.Fatal("the row claims its date beat an identical date")
	}
	if got[0].AboveNext.Comparator != crmcontracts.WorklistComparisonComparatorWaitingDays {
		t.Fatalf("claimed %q; the longer wait is what actually decided",
			got[0].AboveNext.Comparator)
	}
}

// Two instants a few seconds apart render as the same minute on screen. The
// live page printed "16:21 against 16:21" and called it the reason.
func TestADateComparisonIsNeverOfferedBetweenInstantsThatReadAlike(t *testing.T) {
	base := rankInstant.Add(-24 * time.Hour)
	first := candidate("a", levelAgreed, due(base), quiet(9))
	second := candidate("b", levelAgreed, due(base.Add(20*time.Second)), quiet(3))

	got := rankAll([]ranked{first, second})

	// The date DID decide, so the row says so — without offering two values a
	// reader would compare and find equal. Falling through to the next
	// comparator would name something that decided nothing, which is a
	// different lie than the one this fixes.
	if got[0].AboveNext.Comparator != crmcontracts.WorklistComparisonComparatorDeadline {
		t.Fatalf("claimed %q; the date is what decided", got[0].AboveNext.Comparator)
	}
	if got[0].AboveNext.Mine != nil || got[0].AboveNext.Theirs != nil {
		t.Fatal("the row offers two identical-looking times as its reason")
	}
}

// Occurrence publishes a DAY COUNT, and two instants that round to the same
// day count must not be offered as though the (equal) count decided anything
// a reader could check — even hours apart, which the old minute-resolution
// guard would have let through as two different-looking clock times.
func TestOccurrenceOffersNoValuesWhenTheTwoInstantsReadAlike(t *testing.T) {
	base := rankInstant.Add(-90*24*time.Hour + 12*time.Hour)
	first := candidate("a", levelWaiting)
	first.occurredAt = base
	second := candidate("b", levelWaiting)
	second.occurredAt = base.Add(5 * time.Hour)

	got := rankAll([]ranked{first, second})

	if got[0].AboveNext.Mine != nil || got[0].AboveNext.Theirs != nil {
		t.Fatal("the row offers two day counts that would print the same number as its reason")
	}
}

// The occurrence step is the fallback every non-waiting source falls through
// to once nothing else separated a pair. It must publish the shape its
// comparator name promises — a day count each side — rather than the two
// rows' raw occurredAt instants, which would show a reader a date under a
// heading about days.
func TestOccurrenceReportsDaysNotDates(t *testing.T) {
	older := candidate("older", levelWaiting)
	older.occurredAt = rankInstant.Add(-10 * 24 * time.Hour)
	newer := candidate("newer", levelWaiting)
	newer.occurredAt = rankInstant.Add(-2 * 24 * time.Hour)

	got := rankAll([]ranked{newer, older})

	above := got[0].AboveNext
	if above == nil {
		t.Fatal("the leading row explains nothing about why it leads")
	}
	if above.Comparator != crmcontracts.WorklistComparisonComparatorWaitingDays {
		t.Fatalf("claimed %q decided it, wanted waiting_days", above.Comparator)
	}
	if above.Mine == nil || above.Mine.Kind != "days" || above.Mine.Days == nil {
		t.Fatalf("mine = %+v, wanted a days-kind value", above.Mine)
	}
	if above.Theirs == nil || above.Theirs.Kind != "days" || above.Theirs.Days == nil {
		t.Fatalf("theirs = %+v, wanted a days-kind value", above.Theirs)
	}
	if *above.Mine.Days != 10 || *above.Theirs.Days != 2 {
		t.Fatalf("mine=%d theirs=%d, wanted 10 and 2", *above.Mine.Days, *above.Theirs.Days)
	}
}

// The occurrence step reads ranked.asOf, and production stamps it in exactly
// one place — worklistFrom, right before rankAll — rather than at each row's
// classifier. This drives the real production entry point rather than
// rankAll directly, so a caller that stopped stamping it breaks here: with no
// asOf, occurredDaysOf clamps every row to zero days and the step stops
// publishing values, which reads exactly like a passing suite until this
// test is the one that reaches the wiring.
func TestTheRealPipelineStampsAsOfBeforeRanking(t *testing.T) {
	day := crmcontracts.Attention{
		AsOf: rankInstant,
		Bounces: lane(
			item("older", "bounce", withOccurred(rankInstant.Add(-10*24*time.Hour))),
			item("newer", "bounce", withOccurred(rankInstant.Add(-2*24*time.Hour))),
		),
	}

	out := pricedWorklist(t, stubFX{base: "EUR"}, day)

	above := out.Queue[0].AboveNext
	if above == nil || above.Comparator != crmcontracts.WorklistComparisonComparatorWaitingDays {
		t.Fatalf("above = %+v, wanted a waiting_days comparator naming the occurrence gap", above)
	}
	if above.Mine == nil || above.Mine.Days == nil || *above.Mine.Days != 10 {
		t.Fatalf("mine = %+v, wanted a 10-day count — asOf did not reach the occurrence step through the real pipeline", above.Mine)
	}
	if above.Theirs == nil || above.Theirs.Days == nil || *above.Theirs.Days != 2 {
		t.Fatalf("theirs = %+v, wanted 2 days", above.Theirs)
	}
}

// A row that led because the one below it was CROWDED says so.
//
// Crowding is the first thing less() compares and the only step compare() never
// walked, so the two disagreed about a case the page reaches every day: past the
// eighth unanswered customer every further wait is crowded, sorts below the
// other kinds, and the row above it reported whichever later step happened to
// differ. The reader was handed a reason that did not decide anything — the
// exact failure compare() exists to prevent, in the one place nothing checked.
func TestARowThatWonBecauseTheNextIsCrowdedSaysSo(t *testing.T) {
	// Same level, same everything the later steps compare. Without crowding
	// these two are equal, so nothing but crowding can decide the pair.
	lead := candidate("lead", levelWaiting)
	crowded := candidate("crowded", levelWaiting)
	crowded.crowded = true

	got := rankAll([]ranked{crowded, lead})

	if got[0].Id != "lead" {
		t.Fatalf("the crowded row led: %q", got[0].Id)
	}
	if got[0].AboveNext == nil {
		t.Fatal("the leading row explains nothing about why it leads")
	}
	if got[0].AboveNext.Comparator != crmcontracts.WorklistComparisonComparatorCrowded {
		t.Fatalf("claimed %q decided it, but what beat the next row was that it is crowded",
			got[0].AboveNext.Comparator)
	}
}

// THE ORDERING IS AN ORDERING.
//
// less() feeds sort.SliceStable, which is entitled to assume a strict weak
// ordering: no row before itself, no pair each before the other, and no cycle
// of three. A comparator that breaks any of those does not merely mis-order —
// the sort may read past its slice or loop, and the failure arrives as a panic
// in production rather than as a wrong row here.
//
// Checked over a set built to exercise every step, so adding one to rankSteps
// without thinking about its ties fails here rather than on a customer's page.
func TestTheOrderingIsAStrictWeakOrdering(t *testing.T) {
	rows := orderingCorpus()

	for _, a := range rows {
		if less(a, a) {
			t.Fatalf("%q sorts before itself", a.item.Id)
		}
	}
	for _, a := range rows {
		for _, b := range rows {
			if a.item.Id == b.item.Id {
				continue
			}
			if less(a, b) && less(b, a) {
				t.Fatalf("%q and %q each sort before the other", a.item.Id, b.item.Id)
			}
		}
	}
	// Transitivity of the "neither is less" relation, which is the half a
	// comparator built from independent fields most easily breaks.
	equivalent := func(x, y ranked) bool { return !less(x, y) && !less(y, x) }
	for _, a := range rows {
		for _, b := range rows {
			for _, c := range rows {
				if equivalent(a, b) && equivalent(b, c) && !equivalent(a, c) {
					t.Fatalf("%q ties %q ties %q, but %q and %q do not tie",
						a.item.Id, b.item.Id, c.item.Id, a.item.Id, c.item.Id)
				}
			}
		}
	}
}

// THE ORDER DOES NOT DEPEND ON THE INPUT ORDER.
//
// The queue's whole claim is that the server decided the ranking. A rank that
// moved with whatever order the lanes happened to assemble in would be a claim
// the page could not keep, and the reader has no way to tell.
func TestTheRankedOrderIsTheSameWhateverOrderTheRowsArriveIn(t *testing.T) {
	rows := orderingCorpus()
	want := idsOf(rankAll(append([]ranked(nil), rows...)))

	// Every rotation, plus the reverse: enough permutations to catch an
	// ordering that leans on arrival while staying a test somebody can read.
	for shift := 1; shift < len(rows); shift++ {
		rotated := append(append([]ranked(nil), rows[shift:]...), rows[:shift]...)
		if got := idsOf(rankAll(rotated)); !sameOrder(got, want) {
			t.Fatalf("rotating the input by %d changed the page:\n got %v\nwant %v", shift, got, want)
		}
	}
	reversed := make([]ranked, 0, len(rows))
	for i := len(rows) - 1; i >= 0; i-- {
		reversed = append(reversed, rows[i])
	}
	if got := idsOf(rankAll(reversed)); !sameOrder(got, want) {
		t.Fatalf("reversing the input changed the page:\n got %v\nwant %v", got, want)
	}
}

// EVERY ROW'S REASON IS THE ONE THAT DECIDED IT.
//
// The property the two functions exist to keep, asked of every adjacent pair
// the corpus produces rather than of one hand-built case: the step compare()
// names must be the step less() stopped at.
func TestEveryRowNamesTheStepThatActuallyDecidedIt(t *testing.T) {
	ordered := rankAll(orderingCorpus())
	rows := orderingCorpus()
	byID := map[string]ranked{}
	for _, row := range rows {
		byID[row.item.Id] = row
	}

	for i := 0; i+1 < len(ordered); i++ {
		a, b := byID[ordered[i].Id], byID[ordered[i+1].Id]
		named := ordered[i].AboveNext
		if named == nil {
			t.Fatalf("row %q explains nothing about the row below it", ordered[i].Id)
		}
		deciding := "order"
		for _, step := range rankSteps {
			if decided, _ := step.decides(a, b); decided {
				deciding = step.name
				break
			}
		}
		if !namesStep(named.Comparator, deciding) {
			t.Fatalf("row %q claims %q decided it, but %q did",
				ordered[i].Id, named.Comparator, deciding)
		}
	}
}

// namesStep reports whether a comparator on the wire is how the named step
// describes itself. The occurrence step deliberately reports as waiting_days —
// the contract's word for "the older one leads" — and the pin step reports as
// pin though it orders on the level.
func namesStep(comparator crmcontracts.WorklistComparisonComparator, step string) bool {
	switch step {
	case "occurrence":
		// The contract's word for "the older one leads".
		return comparator == crmcontracts.WorklistComparisonComparatorWaitingDays
	case "band":
		// A band difference is caused either by the level or by crowding, and
		// the step names whichever it was — see its explain.
		return comparator == crmcontracts.WorklistComparisonComparatorLevel ||
			comparator == crmcontracts.WorklistComparisonComparatorCrowded
	}
	return string(comparator) == step
}

// orderingCorpus is a set of rows built to reach every step in rankSteps,
// including the ties that send a pair to the next one.
func orderingCorpus() []ranked {
	crowded := candidate("crowded", levelWaiting)
	crowded.crowded = true
	// A SECOND crowded row, so the corpus holds a pair that is crowded on both
	// sides. Without it every crowded comparison has one true side and one
	// false, and a step that wrongly claimed to decide every pair would still
	// produce a consistent order — the contradiction only appears when the
	// deciding field is equal and the step says it decided anyway.
	crowdedToo := candidate("crowded-too", levelWaiting)
	crowdedToo.crowded = true
	strong := candidate("strong", levelRoutine)
	strong.strength = 9
	weak := candidate("weak", levelRoutine)
	weak.strength = 1
	older := candidate("older", levelPromise)
	older.occurredAt = rankInstant.Add(-48 * time.Hour)
	newer := candidate("newer", levelPromise)
	newer.occurredAt = rankInstant.Add(-1 * time.Hour)
	aged := candidate("aged", levelWaiting)
	aged.waitingDays, aged.waitingRank = 12, 12
	fresh := candidate("fresh", levelWaiting)
	fresh.waitingDays, fresh.waitingRank = 2, 2

	return []ranked{
		candidate("pinned", levelPinned),
		crowded,
		candidate("waiting", levelWaiting),
		candidate("soon", levelMaterialRisk, due(rankInstant.Add(24*time.Hour))),
		candidate("later", levelMaterialRisk, due(rankInstant.Add(240*time.Hour))),
		candidate("rich", levelMaterialRisk, expected(900_000_00)),
		candidate("poor", levelMaterialRisk, expected(1_00)),
		crowdedToo,
		aged, fresh, strong, weak, older, newer,
		candidate("plain", levelRoutine),
	}
}

func sameOrder(got, want []string) bool {
	if len(got) != len(want) {
		return false
	}
	for i := range got {
		if got[i] != want[i] {
			return false
		}
	}
	return true
}

// EVERY BAND IS ONE CONTIGUOUS RUN.
//
// The client draws a heading where the band changes and never re-sorts, so a
// band that appeared twice would draw its heading twice and split one kind of
// work across the page with somebody else's rows in between.
//
// This is a property of the ORDERING, not of bandOf: the band is derived from
// the level, and the level is the ordering's second step, so a step inserted
// above the level would break contiguity while every band remained correct.
func TestEachBandIsOneContiguousRun(t *testing.T) {
	ordered := rankAll(orderingCorpus())

	seen := map[string]bool{}
	previous := ""
	for _, item := range ordered {
		if item.Band == nil {
			t.Fatalf("row %q carries no band, so the page cannot head it", item.Id)
		}
		band := string(*item.Band)
		if band == previous {
			continue
		}
		if seen[band] {
			t.Fatalf("band %q appears twice: row %q reopens it after %q", band, item.Id, previous)
		}
		seen[band] = true
		previous = band
	}
}

// The bands travel in DRAW order, including the ones this page has no rows for.
//
// A reader whose Now band is empty is being told something — nothing needs them
// today — and a client that inferred the headings from the rows it received
// could not say it.
func TestEveryBandIsReportedInDrawOrderIncludingTheEmptyOnes(t *testing.T) {
	// One row, so three of the four bands are necessarily empty.
	only := rankAll([]ranked{candidate("just-one", levelRoutine)})
	bands := bandsOf(only)

	if len(bands) != len(bandOrder) {
		t.Fatalf("reported %d bands, wanted all %d", len(bands), len(bandOrder))
	}
	for i, want := range bandOrder {
		if string(bands[i].Band) != want {
			t.Fatalf("band %d is %q, wanted %q — the draw order is not the reported order",
				i, bands[i].Band, want)
		}
	}
	total := 0
	for _, band := range bands {
		total += band.Shown
	}
	if total != len(only) {
		t.Fatalf("the bands account for %d rows over a page of %d", total, len(only))
	}
}

// A DEMOTED WAIT IS DEMOTED, NOT DUMPED.
//
// Crowding moves a row out of `now` — that is what stops a hundred replies
// owning the page. It must not move it below the hygiene: the ninth waiting
// customer is still somebody waiting, and a page that ranked it under a
// duplicate-merge suggestion would have solved the monopoly by lying about
// what matters.
//
// The case the corpus does not reach on its own, because it needs a crowded
// HIGH-level row against an uncrowded LOW-level one.
func TestACrowdedWaitStillLeadsTheHygiene(t *testing.T) {
	crowdedWait := candidate("crowded-wait", levelWaiting)
	crowdedWait.crowded = true
	routine := candidate("routine", levelRoutine)

	got := rankAll([]ranked{routine, crowdedWait})

	if got[0].Id != "crowded-wait" {
		t.Fatalf("the hygiene row led: %v", idsOf(got))
	}
	if got[0].Band == nil || string(*got[0].Band) != "keep_momentum" {
		t.Fatalf("a crowded wait banded as %v, wanted keep_momentum", got[0].Band)
	}
	// And it did leave `now`, or the demotion did nothing.
	uncrowded := candidate("uncrowded-wait", levelWaiting)
	lead := rankAll([]ranked{uncrowded})
	if lead[0].Band == nil || string(*lead[0].Band) != "now" {
		t.Fatalf("an uncrowded wait banded as %v, wanted now — so the demotion above proves nothing",
			lead[0].Band)
	}
}

// THE LEVELS ARE NEVER MIXED, WHICH THE BANDS MUST NOT UNDO.
//
// The bands sort above the level, so a band chosen against the level would let
// a lower-priority row lead a higher-priority one — and the contract promises
// levels are never mixed.
//
// The pairing that reaches it: classifyWaiting demotes a stale unfunded wait to
// levelRoutine, and banding that row by its SOURCE put it in keep_momentum,
// above the blocking decision at level 5 that it had just been demoted past.
func TestAStaleWaitDoesNotOutrankABlockingDecision(t *testing.T) {
	stale := candidate("stale-wait", levelRoutine)
	stale.item.Source = sourceWaiting
	blocking := candidate("blocking", levelBlocking)
	blocking.item.Source = "approval"

	got := rankAll([]ranked{stale, blocking})

	if got[0].Id != "blocking" {
		t.Fatalf("a level-%d row led a level-%d one: %v",
			levelRoutine, levelBlocking, idsOf(got))
	}
	// Both are hygiene, so both head the same band — the demotion put them in
	// the same place rather than merely reordering them.
	for _, item := range got {
		if item.Band == nil || string(*item.Band) != "review" {
			t.Fatalf("row %q banded as %v, wanted review", item.Id, item.Band)
		}
	}
}

// A BAND DIFFERENCE THAT IS NOT A LEVEL DIFFERENCE DOES NOT CLAIM TO BE ONE.
//
// Two agreed tasks band apart because one is a lead's follow-up and the other
// is not. Reporting "level 4 against level 4" would be a reason that refutes
// itself the moment a reader checks it.
func TestABandDifferenceOnTheSameLevelNamesNoLevelValues(t *testing.T) {
	// Both are TASKS. Without a source the category defaults to `system`, both
	// band as review, and the pair ties — a fixture that would pass this test
	// while proving nothing about the distinction it is named for.
	lead := candidate("lead-task", levelAgreed)
	lead.item.Source = sourceTask
	lead.item.Subject = &crmcontracts.AttentionSubject{
		Type: crmcontracts.AttentionSubjectType("lead"),
	}
	plain := candidate("plain-task", levelAgreed)
	plain.item.Source = sourceTask

	got := rankAll([]ranked{plain, lead})

	if got[0].Id != "lead-task" {
		t.Fatalf("the lead follow-up did not lead: %v", idsOf(got))
	}
	if got[0].AboveNext == nil {
		t.Fatal("the leading row explains nothing")
	}
	if got[0].AboveNext.Mine != nil || got[0].AboveNext.Theirs != nil {
		t.Fatalf("the row claims level %v against %v, but the levels are equal",
			got[0].AboveNext.Mine, got[0].AboveNext.Theirs)
	}
}
