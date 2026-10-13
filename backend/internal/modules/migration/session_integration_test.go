// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package migration

import (
	"context"
	"errors"
	"slices"
	"sync"
	"testing"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/platform/database"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

var everyRunStatus = []string{
	StatusPending, StatusValidating, StatusAwaitingApproval, StatusRunning, StatusComplete,
	StatusFailed, StatusUndoing, StatusUndone, StatusCancelled,
}

// seedRunIn writes an approved, unnamed csv run in status, as no store path would.
func seedRunIn(ctx context.Context, t *testing.T, db *database.DB, status string) RunID {
	t.Helper()
	var id RunID
	if err := db.Tx(ctx, func(tx pgx.Tx) error {
		return tx.QueryRow(ctx, `
			INSERT INTO import_run (connector, status, source_ref, source, captured_by, approved_at)
			VALUES ($1, $2, $3, 'test', 'human:fixture', now()) RETURNING id`,
			ConnectorCSV, status, "ws/import/"+ids.NewV7().String()).Scan(&id)
	}); err != nil {
		t.Fatalf("seeding a %s run: %v", status, err)
	}
	return id
}

func newDraft(ctx context.Context, t *testing.T, s *RunStore, sourceName string) Run {
	t.Helper()
	run, err := s.CreateDraftRun(ctx, CreateDraftRunInput{
		SourceRef: "ws/import/" + ids.NewV7().String(), Source: "import_api", FileName: "leads.csv",
		Profile: []byte(`{"files":[{"name":"leads.csv","row_count":3}]}`),
		Mapping: RunMapping{
			Object: ObjectLead, Fields: map[string]string{"Email": "email"}, SourceKey: "Email",
			SourceName: sourceName,
		},
	})
	if err != nil {
		t.Fatalf("CreateDraftRun: %v", err)
	}
	return run
}

// toReport takes a draft through its test run to a report awaiting approval.
func toReport(ctx context.Context, t *testing.T, s *RunStore, run Run) Run {
	t.Helper()
	if _, _, err := s.QueueValidation(ctx, run.ID, run.Version); err != nil {
		t.Fatalf("QueueValidation: %v", err)
	}
	if err := s.AwaitApproval(ctx, run.ID, Report{Imported: 3}); err != nil {
		t.Fatalf("AwaitApproval: %v", err)
	}
	return mustGetStaged(ctx, t, s, run.ID)
}

func mustGetStaged(ctx context.Context, t *testing.T, s *RunStore, id RunID) Run {
	t.Helper()
	run, err := s.GetStaged(ctx, id)
	if err != nil {
		t.Fatalf("GetStaged: %v", err)
	}
	return run
}

func auditCount(ctx context.Context, t *testing.T, db *database.DB, id RunID) int {
	t.Helper()
	var n int
	if err := db.Tx(ctx, func(tx pgx.Tx) error {
		return tx.QueryRow(ctx, `SELECT count(*) FROM audit_log WHERE entity_type = 'import_run' AND entity_id = $1`, id).Scan(&n)
	}); err != nil {
		t.Fatalf("counting audit rows: %v", err)
	}
	return n
}

func TestEveryTransitionMovesOnlyFromItsDeclaredStates(t *testing.T) {
	ctx, db := testWorkspaceCtx(t, adminImportRunGrant())
	s := NewRunStore(db)
	for name, move := range sessionMoves {
		for _, status := range everyRunStatus {
			id := seedRunIn(ctx, t, db, status)
			err := db.Tx(ctx, func(tx pgx.Tx) error { return stageTransition(ctx, tx, id, name, runChange{}) })
			got := mustGetStaged(ctx, t, s, id).Status
			if slices.Contains(move.from, status) {
				if err != nil || got != move.to {
					t.Errorf("%s from %s = (%s, %v), want it to move to %s", name, status, got, err, move.to)
				}
				continue
			}
			if !errors.Is(err, apperrors.ErrConflict) || got != status {
				t.Errorf("%s from %s = (%s, %v), want a conflict that leaves the run %s", name, status, got, err, status)
			}
		}
	}
}

func TestAPatchOfAChoiceReturnsAReportToDraft(t *testing.T) {
	ctx, db := testWorkspaceCtx(t, adminImportRunGrant())
	s := NewRunStore(db)
	run := toReport(ctx, t, s, newDraft(ctx, t, s, ""))

	step := DraftStepMap
	if _, err := s.UpdateDraft(ctx, run.ID, run.Version, DraftPatch{DraftStep: &step}); !errors.Is(err, apperrors.ErrConflict) {
		t.Fatalf("a step-only patch of a report = %v, want a conflict: it changes no choice the report rests on", err)
	}
	object := ObjectCompany
	patched, err := s.UpdateDraft(ctx, run.ID, run.Version, DraftPatch{Object: &object, DraftStep: &step})
	if err != nil {
		t.Fatalf("UpdateDraft: %v", err)
	}
	if patched.Status != StatusPending || patched.Report != nil || patched.Version != run.Version+1 ||
		patched.Mapping == nil || patched.Mapping.Object != ObjectCompany || patched.DraftStep != DraftStepMap {
		t.Errorf("the patched report reads %+v, want a draft of companies at version %d with no report", patched, run.Version+1)
	}
}

func TestAPatchOfNothingWritesNothing(t *testing.T) {
	ctx, db := testWorkspaceCtx(t, adminImportRunGrant())
	s := NewRunStore(db)
	run := newDraft(ctx, t, s, "Fair 2026")
	audits := auditCount(ctx, t, db, run.ID)

	same, fields, empty := "Fair 2026", map[string]string{"Email": "email"}, []PartMapping{}
	got, err := s.UpdateDraft(ctx, run.ID, run.Version, DraftPatch{SourceName: &same, Fields: &fields, Parts: &empty})
	if err != nil {
		t.Fatalf("UpdateDraft: %v", err)
	}
	if got.Version != run.Version || !got.UpdatedAt.Equal(run.UpdatedAt) {
		t.Errorf("a patch repeating every value moved the run to version %d at %s, want it untouched", got.Version, got.UpdatedAt)
	}
	if after := auditCount(ctx, t, db, run.ID); after != audits {
		t.Errorf("a patch repeating every value wrote %d audit row(s), want none", after-audits)
	}
}

func TestAStepAloneSpendsNoVersion(t *testing.T) {
	ctx, db := testWorkspaceCtx(t, adminImportRunGrant())
	s := NewRunStore(db)
	run := newDraft(ctx, t, s, "")
	step := DraftStepMap
	moved, err := s.UpdateDraft(ctx, run.ID, run.Version, DraftPatch{DraftStep: &step})
	if err != nil || moved.Version != run.Version || moved.DraftStep != DraftStepMap {
		t.Fatalf("a step-only patch = (%+v, %v), want the step kept at version %d", moved, err, run.Version)
	}
	key := "Email address"
	if _, err := s.UpdateDraft(ctx, run.ID, run.Version, DraftPatch{SourceKey: &key}); err != nil {
		t.Errorf("a choice saved by a tab that read the run before the step moved = %v, want it accepted", err)
	}
}

func TestVersionMismatchIsAConflict(t *testing.T) {
	ctx, db := testWorkspaceCtx(t, adminImportRunGrant())
	s := NewRunStore(db)
	run := newDraft(ctx, t, s, "")
	key := "Email address"
	if _, err := s.UpdateDraft(ctx, run.ID, run.Version, DraftPatch{SourceKey: &key}); err != nil {
		t.Fatalf("UpdateDraft: %v", err)
	}
	other := "Id"
	if _, err := s.UpdateDraft(ctx, run.ID, run.Version, DraftPatch{SourceKey: &other}); !errors.Is(err, ErrVersionMismatch) ||
		!errors.Is(err, apperrors.ErrConflict) {
		t.Errorf("a patch against the old version = %v, want ErrVersionMismatch, a conflict", err)
	}
	if _, _, err := s.QueueValidation(ctx, run.ID, run.Version); !errors.Is(err, ErrVersionMismatch) {
		t.Errorf("a test run of the old version = %v, want ErrVersionMismatch", err)
	}
	if got := mustGetStaged(ctx, t, s, run.ID); got.Status != StatusPending || got.Mapping.SourceKey != key {
		t.Errorf("after both refusals the run reads %s keyed on %q, want the draft as the first patch left it", got.Status, got.Mapping.SourceKey)
	}
}

func TestValidatingANameSupersedesItsUnwrittenRuns(t *testing.T) {
	ctx, db := testWorkspaceCtx(t, adminImportRunGrant())
	s := NewRunStore(db)
	older := toReport(ctx, t, s, newDraft(ctx, t, s, "Fair 2026"))
	draft := newDraft(ctx, t, s, "Fair 2026")
	newer := newDraft(ctx, t, s, "Fair 2026")

	queued, moved, err := s.QueueValidation(ctx, newer.ID, newer.Version)
	if err != nil {
		t.Fatalf("QueueValidation: %v", err)
	}
	if queued.Status != StatusValidating || !moved {
		t.Errorf("the new test run reads %s (moved %t), want validating and moved by this call", queued.Status, moved)
	}
	got := mustGetStaged(ctx, t, s, older.ID)
	if got.Status != StatusCancelled || got.SupersededBy == nil || *got.SupersededBy != newer.ID ||
		!got.SourceReleased || got.Error != CauseSuperseded {
		t.Errorf("the older report reads %+v, want cancelled, superseded by %s and released", got, newer.ID)
	}
	if got := mustGetStaged(ctx, t, s, draft.ID); got.Status != StatusPending {
		t.Errorf("a draft of the same name reads %s, want it left alone: a draft holds no report", got.Status)
	}
}

func TestASecondTestRunOfOneVersionSaysItDidNotMove(t *testing.T) {
	ctx, db := testWorkspaceCtx(t, adminImportRunGrant())
	s := NewRunStore(db)
	run := newDraft(ctx, t, s, "Fair 2026")
	if _, _, err := s.QueueValidation(ctx, run.ID, run.Version); err != nil {
		t.Fatalf("QueueValidation: %v", err)
	}
	audits := auditCount(ctx, t, db, run.ID)
	again, moved, err := s.QueueValidation(ctx, run.ID, run.Version)
	if err != nil || moved || again.Status != StatusValidating {
		t.Fatalf("a repeated test run = (%s, moved %t, %v), want the validating run, not moved", again.Status, moved, err)
	}
	if after := auditCount(ctx, t, db, run.ID); after != audits {
		t.Errorf("a repeated test run of one version wrote %d audit row(s), want none", after-audits)
	}
	// The second request saves the same choices first; that save must not refuse the test under way.
	same := "Fair 2026"
	if _, err := s.UpdateDraft(ctx, run.ID, run.Version, DraftPatch{SourceName: &same}); err != nil {
		t.Errorf("repeating the tested choices on a validating run = %v, want them answered as they stand", err)
	}
	other := "Expo"
	if _, err := s.UpdateDraft(ctx, run.ID, run.Version, DraftPatch{SourceName: &other}); !errors.Is(err, apperrors.ErrConflict) {
		t.Errorf("changing a choice under a test run = %v, want a conflict", err)
	}
}

func TestTestRunsOfOneNameAtOnceEachSupersedeTheLast(t *testing.T) {
	ctx, db := testWorkspaceCtx(t, adminImportRunGrant())
	s := NewRunStore(db)
	const crowd = 8
	drafts := make([]Run, crowd)
	for i := range drafts {
		drafts[i] = newDraft(ctx, t, s, "Fair 2026")
	}
	errs := make([]error, crowd)
	var start, done sync.WaitGroup
	start.Add(1)
	for i, draft := range drafts {
		done.Go(func() {
			start.Wait()
			_, _, errs[i] = s.QueueValidation(ctx, draft.ID, draft.Version)
		})
	}
	start.Done()
	done.Wait()

	validating := 0
	for i, draft := range drafts {
		if errs[i] != nil {
			t.Errorf("test run %d of one name = %v, want it to supersede the others, never to fail", i, errs[i])
		}
		switch got := mustGetStaged(ctx, t, s, draft.ID); got.Status {
		case StatusValidating:
			validating++
		case StatusCancelled:
			if got.SupersededBy == nil {
				t.Errorf("run %d was cancelled with no successor", i)
			}
		default:
			t.Errorf("run %d reads %s, want validating or superseded", i, got.Status)
		}
	}
	if validating != 1 {
		t.Errorf("%d runs of one name are validating, want exactly one", validating)
	}
}

func TestAWritingRunOfTheSameNameIsAConflictNamingIt(t *testing.T) {
	ctx, db := testWorkspaceCtx(t, adminImportRunGrant())
	s := NewRunStore(db)
	writing := toReport(ctx, t, s, newDraft(ctx, t, s, "Fair 2026"))
	if _, err := s.Approve(ctx, writing.ID, writing.Version, queueNothing); err != nil {
		t.Fatalf("Approve: %v", err)
	}
	draft := newDraft(ctx, t, s, "Fair 2026")

	_, _, err := s.QueueValidation(ctx, draft.ID, draft.Version)
	var busy *SourceBusyError
	if !errors.As(err, &busy) || busy.RunID != writing.ID || !errors.Is(err, apperrors.ErrConflict) {
		t.Fatalf("a test run beside a writing run of its name = %v, want SourceBusyError naming %s", err, writing.ID)
	}
	if got := mustGetStaged(ctx, t, s, draft.ID); got.Status != StatusPending {
		t.Errorf("the refused draft reads %s, want pending", got.Status)
	}
}

func TestResumeRefusesARunThatWasNeverApproved(t *testing.T) {
	ctx, db := testWorkspaceCtx(t, adminImportRunGrant())
	s := NewRunStore(db)
	// Only code before saved sessions failed a test run; such a run was never approved.
	unapproved := seedRunIn(ctx, t, db, StatusFailed)
	if err := db.Tx(ctx, func(tx pgx.Tx) error {
		_, err := tx.Exec(ctx, `UPDATE import_run SET approved_at = NULL WHERE id = $1`, unapproved)
		return err
	}); err != nil {
		t.Fatalf("clearing the approval: %v", err)
	}
	if _, err := s.ResumeApproved(ctx, unapproved, 0, queueNothing); !errors.Is(err, apperrors.ErrConflict) {
		t.Errorf("resuming a failed run nobody approved = %v, want a conflict: it never reached a commit", err)
	}

	approved := toReport(ctx, t, s, newDraft(ctx, t, s, ""))
	running, err := s.Approve(ctx, approved.ID, approved.Version, queueNothing)
	if err != nil {
		t.Fatalf("Approve: %v", err)
	}
	if err := s.FailLease(ctx, Lease{Run: running.ID, Status: StatusRunning, Attempt: running.JobAttempt}, CauseStopped); err != nil {
		t.Fatalf("FailLease: %v", err)
	}
	resumed, err := s.ResumeApproved(ctx, approved.ID, 0, queueNothing)
	if err != nil || resumed.Status != StatusRunning || resumed.Error != "" || resumed.ApprovedAt == nil {
		t.Errorf("resuming an approved failure = (%+v, %v), want running with its approval kept", resumed, err)
	}
}

func TestAMoveWhoseJobCannotBeQueuedStaysWhereItWas(t *testing.T) {
	ctx, db := testWorkspaceCtx(t, adminImportRunGrant())
	s := NewRunStore(db)
	report := toReport(ctx, t, s, newDraft(ctx, t, s, ""))
	refused := errors.New("the queue refused the insert")
	if _, err := s.Approve(ctx, report.ID, report.Version, func(context.Context, pgx.Tx, Run) error { return refused }); !errors.Is(err, refused) {
		t.Fatalf("Approve with a refusing queue = %v, want the queue's refusal", err)
	}
	if got := mustGetStaged(ctx, t, s, report.ID); got.Status != StatusAwaitingApproval || got.JobAttempt != report.JobAttempt {
		t.Fatalf("after a refused queue the run is %s at attempt %d, want %s at %d", got.Status, got.JobAttempt, StatusAwaitingApproval, report.JobAttempt)
	}

	var queued Run
	approved, err := s.Approve(ctx, report.ID, report.Version, func(_ context.Context, _ pgx.Tx, run Run) error { queued = run; return nil })
	if err != nil {
		t.Fatalf("Approve: %v", err)
	}
	if queued.Status != StatusRunning || queued.JobAttempt != report.JobAttempt+1 || approved.JobAttempt != queued.JobAttempt {
		t.Fatalf("the queue saw %s at attempt %d and Approve answered attempt %d; want running at %d both times",
			queued.Status, queued.JobAttempt, approved.JobAttempt, report.JobAttempt+1)
	}
}

func intentKinds(ctx context.Context, t *testing.T, db *database.DB, key string) []string {
	t.Helper()
	var kinds []string
	if err := db.Tx(ctx, func(tx pgx.Tx) error {
		rows, err := tx.Query(ctx, `SELECT kind FROM stored_object_intent WHERE storage_key = $1`, key)
		if err != nil {
			return err
		}
		kinds, err = pgx.CollectRows(rows, pgx.RowTo[string])
		return err
	}); err != nil {
		t.Fatalf("reading the stored-object ledger: %v", err)
	}
	return kinds
}

func TestReleasingASourceRecordsItForTheReaper(t *testing.T) {
	ctx, db := testWorkspaceCtx(t, adminImportRunGrant())
	s := NewRunStore(db)
	key := "ws/import/" + ids.NewV7().String()
	if err := s.RecordImportSourceIntent(ctx, key); err != nil {
		t.Fatalf("RecordImportSourceIntent: %v", err)
	}
	draft, err := s.CreateDraftRun(ctx, CreateDraftRunInput{
		SourceRef: key, Source: "import_api",
		Profile: []byte(`{"files":[]}`),
	})
	if err != nil {
		t.Fatalf("CreateDraftRun: %v", err)
	}
	if kinds := intentKinds(ctx, t, db, key); len(kinds) != 0 || len(draft.Profile) == 0 {
		t.Fatalf("a draft left the ledger at %v with profile %s, want the intent cleared and the profile kept", kinds, draft.Profile)
	}
	cancelled, err := s.Cancel(ctx, draft.ID)
	if err != nil {
		t.Fatalf("Cancel: %v", err)
	}
	if cancelled.Status != StatusCancelled || !cancelled.SourceReleased || cancelled.Profile != nil {
		t.Errorf("the cancelled draft reads %+v, want cancelled, released and with no profile", cancelled)
	}
	if kinds := intentKinds(ctx, t, db, key); !slices.Equal(kinds, []string{"import"}) {
		t.Errorf("the released key is in the ledger as %v, want one import intent for the reaper", kinds)
	}
}

func TestUnreferencedImportKeysAnswersAReleasedKey(t *testing.T) {
	ctx, db := testWorkspaceCtx(t, adminImportRunGrant())
	s := NewRunStore(db)
	draft := newDraft(ctx, t, s, "")
	system := principal.SystemActing(ctx, "stored-object-sweep")

	unreferenced, err := s.UnreferencedImportKeys(system, []string{draft.SourceRef})
	if err != nil || len(unreferenced) != 0 {
		t.Fatalf("a key a draft holds = (%v, %v), want referenced", unreferenced, err)
	}
	if _, err := s.Cancel(ctx, draft.ID); err != nil {
		t.Fatalf("Cancel: %v", err)
	}
	unreferenced, err = s.UnreferencedImportKeys(system, []string{draft.SourceRef})
	if err != nil || !slices.Equal(unreferenced, []string{draft.SourceRef}) {
		t.Errorf("a key only a released run names = (%v, %v), want it unreferenced so the reaper deletes it", unreferenced, err)
	}
}
