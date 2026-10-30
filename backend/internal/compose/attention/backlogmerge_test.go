// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package attention

import (
	"context"
	"testing"
	"time"

	"github.com/margince/margince/backend/internal/compose/worklistsnap"
	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// tailBatchOf is one source's read: a row per hour offset, each kept, its
// position named after its id.
func tailBatchOf(source crmcontracts.WorklistItemSource, more bool, hours ...int) tailBatch {
	batch := tailBatch{source: source, more: more}
	for _, hour := range hours {
		at := rankInstant.Add(time.Duration(hour) * time.Hour)
		id := ids.NewV7().String()
		row := ranked{item: crmcontracts.WorklistItem{Id: id, Source: source}}
		batch.rows = append(batch.rows, tailRow{row: row, at: &at, after: "after-" + id, kept: true})
	}
	return batch
}

func cursorOver(batches ...tailBatch) *backlogCursor {
	rest := &backlogCursor{At: map[string]string{}}
	for _, batch := range batches {
		rest.At[string(batch.source)] = ""
	}
	return rest
}

// A row is merged only when no source with more could still hold an earlier
// one. Each source resumes after the last row it gave.
func TestTheTailMergesSourcesUpToTheEarliestUnreadRow(t *testing.T) {
	tasks := tailBatchOf(sourceTask, true, 1, 3, 5)
	waits := tailBatchOf(sourceWaiting, false, 2, 4, 6)
	rest := cursorOver(tasks, waits)

	got := mergeTail([]tailBatch{tasks, waits}, 10, nil, rest)

	want := []string{
		tasks.rows[0].row.item.Id, waits.rows[0].row.item.Id, tasks.rows[1].row.item.Id,
		waits.rows[1].row.item.Id, tasks.rows[2].row.item.Id,
	}
	if len(got) != len(want) {
		t.Fatalf("merged %d rows, want %d: the waiting row after the last task read must wait", len(got), len(want))
	}
	for i, row := range got {
		if row.item.Id != want[i] {
			t.Fatalf("row %d is %s, want %s", i, row.item.Id, want[i])
		}
	}
	if rest.At[string(sourceTask)] != tasks.rows[2].after || rest.At[string(sourceWaiting)] != waits.rows[1].after {
		t.Errorf("positions %v, want each source after the last row it gave", rest.At)
	}
}

// A page that fills stops between two rows, and each source resumes after the
// last row it gave, not after the last it read.
func TestAFullPageStopsEachSourceAfterWhatItGave(t *testing.T) {
	tasks := tailBatchOf(sourceTask, true, 1, 3)
	waits := tailBatchOf(sourceWaiting, true, 2, 4)
	rest := cursorOver(tasks, waits)

	got := mergeTail([]tailBatch{tasks, waits}, 2, nil, rest)

	if len(got) != 2 || rest.At[string(sourceTask)] != tasks.rows[0].after || rest.At[string(sourceWaiting)] != waits.rows[0].after {
		t.Errorf("gave %d rows, positions %v; want two rows and each source after its first", len(got), rest.At)
	}
}

// A source ends only when it had no more and every row it read was merged.
// A row the page drops, or the ranked rows hold, is still passed.
func TestASourceEndsOnlyWhenItsWholeReadIsMerged(t *testing.T) {
	tasks := tailBatchOf(sourceTask, false, 1, 2)
	tasks.rows[0].kept = false
	waits := tailBatchOf(sourceWaiting, false, 3)
	above := map[worklistsnap.Row]bool{rowIdentity(waits.rows[0].row): true}
	rest := cursorOver(tasks, waits)

	got := mergeTail([]tailBatch{tasks, waits}, 10, above, rest)

	if len(got) != 1 || got[0].item.Id != tasks.rows[1].row.item.Id {
		t.Fatalf("merged %d rows, want only the kept task the ranked rows do not hold", len(got))
	}
	if len(rest.At) != 0 {
		t.Errorf("sources %v are still continued, want both ended", rest.At)
	}
}

// An undated row sorts after every dated one, and a tie on the timestamp goes
// to the source's name.
func TestTheTailOrderPutsUndatedRowsLastAndBreaksTiesBySource(t *testing.T) {
	at := rankInstant
	undated := tailPlace{nil, sourceTask}
	dated := tailPlace{&at, sourceWaiting}
	if !dated.before(undated) || undated.before(dated) {
		t.Error("an undated row does not sort after a dated one")
	}
	if !(tailPlace{&at, sourceWaiting}).before(tailPlace{&at, sourceTask}) {
		t.Error("a tie on the timestamp is not broken by the source's name")
	}
}

// Tail rows that offer Done are judged by the reader's own marks on them. The
// marks are read for the tail itself, beside the ranked candidates' marks.
func TestTheTailReadsDoneMarksForItsOwnRows(t *testing.T) {
	wait := classifyWaiting(WaitingCustomer{ActivityID: ids.NewV7(), Since: rankInstant.Add(-48 * time.Hour)}, rankInstant)
	task := ranked{item: crmcontracts.WorklistItem{Id: ids.NewV7().String(), Source: sourceTask}}
	marked := map[RowRef]time.Time{refOf(wait): rankInstant.Add(-time.Hour)}
	svc := &Service{}
	svc.marked.store = &doneStoreSpy{already: marked}

	done, err := svc.tailDone(context.Background(), []ranked{wait, task})
	if err != nil {
		t.Fatal(err)
	}
	if kept := dropDone([]ranked{wait, task}, done, rankInstant); len(kept) != 1 || kept[0].item.Id != task.item.Id {
		t.Errorf("kept %d rows, want only the task: the waiting row was marked done", len(kept))
	}
}
