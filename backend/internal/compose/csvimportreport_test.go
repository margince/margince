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
