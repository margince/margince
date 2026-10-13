// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package migration

// How a leased commit slice ends when it does not finish. Each case keeps the
// run row true to what landed.

import (
	"context"
	"errors"
	"testing"

	"github.com/jackc/pgx/v5"
)

// refusingWriters lands rows in the identity map and fails on one of them.
type refusingWriters struct {
	*mapWriters
	refuse string
}

var errRowRefused = errors.New("the row could not be written")

func (w *refusingWriters) Ensure(ctx context.Context, object string, row Row) (EnsureResult, error) {
	if row.ExternalID == w.refuse {
		return EnsureResult{}, errRowRefused
	}
	return w.mapWriters.Ensure(ctx, object, row)
}

func TestALeasedSliceWritesNoRunItDoesNotLease(t *testing.T) {
	ctx, db := testWorkspaceCtx(t, adminImportRunGrant())
	s := NewRunStore(db)
	leased, other := approvedRun(ctx, t, s), approvedRun(ctx, t, s)
	w := newMapWriters(s, other.Run)

	if _, err := NewLeasedEngine(s, leased, w).RunSlice(ctx, other.Run, twoObjectSource(), Budget{}); !errors.Is(err, ErrSuperseded) {
		t.Fatalf("a slice of a run its lease does not name = %v, want ErrSuperseded", err)
	}
	if len(w.ensured) != 0 {
		t.Errorf("the slice landed %v in a run it does not lease, want nothing", w.ensured)
	}
	if got := mustGetStaged(ctx, t, s, other.Run); got.Status != StatusRunning || got.Checkpoint != 0 {
		t.Errorf("the other run reads %s at %d, want it running at 0", got.Status, got.Checkpoint)
	}
}

func TestALeasedSliceWhoseWorkerStopsCountsTheRowsItLanded(t *testing.T) {
	ctx, db := testWorkspaceCtx(t, adminImportRunGrant())
	s := NewRunStore(db)
	l := approvedRun(ctx, t, s)
	if _, err := s.Claim(ctx, l); err != nil {
		t.Fatalf("Claim: %v", err)
	}
	stopping, cancel := context.WithCancel(ctx)
	w := newMapWriters(s, l.Run)
	w.landing = cancel

	done, err := NewLeasedEngine(s, l, w).RunSlice(stopping, l.Run, twoObjectSource(), Budget{})
	if done || !errors.Is(err, context.Canceled) {
		t.Fatalf("a slice whose worker stopped = done %v, %v; want it stopped with the cancel", done, err)
	}
	assertCursor(ctx, t, db, l.Run, 1, PhaseImporting, "company")
	if got := mustGetStaged(ctx, t, s, l.Run); got.Status != StatusRunning || createdOf(got) != 1 {
		t.Errorf("the stopped run reads %s with %d created, want running with the one landed row counted",
			got.Status, createdOf(got))
	}
}

func TestARowThatFailsFailsTheLeasedRunWithWhatLandedBeforeIt(t *testing.T) {
	ctx, db := testWorkspaceCtx(t, adminImportRunGrant())
	s := NewRunStore(db)
	l := approvedRun(ctx, t, s)
	if _, err := s.Claim(ctx, l); err != nil {
		t.Fatalf("Claim: %v", err)
	}
	w := &refusingWriters{mapWriters: newMapWriters(s, l.Run), refuse: "company-2"}

	done, err := NewLeasedEngine(s, l, w).RunSlice(ctx, l.Run, twoObjectSource(), Budget{})
	if done || !errors.Is(err, errRowRefused) {
		t.Fatalf("a slice whose row failed = done %v, %v; want the row's error", done, err)
	}
	got := mustGetStaged(ctx, t, s, l.Run)
	if got.Status != StatusFailed || got.Error != CauseStopped {
		t.Errorf("the run reads %s with %q, want failed with %q and the error kept out of it", got.Status, got.Error, CauseStopped)
	}
	if got.Checkpoint != 1 || createdOf(got) != 1 {
		t.Errorf("the failed run reads checkpoint %d with %d created, want the one row before the failure kept",
			got.Checkpoint, createdOf(got))
	}
}

// createdOf is how many rows the run's progress counts as created; -1 for none.
func createdOf(run Run) int {
	if run.Progress == nil {
		return -1
	}
	return run.Progress.Counts.Created
}

func TestALeaseDoesNotOverwriteARunRowItCannotRead(t *testing.T) {
	for column, write := range map[string]func(context.Context, *RunStore, Lease) error{
		"report": func(ctx context.Context, s *RunStore, l Lease) error {
			return s.Flush(ctx, l, Flush{Checkpoint: 2, Leg: Report{Imported: 2}})
		},
		"actor": func(ctx context.Context, s *RunStore, l Lease) error {
			_, err := s.Claim(ctx, l)
			return err
		},
		"progress": func(ctx context.Context, s *RunStore, l Lease) error {
			_, err := s.Claim(ctx, l)
			return err
		},
	} {
		t.Run(column, func(t *testing.T) {
			ctx, db := testWorkspaceCtx(t, adminImportRunGrant())
			s := NewRunStore(db)
			l := approvedRun(ctx, t, s)
			setRunColumn(ctx, t, db, l.Run, column+` = '[]'::jsonb`)

			if err := write(ctx, s, l); err == nil || errors.Is(err, ErrSuperseded) {
				t.Fatalf("writing over an unreadable %s = %v, want an error that is not a lost lease", column, err)
			}
			assertUnclaimedAtZero(ctx, t, db, l.Run)
		})
	}
}

func assertUnclaimedAtZero(ctx context.Context, t *testing.T, db interface {
	Tx(context.Context, func(pgx.Tx) error) error
}, id RunID,
) {
	t.Helper()
	var checkpoint int
	var claimed *int
	if err := db.Tx(ctx, func(tx pgx.Tx) error {
		return tx.QueryRow(ctx, `SELECT checkpoint, claimed_attempt FROM import_run WHERE id = $1`, id).Scan(&checkpoint, &claimed)
	}); err != nil {
		t.Fatalf("reading the run: %v", err)
	}
	if checkpoint != 0 || claimed != nil {
		t.Errorf("the refused write left checkpoint %d, claimed by %v; want the run as approve left it", checkpoint, claimed)
	}
}
