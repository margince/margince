// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package attention

import (
	"context"
	"fmt"
	"slices"
	"testing"
	"time"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/platform/database/storekit"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// keysetTasks resumes after the task its position names, as the store's keyset does.
type keysetTasks struct {
	windowRecorder
	rows []Positioned[Task]
}

func (b *keysetTasks) CountTasks(context.Context, TaskWindow) (int, error) {
	return len(b.rows), nil
}

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

// A walk whose ranked rows end in Prospecting, followed by overdue tasks the
// tail reads. The tail rows head their own last band, so no band reopens on any
// page. No row claims an order over a row it was never ranked against.
func TestTheTailHeadsItsOwnLastBandAfterAMixedRanking(t *testing.T) {
	var ranked, tail []Task
	for i := range 3 {
		ranked = append(ranked, Task{ID: ids.NewV7(), Subject: fmt.Sprintf("Overdue %d", i), DueAt: new(readInstant.Add(-time.Duration(i+1) * time.Hour))})
	}
	for i := range plannedCap - 3 {
		ranked = append(ranked, Task{
			ID: ids.NewV7(), Subject: fmt.Sprintf("Lead %d", i), LinkType: "lead", LinkID: ids.NewV7(),
			DueAt: new(readInstant.Add(time.Duration(i+1) * time.Minute)),
		})
	}
	for i := range 8 {
		tail = append(tail, Task{ID: ids.NewV7(), Subject: fmt.Sprintf("Tail %d", i), DueAt: new(readInstant.Add(-time.Duration(30+i) * time.Hour))})
	}
	backlog := &keysetTasks{}
	all := slices.Concat(ranked, tail)
	slices.SortFunc(all, func(a, b Task) int { return a.DueAt.Compare(*b.DueAt) })
	for _, task := range all {
		after, err := storekit.EncodeOpaque(map[string]string{"id": task.ID.String()})
		if err != nil {
			t.Fatal(err)
		}
		backlog.rows = append(backlog.rows, Positioned[Task]{Row: task, After: after})
	}
	svc := NewService(stubApprovals{}, stubDuplicates{}, &stubTasks{rows: ranked, total: len(all)}, stubReceipts{},
		stubBriefing{}, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, fixedClock).WithTaskBacklog(backlog)

	walk := walkWorklist(t, svc, 7)
	if len(walk) != len(all) {
		t.Fatalf("the walk drew %d rows, want %d", len(walk), len(all))
	}
	var bands []string
	for i, row := range walk {
		bands = append(bands, string(*row.Band))
		if i > 0 && bandRank(bands[i]) < bandRank(bands[i-1]) {
			t.Fatalf("row %d reopens %s after %s: %v", i, bands[i], bands[i-1], bands)
		}
		pastRanking := *row.Band == bandMore
		if pastRanking && (row.DueGroup == nil || *row.DueGroup != crmcontracts.WorklistItemDueGroupWorklistDueGroupOverdue) {
			t.Errorf("tail row %s lost its due group: %v", row.Id, row.DueGroup)
		}
		if row.AboveNext != nil && (pastRanking || i+1 < len(walk) && *walk[i+1].Band == bandMore) {
			t.Errorf("row %d (%s) claims an order over a row past the ranking", i, bands[i])
		}
	}
	if !slices.Contains(bands, bandBuildPipeline) || slices.Index(bands, bandMore) != len(ranked) {
		t.Fatalf("want the ranked rows through Prospecting, then the tail under %q: %v", bandMore, bands)
	}
}

// walkWorklist reads every page of the reader's Worklist, limit rows at a time.
func walkWorklist(t *testing.T, svc *Service, limit int) []crmcontracts.WorklistItem {
	t.Helper()
	var rows []crmcontracts.WorklistItem
	cursor := ""
	for range 20 {
		page, err := svc.Worklist(pageReader(), scopeMine, "", ids.Nil, limit, cursor)
		if err != nil {
			t.Fatal(err)
		}
		for _, row := range page.Queue {
			if row.Band == nil {
				t.Fatalf("row %s carries no band", row.Id)
			}
		}
		rows = append(rows, page.Queue...)
		if page.NextCursor == nil {
			return rows
		}
		cursor = *page.NextCursor
	}
	t.Fatal("the Worklist did not end")
	return nil
}
