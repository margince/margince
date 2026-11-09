// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package datatask

import (
	"context"
	"testing"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/platform/database/storekit"
)

func TestACorrectionBatchRetriesContentionWithoutDuplicatingProgress(t *testing.T) {
	ctx, pool, engine, _ := executorFixture(t)
	attempts := 0
	engine.beforeCommit = func(ctx context.Context, tx pgx.Tx) error {
		attempts++
		if attempts == 1 {
			return storekit.RetryThreadTransaction(ctx, tx)
		}
		return nil
	}
	out, err := engine.Execute(ctx)
	if err != nil || out.Busy || attempts != 2 {
		t.Fatalf("batch: out=%+v err=%v attempts=%d", out, err, attempts)
	}
	progress := progressFor(ctx, t, engine)
	if progress.Batches != 1 || progress.Scanned != 1 || progress.Changed != 1 || progress.ConsecutiveFailures != 0 {
		t.Fatalf("progress=%+v", progress)
	}
	if actions := systemActions(ctx, t, pool, "data_task.probe"); actions != 1 {
		t.Fatalf("committed batch actions=%d, want one", actions)
	}
}

func TestPersistentCorrectionContentionWaitsWithoutRecordingAFailedTask(t *testing.T) {
	ctx, pool, engine, _ := executorFixture(t)
	attempts := 0
	engine.beforeCommit = func(ctx context.Context, tx pgx.Tx) error {
		attempts++
		return storekit.RetryThreadTransaction(ctx, tx)
	}
	out, err := engine.Execute(ctx)
	if err != nil || !out.Busy || attempts != 3 {
		t.Fatalf("contention: out=%+v err=%v attempts=%d", out, err, attempts)
	}
	progress := progressFor(ctx, t, engine)
	if progress.Batches != 0 || progress.Scanned != 0 || progress.Changed != 0 || progress.ConsecutiveFailures != 0 {
		t.Fatalf("failed contention changed progress: %+v", progress)
	}
	if actions := systemActions(ctx, t, pool, "data_task.probe"); actions != 0 {
		t.Fatal("aborted batch audit survived")
	}
}
