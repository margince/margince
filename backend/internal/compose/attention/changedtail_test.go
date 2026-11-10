// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package attention

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/margince/margince/backend/internal/platform/database/storekit"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

type changedTaskBacklog struct {
	windowRecorder
	rows []Positioned[Task]
}

func (b *changedTaskBacklog) CountTasks(context.Context, TaskWindow) (int, error) {
	return len(b.rows), nil
}

func (b *changedTaskBacklog) TasksAfter(_ context.Context, _ TaskWindow, after string, _ int) ([]Positioned[Task], bool, error) {
	if after != "" {
		return nil, false, nil
	}
	return b.rows, false, nil
}

func TestTheChangedBadgeOpensExactlyItsRankedSnapshotAcrossPages(t *testing.T) {
	var tasks []Task
	backlog := &changedTaskBacklog{}
	for i := range plannedCap + 1 {
		task := Task{ID: ids.NewV7(), Subject: fmt.Sprintf("Task %d", i), DueAt: new(readInstant.Add(time.Hour))}
		tasks = append(tasks, task)
		after, err := storekit.EncodeOpaque(map[string]string{"id": task.ID.String()})
		if err != nil {
			t.Fatal(err)
		}
		backlog.rows = append(backlog.rows, Positioned[Task]{Row: task, After: after})
	}
	svc := NewService(stubApprovals{}, stubDuplicates{}, &stubTasks{rows: tasks[:plannedCap], total: len(tasks)}, stubReceipts{},
		stubBriefing{ran: true, asOf: readInstant.Add(-time.Hour)},
		nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, fixedClock).WithTaskBacklog(backlog)
	walk := func(filter string) (int, int) {
		count, badge, cursor := 0, -1, ""
		for range 20 {
			page, err := svc.Worklist(pageReader(), scopeMine, filter, ids.Nil, 3, cursor)
			if err != nil {
				t.Fatal(err)
			}
			if badge < 0 {
				badge = page.Readings.ChangedSinceBrief
			}
			count += len(page.Queue)
			if filter == string(filterChangedSinceBrief) {
				for _, row := range page.Queue {
					if row.ChangedSinceBrief == nil || !*row.ChangedSinceBrief {
						t.Fatalf("changed page served an unmarked row: %+v", row)
					}
				}
			}
			if page.NextCursor == nil {
				return count, badge
			}
			cursor = *page.NextCursor
		}
		t.Fatal("the Worklist did not end")
		return 0, 0
	}
	all, _ := walk("")
	changed, badge := walk(string(filterChangedSinceBrief))
	if all != len(tasks) || changed != badge || changed != plannedCap {
		t.Fatalf("all=%d changed=%d badge=%d; want %d total and %d ranked changes", all, changed, badge, len(tasks), plannedCap)
	}
}

func TestTheChangedBadgeCountsOneFreshGroupAsOneRow(t *testing.T) {
	cutoff := readInstant.Add(-6 * time.Hour)
	rule := ids.New[ids.AutomationKind]()
	var runs []TroubledAutomationRun
	for range batchFloor {
		runs = append(runs, TroubledAutomationRun{
			ID: ids.NewV7(), AutomationID: rule,
			Name: "Route new leads", Outcome: "failed", Reason: "the assignee seat is gone", OccurredAt: cutoff.Add(time.Hour),
		})
	}
	svc := NewService(stubApprovals{}, stubDuplicates{}, &stubTasks{}, stubReceipts{},
		stubBriefing{ran: true, asOf: cutoff}, nil, nil, nil, nil, nil, nil, nil, nil, nil,
		&stubAutomations{rows: runs}, nil, nil, fixedClock)
	page, err := svc.Worklist(pageReader(), scopeUnassigned, string(filterChangedSinceBrief), ids.Nil, 25, "")
	if err != nil {
		t.Fatal(err)
	}
	if len(page.Queue) != 1 || page.Queue[0].Batch == nil || page.Readings.ChangedSinceBrief != 1 {
		t.Fatalf("fresh members counted differently from their drawn group: %+v", page)
	}
}
