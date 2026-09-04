// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package attention

// A pinned row escapes the fold that would have hidden it in a group.
//
// The two rules meet in the order they run: pins are applied before the routine
// decisions are folded, and folding matches on `Level == levelRoutine`. So a
// pinned approval is no longer routine and stays its own row — which is what a
// reader who pinned it asked for, and the opposite of what folding it would
// give them.
//
// Worth holding rather than leaving to the order, because the order is easy to
// change and the failure is quiet: the pin would still be stored, the row would
// still be raised, and it would then be folded into a group with no way for the
// reader to see the row they pinned.

import (
	"testing"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
)

func TestAPinnedDecisionStaysItsOwnRowRatherThanFolding(t *testing.T) {
	t.Parallel()

	needs := make([]crmcontracts.AttentionItem, 0, 12)
	for i := range 12 {
		needs = append(needs, item(string(rune('a'+i)), "approval", withKind("capture_counterparty")))
	}
	day := crmcontracts.Attention{AsOf: rankInstant, NeedsYou: needs}
	rows := classifyDay(day, rankInstant, dayMoney{})

	// One of the twelve, pinned — then folded the way the assembler folds.
	pinned := applyPins(rows, map[RowRef]bool{{Source: "approval", RowID: "a"}: true})
	folded := foldRoutineDecisionsBounded(pinned, false)

	var stoodAlone bool
	for _, row := range folded {
		if row.item.Id == "a" {
			stoodAlone = true
		}
	}
	if !stoodAlone {
		t.Fatal("the pinned decision was folded into a group, so the reader cannot " +
			"see the row they asked to lead their day")
	}
	// And the other eleven DID fold, without which this would pass against a
	// fold that had stopped working.
	if len(folded) >= len(rows) {
		t.Fatalf("nothing folded: %d rows in, %d out", len(rows), len(folded))
	}
}
