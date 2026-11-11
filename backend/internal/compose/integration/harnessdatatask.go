// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package integration

import (
	"context"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/platform/clocktest"
	"github.com/margince/margince/backend/internal/platform/datatask"
)

// RunDataTask seeds one data task and runs its batches on the test clock. The
// cap on runs ends a task that never reports done instead of hanging the test.
func (e *Env) RunDataTask(ctx context.Context, t *testing.T, task datatask.Task) {
	t.Helper()
	now := clocktest.Now(t)
	engine, err := datatask.New(e.Pool, []datatask.Task{task}, "test", func() time.Time { return now },
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
