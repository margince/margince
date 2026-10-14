// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package migration

import (
	"context"
	"testing"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// bindLead records a ref the run bound to a lead it did not create, as a
// row naming its record by id does.
func bindLead(ctx context.Context, t *testing.T, s *RunStore, runID RunID, externalID string) ids.UUID {
	t.Helper()
	native := ids.NewV7()
	if err := s.tx(ctx, func(tx pgx.Tx) error {
		return s.RecordIdentityTx(ctx, tx, runID, "import:csv", ObjectLead, externalID, native, OriginBound)
	}); err != nil {
		t.Fatalf("RecordIdentityTx bound: %v", err)
	}
	return native
}

func TestUndoNeverArchivesABoundRecord(t *testing.T) {
	ctx, db := testWorkspaceCtx(t, adminImportRunGrant())
	s := NewRunStore(db)
	run := completeCSVRun(ctx, t, s)
	created := landLead(ctx, t, s, run.ID, "row-1")
	bound := bindLead(ctx, t, s, run.ID, "row-2")
	touchedBound := bindLead(ctx, t, s, run.ID, "row-3")
	markHumanTouched(ctx, t, db, "update", touchedBound)

	w := &fakeUndoWriters{}
	rep, err := undoRun(ctx, t, s, run.ID, w)
	if err != nil {
		t.Fatalf("undo: %v", err)
	}
	if w.reversed[bound] || w.reversed[touchedBound] {
		t.Fatalf("reversed = %v, want no bound record archived: the run only corrected it", w.reversed)
	}
	if rep.ReversedCount != 1 || !w.reversed[created] {
		t.Fatalf("undo report = %+v, want the one created row reversed", rep)
	}
	if len(rep.Kept) != 0 || len(rep.Errored) != 0 {
		t.Fatalf("undo report = %+v, want a bound row neither kept nor errored: undo never reads it", rep)
	}
}

func TestABoundRowIsFoundByEveryBindingReader(t *testing.T) {
	ctx, db := testWorkspaceCtx(t, adminImportRunGrant())
	s := NewRunStore(db)
	run := completeCSVRun(ctx, t, s)
	bound := bindLead(ctx, t, s, run.ID, "row-1")

	binding, found, err := s.LookupBinding(ctx, "import:csv", ObjectLead, "row-1")
	if err != nil || !found || binding.NativeID != bound {
		t.Fatalf("LookupBinding = %+v, %v, %v; want the bound lead", binding, found, err)
	}
	all, err := s.BindingsOf(ctx, "import:csv", ObjectLead)
	if err != nil || all["row-1"].NativeID != bound {
		t.Fatalf("BindingsOf = %v, %v; want row-1 bound to its lead", all, err)
	}
}

// A replayed row reads Unchanged and is counted as the create it was. A bound
// row was never a create, so a crash after it must not turn it into one.
func TestABoundRowIsNeverReplayedAsACreation(t *testing.T) {
	ctx, db := testWorkspaceCtx(t, adminImportRunGrant())
	s := NewRunStore(db)
	l := approvedRun(ctx, t, s)
	landLead(ctx, t, s, l.Run, "row-1")
	bindLead(ctx, t, s, l.Run, "row-2")

	replayed, err := (&leasedRecords{runs: s, lease: l}).replayed(ctx)
	if err != nil {
		t.Fatalf("replayed: %v", err)
	}
	if !replayed[ObjectLead+"/row-1"] || replayed[ObjectLead+"/row-2"] || len(replayed) != 1 {
		t.Fatalf("replayed = %v, want only the created row", replayed)
	}
}

// recordCreated records a ref the run created, in its own transaction.
func recordCreated(ctx context.Context, s *RunStore, runID RunID, system, object, externalID string, native ids.UUID) error {
	return s.tx(ctx, func(tx pgx.Tx) error {
		return s.RecordIdentityTx(ctx, tx, runID, system, object, externalID, native, OriginCreated)
	})
}

// lookupNative is the native id a ref resolves to, if any.
func lookupNative(ctx context.Context, s *RunStore, system, object, externalID string) (ids.UUID, bool, error) {
	binding, found, err := s.LookupBinding(ctx, system, object, externalID)
	return binding.NativeID, found, err
}

func TestRecordingARefMappedToAnotherRecordIsRefusedAndAReplayIsNot(t *testing.T) {
	ctx, db := testWorkspaceCtx(t, adminImportRunGrant())
	s := NewRunStore(db)
	l := approvedRun(ctx, t, s)
	first := ids.NewV7()
	if err := recordCreated(ctx, s, l.Run, "import:csv", ObjectLead, "row-1", first); err != nil {
		t.Fatalf("first record: %v", err)
	}
	if err := recordCreated(ctx, s, l.Run, "import:csv", ObjectLead, "row-1", first); err != nil {
		t.Fatalf("replaying the same record must stay a no-op: %v", err)
	}
	if err := recordCreated(ctx, s, l.Run, "import:csv", ObjectLead, "row-1", ids.NewV7()); err == nil {
		t.Fatal("a ref already mapped to another record was recorded without an error")
	}
	if got, _, _ := lookupNative(ctx, s, "import:csv", ObjectLead, "row-1"); got != first {
		t.Fatalf("ref resolves to %v, want the first record %v", got, first)
	}
}
