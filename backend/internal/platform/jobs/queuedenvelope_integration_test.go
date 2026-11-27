// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package jobs_test

import (
	"context"
	"testing"

	"github.com/riverqueue/river"
)

// envelopeArgs is what a job carries once the workspace id has left its
// arguments.
type envelopeArgs struct {
	Note string `json:"note"`
}

func (envelopeArgs) Kind() string { return "queued_envelope" }

type envelopeWorker struct {
	river.WorkerDefaults[envelopeArgs]
	seen chan string
}

func (w *envelopeWorker) Work(_ context.Context, job *river.Job[envelopeArgs]) error {
	w.seen <- job.Args.Note
	return nil
}

// A job queued before an upgrade still carries `workspace_id`. The worker that
// no longer declares the field must run it, so an upgrade loses no queued work.
func TestAJobQueuedWithAWorkspaceIDStillRunsOnArgsThatDroppedTheField(t *testing.T) {
	worker := &envelopeWorker{seen: make(chan string, 1)}
	r, pool := migratedAppPool(t, func(w *river.Workers) { river.AddWorker(w, worker) })
	ctx := t.Context()

	if _, err := pool.Exec(ctx, `
		INSERT INTO river_job (kind, args, queue, state, max_attempts, priority)
		VALUES ('queued_envelope', '{"note": "queued before", "workspace_id": "00000000-0000-7000-8000-000000000001"}', 'default', 'available', 1, 1)`); err != nil {
		t.Fatalf("queueing the old envelope: %v", err)
	}
	if err := r.Start(ctx); err != nil {
		t.Fatalf("Start: %v", err)
	}
	defer func() {
		if err := r.Stop(ctx); err != nil {
			t.Errorf("Stop: %v", err)
		}
	}()
	waitCtx, cancel := context.WithTimeout(ctx, riverLifecycleBudget)
	defer cancel()
	select {
	case note := <-worker.seen:
		if note != "queued before" {
			t.Fatalf("the worker saw %q, want the queued job's own field", note)
		}
	case <-waitCtx.Done():
		t.Fatalf("the queued envelope with a workspace_id never ran: %v", waitCtx.Err())
	}
}
