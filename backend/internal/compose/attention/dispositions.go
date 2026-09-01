// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package attention

// Which rows a reader may put DOWN, and which verb each row is for.
//
// Both travel on the wire because both are the server's to decide. A client
// inferring "a waiting row can be snoozed" from `source` would be maintaining a
// second copy of a rule this package owns, and the copy fails silently in both
// directions: a verb drawn for a row the server refuses, or a verb withheld
// from a rep entitled to it.

import (
	crmcontracts "github.com/margince/margince/backend/internal/contracts"
)

// The ways a row can be set aside, spelled as the contract spells them.
const (
	disposeSnooze   = "snooze"
	disposeNotMine  = "not_mine"
	disposeNotSales = "not_sales"
)

// waitingDispositions are the three judgements a rep may make about an
// unanswered message.
//
// Only this source offers them, and the reason is worth stating: every other
// row on the queue is answered by the surface that owns it — an approval is
// decided, a task is completed, a duplicate is merged — and doing that is what
// makes it leave. An unanswered message has no such verb. Replying is not
// always the answer, and the three cases where it is not are exactly these.
func waitingDispositions() []crmcontracts.WorklistItemDispositions {
	return []crmcontracts.WorklistItemDispositions{
		disposeSnooze, disposeNotMine, disposeNotSales,
	}
}

// primaryActionFor is the one verb a row is FOR, out of the several it offers.
//
// The queue is ranked, so a reader arriving at a row should not have to weigh
// three equally-drawn controls to find the step the ranking already implies.
//
// Absent is a real answer rather than a gap: a duplicate pair genuinely asks
// the reader to choose which record survives, and naming one side as the
// obvious step would be this surface deciding something it has no basis to.
// A row carrying a Move keeps one anyway. The move is drawn as its own control
// beside the verbs rather than among them, so naming a primary verb here does
// not compete with it — the reader gets the prepared step first and the best of
// the ordinary verbs after it.
func primaryActionFor(item crmcontracts.WorklistItem) *crmcontracts.WorklistItemPrimaryAction {
	for _, want := range primaryVerbOrder {
		for _, offered := range item.Actions {
			if string(offered) == want {
				action := crmcontracts.WorklistItemPrimaryAction(want)
				return &action
			}
		}
	}
	return nil
}

// primaryVerbOrder ranks the verbs by how much of the row's work each finishes.
//
// `decide` and `merge` settle the row outright, so they lead. `complete` and
// `act` finish the work behind it. `open` is last of the acting verbs because
// it only takes the reader somewhere — it is the answer when nothing better is
// offered, never in preference to one.
//
// `snooze`, `dismiss` and `set_aside` are absent on purpose: putting work down
// is never the step a queue should suggest first, whatever the ranking says.
var primaryVerbOrder = []string{"decide", "merge", "complete", "act", "acknowledge", "open"}
