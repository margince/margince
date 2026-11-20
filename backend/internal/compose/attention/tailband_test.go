// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package attention

import (
	"context"
	"fmt"
	"testing"
	"time"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/platform/database/storekit"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// keysetTasks resumes after the task its position names, as the store's keyset does.
type keysetTasks struct{ changedTaskBacklog }

func (b *keysetTasks) TasksAfter(_ context.Context, _ TaskWindow, after string, limit int) ([]Positioned[Task], bool, error) {
	from := 0
	for i, row := range b.rows {
		if row.After == after {
			from = i + 1
		}
	}
	rest := b.rows[from:]
	if len(rest) > limit {
		return rest[:limit], true, nil
	}
	return rest, false, nil
}

// Overdue tasks past the lane's bound reach the page through the tail. Their
// own facts say Now. Still, on no page of the walk does a band climb back above
// the crowded rows the ranking drew before them.
func TestATailRowNeverHeadsABandAboveTheRankedRows(t *testing.T) {
	var tasks []Task
	backlog := &keysetTasks{}
	count := plannedCap + 8
	for i := range count {
		// Oldest first, the order the store's keyset reads them in.
		due := readInstant.Add(-time.Duration(count-i) * time.Hour)
		task := Task{ID: ids.NewV7(), Subject: fmt.Sprintf("Overdue %d", i), DueAt: &due}
		tasks = append(tasks, task)
		after, err := storekit.EncodeOpaque(map[string]string{"id": task.ID.String()})
		if err != nil {
			t.Fatal(err)
		}
		backlog.rows = append(backlog.rows, Positioned[Task]{Row: task, After: after})
	}
	svc := NewService(stubApprovals{}, stubDuplicates{}, &stubTasks{rows: tasks[:plannedCap], total: len(tasks)}, stubReceipts{},
		stubBriefing{}, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, fixedClock).WithTaskBacklog(backlog)

	var bands []crmcontracts.WorklistItemBand
	cursor := ""
	for range 20 {
		page, err := svc.Worklist(pageReader(), scopeMine, "", ids.Nil, 7, cursor)
		if err != nil {
			t.Fatal(err)
		}
		for _, row := range page.Queue {
			if row.Band == nil {
				t.Fatalf("row %s carries no band", row.Id)
			}
			bands = append(bands, *row.Band)
		}
		if page.NextCursor == nil {
			break
		}
		cursor = *page.NextCursor
	}
	if len(bands) != len(tasks) || bands[0] != bandNow {
		t.Fatalf("the walk drew bands %v, want %d rows led by Now", bands, len(tasks))
	}
	for i := 1; i < len(bands); i++ {
		if bandRank(string(bands[i])) < bandRank(string(bands[i-1])) {
			t.Fatalf("row %d heads %s below a %s row: the walk drew %v", i, bands[i], bands[i-1], bands)
		}
	}
}
