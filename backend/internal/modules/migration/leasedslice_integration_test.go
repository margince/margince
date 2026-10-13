// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package migration

// How a leased commit slice ends when it does not finish. Each case keeps the
// run row true to what landed.

import (
	"context"
	"errors"
	"fmt"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/shared/apperrors"
)

// refusingWriters lands rows in the identity map and fails on one of them,
// with refusal, or errRowRefused when that is nil.
type refusingWriters struct {
	*mapWriters
	refuse  string
	refusal error
}

var errRowRefused = errors.New("the row could not be written")

func (w *refusingWriters) Ensure(ctx context.Context, object string, row Row) (EnsureResult, error) {
	if row.ExternalID == w.refuse {
		if w.refusal != nil {
			return EnsureResult{}, w.refusal
		}
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

// A run the walk stopped carries the sentence for its cause, and never the
// error's own text, which can hold SQL.
func TestARunTheWalkStoppedCarriesTheSentenceForItsCause(t *testing.T) {
	for name, tc := range map[string]struct {
		refusal error
		want    string
	}{
		"a grant the approver lost": {fmt.Errorf("creating company: %w", apperrors.ErrPermissionDenied), CausePermission},
		"a named stop":              {RunStop{Cause: CauseStorageUnavailable, Err: errRowRefused}, CauseStorageUnavailable},
		"anything else":             {errors.New(`ERROR: relation "contact" does not exist (SQLSTATE 42P01)`), CauseStopped},
	} {
		t.Run(name, func(t *testing.T) {
			ctx, db := testWorkspaceCtx(t, adminImportRunGrant())
			s := NewRunStore(db)
			l := approvedRun(ctx, t, s)
			if _, err := s.Claim(ctx, l); err != nil {
				t.Fatalf("Claim: %v", err)
			}
			w := &refusingWriters{mapWriters: newMapWriters(s, l.Run), refuse: "company-2", refusal: tc.refusal}
			_, err := NewLeasedEngine(s, l, w).RunSlice(ctx, l.Run, twoObjectSource(), Budget{})
			if !errors.Is(err, tc.refusal) || !errors.Is(err, ErrStopRecorded) {
				t.Fatalf("the slice = %v, want the row's error, marked as recorded on the run", err)
			}
			if got := mustGetStaged(ctx, t, s, l.Run); got.Status != StatusFailed || got.Error != tc.want {
				t.Errorf("the run reads %s with %q, want failed with %q", got.Status, got.Error, tc.want)
			}
		})
	}
}

// A slice whose setup or first read outlasts its time budget still lands a move
// before it stops, so a run of seven moves ends in at most seven slices. Only the
// first slice counts the source; later ones read the totals its flush recorded.
func TestASliceWhoseSetupOutlastsItsBudgetStillLandsARow(t *testing.T) {
	ctx, db := testWorkspaceCtx(t, adminImportRunGrant())
	s := NewRunStore(db)
	l := approvedRun(ctx, t, s)
	clock := time.Date(2026, 3, 2, 9, 0, 0, 0, time.UTC)
	w := newMapWriters(s, l.Run)
	w.reconciling = func() { clock = clock.Add(time.Hour) }
	w.landing = func() { clock = clock.Add(2 * time.Second) }
	src := twoObjectSource()
	src.reading = func() { clock = clock.Add(time.Hour) }
	b := Budget{Time: time.Second, Clock: func() time.Time { return clock }}

	const moves = 7
	slices := 0
	for done := false; !done; slices++ {
		if slices == moves {
			t.Fatalf("%d slices did not finish a run of %d moves; every slice must land one", slices, moves)
		}
		before := mustGetStaged(ctx, t, s, l.Run).Checkpoint
		done = sliceOf(ctx, t, s, l, w, src, b)
		if after := mustGetStaged(ctx, t, s, l.Run).Checkpoint; !done && after <= before {
			t.Fatalf("slice %d left the checkpoint at %d; a slice lands a move before its budget stops it", slices+1, after)
		}
	}
	if src.counted != 1 {
		t.Errorf("%d slices counted the source %d times, want once", slices, src.counted)
	}
}

// A run approved again after it failed is walked once more, and its report
// counts that walk, as it counts the first.
func TestEachAttemptOfARunCountsAsAWalk(t *testing.T) {
	ctx, db := testWorkspaceCtx(t, adminImportRunGrant())
	s := NewRunStore(db)
	whole := approvedRun(ctx, t, s)
	if !slice(ctx, t, s, whole, newMapWriters(s, whole.Run), Budget{}) {
		t.Fatal("an unbounded slice did not finish the run")
	}

	first := approvedRun(ctx, t, s)
	w := newMapWriters(s, first.Run)
	slice(ctx, t, s, first, w, Budget{Rows: 2})
	if err := s.FailLease(ctx, first, CauseStopped); err != nil {
		t.Fatalf("FailLease: %v", err)
	}
	resumed, err := s.ResumeApproved(ctx, first.Run, 0, queueNothing)
	if err != nil {
		t.Fatalf("ResumeApproved: %v", err)
	}
	second := Lease{Run: resumed.ID, Status: StatusRunning, Attempt: resumed.JobAttempt}
	slice(ctx, t, s, second, w, Budget{Rows: 1})
	if !slice(ctx, t, s, second, w, Budget{}) {
		t.Fatal("the resumed attempt did not finish the run")
	}

	want := mustGetStaged(ctx, t, s, whole.Run).Report.Walks() + 1
	if got := mustGetStaged(ctx, t, s, first.Run).Report.Walks(); got != want {
		t.Errorf("the run walked by two attempts reports %d walks, want %d", got, want)
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
