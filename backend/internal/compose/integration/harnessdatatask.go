// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package integration

import (
	"context"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/margince/margince/backend/internal/platform/clocktest"
	"github.com/margince/margince/backend/internal/platform/datatask"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// RunDataTask seeds one data task and runs its batches on the test clock. The
// cap on runs ends a task that never reports done instead of hanging the test.
func (e *Env) RunDataTask(ctx context.Context, t *testing.T, task datatask.Task) {
	t.Helper()
	runDataTask(ctx, t, e.Pool, task)
}

// RunDataTask is Env.RunDataTask for the search fixture's database.
func (e *SearchEnv) RunDataTask(ctx context.Context, t *testing.T, task datatask.Task) {
	t.Helper()
	runDataTask(ctx, t, e.Pool, task)
}

func runDataTask(ctx context.Context, t *testing.T, pool *pgxpool.Pool, task datatask.Task) {
	t.Helper()
	now := clocktest.Now(t)
	engine, err := datatask.New(pool, []datatask.Task{task}, "test", func() time.Time { return now },
		func(context.Context, pgx.Tx, datatask.Progress) error { return nil })
	if err != nil {
		t.Fatal(err)
	}
	if err := engine.Seed(ctx); err != nil {
		t.Fatal(err)
	}
	for range 40 {
		if _, err := engine.Execute(ctx); err != nil {
			t.Fatal(err)
		}
	}
}

// RestrictActivity seeds a retention hold with the evidence the database requires.
func (e *Env) RestrictActivity(t *testing.T, activity ids.UUID) {
	t.Helper()
	e.WsExec(t, `INSERT INTO activity_retention_evidence (activity_id, basis, qualified_at, decided_by_name, reason)
 VALUES ($1, 'controller_pin', now(), 'Datenschutz', 'litigation hold')`, activity)
	e.WsExec(t, `UPDATE activity SET restricted_at = now(), archived_at = coalesce(archived_at, now()), restricted_reason = 'litigation hold',
 restricted_until = now() + interval '1 year', retention_class = 'commercial_correspondence', retention_class_at = now() WHERE id = $1`, activity)
}
