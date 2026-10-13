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
