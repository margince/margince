// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package attention

// What a waiting row offers a reader who is not going to answer it now.
//
// The rule these hold is one sentence: do not offer an action the record cannot
// perform. A message can reach this queue without a thread — a first contact
// from an address nobody has written to, or a provider that hands over no chain
// to root on — and two of the three dispositions are keyed on the thread.
//
// Both failures were silent in their own way. `not_sales` answered a validation
// error after the reader had pressed it; a reply-snooze stored fine and then
// never woke, because the wake condition matches thread_key by equality and a
// NULL satisfies no equality. The second is the worse of the two: it looks like
// the row was put down on purpose.

import (
	"testing"
	"time"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// waitingWithThread builds a wait the classifier will rank, threaded or not.
func waitingWithThread(threaded bool) WaitingCustomer {
	return WaitingCustomer{
		ActivityID: ids.MustParse("01a05500-0000-7000-8000-0000000000d1"),
		Subject:    "Can you hold the price until Friday?",
		Since:      rankInstant.Add(-3 * 24 * time.Hour),
		Threaded:   threaded,
	}
}

// offeredOn renders one classified row the way the queue does and answers what
// it offers. Through renderInOrder rather than reading the classifier, because
// the stamping happens there — a test that read the classifier would pass while
// the queue offered something else.
func offeredOn(t *testing.T, waiting WaitingCustomer) []crmcontracts.WorklistItemDispositions {
	t.Helper()
	rows := renderInOrder([]ranked{classifyWaiting(waiting, rankInstant)}, ids.Nil)
	if len(rows) != 1 {
		t.Fatalf("rendered %d rows from one wait", len(rows))
	}
	if rows[0].Dispositions == nil {
		t.Fatal("a waiting row offered no dispositions at all — it is the one source that has them")
	}
	return *rows[0].Dispositions
}

func TestAThreadedWaitOffersAllThreeJudgements(t *testing.T) {
	got := offeredOn(t, waitingWithThread(true))

	for _, want := range []string{disposeSnooze, disposeNotMine, disposeNotSales} {
		if !offers(got, want) {
			t.Errorf("a threaded wait does not offer %q, and every one of the three is a case where "+
				"replying is not the answer", want)
		}
	}
}

// The case this exists for.
func TestAThreadlessWaitOffersOnlyWhatItCanPerform(t *testing.T) {
	got := offeredOn(t, waitingWithThread(false))

	if offers(got, disposeNotSales) {
		t.Error("a threadless wait offers not_sales, which judges the THREAD — SetThreadNotSales " +
			"refuses a row without one, so the reader presses it and gets a validation error")
	}
	if offers(got, disposeSnooze) {
		t.Error("a threadless wait offers snooze, and the reply wake matches thread_key by equality — " +
			"a NULL satisfies none, so the row is hidden permanently even when the customer writes back")
	}
	// Something, rather than nothing: not_mine is per-reader and keyed on the
	// activity, so whoever is looking at the row can still set it aside.
	if !offers(got, disposeNotMine) {
		t.Error("a threadless wait offers nothing at all, leaving a reader with a row they cannot put down")
	}
}

func offers(set []crmcontracts.WorklistItemDispositions, want string) bool {
	for _, got := range set {
		if string(got) == want {
			return true
		}
	}
	return false
}
