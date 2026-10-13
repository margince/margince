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
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
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

func TestAStoppedCommitWithoutItsFileCannotResume(t *testing.T) {
	ctx, db := testWorkspaceCtx(t, adminImportRunGrant())
	s := NewRunStore(db)
	released := seedFairRun(ctx, t, db, StatusFailed)
	if err := db.Tx(ctx, func(tx pgx.Tx) error {
		_, err := tx.Exec(ctx, `UPDATE import_run SET source_released_at = now(), error = $2 WHERE id = $1`, released, CauseSourceTaken)
		return err
	}); err != nil {
		t.Fatalf("releasing the run's file: %v", err)
	}
	if _, err := s.ResumeApproved(ctx, released, 0); !errors.Is(err, apperrors.ErrConflict) {
		t.Errorf("resuming a stopped commit whose file is released = %v, want a conflict: the commit would read nothing", err)
	}
	if got := mustGetStaged(ctx, t, s, released); got.Status != StatusFailed {
		t.Errorf("the refused run reads %s, want failed", got.Status)
	}
}

// openerWith is the caller of ctx under another identity holding only grant.
func openerWith(ctx context.Context, id string, grant principal.ObjectGrant) context.Context {
	return principal.WithActor(ctx, principal.Principal{
		Type: principal.PrincipalHuman, ID: id, SeatType: principal.SeatFull,
		Permissions: principal.Permissions{
			Objects: map[string]principal.ObjectGrant{ImportRunObject: grant}, RowScope: principal.RowScopeAll,
		},
	})
}

func TestTheOpenerOfADraftDiscardsItWithTheGrantThatOpenedIt(t *testing.T) {
	ctx, db := testWorkspaceCtx(t, adminImportRunGrant())
	s := NewRunStore(db)
	createOnly := principal.ObjectGrant{Create: true}
	opener := openerWith(ctx, "human:"+ids.NewV7().String(), createOnly)
	draft := newDraft(opener, t, s, "")

	other := openerWith(ctx, "human:"+ids.NewV7().String(), createOnly)
	if err := s.DiscardOwnDraft(other, draft.SourceRef); !errors.Is(err, apperrors.ErrPermissionDenied) {
		t.Errorf("discarding someone else's draft = %v, want permission denied", err)
	}
	if err := s.DiscardOwnDraft(opener, draft.SourceRef); err != nil {
		t.Fatalf("its opener discarding the draft with only the create grant = %v, want it cancelled", err)
	}
	if got := mustGetStaged(ctx, t, s, draft.ID); got.Status != StatusCancelled || !got.SourceReleased {
		t.Errorf("the discarded draft reads %s (released %t), want cancelled and released", got.Status, got.SourceReleased)
	}
	if err := s.DiscardOwnDraft(opener, draft.SourceRef); err != nil {
		t.Errorf("discarding a released upload again = %v, want a no-op", err)
	}
}

func TestAnApprovedRunKeepsNoProfile(t *testing.T) {
	ctx, db := testWorkspaceCtx(t, adminImportRunGrant())
	s := NewRunStore(db)
	report := toReport(ctx, t, s, newDraft(ctx, t, s, ""))
	if len(report.Profile) == 0 {
		t.Fatal("the report has no profile to drop; the test proves nothing")
	}
	if _, err := s.Approve(ctx, report.ID, report.Version); err != nil {
		t.Fatalf("Approve: %v", err)
	}
	if err := s.FailRunning(ctx, report.ID, CauseStopped); err != nil {
		t.Fatalf("FailRunning: %v", err)
	}
	if got := mustGetStaged(ctx, t, s, report.ID); got.Profile != nil || got.SourceReleased {
		t.Errorf("a stopped commit reads profile %s (released %t), want no profile and its file kept for a resume", got.Profile, got.SourceReleased)
	}
}
