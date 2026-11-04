// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package attention

import (
	"testing"
	"time"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

func quietRisk(deal ids.UUID, kind string) ranked {
	row := atRiskRow(deal)
	row.item.Kind = &kind
	row.item.Because = []crmcontracts.WorklistReason{reason("material", nil)}
	return row
}

func reviewTask(task ids.UUID) ranked {
	return ranked{item: crmcontracts.WorklistItem{
		Id: task.String(), Source: crmcontracts.WorklistItemSourceTask,
		Actions: []crmcontracts.WorklistItemActions{crmcontracts.WorklistItemActionsComplete},
		Because: []crmcontracts.WorklistReason{reason("overdue", nil)},
	}}
}

func TestOneHomeActionPerReview(t *testing.T) {
	t.Parallel()
	silent, overdue, untasked := ids.NewV7(), ids.NewV7(), ids.NewV7()
	task, owner := ids.NewV7(), ids.NewV7()
	reviews := []DealReview{
		{ID: ids.NewV7(), DealID: silent, DealName: "Silent", Task: &task},
		{ID: ids.NewV7(), DealID: overdue, DealName: "Overdue"},
		{ID: ids.NewV7(), DealID: untasked, DealName: "Untasked", Owner: &owner, OpenedAt: time.Date(2026, 11, 2, 9, 0, 0, 0, time.UTC)},
	}
	other := ids.NewV7()
	rows := foldDealReviews([]ranked{
		reviewTask(task), quietRisk(silent, "quiet"), quietRisk(overdue, "close_overdue"), reviewTask(other),
	}, reviews)

	bySource := map[crmcontracts.WorklistItemSource]int{}
	var silentRow *ranked
	for i, row := range rows {
		bySource[row.item.Source]++
		if row.item.Id == reviews[0].ID.String() {
			silentRow = &rows[i]
		}
	}
	if bySource[sourceDealReview] != 3 || bySource[sourceTask] != 1 || bySource[sourceAtRisk] != 1 {
		t.Fatalf("rows by source %v, want 3 reviews, the unrelated task and the overdue deal's own row", bySource)
	}
	if silentRow == nil || len(silentRow.item.Because) != 2 || silentRow.item.Title == nil || *silentRow.item.Title != "Silent" {
		t.Fatalf("the silent deal's review row is %+v, want one row holding both folded reasons", silentRow)
	}
	if len(silentRow.item.Actions) != 1 || silentRow.item.Actions[0] != crmcontracts.WorklistItemActionsOpen {
		t.Errorf("the review row offers %v, want only open: the task's verbs write to the task", silentRow.item.Actions)
	}
	for _, row := range rows {
		if row.item.Id == reviews[2].ID.String() && (row.ownerRef.kind != ownerNamed || row.ownerRef.user != owner) {
			t.Errorf("a review with nothing to fold is owned by %+v, want the deal's owner", row.ownerRef)
		}
	}
}

func TestNoReviewsLeaveTheRowsAlone(t *testing.T) {
	t.Parallel()
	rows := []ranked{quietRisk(ids.NewV7(), "quiet")}
	if got := foldDealReviews(rows, nil); len(got) != 1 || got[0].item.Source != sourceAtRisk {
		t.Fatalf("with no open review the rows became %+v", got)
	}
}
