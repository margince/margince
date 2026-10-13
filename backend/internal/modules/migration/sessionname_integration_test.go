// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package migration

import (
	"context"
	"errors"
	"testing"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/platform/database"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// fairName is the source name every run of these tests is filed under.
const fairName = "Fair 2026"

// seedFairRun writes an approved csv run of fairName in status, as a commit
// that got that far leaves it.
func seedFairRun(ctx context.Context, t *testing.T, db *database.DB, status string) RunID {
	t.Helper()
	var id RunID
	if err := db.Tx(ctx, func(tx pgx.Tx) error {
		return tx.QueryRow(ctx, `
			INSERT INTO import_run (connector, status, source_ref, source, captured_by, approved_at, mapping)
			VALUES ($1, $2, $3, 'test', 'human:fixture', now(), jsonb_build_object('source_name', $4::text))
			RETURNING id`,
			ConnectorCSV, status, "ws/import/"+ids.NewV7().String(), fairName).Scan(&id)
	}); err != nil {
		t.Fatalf("seeding a %s run: %v", status, err)
	}
	return id
}

func wantBusyNaming(t *testing.T, what string, err error, holder RunID) {
	t.Helper()
	busy, ok := errors.AsType[*SourceBusyError](err)
	if !ok || busy.RunID != holder || !errors.Is(err, apperrors.ErrConflict) {
		t.Errorf("%s = %v, want SourceBusyError naming %s", what, err, holder)
	}
}

func TestAResumeBesideAnActiveRunOfItsNameIsAConflictNamingIt(t *testing.T) {
	ctx, db := testWorkspaceCtx(t, adminImportRunGrant())
	s := NewRunStore(db)
	stopped := seedFairRun(ctx, t, db, StatusFailed)
	report := toReport(ctx, t, s, newDraft(ctx, t, s, fairName))

	_, err := s.ResumeApproved(ctx, stopped, 0)
	wantBusyNaming(t, "resuming beside a report of the name", err, report.ID)

	if _, err := s.Approve(ctx, report.ID, report.Version); err != nil {
		t.Fatalf("Approve: %v", err)
	}
	_, err = s.ResumeApproved(ctx, stopped, 0)
	wantBusyNaming(t, "resuming beside a commit of the name", err, report.ID)
	if got := mustGetStaged(ctx, t, s, stopped); got.Status != StatusFailed {
		t.Errorf("the refused run reads %s, want failed and still resumable", got.Status)
	}
}

func TestAnUndoClosesTheUnwrittenRunsOfItsName(t *testing.T) {
	ctx, db := testWorkspaceCtx(t, adminImportRunGrant())
	s := NewRunStore(db)
	done := seedFairRun(ctx, t, db, StatusComplete)
	report := toReport(ctx, t, s, newDraft(ctx, t, s, fairName))

	if err := s.ClaimUndo(ctx, done); err != nil {
		t.Fatalf("an undo beside a report of its name = %v, want it to take the name back", err)
	}
	got := mustGetStaged(ctx, t, s, report.ID)
	if got.Status != StatusCancelled || got.SupersededBy == nil || *got.SupersededBy != done ||
		!got.SourceReleased || got.Error != CauseReplacedByUndo {
		t.Errorf("the report reads %+v, want cancelled, superseded by the undo %s and released", got, done)
	}
}

func TestAnUndoBesideAWritingRunOfItsNameIsAConflictNamingIt(t *testing.T) {
	ctx, db := testWorkspaceCtx(t, adminImportRunGrant())
	s := NewRunStore(db)
	done := seedFairRun(ctx, t, db, StatusComplete)
	writing := seedFairRun(ctx, t, db, StatusRunning)

	wantBusyNaming(t, "an undo beside a commit of its name", s.ClaimUndo(ctx, done), writing)
	if got := mustGetStaged(ctx, t, s, done); got.Status != StatusComplete {
		t.Errorf("the refused undo left the run %s, want complete", got.Status)
	}
}

func TestAReplacedRunAnswersWithTheRunThatReplacedIt(t *testing.T) {
	ctx, db := testWorkspaceCtx(t, adminImportRunGrant())
	s := NewRunStore(db)
	older := toReport(ctx, t, s, newDraft(ctx, t, s, fairName))
	newer := newDraft(ctx, t, s, fairName)
	if _, _, err := s.QueueValidation(ctx, newer.ID, newer.Version); err != nil {
		t.Fatalf("QueueValidation: %v", err)
	}

	_, err := s.Approve(ctx, older.ID, older.Version)
	replaced, ok := errors.AsType[*SupersededError](err)
	if !ok || replaced.By != newer.ID || !errors.Is(err, apperrors.ErrConflict) {
		t.Errorf("approving a replaced report = %v, want SupersededError naming %s", err, newer.ID)
	}
}

func TestASecondCancelAnswersTheCancelledRun(t *testing.T) {
	ctx, db := testWorkspaceCtx(t, adminImportRunGrant())
	s := NewRunStore(db)
	draft := newDraft(ctx, t, s, "")
	if _, err := s.Cancel(ctx, draft.ID); err != nil {
		t.Fatalf("Cancel: %v", err)
	}
	audits := auditCount(ctx, t, db, draft.ID)
	again, err := s.Cancel(ctx, draft.ID)
	if err != nil || again.Status != StatusCancelled {
		t.Fatalf("a second cancel = (%s, %v), want the cancelled run as it stands", again.Status, err)
	}
	if after := auditCount(ctx, t, db, draft.ID); after != audits {
		t.Errorf("a second cancel wrote %d audit row(s), want none", after-audits)
	}
}

func TestAFlipRunKeepsNoSession(t *testing.T) {
	ctx, db := testWorkspaceCtx(t, adminImportRunGrant())
	s := NewRunStore(db)
	draft := newDraft(ctx, t, s, "")
	if err := db.Tx(ctx, func(tx pgx.Tx) error {
		_, err := tx.Exec(ctx, `UPDATE import_run SET connector = $2 WHERE id = $1`, draft.ID, connectorSalesforce)
		return err
	}); err != nil {
		t.Fatalf("turning the draft into a flip run: %v", err)
	}
	key := "Id"
	if _, err := s.UpdateDraft(ctx, draft.ID, draft.Version, DraftPatch{SourceKey: &key}); !errors.Is(err, apperrors.ErrConflict) {
		t.Errorf("saving a choice on a flip run = %v, want a conflict: only a csv run keeps a session", err)
	}
}
