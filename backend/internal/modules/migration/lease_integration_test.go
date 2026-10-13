// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package migration

import (
	"context"
	"errors"
	"reflect"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/platform/database"
	"github.com/margince/margince/backend/internal/platform/database/storekit"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// mapWriters lands rows in the real identity map, so a replayed row reads as
// bound to its run the way the csv writer's does. It keeps no native record.
type mapWriters struct {
	s      *RunStore
	run    RunID
	system string
	// ensured counts the creates per object/external id.
	ensured map[string]int
	// dieAt names a row whose Ensure stands for the process dying: it panics
	// once, before the row lands.
	dieAt string
	// landing, when set, runs as each row lands: the time a write takes.
	landing func()
}

func newMapWriters(s *RunStore, run RunID) *mapWriters {
	return &mapWriters{s: s, run: run, system: "import:csv:" + run.String(), ensured: map[string]int{}}
}

func (w *mapWriters) Exists(ctx context.Context, object, ext string) (bool, error) {
	_, found, err := w.s.LookupIdentity(ctx, w.system, object, ext)
	return found, err
}

func (*mapWriters) ReconcileIdentities(context.Context) error { return nil }

func (w *mapWriters) Ensure(ctx context.Context, object string, row Row) (EnsureResult, error) {
	if row.ExternalID == w.dieAt {
		w.dieAt = ""
		panic(errProcessDied)
	}
	if _, found, err := w.s.LookupIdentity(ctx, w.system, object, row.ExternalID); err != nil || found {
		return EnsureResult{Unchanged: found}, err
	}
	if err := w.s.RecordIdentity(ctx, w.run, w.system, object, row.ExternalID, ids.NewV7()); err != nil {
		return EnsureResult{}, err
	}
	w.ensured[object+"/"+row.ExternalID]++
	if w.landing != nil {
		w.landing()
	}
	return EnsureResult{Created: true}, nil
}

func (*mapWriters) Associate(_ context.Context, a Assoc) (AssocResult, error) {
	if a.ToType == "nowhere" {
		return AssocResult{Reason: "endpoint_not_imported"}, nil
	}
	return AssocResult{Applied: true}, nil
}

var errProcessDied = errors.New("the worker process died")

// approvedRun takes a draft to running, as approve does, and answers the lease
// the commit job is queued under.
func approvedRun(ctx context.Context, t *testing.T, s *RunStore) Lease {
	t.Helper()
	run := toReport(ctx, t, s, newDraft(ctx, t, s, ""))
	approved, err := s.Approve(ctx, run.ID, run.Version)
	if err != nil {
		t.Fatalf("Approve: %v", err)
	}
	if approved.JobAttempt <= run.JobAttempt {
		t.Fatalf("approve left the attempt at %d, want a new one past %d", approved.JobAttempt, run.JobAttempt)
	}
	return Lease{Run: approved.ID, Status: StatusRunning, Attempt: approved.JobAttempt}
}

// slice claims the lease and runs one slice of it, as the commit job does.
func slice(ctx context.Context, t *testing.T, s *RunStore, l Lease, w Writers, b Budget) bool {
	t.Helper()
	if _, err := s.Claim(ctx, l); err != nil {
		t.Fatalf("Claim: %v", err)
	}
	done, err := NewLeasedEngine(s, l, w).RunSlice(ctx, l.Run, twoObjectSource(), b)
	if err != nil {
		t.Fatalf("RunSlice: %v", err)
	}
	return done
}

func TestAStaleAttemptWritesNothing(t *testing.T) {
	ctx, db := testWorkspaceCtx(t, adminImportRunGrant())
	s := NewRunStore(db)
	current := approvedRun(ctx, t, s)
	stale := current
	stale.Attempt--

	if _, err := s.Claim(ctx, stale); !errors.Is(err, ErrSuperseded) {
		t.Errorf("a stale claim = %v, want ErrSuperseded", err)
	}
	if err := s.Flush(ctx, stale, Flush{Checkpoint: 3, Leg: Report{Imported: 3}}); !errors.Is(err, ErrSuperseded) {
		t.Errorf("a stale flush = %v, want ErrSuperseded", err)
	}
	if err := s.Heartbeat(ctx, stale); !errors.Is(err, ErrSuperseded) {
		t.Errorf("a stale heartbeat = %v, want ErrSuperseded", err)
	}
	w := newMapWriters(s, current.Run)
	if _, err := NewLeasedEngine(s, stale, w).RunSlice(ctx, current.Run, twoObjectSource(), Budget{}); !errors.Is(err, ErrSuperseded) {
		t.Errorf("a stale slice = %v, want ErrSuperseded", err)
	}
	if len(w.ensured) != 0 {
		t.Errorf("a stale slice landed %v, want nothing", w.ensured)
	}
	run := mustGetStaged(ctx, t, s, current.Run)
	if run.Checkpoint != 0 || run.Report.Imported != 3 || run.Status != StatusRunning {
		t.Errorf("after the stale attempt the run reads checkpoint %d, imported %d, %s; want it as approve left it",
			run.Checkpoint, run.Report.Imported, run.Status)
	}
}

func TestASupersededAttemptCannotFailTheRun(t *testing.T) {
	ctx, db := testWorkspaceCtx(t, adminImportRunGrant())
	s := NewRunStore(db)
	first := approvedRun(ctx, t, s)
	if err := s.FailLease(ctx, first, CauseStopped); err != nil {
		t.Fatalf("FailLease: %v", err)
	}
	resumed, err := s.ResumeApproved(ctx, first.Run, 0)
	if err != nil {
		t.Fatalf("ResumeApproved: %v", err)
	}

	if err := s.FailLease(ctx, first, CauseOverBudget); !errors.Is(err, ErrSuperseded) {
		t.Fatalf("the first attempt failing the resumed run = %v, want ErrSuperseded", err)
	}
	if got := mustGetStaged(ctx, t, s, first.Run); got.Status != StatusRunning || got.Error != "" ||
		got.JobAttempt != resumed.JobAttempt {
		t.Errorf("the resumed run reads %s, error %q, attempt %d; want running under attempt %d, untouched",
			got.Status, got.Error, got.JobAttempt, resumed.JobAttempt)
	}
}

func TestASliceStopsAtItsBudgetAndResumesAtItsCursor(t *testing.T) {
	ctx, db := testWorkspaceCtx(t, adminImportRunGrant())
	s := NewRunStore(db)
	l := approvedRun(ctx, t, s)
	w := newMapWriters(s, l.Run)

	// Landing the first row takes the clock to the deadline, so the slice
	// stops there on time rather than on rows.
	start := time.Date(2026, 3, 2, 9, 0, 0, 0, time.UTC)
	clock := start
	w.landing = func() { clock = start.Add(time.Second) }
	e := NewLeasedEngine(s, l, w)
	e.now = func() time.Time { return clock }
	if _, err := s.Claim(ctx, l); err != nil {
		t.Fatalf("Claim: %v", err)
	}
	done, err := e.RunSlice(ctx, l.Run, twoObjectSource(), Budget{Until: start.Add(time.Second)})
	if err != nil || done {
		t.Fatalf("a slice past its deadline = done %v, %v; want it stopped", done, err)
	}
	w.landing = nil
	assertCursor(ctx, t, db, l.Run, 1, PhaseImporting, "company")

	if slice(ctx, t, s, l, w, Budget{Rows: 3}) {
		t.Fatal("a slice of three rows finished a run of seven moves")
	}
	assertCursor(ctx, t, db, l.Run, 4, PhaseImporting, "contact")
	if !slice(ctx, t, s, l, w, Budget{Rows: 3}) {
		t.Fatal("the last three moves did not finish the run")
	}
	for key, n := range w.ensured {
		if n != 1 {
			t.Errorf("%s was created %d times; a slice resumes at its cursor", key, n)
		}
	}
	if len(w.ensured) != 4 {
		t.Errorf("the slices created %d rows, want the four with a payload", len(w.ensured))
	}
	if got := mustGetStaged(ctx, t, s, l.Run); got.Status != StatusComplete || !got.SourceReleased {
		t.Errorf("the run ended %s, released %v; want complete with its file released", got.Status, got.SourceReleased)
	}
}

// assertCursor reads what the last flush wrote.
func assertCursor(ctx context.Context, t *testing.T, db *database.DB, id RunID, checkpoint int, phase, object string) {
	t.Helper()
	var gotCheckpoint int
	var gotPhase, gotObject string
	var claimed bool
	if err := db.Tx(ctx, func(tx pgx.Tx) error {
		return tx.QueryRow(ctx, `
			SELECT checkpoint, phase, phase_object, claimed_attempt = job_attempt AND started_at IS NOT NULL
			  FROM import_run WHERE id = $1`, id).Scan(&gotCheckpoint, &gotPhase, &gotObject, &claimed)
	}); err != nil {
		t.Fatalf("reading the run's cursor: %v", err)
	}
	if gotCheckpoint != checkpoint || gotPhase != phase || gotObject != object || !claimed {
		t.Errorf("the run reads checkpoint %d, phase %s on %s, claimed %v; want %d, %s on %s, claimed",
			gotCheckpoint, gotPhase, gotObject, claimed, checkpoint, phase, object)
	}
}

// A sliced commit whose process died after landing a row it never flushed
// still reports what an uninterrupted commit of the same source reports.
func TestASlicedRunsReportEqualsAnUninterruptedOne(t *testing.T) {
	ctx, db := testWorkspaceCtx(t, adminImportRunGrant())
	s := NewRunStore(db)

	whole := approvedRun(ctx, t, s)
	if !slice(ctx, t, s, whole, newMapWriters(s, whole.Run), Budget{FlushRows: 2}) {
		t.Fatal("an unbounded slice did not finish the run")
	}

	sliced := approvedRun(ctx, t, s)
	w := newMapWriters(s, sliced.Run)
	slice(ctx, t, s, sliced, w, Budget{Rows: 2})
	// p-1 lands after the flush; the process dies on p-3 before any flush.
	w.dieAt = "p-3"
	func() {
		defer func() {
			if died := recover(); died != errProcessDied {
				t.Fatalf("the slice ended with %v, want the injected death", died)
			}
		}()
		slice(ctx, t, s, sliced, w, Budget{})
	}()
	slice(ctx, t, s, sliced, w, Budget{Rows: 2})
	if !slice(ctx, t, s, sliced, w, Budget{}) {
		t.Fatal("the last slice did not finish the run")
	}

	want, got := mustGetStaged(ctx, t, s, whole.Run), mustGetStaged(ctx, t, s, sliced.Run)
	if !reflect.DeepEqual(got.Report, want.Report) {
		t.Errorf("the sliced run reports %+v, want the uninterrupted run's %+v", *got.Report, *want.Report)
	}
	if !reflect.DeepEqual(got.Progress.Counts, want.Progress.Counts) {
		t.Errorf("the sliced run counts %+v, want the uninterrupted run's %+v", got.Progress.Counts, want.Progress.Counts)
	}
	if created := want.Progress.Counts.Created; created != 4 {
		t.Errorf("the uninterrupted run counts %d created, want the four rows with a payload", created)
	}
}

func TestUndoReversesARowTheImportItselfLinked(t *testing.T) {
	ctx, db := testWorkspaceCtx(t, adminImportRunGrant())
	s := NewRunStore(db)
	run := completeCSVRun(ctx, t, s)
	linked := landLead(ctx, t, s, run.ID, "row-1")
	edited := landLead(ctx, t, s, run.ID, "row-2")
	// The commit's link phase writes as the approver, under the run's batch.
	if err := db.Tx(ctx, func(tx pgx.Tx) error {
		_, err := storekit.Audit(storekit.WithBatch(ctx, run.ID), tx, "update", ObjectLead, linked,
			map[string]any{"company_id": nil}, map[string]any{"company_id": ids.NewV7()})
		return err
	}); err != nil {
		t.Fatalf("auditing the import's own link: %v", err)
	}
	markHumanTouched(ctx, t, db, "update", edited)

	if err := s.ClaimUndo(ctx, run.ID); err != nil {
		t.Fatalf("ClaimUndo: %v", err)
	}
	undoing := mustGetStaged(ctx, t, s, run.ID)
	l := Lease{Run: run.ID, Status: StatusUndoing, Attempt: undoing.JobAttempt}
	w := &fakeUndoWriters{}
	if done, err := s.UndoSlice(ctx, l, w, Budget{Rows: 1}); err != nil || done {
		t.Fatalf("a one-row undo slice = done %v, %v; want it stopped after one row", done, err)
	}
	if done, err := s.UndoSlice(ctx, l, w, Budget{}); err != nil || !done {
		t.Fatalf("the second undo slice = done %v, %v; want it finished", done, err)
	}

	got := mustGetStaged(ctx, t, s, run.ID)
	if !w.reversed[linked] || got.UndoReport.ReversedCount != 1 {
		t.Errorf("reversed %v with report %+v; the row only the import itself touched must be reversed", w.reversed, *got.UndoReport)
	}
	if len(got.UndoReport.Kept) != 1 || got.UndoReport.Kept[0].ID != edited {
		t.Errorf("kept %+v, want only the row a human edited", got.UndoReport.Kept)
	}
	if got.Status != StatusUndone {
		t.Errorf("the run ended %s, want undone", got.Status)
	}
}

func TestTwoAttemptsNeverHoldTheRunLockTogether(t *testing.T) {
	ctx, db := testWorkspaceCtx(t, adminImportRunGrant())
	s := NewRunStore(db)
	run := completeCSVRun(ctx, t, s)

	release, err := s.LockRunWrites(ctx, run.ID)
	if err != nil {
		t.Fatalf("LockRunWrites: %v", err)
	}
	if _, err := s.LockRunWrites(ctx, run.ID); !errors.Is(err, ErrRunWritesHeld) {
		t.Errorf("a second commit slice taking the lock = %v, want ErrRunWritesHeld", err)
	}
	// The undo of the run takes the same key as its commit.
	if err := s.ClaimUndo(ctx, run.ID); !errors.Is(err, ErrRunWritesHeld) || !errors.Is(err, apperrors.ErrConflict) {
		t.Errorf("an undo while a commit slice holds the run = %v, want ErrRunWritesHeld, a conflict", err)
	}
	release()

	again, err := s.LockRunWrites(ctx, run.ID)
	if err != nil {
		t.Fatalf("the lock after its holder released it = %v, want it taken", err)
	}
	again()
}
