// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package attention

// What a lead is told, and what they are refused.

import (
	"testing"
	"time"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// TestEveryExceptionNamesWhatItWasJudgedAgainst is the promise the contract
// makes and the reason this surface is worth reading.
//
// A row saying "this deal is at risk" without its basis is a verdict a lead
// cannot dispute. The threshold is what turns it into a claim they can check —
// and for a response breach it must be the POLICY's own state rather than a
// number chosen here, so the manager and the rep read one rule.
func TestEveryExceptionNamesWhatItWasJudgedAgainst(t *testing.T) {
	t.Parallel()
	rows := []ranked{
		lateLead(),
		materialDeal(),
	}

	found := exceptionsIn(rows, rankInstant)

	if len(found) == 0 {
		t.Fatal("no exception was raised, so this test judges nothing")
	}
	for _, at := range found {
		if at.Threshold == "" {
			t.Errorf("a %q exception names no basis: a lead cannot dispute a verdict "+
				"that does not say what it was judged against", at.Kind)
		}
	}
}

// TestABreachedReplyIsJudgedByThePolicysOwnState.
//
// The one threshold that must not be invented here. leadStanding decides breach
// from the lead-response policy; if this page decided it again from a clock of
// its own the two would agree today and drift the first time either moved.
func TestABreachedReplyIsJudgedByThePolicysOwnState(t *testing.T) {
	t.Parallel()

	found := exceptionsIn([]ranked{lateLead()}, rankInstant)

	if len(found) != 1 {
		t.Fatalf("a breached lead raised %d exceptions, want one", len(found))
	}
	if found[0].Kind != crmcontracts.TeamExceptionResponseBreached {
		t.Errorf("a breached lead raised %q", found[0].Kind)
	}
	if found[0].Threshold != string(crmcontracts.LeadSlaStateBreached) {
		t.Errorf("the breach is judged against %q, want the policy's own state %q",
			found[0].Threshold, crmcontracts.LeadSlaStateBreached)
	}
}

// TestARowWithNoRecordIsNotAnException.
//
// Every intervention this page offers needs something to open. A row with no
// subject is real work on the rep's own queue and simply not this page's
// business — raising it here would hand a lead a line they cannot act on.
func TestARowWithNoRecordIsNotAnException(t *testing.T) {
	t.Parallel()
	subjectless := lateLead()
	subjectless.item.Subject = nil

	if found := exceptionsIn([]ranked{subjectless}, rankInstant); len(found) != 0 {
		t.Errorf("a row with no record raised %d exceptions, want none", len(found))
	}
}

// TestTheWorstKindLeads holds the order a lead reads.
//
// By KIND rather than by age: a breached response and an unowned deal are
// different kinds of wrong, and ordering them on one clock would put a week-old
// piece of hygiene above a customer who has waited since this morning.
func TestTheWorstKindLeads(t *testing.T) {
	t.Parallel()
	found := exceptionsIn([]ranked{materialDeal(), lateLead()}, rankInstant)
	if len(found) != 2 {
		t.Fatalf("raised %d exceptions, want two", len(found))
	}

	sortExceptions(found)

	if found[0].Kind != crmcontracts.TeamExceptionResponseBreached {
		t.Errorf("the page leads with %q, want the breached reply", found[0].Kind)
	}
}

// TestNoExceptionClaimsARepIsOverloaded.
//
// "This rep is overloaded" needs a configured capacity to be a fact rather than
// an opinion, and this installation has none. The board's counts stay a
// reading; a kind here would make the claim anyway.
func TestNoExceptionClaimsARepIsOverloaded(t *testing.T) {
	t.Parallel()
	for _, kind := range []crmcontracts.TeamExceptionKind{
		crmcontracts.TeamExceptionResponseBreached,
		crmcontracts.TeamExceptionRevenueAtRisk,
		crmcontracts.TeamExceptionUnassigned,
		crmcontracts.TeamExceptionRepeatedFailure,
	} {
		if !kind.Valid() {
			t.Errorf("%q is not a kind the contract declares", kind)
		}
	}
	// The contract's own vocabulary, read rather than restated: a capacity kind
	// added later fails here rather than shipping as an opinion.
	if crmcontracts.TeamExceptionKind("capacity").Valid() {
		t.Error("the contract declares a capacity exception, which cannot be a fact " +
			"without a configured capacity to judge against")
	}
}

// riskDeal is the deal the material fixture is about.
var riskDeal = ids.MustParse("00000000-0000-7000-8000-00000000d001")

// lateLead is a first reply the policy says is already overdue.
func lateLead() ranked {
	return classifyLead(OwedLead{
		ID:         leadOwed,
		Name:       "Kirsten at LOXXESS",
		DeadlineAt: rankInstant.Add(-2 * time.Hour),
		State:      string(crmcontracts.LeadSlaStateBreached),
	}, rankInstant)
}

// materialDeal is revenue the day judged worth the interruption.
//
// It needs BOTH halves to be one: a bar it clears, so the classifier files it
// at the material level, and a subject, because an exception a lead cannot open
// is not one this page raises. A fixture missing either is a deal that reads
// urgent to whoever wrote it and raises nothing here.
func materialDeal() ranked {
	return classifyRisk(
		item("risk", "deal_at_risk",
			withDeal(900_000_00), withDealSubject(riskDeal)),
		rankInstant,
		materialBar{minor: 100_000_00, known: true},
		dayMoney{})
}
