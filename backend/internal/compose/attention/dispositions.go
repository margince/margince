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
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// The ways a row can be set aside, spelled as the contract spells them.
const (
	disposeHandled  = "handled"
	disposeSnooze   = "snooze"
	disposeNotMine  = "not_mine"
	disposeNotSales = "not_sales"
)

// waitingDispositions are the judgements a reader may make about an unanswered
// message. Only this source offers them, because only here can a row be dealt
// with off the surface that owns it. `handled` leads: it says the work is
// finished, by phone, in a meeting, or with no reply owed.
//
// Two of the four are keyed on the thread, and a message can reach this queue
// without one. A first contact from an unknown address has none, and so does
// a provider that hands over no chain to root on:
//
//   - `not_sales` judges the thread, and resolving it refuses a row without one.
//   - `snooze` wakes on a later message with the same thread_key. A NULL never
//     equals anything, so a threadless row would stay hidden after a reply.
//
// The equality stays: matching NULL thread keys would let one unthreaded
// outbound silence every unthreaded question in the workspace. `handled` and
// `not_mine` are per-reader and keyed on the activity, so they survive.
//
// `not_mine` is withheld from the row's own owner. It means "somebody else
// must still see this", and on the owner's row there is nobody else.
func waitingDispositions(row ranked, reader ids.UUID) []crmcontracts.WorklistItemDispositions {
	offered := []crmcontracts.WorklistItemDispositions{disposeHandled}
	if row.threaded {
		offered = append(offered, disposeSnooze)
	}
	if !namesReaderAsOwner(row, reader) {
		offered = append(offered, disposeNotMine)
	}
	if row.threaded {
		offered = append(offered, disposeNotSales)
	}
	return offered
}

// namesReaderAsOwner reports whether a row positively names the reader as the
// one who owes it. A row naming nobody, or an owner this lane could not read,
// is not the reader's by this test.
func namesReaderAsOwner(row ranked, reader ids.UUID) bool {
	named, ok := answersTo(row)
	return ok && named == reader
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
