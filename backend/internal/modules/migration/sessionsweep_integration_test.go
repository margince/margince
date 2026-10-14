// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package migration

import (
	"context"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/platform/database"
)

// ageRun makes the run's last write as old as age and sets its error.
func ageRun(ctx context.Context, t *testing.T, db *database.DB, id RunID, age time.Duration, cause string) {
	t.Helper()
	if err := db.Tx(ctx, func(tx pgx.Tx) error {
		_, err := tx.Exec(ctx, `
			UPDATE import_run SET updated_at = now() - make_interval(secs => $2), error = nullif($3, '')
			 WHERE id = $1`, id, age.Seconds(), cause)
		return err
	}); err != nil {
		t.Fatalf("ageing run %s: %v", id, err)
	}
}

// sweptRun is what one rule must leave a run of a given state as.
type sweptRun struct {
	status, cause string
	released      bool
}

func TestEachSweepRuleSettlesOnlyTheRunsItNames(t *testing.T) {
	ctx, db := testWorkspaceCtx(t, adminImportRunGrant())
	s := NewRunStore(db)
	const week = 7 * 24 * time.Hour
	rules := map[string]func() (int, error){
		"stopped": func() (int, error) { return s.CancelStoppedTestRuns(ctx) },
		"expire":  func() (int, error) { return s.CancelIdleDrafts(ctx, week) },
		"release": func() (int, error) { return s.ReleaseFinishedSources(ctx) },
	}
	for _, tc := range []struct {
		status, cause string
		age           time.Duration
		rule          string
		want          sweptRun
	}{
		{StatusFailed, CauseValidationStopped, time.Hour, "stopped", sweptRun{StatusCancelled, CauseValidationStopped, true}},
		{StatusPending, "", week + time.Hour, "expire", sweptRun{StatusCancelled, CauseDraftExpired, true}},
		{StatusAwaitingApproval, "", week + time.Hour, "expire", sweptRun{StatusCancelled, CauseDraftExpired, true}},
		{StatusComplete, "", 0, "release", sweptRun{StatusComplete, "", true}},
		{StatusUndone, "", 0, "release", sweptRun{StatusUndone, "", true}},
		{StatusValidating, "", 30 * time.Second, "", sweptRun{StatusValidating, "", false}},
		{StatusPending, "", week - time.Hour, "", sweptRun{StatusPending, "", false}},
		{StatusFailed, CauseStopped, week, "", sweptRun{StatusFailed, CauseStopped, false}},
		{StatusRunning, "", week, "", sweptRun{StatusRunning, "", false}},
		{StatusUndoing, "", week, "", sweptRun{StatusUndoing, "", false}},
	} {
		id := seedRunIn(ctx, t, db, tc.status)
		ageRun(ctx, t, db, id, tc.age, tc.cause)
		for name, rule := range rules {
			want := 0
			if name == tc.rule {
				want = 1
			}
			if n, err := rule(); n != want || err != nil {
				t.Errorf("%s over a %s run aged %s = (%d, %v), want %d settled", name, tc.status, tc.age, n, err, want)
			}
		}
		got := mustGetStaged(ctx, t, s, id)
		if (sweptRun{got.Status, got.Error, got.SourceReleased}) != tc.want {
			t.Errorf("a %s run aged %s reads (%s, %q, released %v), want %+v",
				tc.status, tc.age, got.Status, got.Error, got.SourceReleased, tc.want)
		}
	}
}

func TestOneSweepPassSettlesAtMostOneBatch(t *testing.T) {
	ctx, db := testWorkspaceCtx(t, adminImportRunGrant())
	s := NewRunStore(db)
	for range sweepBatch + 1 {
		seedRunIn(ctx, t, db, StatusComplete)
	}
	for pass, want := range []int{sweepBatch, 1, 0} {
		if n, err := s.ReleaseFinishedSources(ctx); n != want || err != nil {
			t.Fatalf("pass %d released (%d, %v), want %d", pass+1, n, err, want)
		}
	}
}
