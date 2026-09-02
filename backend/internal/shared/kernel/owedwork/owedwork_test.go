// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package owedwork_test

import (
	"testing"
	"time"

	"github.com/margince/margince/backend/internal/shared/kernel/owedwork"
)

var now = time.Date(2026, 9, 2, 12, 0, 0, 0, time.UTC)

// at builds a promise due the given number of days from now, negative for
// late. The ref is the caller's own handle, and every assertion below reads it
// rather than comparing whole items — which is what a surface does.
func at(ref string, days int, source owedwork.Source) owedwork.Item {
	due := now.AddDate(0, 0, days)
	return owedwork.Item{Ref: ref, Source: source, DueAt: &due, FiledAt: now.AddDate(0, 0, -30)}
}

func undated(ref string, filedDaysAgo int, source owedwork.Source) owedwork.Item {
	return owedwork.Item{Ref: ref, Source: source, FiledAt: now.AddDate(0, 0, -filedDaysAgo)}
}

func refOf(t *testing.T, item owedwork.Item) string {
	t.Helper()
	ref, ok := item.Ref.(string)
	if !ok {
		t.Fatalf("ref is %T, not the string the test put in", item.Ref)
	}
	return ref
}

// The rule that made this package necessary: a claim and a task rank by their
// dates alone. Given the same date, the answer may not depend on which table
// the promise was read from.
func TestSourceDoesNotChangeTheRanking(t *testing.T) {
	asClaim := []owedwork.Item{at("late", -5, owedwork.FromClaim), at("later", -1, owedwork.FromTask)}
	asTask := []owedwork.Item{at("late", -5, owedwork.FromTask), at("later", -1, owedwork.FromClaim)}

	fromClaims, ok := owedwork.MostRecentlySlipped(asClaim, now)
	if !ok {
		t.Fatal("two overdue promises and none was named")
	}
	fromTasks, ok := owedwork.MostRecentlySlipped(asTask, now)
	if !ok {
		t.Fatal("two overdue promises and none was named")
	}
	if got, want := refOf(t, fromClaims), refOf(t, fromTasks); got != want {
		t.Errorf("swapping which source holds which date changed the answer: %q then %q", want, got)
	}
	if got := refOf(t, fromClaims); got != "later" {
		t.Errorf("named %q; the most recently slipped promise is %q", got, "later")
	}
}

func TestMostRecentlySlippedPrefersTheRecoverablePromise(t *testing.T) {
	items := []owedwork.Item{
		at("ancient", -90, owedwork.FromTask),
		at("yesterday", -1, owedwork.FromClaim),
		at("ahead", 3, owedwork.FromTask),
	}
	got, ok := owedwork.MostRecentlySlipped(items, now)
	if !ok {
		t.Fatal("two promises are late and none was named")
	}
	if ref := refOf(t, got); ref != "yesterday" {
		t.Errorf("named %q; the latest slip is %q, the one still worth rescuing", ref, "yesterday")
	}
}

func TestATieGoesToThePromiseThatCanQuoteItself(t *testing.T) {
	items := []owedwork.Item{
		at("retyped", -2, owedwork.FromTask),
		at("quoted", -2, owedwork.FromClaim),
	}
	got, ok := owedwork.MostRecentlySlipped(items, now)
	if !ok {
		t.Fatal("both promises are late and none was named")
	}
	if ref := refOf(t, got); ref != "quoted" {
		t.Errorf("named %q; a tie goes to the claim, which carries the sentence it was read from", ref)
	}
}

func TestNothingLateIsNotAnOverduePromise(t *testing.T) {
	items := []owedwork.Item{at("ahead", 3, owedwork.FromTask), undated("someday", 10, owedwork.FromClaim)}
	if item, ok := owedwork.MostRecentlySlipped(items, now); ok {
		t.Errorf("named %q as overdue; neither promise is past a date", refOf(t, item))
	}
}

// A promise falling due exactly now is due, not late — the reading
// kernel/deadline states and this package inherits.
func TestAPromiseDueThisInstantIsNotYetLate(t *testing.T) {
	item := owedwork.Item{Ref: "onTheDot", Source: owedwork.FromTask, DueAt: &now, FiledAt: now}
	if item.Overdue(now) {
		t.Error("called a promise late at the instant it fell due")
	}
	if _, ok := owedwork.Soonest([]owedwork.Item{item}, now); !ok {
		t.Error("a promise due this instant is still owed and should be the next thing to do")
	}
}

func TestSoonestTakesTheNearestDeadlineAmongWhatIsNotLate(t *testing.T) {
	items := []owedwork.Item{
		at("late", -1, owedwork.FromTask),
		at("nextWeek", 7, owedwork.FromClaim),
		at("tomorrow", 1, owedwork.FromTask),
	}
	got, ok := owedwork.Soonest(items, now)
	if !ok {
		t.Fatal("two promises are still ahead and none was named")
	}
	if ref := refOf(t, got); ref != "tomorrow" {
		t.Errorf("named %q; the nearest deadline not yet passed is %q", ref, "tomorrow")
	}
}

// An undated promise is owed. A queue that drops it tells a reader they are
// clear when they are not.
func TestAnUndatedPromiseIsStillOwed(t *testing.T) {
	items := []owedwork.Item{undated("whitepaper", 4, owedwork.FromClaim)}
	got, ok := owedwork.Soonest(items, now)
	if !ok {
		t.Fatal("an undated promise reached no queue at all")
	}
	if ref := refOf(t, got); ref != "whitepaper" {
		t.Errorf("named %q, not the one promise there is", ref)
	}
}

func TestUndatedSortsBehindEveryDatedPromise(t *testing.T) {
	items := []owedwork.Item{
		undated("someday", 40, owedwork.FromClaim),
		at("nextMonth", 30, owedwork.FromTask),
	}
	got, ok := owedwork.Soonest(items, now)
	if !ok {
		t.Fatal("nothing was named from two open promises")
	}
	if ref := refOf(t, got); ref != "nextMonth" {
		t.Errorf("named %q; a dated promise comes before an undated one however far off", ref)
	}
}

func TestTheOldestUndatedPromiseComesFirst(t *testing.T) {
	items := []owedwork.Item{
		undated("recent", 2, owedwork.FromTask),
		undated("waitingLongest", 60, owedwork.FromClaim),
	}
	got, ok := owedwork.Soonest(items, now)
	if !ok {
		t.Fatal("nothing was named from two undated promises")
	}
	if ref := refOf(t, got); ref != "waitingLongest" {
		t.Errorf("named %q; among undated promises the one waiting longest comes first", ref)
	}
}

// The whole-list order, which is what a card at the top of a page and the list
// beneath it must share.
func TestSortedLeadsWithTheOverdueBlock(t *testing.T) {
	items := []owedwork.Item{
		undated("someday", 5, owedwork.FromClaim),
		at("tomorrow", 1, owedwork.FromTask),
		at("ancient", -90, owedwork.FromTask),
		at("yesterday", -1, owedwork.FromClaim),
	}
	var got []string
	for _, item := range owedwork.Sorted(items, now) {
		got = append(got, refOf(t, item))
	}
	want := []string{"yesterday", "ancient", "tomorrow", "someday"}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("order was %v; wanted %v", got, want)
		}
	}
}

// The card names one promise and the list ranks them all. Both read this
// package, and this test is what holds them to the same answer: it fails if
// MostRecentlySlipped and Sorted ever disagree about which promise leads.
func TestTheTopOfTheListIsTheCardsPromise(t *testing.T) {
	items := []owedwork.Item{
		at("ancient", -90, owedwork.FromTask),
		at("yesterday", -1, owedwork.FromClaim),
		at("ahead", 2, owedwork.FromTask),
	}
	card, ok := owedwork.MostRecentlySlipped(items, now)
	if !ok {
		t.Fatal("nothing was named for the card")
	}
	first := owedwork.Sorted(items, now)[0]
	if refOf(t, card) != refOf(t, first) {
		t.Errorf("the card names %q and the list leads with %q", refOf(t, card), refOf(t, first))
	}
}

func TestSortedKeepsEveryPromise(t *testing.T) {
	items := []owedwork.Item{
		at("late", -1, owedwork.FromTask),
		at("ahead", 1, owedwork.FromClaim),
		undated("someday", 3, owedwork.FromTask),
	}
	if got := len(owedwork.Sorted(items, now)); got != len(items) {
		t.Errorf("sorted %d promises into %d rows; ranking may not drop work", len(items), got)
	}
}

func TestSortedLeavesTheCallersSliceAlone(t *testing.T) {
	items := []owedwork.Item{at("ancient", -90, owedwork.FromTask), at("yesterday", -1, owedwork.FromClaim)}
	owedwork.Sorted(items, now)
	if ref := refOf(t, items[0]); ref != "ancient" {
		t.Errorf("Sorted reordered the caller's own slice; it now leads with %q", ref)
	}
}

func TestNoPromisesIsNoAnswer(t *testing.T) {
	if _, ok := owedwork.MostRecentlySlipped(nil, now); ok {
		t.Error("named an overdue promise from an empty record")
	}
	if _, ok := owedwork.Soonest(nil, now); ok {
		t.Error("named a next promise from an empty record")
	}
	if got := owedwork.Sorted(nil, now); len(got) != 0 {
		t.Errorf("sorted an empty record into %d rows", len(got))
	}
}
