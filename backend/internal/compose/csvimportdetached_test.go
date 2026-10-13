// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/margince/margince/backend/internal/modules/migration"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

func TestApproveAnswersBeforeTheCommitHasRun(t *testing.T) {
	var held func()
	h := importHandlers{detach: func(task func()) { held = task }}
	ran := false
	h.runDetached(context.Background(), migration.RunID{}, "commit", func(context.Context) error { ran = true; return nil })
	if ran || held == nil {
		t.Fatal("the work ran inside the request; it must wait for the runner")
	}
	held()
	if !ran {
		t.Fatal("the runner never ran the work")
	}
}

func TestADetachedPanicFailsTheRunWithItsCause(t *testing.T) {
	var failed migration.RunID
	var cause string
	h := importHandlers{detach: RunImportInline, failRunning: func(_ context.Context, id migration.RunID, why string) error {
		failed, cause = id, why
		return nil
	}}
	id := migration.RunID(ids.NewV7())
	h.runDetached(context.Background(), id, "commit", func(context.Context) error { panic("boom") })
	if failed != id || cause != migration.CauseStopped {
		t.Fatalf("failed %v with %q; want run %v failed with %q, the panic's text kept to the log", failed, cause, id, migration.CauseStopped)
	}
}

func TestADetachedTaskOutlivesItsRequest(t *testing.T) {
	request, cancel := context.WithCancel(context.Background())
	var held func()
	h := importHandlers{detach: func(task func()) { held = task }}
	var seen error
	h.runDetached(request, migration.RunID{}, "undo", func(ctx context.Context) error {
		seen = ctx.Err()
		if _, bounded := ctx.Deadline(); !bounded {
			t.Error("the detached work carries no deadline; it must be bounded by the commit budget")
		}
		return nil
	})
	cancel()
	held()
	if seen != nil {
		t.Fatalf("the work saw %v after its request ended; it must run on", seen)
	}
}

func TestACommitPastItsBudgetRecordsThatCauseOnALiveContext(t *testing.T) {
	var cause string
	var recordedOn error
	h := importHandlers{
		detach: RunImportInline, budget: time.Nanosecond,
		failRunning: func(ctx context.Context, _ migration.RunID, why string) error {
			cause, recordedOn = why, ctx.Err()
			return nil
		},
	}
	h.runDetached(context.Background(), migration.RunID(ids.NewV7()), "commit", func(ctx context.Context) error {
		<-ctx.Done()
		return ctx.Err()
	})
	if cause != migration.CauseOverBudget {
		t.Fatalf("cause = %q, want %q", cause, migration.CauseOverBudget)
	}
	if recordedOn != nil {
		t.Fatalf("the failure was recorded on a context already done (%v); the write would not land", recordedOn)
	}
}

func TestACommitThatFailsWithinItsBudgetIsLeftToTheEngine(t *testing.T) {
	recorded := false
	h := importHandlers{detach: RunImportInline, failRunning: func(context.Context, migration.RunID, string) error {
		recorded = true
		return nil
	}}
	h.runDetached(context.Background(), migration.RunID(ids.NewV7()), "commit", func(context.Context) error {
		return errors.New("a row refused")
	})
	if recorded {
		t.Fatal("a failure the engine saw was recorded a second time over its own cause")
	}
}
