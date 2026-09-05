// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package attention

// A drifting deal nobody inside the account is arguing for.
//
// The three cases here are the three the lane must tell apart: a committee
// with no engaged champion is a finding, a committee the reader could not read
// in full is NOT, and a deal with no committee at all is not either. Only the
// first is a sentence a rep should act on, and the other two render identically
// to it the moment the fact is carried as a plain boolean rather than a
// tri-state.

import (
	"testing"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// reasonNoChampion is the verdict under test.
const reasonNoChampion = crmcontracts.WorklistReasonKind("no_champion")

func TestADealNobodyIsCarryingSaysSo(t *testing.T) {
	uncovered := true
	row := oneRiskRow(t, withDealFacts(RiskyDeal{
		DealID: ids.NewV7(), Name: "Fleet retrofit",
		QuietDays: 19, NoChampion: &uncovered,
	}))
	if !hasReason(row, reasonNoChampion) {
		t.Errorf("the row states %v, want a no_champion reason", kindsOf(row))
	}
}

// The BOUNDARY case, and the one direction this claim must never fail in.
//
// A champion the reader may not read is still a champion: the seat is absent
// from every row-scoped read, so a lane treating "I saw no champion" as "there
// is no champion" tells a rep nobody is arguing for their deal while somebody
// is. The rep then recruits a second champion into a committee that has one.
func TestAWithheldCommitteeMakesNoClaimAboutItsChampion(t *testing.T) {
	row := oneRiskRow(t, withDealFacts(RiskyDeal{
		DealID: ids.NewV7(), Name: "Fleet retrofit",
		QuietDays: 19, NoChampion: nil,
	}))
	if hasReason(row, reasonNoChampion) {
		t.Error("the row claims nobody is carrying a deal whose committee it could not read")
	}
}

// A deal carrying no seats has no coverage gap — it has no committee.
//
// Absent for the same reason as the withheld case, and tested separately
// because the two arrive by different routes through ChampionCoverFor: one is
// a row the reader could not see, the other a row that does not exist. A fix
// repairing one could leave the other reporting.
func TestADealWithNoCommitteeIsNotCalledUnchampioned(t *testing.T) {
	row := oneRiskRow(t, withDealFacts(RiskyDeal{
		DealID: ids.NewV7(), Name: "Nobody has been near it", QuietDays: 40,
	}))
	if hasReason(row, reasonNoChampion) {
		t.Error("the row claims nobody is carrying a deal that has no committee at all")
	}
}

// oneRiskRow runs one deal through the whole endpoint and returns its row.
//
// Through Worklist rather than the classifier directly, because the fact
// crosses two shapes on the way — the lane's RiskyDeal, then the item's deal
// facts — and a test calling classifyRisk with a hand-built item would prove
// the reason fires while the wiring that carries the fact to it stayed broken.
func oneRiskRow(t *testing.T, deal RiskyDeal) crmcontracts.WorklistItem {
	t.Helper()
	svc := NewService(
		stubApprovals{}, stubDuplicates{}, &stubTasks{}, stubReceipts{},
		stubBriefing{}, nil, stubAtRisk{rows: []RiskyDeal{deal}}, nil, nil,
		nil, nil, nil, nil, nil, nil, nil, nil, nil, fixedClock)
	out, err := svc.Worklist(meetingPrepReader(), "", "", ids.UUID{}, 25, "")
	if err != nil {
		t.Fatalf("worklist: %v", err)
	}
	if len(out.Queue) != 1 {
		t.Fatalf("the queue carries %d rows, wanted the one deal", len(out.Queue))
	}
	return out.Queue[0]
}

// withDealFacts gives the deal an amount, so the row carries deal facts at all.
//
// Without it the two absent-champion cases passed for the wrong reason: the
// renderer sends no deal facts for a deal with no stage, owner, amount,
// currency or champion answer, so `item.Deal` was nil and the clause under
// test was never reached. Reverting the nil check to treat an unknown answer
// as a finding left both tests green — a mutation the fixture, not the code,
// was surviving.
func withDealFacts(deal RiskyDeal) RiskyDeal {
	minor, currency := int64(4_000_000), "EUR"
	deal.AmountMinor, deal.Currency = &minor, &currency
	return deal
}

// The reason and the fact are two answers, and a client reads both.
//
// The tests above prove the `because` line fires; this proves the typed field
// beside it says the same thing. They are separate code paths — classifyRisk
// appends the reason from the lane item, dealFactsOf projects the facts object
// — so one carrying the tri-state proves nothing about the other. A card
// drawing a champion-coverage badge reads the field, not the reason.
func TestTheDealCardSaysNobodyIsCarryingIt(t *testing.T) {
	uncovered := true
	row := oneRiskRow(t, withDealFacts(RiskyDeal{
		DealID: ids.NewV7(), Name: "Fleet retrofit",
		QuietDays: 19, NoChampion: &uncovered,
	}))
	if row.Deal == nil {
		t.Fatal("the row carries no deal facts at all")
	}
	if row.Deal.NoChampion == nil || !*row.Deal.NoChampion {
		t.Errorf("the card states no_champion=%v, want a stated true", row.Deal.NoChampion)
	}
}

// Absent, never false — the distinction the contract spells out at length.
//
// `false` here would be the card claiming the committee is covered, over a
// read that never saw the committee. A rep reading that stops looking for a
// champion on a deal whose champion they simply may not see.
func TestTheCardMakesNoChampionClaimAboutAWithheldCommittee(t *testing.T) {
	row := oneRiskRow(t, withDealFacts(RiskyDeal{
		DealID: ids.NewV7(), Name: "Fleet retrofit",
		QuietDays: 19, NoChampion: nil,
	}))
	if row.Deal == nil {
		t.Fatal("the row carries no deal facts at all")
	}
	if row.Deal.NoChampion != nil {
		t.Errorf("the card answers %v about a committee it could not read, want no answer", *row.Deal.NoChampion)
	}
}
