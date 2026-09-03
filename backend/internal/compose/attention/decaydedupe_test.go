// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package attention

// One person is one row.
//
// The decay lane and the waiting lane say opposite things about the same
// contact and both are true: nobody has spoken to Dana in sixty days, AND Dana
// wrote last week and is waiting for an answer. Drawn together they read as a
// contradiction, and the rep is left to work out which to believe — on a page
// whose whole argument is that it can be trusted and worked to the bottom.
//
// These run the suppressor directly. What it decides is which row survives, and
// that question needs no assembled day to ask.

import (
	"testing"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

func decayRowFor(person ids.UUID) ranked {
	return ranked{item: crmcontracts.WorklistItem{
		Id:      person.String(),
		Source:  sourceDecay,
		Subject: subjectOf(subjectPerson, person),
	}}
}

func waitingRowFor(person ids.UUID) ranked {
	return ranked{item: crmcontracts.WorklistItem{
		Id:      ids.NewV7().String(),
		Source:  sourceWaiting,
		Subject: subjectOf(subjectPerson, person),
	}}
}

func sourcesOf(rows []ranked) []crmcontracts.WorklistItemSource {
	out := make([]crmcontracts.WorklistItemSource, 0, len(rows))
	for _, row := range rows {
		out = append(out, row.item.Source)
	}
	return out
}

// The waiting row wins. It is the more urgent and the more actionable of the
// two, because it names the message to reply to — a silence has none to point
// at. Same rule the drifting-deal suppressor keeps, for the same reason.
func TestAContactAlreadyWaitingIsNotAlsoReportedAsGoneQuiet(t *testing.T) {
	dana := ids.NewV7()

	kept := dropDecayAlreadyWaiting([]ranked{decayRowFor(dana), waitingRowFor(dana)})

	if len(kept) != 1 {
		t.Fatalf("the page carries %v for one contact, want the waiting row alone",
			sourcesOf(kept))
	}
	if kept[0].item.Source != sourceWaiting {
		t.Fatalf("the surviving row is %q, want the waiting row — it names the message "+
			"to reply to and the silence names nothing", kept[0].item.Source)
	}
}

// A contact nobody is waiting on keeps their decay row. Without this the test
// above passes on a suppressor that drops every decay row it sees, which is the
// lane deleting itself.
func TestAQuietContactNobodyIsWaitingOnKeepsTheirRow(t *testing.T) {
	dana := ids.NewV7()
	someoneElse := ids.NewV7()

	kept := dropDecayAlreadyWaiting([]ranked{
		decayRowFor(dana), waitingRowFor(someoneElse),
	})

	if len(kept) != 2 {
		t.Fatalf("the page carries %v, want both rows — they are about different people",
			sourcesOf(kept))
	}
}

// A page with no waiting rows at all is returned untouched. The early return
// this covers is not decoration: it is what keeps the common shape of the day —
// a queue with no unanswered mail — from walking every row twice.
func TestADayWithNobodyWaitingKeepsEveryQuietContact(t *testing.T) {
	rows := []ranked{decayRowFor(ids.NewV7()), decayRowFor(ids.NewV7())}

	if kept := dropDecayAlreadyWaiting(rows); len(kept) != 2 {
		t.Fatalf("the page carries %v, want both silences", sourcesOf(kept))
	}
}

// A waiting row about a DEAL does not silence a contact. The two lanes both
// carry subjects, and matching on the id alone rather than on the subject type
// would let an unrelated record's id suppress a person who shares nothing with
// it but a coincidence.
func TestAWaitOnADealDoesNotSilenceAContact(t *testing.T) {
	dana := ids.NewV7()
	onADeal := ranked{item: crmcontracts.WorklistItem{
		Id:      ids.NewV7().String(),
		Source:  sourceWaiting,
		Subject: subjectOf(subjectDeal, dana),
	}}

	kept := dropDecayAlreadyWaiting([]ranked{decayRowFor(dana), onADeal})

	if len(kept) != 2 {
		t.Fatalf("the page carries %v, want both — the wait is about a deal, and the "+
			"silence is about a person", sourcesOf(kept))
	}
}
