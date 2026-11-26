// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package compose

import (
	"context"
	"testing"
	"time"

	"github.com/riverqueue/river"

	"github.com/margince/margince/backend/internal/compose/integration"
	"github.com/margince/margince/backend/internal/platform/clocktest"
)

// The worker reads each kind's newest attempt, and a kind that ran an hour ago
// is not run again at start (#11026).
func TestReadLastRunsFeedsTheCatchUp(t *testing.T) {
	e := integration.Setup(t)
	ctx := context.Background()
	now := clocktest.Now(t)
	newest := now.Add(-time.Hour)
	for _, at := range []time.Time{now.Add(-3 * time.Hour), newest} {
		if _, err := e.Pool.Exec(ctx, `
			INSERT INTO river_job (state, kind, queue, priority, args, tags, errors, max_attempts, attempt,
			                       created_at, scheduled_at, attempted_at, finalized_at)
			VALUES ('completed', 'catch_up_probe', 'default', 1, '{}'::jsonb, '{}'::varchar(255)[], '{}'::jsonb[], 3, 1,
			        $1, $1, $1, $1)`, at); err != nil {
			t.Fatalf("seeding a run at %s: %v", at, err)
		}
	}

	runs, err := ReadLastRuns(ctx, e.Pool)
	if err != nil {
		t.Fatalf("ReadLastRuns: %v", err)
	}
	if last := runs["catch_up_probe"]; !last.Equal(newest) {
		t.Fatalf("last run read as %s, want the newest attempt %s", last, newest)
	}

	book := newScheduleBook(nil)
	book.CatchUpFrom(runs)
	_, s := book.periodicJob("catch_up_probe", "", river.PeriodicInterval(24*time.Hour), nil)
	if first := s.Next(now); first.Before(newest.Add(24 * time.Hour)) {
		t.Fatalf("first run at %s, before one interval after the last run at %s", first, newest)
	}
}
