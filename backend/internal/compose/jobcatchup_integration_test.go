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
)

// The worker reads each kind's newest attempt, and a kind that ran an hour ago
// is not run again at start (#11026).
func TestReadLastRunsFeedsTheCatchUp(t *testing.T) {
	e := integration.Setup(t)
	ctx := context.Background()
	for _, age := range []string{"3 hours", "1 hour"} {
		if _, err := e.Pool.Exec(ctx, `
			INSERT INTO river_job (state, kind, queue, priority, args, tags, errors, max_attempts, attempt,
			                       created_at, scheduled_at, attempted_at, finalized_at)
			VALUES ('completed', 'catch_up_probe', 'default', 1, '{}'::jsonb, '{}'::varchar(255)[], '{}'::jsonb[], 3, 1,
			        now(), now(), now() - $1::interval, now())`, age); err != nil {
			t.Fatalf("seeding a %s old run: %v", age, err)
		}
	}

	runs, err := ReadLastRuns(ctx, e.Pool)
	if err != nil {
		t.Fatalf("ReadLastRuns: %v", err)
	}
	last, ok := runs["catch_up_probe"]
	if !ok || time.Since(last) > 61*time.Minute || time.Since(last) < 59*time.Minute {
		t.Fatalf("last run read as %s (found %v), want the newest attempt, about an hour ago", last, ok)
	}

	book := newScheduleBook(nil)
	book.CatchUpFrom(runs)
	_, s := book.periodicJob("catch_up_probe", "", river.PeriodicInterval(24*time.Hour), nil)
	now := time.Now()
	if first := s.Next(now); first.Before(last.Add(24 * time.Hour)) {
		t.Fatalf("first run at %s, before one interval after the last run at %s", first, last)
	}
}
