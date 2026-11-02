// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package compose

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/modules/activities"
	"github.com/margince/margince/backend/internal/modules/capture"
	"github.com/margince/margince/backend/internal/platform/clocktest"
	"github.com/margince/margince/backend/internal/platform/database"
	"github.com/margince/margince/backend/internal/platform/datatask"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

func TestSubjectThreadTaskRunsThroughTheLimitOneExecutor(t *testing.T) {
	f := newThreadOwnerFixture(t)
	ctx := principal.SystemActing(f.Admin(), "system:thread-task-test")
	task := threadSplitDataTask(f.Pool)
	for i := range task.Steps {
		task.Steps[i].Limit = 1
	}
	now := clocktest.Now(t)
	engine, err := datatask.New(f.Pool, []datatask.Task{task}, "test", func() time.Time { return now }, func(context.Context, pgx.Tx, datatask.Progress) error { return nil })
	if err != nil {
		t.Fatal(err)
	}
	if err := engine.Seed(ctx); err != nil {
		t.Fatal(err)
	}
	for range 2000 {
		if _, err := engine.Execute(ctx); err != nil {
			t.Fatal(err)
		}
		progress, err := engine.List(ctx)
		if err != nil {
			t.Fatal(err)
		}
		done := true
		changed := int64(0)
		for _, p := range progress {
			if p.ConsecutiveFailures > 0 {
				t.Fatalf("executor rejected %s: class=%v", p.Step, p.LastErrorClass)
			}
			if p.Status != datatask.Done {
				done = false
			}
			changed += p.Changed
		}
		if done {
			if changed != 1 {
				t.Fatalf("executor changed %d messages", changed)
			}
			if got := threadKeyOfMessage(t, f.Env, f.destination); got != f.destination {
				t.Fatalf("repaired key=%q", got)
			}
			return
		}
	}
	t.Fatal("executor did not finish the persisted repair")
}

func TestSubjectThreadTaskRejectsMalformedCursorsWithoutProgress(t *testing.T) {
	f := newThreadOwnerFixture(t)
	ctx := principal.SystemActing(f.Admin(), "system:thread-task-test")
	for _, step := range threadSplitDataTask(f.Pool).Steps {
		t.Run(step.Name, func(t *testing.T) {
			for _, raw := range []json.RawMessage{json.RawMessage(`{"unexpected":true}`), json.RawMessage(`"not-a-cursor"`), json.RawMessage(`-1`)} {
				err := database.WithWorkspaceTx(ctx, f.Pool, func(tx pgx.Tx) error {
					_, err := step.Batch(ctx, tx, datatask.BatchIn{Cursor: raw, Limit: 1, RunID: ids.NewV7()})
					return err
				})
				if err == nil {
					t.Fatalf("accepted malformed cursor %s", raw)
				}
			}
		})
	}
}

func TestSubjectThreadTaskRollsBackItsPersistedBatch(t *testing.T) {
	f := newThreadOwnerFixture(t)
	ctx := principal.SystemActing(f.Admin(), "system:thread-task-test")
	step := threadSplitDataTask(f.Pool).Steps[0]
	interrupted := errors.New("interrupted before batch commit")
	var cursor json.RawMessage
	if err := database.WithWorkspaceTx(ctx, f.Pool, func(tx pgx.Tx) error {
		out, err := step.Batch(ctx, tx, datatask.BatchIn{Limit: 1, RunID: ids.NewV7()})
		if err != nil {
			return err
		}
		cursor = out.Cursor
		return interrupted
	}); !errors.Is(err, interrupted) {
		t.Fatalf("rollback result: %v", err)
	}
	if len(cursor) == 0 {
		t.Fatal("batch did not advance its candidate cursor")
	}
	if got := threadKeyOfMessage(t, f.Env, f.destination); got != f.source {
		t.Fatalf("rollback changed grouping to %q", got)
	}
	// Live capture can already have queued the source; rollback must preserve that queue.
	if err := database.WithWorkspaceTx(ctx, f.Pool, func(tx pgx.Tx) error {
		out, err := step.Batch(ctx, tx, datatask.BatchIn{Limit: 1, RunID: ids.NewV7()})
		if err != nil {
			return err
		}
		if string(out.Cursor) != string(cursor) {
			t.Fatalf("rollback changed next cursor: %s -> %s", cursor, out.Cursor)
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
}

func TestSubjectThreadRepairResumesAGiantConversationInBoundedBatches(t *testing.T) {
	f := newThreadOwnerFixture(t)
	addr := seatAddress(t, f.Env, f.Rep1)
	previous := f.destination
	extra := 101
	for i := range extra {
		key := fmt.Sprintf("large-%03d@counterparty.example", i)
		captureThreadMail(t, f.Env, f.Rep1, subjectThreadMail(addr, key, "Logo", ""))
		id := capturedThreadActivity(t, f.Env, key)
		oldKey := threadKeyOfMessage(t, f.Env, key)
		if err := database.WithWorkspaceTx(f.Admin(), f.Pool, func(tx pgx.Tx) error {
			if err := activities.RecordMailReferencesTx(f.Admin(), tx, id, []string{previous}); err != nil {
				return err
			}
			return mergeThreadsTx(f.Admin(), tx, oldKey, f.source)
		}); err != nil {
			t.Fatal(err)
		}
		previous = key
	}
	if err := database.WithWorkspaceTx(f.Admin(), f.Pool, func(tx pgx.Tx) error { return capture.QueueThreadRepairTx(f.Admin(), tx, f.source) }); err != nil {
		t.Fatal(err)
	}
	moved, run := 0, ids.NewV7()
	for range 5000 {
		done := false
		if err := database.WithWorkspaceTx(f.Admin(), f.Pool, func(tx pgx.Tx) error {
			scanned, changed, finished, err := capture.ThreadRepairBatchTx(f.Admin(), tx, 7, run, threadRepairApplier(f.Pool))
			if scanned > 7 {
				t.Fatalf("bounded batch scanned %d units", scanned)
			}
			moved += changed
			done = finished
			return err
		}); err != nil {
			t.Fatal(err)
		}
		if !done {
			continue
		}
		if moved != extra+1 {
			t.Fatalf("moved %d messages, want %d", moved, extra+1)
		}
		for _, key := range []string{f.destination, previous} {
			if got := threadKeyOfMessage(t, f.Env, key); got != f.destination {
				t.Fatalf("%s grouped as %q", key, got)
			}
		}
		return
	}
	t.Fatal("giant repair failed to resume to completion")
}
