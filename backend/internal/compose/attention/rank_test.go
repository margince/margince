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

func quiet(days int) func(*ranked) {
	return func(r *ranked) { r.waitingDays = days }
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

	if got[0].AboveNext.Comparator == crmcontracts.WorklistComparisonComparatorDeadline {
		t.Fatal("the row offers two identical-looking times as its reason")
	}
}
