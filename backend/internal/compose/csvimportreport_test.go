// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

import (
	"testing"

	"github.com/margince/margince/backend/internal/modules/migration"
)

func TestADealThatLandsClosedIsCountedWonOrLostAndAnOpenOneIsNot(t *testing.T) {
	stages := testDealStages()
	row := func(stage string) migration.Row {
		return migration.Row{Fields: map[string]any{dealTargetStage: stage}}
	}
	var p prediction
	for _, stage := range []string{"Won", "won", "Lost", "Qualified"} {
		p.countClosedDeal(stages, migration.ObjectDeal, row(stage), predictCreate)
	}
	p.countClosedDeal(stages, migration.ObjectDeal, row("Won"), predictUpdate)
	p.countClosedDeal(stages, migration.ObjectContact, row("Won"), predictCreate)
	if p.closedWon != 2 || p.closedLost != 1 {
		t.Fatalf("won %d lost %d, want 2 won and 1 lost: only a created deal counts, on a closed stage", p.closedWon, p.closedLost)
	}
}

func TestOnlyTheLinksOfLandedRowsAreOffered(t *testing.T) {
	rep := &migration.Report{Associations: 1, AssociationsSkipped: []migration.SkippedAssoc{
		{From: "contact/a", To: "company/Gone", Code: migration.ReasonLinkContactNotImported},
		{From: "contact/b", To: "company/Gone", Code: migration.ReasonLinkRowNotImported},
		{From: "contact/c", To: "company/Nowhere", Code: migration.ReasonLinkNotFound},
		{From: "contact/d", To: "company/Shared", Code: migration.ReasonLinkAmbiguous, RowSkipped: true},
	}}
	links := linksOf(rep, true)
	if links.Offered != 2 || links.Applied != 1 {
		t.Errorf("offered %d applied %d, want 2 offered (the written link and the unfound one) and 1 applied",
			links.Offered, links.Applied)
	}
	if links.Unresolved == nil || len(*links.Unresolved) != 4 {
		t.Errorf("unresolved = %v, want every link the run did not write listed with its reason", links.Unresolved)
	}
}
