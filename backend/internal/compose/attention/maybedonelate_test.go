// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package attention

import (
	"slices"
	"testing"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

func TestATaskThatAsksWhetherItIsDoneIsNotOverdueUntilNotYet(t *testing.T) {
	due := rankInstant.AddDate(0, 0, -7)
	for name, asks := range map[string]*crmcontracts.WorklistMayBeDone{"asking": {ClaimKey: "moment:may_be_done:x"}, "after Not yet": nil} {
		item := taskItem(Task{ID: ids.NewV7(), Subject: "Send the file", DueAt: &due, MayBeDone: asks}, rankInstant, rankInstant.AddDate(0, 0, 7), nil)
		row := classifyTask(item, rankInstant)
		want := asks == nil
		if item.Overdue == nil || *item.Overdue != want || row.item.Overdue == nil || *row.item.Overdue != want || row.overdue != want {
			t.Errorf("%s: attention overdue=%v, worklist overdue=%v, ranked=%v; want %v", name, item.Overdue, row.item.Overdue, row.overdue, want)
		}
		kinds := make([]crmcontracts.WorklistReasonKind, 0, len(row.item.Because))
		for _, because := range row.item.Because {
			kinds = append(kinds, because.Kind)
		}
		if slices.Contains(kinds, "overdue") != want || slices.Contains(kinds, "due_today") {
			t.Errorf("%s: reasons=%v", name, kinds)
		}
	}
}
