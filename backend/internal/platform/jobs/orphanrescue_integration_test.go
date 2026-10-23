// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package jobs_test

// A job whose worker was killed keeps state=running with nothing to end it.
// A re-enqueue of the same work dedupes against that row. How long it lives is
// how long a person waits for work that is not happening. The job's own
// declared timeout decides that, not a longer global default.

import (
	"context"
	"testing"
	"time"

	"github.com/riverqueue/river"

	"github.com/margince/margince/backend/internal/platform/jobs"
)

type orphanArgs struct{}

func (orphanArgs) Kind() string { return "orphan_rescue" }

type orphanWorker struct{ ran chan struct{} }

func (w *orphanWorker) Work(context.Context, *river.Job[orphanArgs]) error {
	close(w.ran)
	return nil
}

func TestAJobOrphanedPastItsOwnTimeoutIsRescuedAndWorked(t *testing.T) {
	worker := &orphanWorker{ran: make(chan struct{})}
	declared := jobs.Spec{Timeout: jobs.TimeoutPolicy{Fixed: 2 * time.Second}}
	_, pool := migratedAppPool(t)
	ctx := t.Context()

	workers := river.NewWorkers()
	river.AddWorker(workers, jobs.Govern[orphanArgs](worker, declared))
	runner, err := jobs.New(pool, jobs.Config{
		Queues:   map[string]river.QueueConfig{river.QueueDefault: {MaxWorkers: 1}},
		Workers:  workers,
		TestOnly: true,
	}, quietLogger())
	if err != nil {
		t.Fatalf("jobs.New: %v", err)
	}

	// Inserted on a queue nobody serves, then made to look like a claim by a
	// worker that is gone. It is running, attempted well past the job's declared
	// timeout, by a client id that is not this one.
	if err := runner.Enqueue(ctx, orphanArgs{}, &river.InsertOpts{Queue: "unserved", MaxAttempts: 3}); err != nil {
		t.Fatalf("Enqueue: %v", err)
	}
	if _, err := pool.Exec(ctx, `
		UPDATE river_job
		SET state = 'running', queue = 'default', attempt = 1,
		    attempted_at = now() - interval '5 minutes', attempted_by = ARRAY['a_worker_that_was_killed']
		WHERE kind = $1`, orphanArgs{}.Kind()); err != nil {
		t.Fatalf("orphaning the job: %v", err)
	}

	if err := runner.Start(ctx); err != nil {
		t.Fatalf("Start: %v", err)
	}
	defer func() {
		if err := runner.Stop(context.WithoutCancel(ctx)); err != nil {
			t.Errorf("Stop: %v", err)
		}
	}()

	select {
	case <-worker.ran:
	case <-time.After(riverLifecycleBudget):
		t.Fatal("a job running five minutes past its two-second timeout was never rescued, " +
			"so a re-enqueue of its work would dedupe against it for as long as the global default holds")
	}
}
