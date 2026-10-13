// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package attention

import (
	"testing"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

const reasonChampionUnknown = crmcontracts.WorklistReasonKind("champion_unknown")

// A deal nobody has recorded a champion for says so, and never as "no
// champion": the rep's move is to find out who argues for it, not to go and
// recruit somebody for a deal that may already have them.
func TestADealWithNoRecordedChampionSaysItIsUnknown(t *testing.T) {
	row := oneRiskRow(t, withDealFacts(RiskyDeal{
		DealID: ids.NewV7(), Name: "Fleet retrofit", QuietDays: 19, ChampionUnknown: true,
	}))
	if !hasReason(row, reasonChampionUnknown) {
		t.Errorf("the row states %v, want a champion_unknown reason", kindsOf(row))
	}
	if hasReason(row, reasonNoChampion) {
		t.Error("an unknown champion was stated as no champion")
	}
}

// The two facts are exclusive. Should a producer ever set both, the finding
// wins and the row does not contradict itself.
func TestAKnownGapIsNotAlsoCalledUnknown(t *testing.T) {
	uncovered := true
	row := oneRiskRow(t, withDealFacts(RiskyDeal{
		DealID: ids.NewV7(), Name: "Fleet retrofit", QuietDays: 19,
		NoChampion: &uncovered, ChampionUnknown: true,
	}))
	if !hasReason(row, reasonNoChampion) || hasReason(row, reasonChampionUnknown) {
		t.Errorf("the row states %v, want no_champion alone", kindsOf(row))
	}
}

// A deal whose coverage is known says nothing about not knowing it.
func TestAKnownCommitteeIsNotCalledUnknown(t *testing.T) {
	row := oneRiskRow(t, withDealFacts(RiskyDeal{
		DealID: ids.NewV7(), Name: "Fleet retrofit", QuietDays: 19,
	}))
	if hasReason(row, reasonChampionUnknown) {
		t.Errorf("the row states %v, want no champion_unknown reason", kindsOf(row))
	}
}
