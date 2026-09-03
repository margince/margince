// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package meetingbrief

import (
	"slices"
	"testing"
)

// The excerpt budget is six threads, and a moment can hold more. What a reader
// preparing for a meeting needs is what was said LAST, so the six that get
// their bodies read are the six most recent.
//
// The shape here is the one that produced the bug: a fortnight of calendar
// invitations, then a note written minutes before the meeting. Every row sits
// inside arcGapDays, so clusterThreads folds them into ONE moment and the cap
// is decided purely by walk order.
func TestTheNewestConversationIsExcerptedBeforeTheOldest(t *testing.T) {
	var history []HistoryIn
	for i := range 8 {
		history = append(history, mail(
			"invite-"+string(rune('a'+i)), i+1,
			"Invitation: Forum "+string(rune('a'+i)), "inbound"))
	}
	note := HistoryIn{ID: "the-note", Kind: "note", Subject: "CRM", At: at(9)}
	history = append(history, note)

	moments := clusterThreads(threadsOf(history))
	if len(moments) != 1 {
		t.Fatalf("moments = %d, want 1 — every row is inside arcGapDays", len(moments))
	}
	targets := excerptTargets(moments)
	if !slices.Contains(targets, note.ID) {
		t.Errorf("the note's body was never read: targets = %v", targets)
	}
	if len(targets) > excerptThreads*excerptRowsPerThread {
		t.Errorf("targets = %d, want at most %d — the budget still binds",
			len(targets), excerptThreads*excerptRowsPerThread)
	}
}

// The cap is what makes the order matter: with more threads than budget, the
// oldest are the ones dropped, not the newest.
func TestTheOldestConversationsAreWhatTheBudgetDrops(t *testing.T) {
	var history []HistoryIn
	for i := range excerptThreads + 2 {
		history = append(history, mail(
			"thread-"+string(rune('a'+i)), i+1,
			"Subject "+string(rune('a'+i)), "inbound"))
	}
	targets := excerptTargets(clusterThreads(threadsOf(history)))
	if slices.Contains(targets, "thread-a") {
		t.Errorf("the oldest thread was excerpted over a newer one: %v", targets)
	}
	if !slices.Contains(targets, "thread-h") {
		t.Errorf("the newest thread was not excerpted: %v", targets)
	}
}

// Ordering for the budget must not reorder the arc a reader is shown. The
// moment keeps the threads in the order it was built with.
func TestPickingExcerptsLeavesTheMomentsOwnOrderAlone(t *testing.T) {
	history := []HistoryIn{
		mail("older", 1, "First", "inbound"),
		mail("newer", 5, "Second", "outbound"),
	}
	moments := clusterThreads(threadsOf(history))
	before := make([]string, 0, len(moments[0].Threads))
	for _, current := range moments[0].Threads {
		before = append(before, current.Key)
	}
	excerptTargets(moments)
	for i, current := range moments[0].Threads {
		if current.Key != before[i] {
			t.Fatalf("moment thread %d = %q, want %q — the arc's order moved",
				i, current.Key, before[i])
		}
	}
}
