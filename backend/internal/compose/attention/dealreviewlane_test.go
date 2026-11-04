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

// Below a lane's cap the fold above the tail cannot see the rows, so the tail
// itself leaves a review's task and its deal's quiet row out.
func TestTheTailLeavesOutWhatAReviewReports(t *testing.T) {
	t.Parallel()
	deal, task := ids.NewV7(), ids.NewV7()
	read := reviewRead{open: []DealReview{{ID: ids.NewV7(), DealID: deal, Task: &task, TaskOpen: true}}}
	if !read.foldsTask(reviewTask(task)) || read.foldsTask(reviewTask(ids.NewV7())) {
		t.Error("the tail must leave out the review's own task and no other")
	}
	if !read.foldsRisk(quietRisk(deal, "quiet")) || read.foldsRisk(quietRisk(deal, "close_overdue")) ||
		read.foldsRisk(quietRisk(ids.NewV7(), "quiet")) {
		t.Error("the tail must leave out the review deal's quiet row only")
	}
}

// A store's task count still holds an open review's task, which the fold moved
// to the review row, so the task totals are left absent rather than overstated.
func TestAnOpenReviewTaskLeavesTheTaskTotalsAbsent(t *testing.T) {
	t.Parallel()
	task := ids.NewV7()
	tasks := totalKey{sourceTask, "tasks"}
	s := &Service{}
	s.backlog.exact = map[totalKey]int{tasks: 12}
	if got := s.exactSources(crmcontracts.Attention{}, scopeMine, nil); got[tasks] != 12 {
		t.Fatalf("with no review the task total is %d, want the store's 12", got[tasks])
	}
	s.reviews = reviewRead{open: []DealReview{{ID: ids.NewV7(), DealID: ids.NewV7(), Task: &task, TaskOpen: true}}}
	if _, kept := s.exactSources(crmcontracts.Attention{}, scopeMine, nil)[tasks]; kept {
		t.Error("an open review task left the store's task count standing, which counts it twice")
	}
}
