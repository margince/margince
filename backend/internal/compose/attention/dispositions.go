// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package attention

// Which rows a reader may put DOWN, and which verb each row is for. Both travel
// on the wire because both are the server's to decide: a client inferring them
// from `source` keeps a second copy of a rule this package owns, and it fails
// silently both ways — a verb drawn for a row the server refuses, or one
// withheld from a rep entitled to it.

import (
	crmcontracts "github.com/margince/margince/backend/internal/contracts"
)

// The ways a row can be set aside, spelled as the contract spells them.
const (
	disposeSnooze   = "snooze"
	disposeNotMine  = "not_mine"
	disposeNotSales = "not_sales"
)

// waitingDispositions are the judgements a rep may make about an unanswered
// message. Only this source offers them: every other row leaves by the verb its
// own surface owns — an approval is decided, a task completed, a duplicate
// merged — and an unanswered message has no such verb.
//
// TWO OF THE THREE ARE KEYED ON THE THREAD, and a message can reach this queue
// without one — a first contact from an unknown address, or a provider handing
// over no chain to root on:
//
//   - `not_sales` judges the THREAD, and resolving it refuses a row without one.
//   - `snooze` wakes on a later message with the SAME thread_key, plain
//     equality, which a NULL never satisfies; a threadless row would hide
//     permanently even once the customer wrote back.
//
// The equality stays: matching NULL thread keys would let one unthreaded
// outbound silence every unthreaded question in the workspace. `not_mine` is
// per-reader and keyed on the activity, so it survives and leaves a threadless
// row something to do rather than nothing.
func waitingDispositions(threaded bool) []crmcontracts.WorklistItemDispositions {
	if !threaded {
		return []crmcontracts.WorklistItemDispositions{disposeNotMine}
	}
	return []crmcontracts.WorklistItemDispositions{
		disposeSnooze, disposeNotMine, disposeNotSales,
	}
}

// primaryActionFor is the one verb a row is FOR, out of the several it offers,
// so a reader need not weigh three equally-drawn controls to find the step the
// ranking already implies.
//
// Absent is a real answer rather than a gap: a duplicate pair genuinely asks the
// reader which record survives, and naming one side would be this surface
// deciding something it has no basis to.
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

// primaryVerbOrder ranks the verbs by how much of the row's work each finishes:
// `decide` and `merge` settle the row outright, `complete` and `act` finish the
// work behind it, and `open` only takes the reader somewhere. `snooze`,
// `dismiss` and `set_aside` are absent on purpose — putting work down is never
// the step a queue should suggest first.
var primaryVerbOrder = []string{"decide", "merge", "complete", "act", "acknowledge", "open"}
