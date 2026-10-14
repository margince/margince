// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package migration

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// fakeUndoWriters is UndoWriters over a plain map, because this package never
// imports contacts. The undo slice's own SQL is what these tests prove.
type fakeUndoWriters struct {
	reversed map[ids.UUID]bool
	// failAlways, if set, refuses every time this native id is reversed —
	// wrapped in apperrors.ErrConflict, a deterministic per-ROW refusal (a
	// business rule, a row-scope miss) that must land in the errored
	// bucket and never stop the rest of the run.
	failAlways ids.UUID
	// failUnreachable, if set, errors every time this native id is
	// reversed with a plain (unclassified) error — simulating the estate
	// itself being unreachable (a dropped connection, a timeout), which
	// must abort the pass and leave the run resumable rather than land in
	// errored.
	failUnreachable ids.UUID
}

func (w *fakeUndoWriters) Reverse(_ context.Context, _ string, nativeID ids.UUID, _ time.Time) error {
	if nativeID == w.failAlways {
		return fmt.Errorf("simulated business-rule refusal: %w", apperrors.ErrConflict)
	}
	if nativeID == w.failUnreachable {
		return errors.New("simulated infrastructure failure")
	}
	if w.reversed == nil {
		w.reversed = map[ids.UUID]bool{}
	}
	w.reversed[nativeID] = true
	return nil
}

// completeCSVRun drives a fresh staged run all the way to `complete`, the
// only state an undo starts from.
func completeCSVRun(ctx context.Context, t *testing.T, s *RunStore) Run {
	t.Helper()
	run := toReport(ctx, t, s, newDraft(ctx, t, s, ""))
	if _, err := s.Approve(ctx, run.ID, run.Version, queueNothing); err != nil {
		t.Fatalf("Approve: %v", err)
	}
	if err := s.complete(ctx, run.ID, Report{Imported: 2}); err != nil {
		t.Fatalf("complete: %v", err)
	}
	got, err := s.GetStaged(ctx, run.ID)
	if err != nil {
		t.Fatalf("GetStaged: %v", err)
	}
	return got
}

// landLead records one import_record_map row for the run, the bookkeeping a
// real Writers.Ensure commits alongside the native row.
func landLead(ctx context.Context, t *testing.T, s *RunStore, runID RunID, externalID string) ids.UUID {
	t.Helper()
	native := ids.NewV7()
	if err := recordCreated(ctx, s, runID, "import:csv", ObjectLead, externalID, native); err != nil {
		t.Fatalf("RecordIdentity: %v", err)
	}
	return native
}

// markHumanTouched inserts the audit_log row humanTouchedSince reads: a
// human action, occurring now — always after the row's own created_at,
// which a moment-earlier RecordIdentity call already committed.
func markHumanTouched(ctx context.Context, t *testing.T, db interface {
	Tx(context.Context, func(pgx.Tx) error) error
}, action string, nativeID ids.UUID,
) {
	t.Helper()
	err := db.Tx(ctx, func(tx pgx.Tx) error {
		_, err := tx.Exec(ctx, `
			INSERT INTO audit_log (id, actor_type, actor_id, action, entity_type, entity_id, occurred_at)
			VALUES ($1, 'human', 'human:tester', $2, $3, $4, now())`,
			ids.NewV7(), action, ObjectLead, nativeID)
		return err
	})
	if err != nil {
		t.Fatalf("seeding a human-touch audit row: %v", err)
	}
}

// claimUndoLease queues an undo attempt of the run, as the undo request does,
// and claims the lease its job is queued under.
func claimUndoLease(ctx context.Context, t *testing.T, s *RunStore, id RunID) (Lease, error) {
	t.Helper()
	moved, err := s.ClaimUndo(ctx, id, queueNothing)
	if err != nil {
		return Lease{}, err
	}
	l := Lease{Run: id, Status: StatusUndoing, Attempt: moved.JobAttempt}
	if _, err := s.Claim(ctx, l); err != nil {
		t.Fatalf("Claim: %v", err)
	}
	return l, nil
}

// undoRun queues an undo of the run and works it in one unbounded slice under
// the run's write lock, as the undo job does. It answers the stored report.
func undoRun(ctx context.Context, t *testing.T, s *RunStore, id RunID, w UndoWriters) (UndoReport, error) {
	t.Helper()
	l, err := claimUndoLease(ctx, t, s, id)
	if err != nil {
		return UndoReport{}, err
	}
	release, err := s.LockRunWrites(ctx, id)
	if err != nil {
		t.Fatalf("LockRunWrites: %v", err)
	}
	defer release()
	done, err := s.UndoSlice(ctx, l, w, Budget{})
	if err != nil {
		return UndoReport{}, err
	}
	if !done {
		t.Fatal("an unbounded undo slice stopped with rows left")
	}
	got := mustGetStaged(ctx, t, s, id)
	if got.UndoReport == nil {
		t.Fatal("a finished undo stored no report")
	}
	return *got.UndoReport, nil
}

func TestUndoReversesUntouchedRowsAndKeepsHumanEdited(t *testing.T) {
	ctx, db := testWorkspaceCtx(t, adminImportRunGrant())
	s := NewRunStore(db)
	run := completeCSVRun(ctx, t, s)

	untouched := landLead(ctx, t, s, run.ID, "row-1")
	edited := landLead(ctx, t, s, run.ID, "row-2")
	markHumanTouched(ctx, t, db, "update", edited)

	w := &fakeUndoWriters{}
	rep, err := undoRun(ctx, t, s, run.ID, w)
	if err != nil {
		t.Fatalf("undo: %v", err)
	}
	if rep.ReversedCount != 1 || !w.reversed[untouched] {
		t.Fatalf("undo report = %+v, reversed = %v, want the untouched row reversed and nothing else", rep, w.reversed)
	}
	if len(rep.Kept) != 1 || rep.Kept[0].ID != edited || rep.Kept[0].Object != ObjectLead {
		t.Fatalf("kept = %+v, want the human-edited row named", rep.Kept)
	}
	if len(rep.Errored) != 0 {
		t.Fatalf("errored = %+v, want none", rep.Errored)
	}
	if w.reversed[edited] {
		t.Fatal("the human-edited row was reversed — A93 requires it be left in place")
	}

	got, err := s.GetStaged(ctx, run.ID)
	if err != nil {
		t.Fatalf("GetStaged after undo: %v", err)
	}
	if got.Status != StatusUndone || got.UndoReport == nil || got.UndoReport.ReversedCount != 1 {
		t.Fatalf("run after undo = %+v, want status undone with the report persisted", got)
	}

	// Undoing an already-undone run is a conflict, not a no-op.
	if _, err := s.ClaimUndo(ctx, run.ID, queueNothing); !errors.Is(err, apperrors.ErrConflict) {
		t.Fatalf("second undo err = %v, want ErrConflict", err)
	}
}

// A93's protection is "has a human touched this row", not narrowly
// "updated" it: a lead a human independently disqualified outside the
// import (an 'archive' audit action, not 'update') must still be kept, not
// reversed out from under them.
func TestUndoKeepsARowAHumanTouchedThroughAnyAction(t *testing.T) {
	ctx, db := testWorkspaceCtx(t, adminImportRunGrant())
	s := NewRunStore(db)
	run := completeCSVRun(ctx, t, s)

	disqualified := landLead(ctx, t, s, run.ID, "row-1")
	markHumanTouched(ctx, t, db, "archive", disqualified)

	w := &fakeUndoWriters{}
	rep, err := undoRun(ctx, t, s, run.ID, w)
	if err != nil {
		t.Fatalf("undo: %v", err)
	}
	if rep.ReversedCount != 0 || len(rep.Kept) != 1 || rep.Kept[0].ID != disqualified {
		t.Fatalf("undo report = %+v, want the disqualified row kept, not reversed", rep)
	}
	if w.reversed[disqualified] {
		t.Fatal("a row a human touched (any action) was reversed")
	}
}

// A row Reverse cannot process — a business rule refuses it, or it is no
// longer visible — is recorded and left in place, and the rest of the run
// still completes. The old behaviour (abort on the first such row) wedged
// the run in `undoing` forever, since a deterministic per-row refusal fails
// identically on every retry.
func TestUndoRecordsAnUnreversibleRowAndContinues(t *testing.T) {
	ctx, db := testWorkspaceCtx(t, adminImportRunGrant())
	s := NewRunStore(db)
	run := completeCSVRun(ctx, t, s)

	stuck := landLead(ctx, t, s, run.ID, "row-1")
	fine := landLead(ctx, t, s, run.ID, "row-2")

	w := &fakeUndoWriters{failAlways: stuck}
	rep, err := undoRun(ctx, t, s, run.ID, w)
	if err != nil {
		t.Fatalf("undo: %v", err)
	}
	if rep.ReversedCount != 1 || !w.reversed[fine] {
		t.Fatalf("undo report = %+v, want the other row reversed despite the stuck one", rep)
	}
	if len(rep.Errored) != 1 || rep.Errored[0].ID != stuck || rep.Errored[0].Reason == "" {
		t.Fatalf("errored = %+v, want the stuck row named with a reason", rep.Errored)
	}
	got, err := s.GetStaged(ctx, run.ID)
	if err != nil {
		t.Fatalf("GetStaged: %v", err)
	}
	if got.Status != StatusUndone {
		t.Fatalf("status = %q, want undone — one unreversible row must not wedge the whole run", got.Status)
	}
}

// An error that is NOT a row refusal — the estate itself unreachable, not a
// fact about one record — must abort the pass and leave the run resumable,
// never landing in errored (which would misreport every row after it as
// individually unreversible) and never completing the run as `undone` (which
// would make it unrecoverable).
func TestUndoStopsResumableOnAnUnclassifiedFailure(t *testing.T) {
	ctx, db := testWorkspaceCtx(t, adminImportRunGrant())
	s := NewRunStore(db)
	run := completeCSVRun(ctx, t, s)

	unreachable := landLead(ctx, t, s, run.ID, "row-1")

	w := &fakeUndoWriters{failUnreachable: unreachable}
	if _, err := undoRun(ctx, t, s, run.ID, w); err == nil {
		t.Fatal("an undo with an unreachable-estate error returned nil, want the error")
	}
	got, err := s.GetStaged(ctx, run.ID)
	if err != nil {
		t.Fatalf("GetStaged: %v", err)
	}
	if got.Status != StatusUndoing {
		t.Fatalf("status = %q, want undoing — the run must stay resumable, not finish as undone", got.Status)
	}
	if got.UndoReport != nil && len(got.UndoReport.Errored) != 0 {
		t.Fatalf("errored = %+v, want the unreachable row NOT recorded as an individual refusal", got.UndoReport.Errored)
	}

	// A later attempt with a writer that can reach the estate resumes and
	// reverses the row the first attempt never got to record either way.
	resumed := &fakeUndoWriters{}
	rep, err := undoRun(ctx, t, s, run.ID, resumed)
	if err != nil {
		t.Fatalf("resumed undo: %v", err)
	}
	if rep.ReversedCount != 1 || !resumed.reversed[unreachable] {
		t.Fatalf("resumed undo report = %+v, want the row reversed", rep)
	}
}

func TestARestartedUndoDoesNotKeepTheCauseOfTheStopBeforeIt(t *testing.T) {
	ctx, db := testWorkspaceCtx(t, adminImportRunGrant())
	s := NewRunStore(db)
	run := completeCSVRun(ctx, t, s)
	landLead(ctx, t, s, run.ID, "row-1")

	l, err := claimUndoLease(ctx, t, s, run.ID)
	if err != nil {
		t.Fatalf("claimUndoLease: %v", err)
	}
	if err := s.FailLease(ctx, l, CauseStale); err != nil {
		t.Fatalf("FailLease: %v", err)
	}
	if got := mustGetStaged(ctx, t, s, run.ID); got.Status != StatusUndoing || got.Error == "" {
		t.Fatalf("a stopped undo reads %q with error %q, want undoing carrying its cause", got.Status, got.Error)
	}

	if _, err := claimUndoLease(ctx, t, s, run.ID); err != nil {
		t.Fatalf("restarting the undo: %v", err)
	}
	if got := mustGetStaged(ctx, t, s, run.ID); got.Error != "" {
		t.Errorf("a restarted undo still reads error %q, want none while it runs", got.Error)
	}
}

func TestUndoRefusesEveryConnectorButCSV(t *testing.T) {
	ctx, db := testWorkspaceCtx(t, adminImportRunGrant())
	s := NewRunStore(db)
	run, err := s.Create(ctx, CreateRunInput{Connector: connectorSalesforce, SourceRef: "portal", Source: "import_api"})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	if err := s.complete(ctx, run.ID, Report{}); err != nil {
		t.Fatalf("complete: %v", err)
	}
	if _, err := s.ClaimUndo(ctx, run.ID, queueNothing); !errors.Is(err, apperrors.ErrConflict) {
		t.Fatalf("undo of a mirror run err = %v, want ErrConflict — undo is csv-only", err)
	}
}

func TestUndoRefusesAnythingButComplete(t *testing.T) {
	ctx, db := testWorkspaceCtx(t, adminImportRunGrant())
	s := NewRunStore(db)
	run := newDraft(ctx, t, s, "")
	if _, _, err := s.QueueValidation(ctx, run.ID, run.Version, RunMapping{}, queueNothing); err != nil {
		t.Fatalf("QueueValidation: %v", err)
	}
	if _, err := s.ClaimUndo(ctx, run.ID, queueNothing); !errors.Is(err, apperrors.ErrConflict) {
		t.Fatalf("undo of a validating run err = %v, want ErrConflict", err)
	}
}

// A second undo while a slice of the same run is under way is a conflict, not
// a second pass over the same rows. beginUndo's row lock covers only its own
// transaction, so the write lock the slice holds is what refuses it.
func TestUndoRefusesAConcurrentSecondCallOnTheSameRun(t *testing.T) {
	ctx, db := testWorkspaceCtx(t, adminImportRunGrant())
	s := NewRunStore(db)
	run := completeCSVRun(ctx, t, s)
	landLead(ctx, t, s, run.ID, "row-1")

	release, err := s.LockRunWrites(ctx, run.ID)
	if err != nil {
		t.Fatalf("LockRunWrites: %v", err)
	}
	defer release()

	if _, err := s.ClaimUndo(ctx, run.ID, queueNothing); !errors.Is(err, apperrors.ErrConflict) {
		t.Fatalf("undo while the lock is held err = %v, want ErrConflict", err)
	}
}

// A run resumed (or re-entered on a later page) must not miss an edit made
// while it was still awaiting the next page — the reference instant is per
// row (import_record_map.created_at), not the run's own last-touched time,
// which advances every page.
func TestUndoResumesFromAPersistedCheckpointAcrossPages(t *testing.T) {
	ctx, db := testWorkspaceCtx(t, adminImportRunGrant())
	s := NewRunStore(db)
	run := completeCSVRun(ctx, t, s)

	first := landLead(ctx, t, s, run.ID, "row-1")
	second := landLead(ctx, t, s, run.ID, "row-2")

	// A first attempt reverses one row, persists its checkpoint and dies.
	l, err := claimUndoLease(ctx, t, s, run.ID)
	if err != nil {
		t.Fatalf("claiming the first undo attempt: %v", err)
	}
	died := &fakeUndoWriters{}
	if done, err := s.UndoSlice(ctx, l, died, Budget{Rows: 1}); err != nil || done {
		t.Fatalf("a one-row undo slice = done %v, %v; want it stopped after one row", done, err)
	}
	if !died.reversed[first] || died.reversed[second] {
		t.Fatalf("the first attempt reversed %v, want only the first row", died.reversed)
	}

	resumed := &fakeUndoWriters{}
	rep, err := undoRun(ctx, t, s, run.ID, resumed)
	if err != nil {
		t.Fatalf("resumed undo: %v", err)
	}
	if resumed.reversed[first] {
		t.Fatal("the resumed attempt redid a row the persisted checkpoint already covered")
	}
	if !resumed.reversed[second] {
		t.Fatal("the resumed attempt never reached the row after the checkpoint")
	}
	// The count survives across attempts: 1 carried and 1 the resumed attempt reversed.
	if rep.ReversedCount != 2 {
		t.Fatalf("reversed_count = %d, want 2 (1 carried + 1 new)", rep.ReversedCount)
	}
}

// An undo resolves its run before it queues anything. A run id that names no
// run answers not-found.
func TestUndoOfAnUnknownRunIsNotFound(t *testing.T) {
	ctx, db := testWorkspaceCtx(t, adminImportRunGrant())
	s := NewRunStore(db)
	if _, err := s.ClaimUndo(ctx, RunID(ids.NewV7()), queueNothing); !errors.Is(err, apperrors.ErrNotFound) {
		t.Fatalf("an undo of an unknown run = %v, want ErrNotFound", err)
	}
}

// Undo reads the identity map and never the file, so a finished run lets the
// file go. No run can then test its upload again.
func TestACompletedRunReleasesItsSource(t *testing.T) {
	ctx, db := testWorkspaceCtx(t, adminImportRunGrant())
	s := NewRunStore(db)
	done := completeCSVRun(ctx, t, s)
	if !done.SourceReleased || done.Profile != nil {
		t.Fatalf("the completed run reads released=%v profile=%s, want its file and profile released", done.SourceReleased, done.Profile)
	}
	if kinds := intentKinds(ctx, t, db, done.SourceRef); !slices.Equal(kinds, []string{"import"}) {
		t.Errorf("the released key is in the ledger as %v, want one import intent for the reaper", kinds)
	}
	holder, err := s.RunOfSource(ctx, done.SourceRef)
	if err != nil || holder.ID != done.ID || !holder.SourceReleased {
		t.Errorf("RunOfSource = (%+v, %v), want the completed run, released", holder, err)
	}
	if _, err := s.RunOfSource(ctx, "ws/import/"+ids.NewV7().String()); !errors.Is(err, apperrors.ErrNotFound) {
		t.Errorf("RunOfSource of an upload no run names = %v, want not found", err)
	}
}

// archivingUndoWriters archives each row once, answering nil for one already
// archived as the real writer does. dieAfter stands for the process dying once
// that row's archive committed, before the page's checkpoint did.
type archivingUndoWriters struct {
	archived map[ids.UUID]int
	dieAfter ids.UUID
}

func (w *archivingUndoWriters) Reverse(_ context.Context, _ string, nativeID ids.UUID, _ time.Time) error {
	if w.archived[nativeID] > 0 {
		return nil
	}
	w.archived[nativeID]++
	if nativeID == w.dieAfter {
		w.dieAfter = ids.Nil
		panic(errProcessDied)
	}
	return nil
}

// A page replayed after the process died counts each row it reversed once. The
// checkpoint and the report are one write, so both lost the same rows.
func TestAReplayedUndoPageCountsEachRowOnce(t *testing.T) {
	ctx, db := testWorkspaceCtx(t, adminImportRunGrant())
	s := NewRunStore(db)
	run := completeCSVRun(ctx, t, s)
	rows := []ids.UUID{landLead(ctx, t, s, run.ID, "row-1"), landLead(ctx, t, s, run.ID, "row-2"), landLead(ctx, t, s, run.ID, "row-3")}
	l, err := claimUndoLease(ctx, t, s, run.ID)
	if err != nil {
		t.Fatalf("claiming the undo: %v", err)
	}
	w := &archivingUndoWriters{archived: map[ids.UUID]int{}}
	got, err := s.mapRowsForRun(ctx, run.ID, 0, undoPageSize)
	if err != nil || len(got) != len(rows) {
		t.Fatalf("reading the page = %d rows, %v; want %d", len(got), err, len(rows))
	}
	w.dieAfter = got[1].nativeID

	func() {
		defer func() {
			if died := recover(); died != errProcessDied {
				t.Fatalf("the slice ended with %v, want the injected death", died)
			}
		}()
		done, err := s.UndoSlice(ctx, l, w, Budget{})
		t.Fatalf("the slice answered done %v, %v; want the injected death", done, err)
	}()
	if done, err := s.UndoSlice(ctx, l, w, Budget{}); err != nil || !done {
		t.Fatalf("the replayed slice = done %v, %v; want it finished", done, err)
	}

	for _, id := range rows {
		if w.archived[id] != 1 {
			t.Errorf("row %s was archived %d times, want once", id, w.archived[id])
		}
	}
	if reversed := mustGetStaged(ctx, t, s, run.ID).UndoReport.ReversedCount; reversed != len(rows) {
		t.Errorf("the undo reports %d reversed, want each of the %d rows once", reversed, len(rows))
	}
}
