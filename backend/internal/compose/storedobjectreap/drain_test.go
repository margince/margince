// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package storedobjectreap

import (
	"context"
	"testing"
	"time"
)

// ledger stands in for the stored-object ledger: keys waiting, of which the
// store refuses to delete the first `refused`.
type ledger struct {
	waiting, refused, batches int
}

func (l *ledger) batch(size int) Batch {
	return func(context.Context) (int, int, error) {
		l.batches++
		listed := min(size, l.waiting)
		settled := max(0, listed-l.refused)
		l.waiting -= settled
		return listed, settled, nil
	}
}

var at = time.Date(2026, time.November, 12, 3, 0, 0, 0, time.UTC)

func fixed() time.Time { return at }

// One run takes batch after batch until the backlog is drained. A backlog
// larger than one batch does not push keys to a later run.
func TestOneRunDrainsMoreThanOneBatch(t *testing.T) {
	l := &ledger{waiting: 5}
	if err := Drain(context.Background(), l.batch(2), 2, at.Add(time.Minute), fixed); err != nil {
		t.Fatal(err)
	}
	if l.waiting != 0 || l.batches != 3 {
		t.Errorf("one run left %d keys after %d batches, want 0 after 3", l.waiting, l.batches)
	}
}

// A batch whose every delete the store refuses settles nothing, so the run
// stops instead of reading the same keys until its budget is spent.
func TestARunStopsAtABatchItCannotSettle(t *testing.T) {
	l := &ledger{waiting: 2, refused: 2}
	// A minute per batch, so a run that kept reading ends at the budget.
	ticking := func() time.Time { return at.Add(time.Duration(l.batches) * time.Minute) }
	if err := Drain(context.Background(), l.batch(2), 2, at.Add(time.Hour), ticking); err != nil {
		t.Fatal(err)
	}
	if l.batches != 1 {
		t.Errorf("the run read %d batches of keys it cannot delete, want 1", l.batches)
	}
}

// A run past its budget takes no further batch, and the next run goes on.
func TestARunPastItsBudgetLeavesTheRestForTheNextRun(t *testing.T) {
	l := &ledger{waiting: 3}
	if err := Drain(context.Background(), l.batch(1), 1, at, fixed); err != nil {
		t.Fatal(err)
	}
	if l.batches != 1 || l.waiting != 2 {
		t.Errorf("a run past its budget read %d batches and left %d keys, want 1 and 2", l.batches, l.waiting)
	}
}
