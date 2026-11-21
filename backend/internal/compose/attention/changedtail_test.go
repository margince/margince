// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package attention

import (
	"fmt"
	"testing"
	"time"

	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// The badge counts the news its door serves. Walked page by page, the changed
// filter opens every row that moved after the brief, and no other row.
func TestTheChangedBadgeAndItsDoorAgreeAcrossPages(t *testing.T) {
	cutoff := readInstant.Add(-6 * time.Hour)
	const fresh, stale = 7, 4
	var runs []TroubledAutomationRun
	for i := range fresh + stale {
		at := cutoff.Add(time.Hour)
		if i >= fresh {
			at = cutoff.Add(-time.Hour)
		}
		runs = append(runs, TroubledAutomationRun{
			ID: ids.NewV7(), AutomationID: ids.New[ids.AutomationKind](),
			Name: fmt.Sprintf("Rule %d", i), Outcome: "failed", Reason: "the assignee seat is gone", OccurredAt: at,
		})
	}
	svc := NewService(stubApprovals{}, stubDuplicates{}, &stubTasks{}, stubReceipts{},
		stubBriefing{ran: true, asOf: cutoff}, nil, nil, nil, nil, nil, nil, nil, nil, nil,
		&stubAutomations{rows: runs}, nil, nil, fixedClock)
	walk := func(filter string) (int, int) {
		count, badge, cursor := 0, -1, ""
		for range 20 {
			page, err := svc.Worklist(pageReader(), scopeUnassigned, filter, ids.Nil, 3, cursor)
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
	if all != fresh+stale || changed != badge || changed != fresh {
		t.Fatalf("all=%d changed=%d badge=%d; want %d total and %d changed", all, changed, badge, fresh+stale, fresh)
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
