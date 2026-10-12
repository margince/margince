// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

import (
	"context"
	"strings"
	"testing"

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
	if failed != id || !strings.Contains(cause, "boom") {
		t.Fatalf("failed %v with %q; want run %v failed naming the panic", failed, cause, id)
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
