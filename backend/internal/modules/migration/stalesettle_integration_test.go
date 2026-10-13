// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package migration

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/platform/database"
)

// The sweep read the attempt as stale, then its slice beat before the settle.
// Past the resume limit the run is left running, not failed under the slice.
func TestARunPastTheLimitWhoseSliceBeatSinceTheReadStaysRunning(t *testing.T) {
	ctx, db := testWorkspaceCtx(t, adminImportRunGrant())
	s := NewRunStore(db)
	l := approvedRun(ctx, t, s)
	if _, err := s.Claim(ctx, l); err != nil {
		t.Fatalf("Claim: %v", err)
	}
	requeue := func(context.Context, pgx.Tx, Run) (bool, error) {
		t.Fatal("a run past the limit is never queued again")
		return false, nil
	}

	err := s.settleStale(ctx, dueAttempt{lease: l, resumes: 3}, time.Minute, 3, requeue)
	if !errors.Is(err, ErrSuperseded) {
		t.Fatalf("settling a run whose slice beat since the read = %v, want ErrSuperseded", err)
	}
	if got := mustGetStaged(ctx, t, s, l.Run); got.Status != StatusRunning || got.Error != "" {
		t.Errorf("the run reads %s with %q, want it running as its slice left it", got.Status, got.Error)
	}
}

// Approving, approving again and undoing each start a new budget: the sweep
// counts the resumes of the attempt they open from zero.
func TestAMoveThatQueuesWorkCountsItsResumesFromZero(t *testing.T) {
	for name, move := range map[string]func(context.Context, *testing.T, *RunStore, *database.DB) RunID{
		"an approval": func(ctx context.Context, t *testing.T, s *RunStore, db *database.DB) RunID {
			run := toReport(ctx, t, s, newDraft(ctx, t, s, ""))
			setRunColumn(ctx, t, db, run.ID, `resume_count = 3`)
			if _, err := s.Approve(ctx, run.ID, run.Version, queueNothing); err != nil {
				t.Fatalf("Approve: %v", err)
			}
			return run.ID
		},
		"an approval again": func(ctx context.Context, t *testing.T, s *RunStore, db *database.DB) RunID {
			l := approvedRun(ctx, t, s)
			setRunColumn(ctx, t, db, l.Run, `resume_count = 3`)
			if err := s.FailLease(ctx, l, CauseStale); err != nil {
				t.Fatalf("FailLease: %v", err)
			}
			if _, err := s.ResumeApproved(ctx, l.Run, 0, queueNothing); err != nil {
				t.Fatalf("ResumeApproved: %v", err)
			}
			return l.Run
		},
		"an undo": func(ctx context.Context, t *testing.T, s *RunStore, db *database.DB) RunID {
			run := completeCSVRun(ctx, t, s)
			setRunColumn(ctx, t, db, run.ID, `resume_count = 3`)
			if _, err := s.ClaimUndo(ctx, run.ID, queueNothing); err != nil {
				t.Fatalf("ClaimUndo: %v", err)
			}
			return run.ID
		},
	} {
		t.Run(name, func(t *testing.T) {
			ctx, db := testWorkspaceCtx(t, adminImportRunGrant())
			id := move(ctx, t, NewRunStore(db), db)
			var resumes int
			if err := db.Tx(ctx, func(tx pgx.Tx) error {
				return tx.QueryRow(ctx, `SELECT resume_count FROM import_run WHERE id = $1`, id).Scan(&resumes)
			}); err != nil {
				t.Fatalf("reading the run's resumes: %v", err)
			}
			if resumes != 0 {
				t.Errorf("the new attempt starts with %d resumes, want 0", resumes)
			}
		})
	}
}
