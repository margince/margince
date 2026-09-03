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
	"time"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// Both fixtures go through the REAL classifiers rather than being built by
// hand. The defect this pass exists to close lives in classifyWaiting's subject
// choice — a wait naming both a deal and a person takes the deal — so a
// hand-built row with the subject the test wants is a row that proves the
// suppressor agrees with the test, not that it agrees with the product.
func decayRowFor(person ids.UUID) ranked {
	return classifyDecay(lapsedItem(QuietRelationship{
		PersonID: person, Name: "Dana Weiss", QuietDays: 63, LastAt: dedupeInstant,
	}), dedupeInstant)
}

// waitingRowFor is a wait about a person and nothing else.
func waitingRowFor(person ids.UUID) ranked {
	return classifyWaiting(WaitingCustomer{
		ActivityID: ids.NewV7(), Subject: "Re: the retrofit quote",
		Since: dedupeInstant.AddDate(0, 0, -2), PersonID: person,
	}, dedupeInstant)
}

// waitingOnADealFor is the case that made this pass wrong: a wait naming BOTH
// the contact and the deal their thread belongs to. classifyWaiting gives the
// deal the subject, so the person is on the row and nowhere in its subject.
func waitingOnADealFor(person ids.UUID) ranked {
	return classifyWaiting(WaitingCustomer{
		ActivityID: ids.NewV7(), Subject: "Re: the retrofit quote",
		Since:    dedupeInstant.AddDate(0, 0, -2),
		PersonID: person, DealID: ids.NewV7(),
	}, dedupeInstant)
}

// dedupeInstant is the one moment every row here is classified against.
var dedupeInstant = time.Date(2026, 8, 31, 9, 0, 0, 0, time.UTC)

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

// The case a subject-keyed lookup misses entirely.
//
// A wait naming both the contact and their deal takes the DEAL as its subject,
// so the person is on the row and nowhere in its subject. Those are exactly the
// contacts most likely to also be lapsing — somebody with an open deal — and
// the page showed them twice: unanswered in one row, gone quiet in another.
func TestAContactWaitingOnADealThreadIsStillNotReportedAsGoneQuiet(t *testing.T) {
	dana := ids.NewV7()

	kept := dropDecayAlreadyWaiting([]ranked{decayRowFor(dana), waitingOnADealFor(dana)})

	if len(kept) != 1 {
		t.Fatalf("the page carries %v for one contact, want the waiting row alone — "+
			"the wait's SUBJECT is the deal, so only the row's own person can match",
			sourcesOf(kept))
	}
	if kept[0].item.Source != sourceWaiting {
		t.Fatalf("the surviving row is %q, want the waiting row", kept[0].item.Source)
	}
}

// A waiting row about a DEAL and NOBODY does not silence a contact. The two lanes both
// carry subjects, and matching on the id alone rather than on the subject type
// would let an unrelated record's id suppress a person who shares nothing with
// it but a coincidence.
func TestAWaitNamingNoContactDoesNotSilenceOne(t *testing.T) {
	dana := ids.NewV7()
	// A thread filed under a deal alone, naming no person. Its subject id
	// happens to equal the contact's — the coincidence a subject-keyed match
	// would fall for.
	onADealOnly := classifyWaiting(WaitingCustomer{
		ActivityID: ids.NewV7(), Subject: "Re: the retrofit quote",
		Since: dedupeInstant.AddDate(0, 0, -2), DealID: dana,
	}, dedupeInstant)

	kept := dropDecayAlreadyWaiting([]ranked{decayRowFor(dana), onADealOnly})

	if len(kept) != 2 {
		t.Fatalf("the page carries %v, want both — the wait names no person at all, "+
			"and an id that merely matches is a coincidence", sourcesOf(kept))
	}
}

// The money survives the row that carried it.
//
// The two lanes answer different questions about a deal: a wait asks whether one
// rides on THIS THREAD, the decay lane asks whether the person sits on any open
// deal the reader can see. So the decay row can be the only row on the page that
// knows money rests on this contact — and dropping it silently took that with
// it, which for a wait past the staleness window is the difference between
// agreed work and routine tidying.
func TestTheMoneyOnASilencedContactSurvivesOntoTheWaitingRow(t *testing.T) {
	dana := ids.NewV7()
	quiet := classifyDecay(lapsedItem(QuietRelationship{
		PersonID: dana, Name: "Dana Weiss", QuietDays: 63,
		LastAt: dedupeInstant, HasOpenDeal: true,
	}), dedupeInstant)
	if !hasReason(quiet.item, reasonExpectedRevenue) {
		t.Fatalf("the decay row states %v and not that money rests on it — the fixture "+
			"proves nothing about what the drop would lose", quiet.item.Because)
	}

	kept := dropDecayAlreadyWaiting([]ranked{quiet, waitingRowFor(dana)})

	if len(kept) != 1 {
		t.Fatalf("the page carries %v, want the waiting row alone", sourcesOf(kept))
	}
	if !hasReason(kept[0].item, reasonExpectedRevenue) {
		t.Fatalf("the surviving row states %v — the money the dropped row knew about "+
			"left the page with it", kept[0].item.Because)
	}
}

// And a contact with NO money on them gains no such claim. Without this the
// test above passes on a pass that stamps every survivor as funded, which would
// be the row inventing a fact rather than inheriting one.
func TestASilencedContactWithNoDealAddsNoRevenueClaim(t *testing.T) {
	dana := ids.NewV7()

	kept := dropDecayAlreadyWaiting([]ranked{decayRowFor(dana), waitingRowFor(dana)})

	if len(kept) != 1 {
		t.Fatalf("the page carries %v, want the waiting row alone", sourcesOf(kept))
	}
	if hasReason(kept[0].item, reasonExpectedRevenue) {
		t.Fatalf("the surviving row claims money rests on it: %v", kept[0].item.Because)
	}
}
