// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

import (
	"context"
	"errors"
	"slices"
	"testing"

	"github.com/riverqueue/river"

	"github.com/margince/margince/backend/internal/platform/jobs"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// syncInsertRecorder is the job runner's insert seam, remembering one insert.
type syncInsertRecorder struct {
	args   river.JobArgs
	opts   *river.InsertOpts
	refuse error
}

func (r *syncInsertRecorder) Enqueue(_ context.Context, args river.JobArgs, opts *river.InsertOpts) error {
	r.args, r.opts = args, opts
	return r.refuse
}

// A press of "sync now" queues this workspace's pass on the declared queue,
// and a second press while one is queued or running joins it.
func TestQueueSyncQueuesThisWorkspacesPassOnceWhileOneIsActive(t *testing.T) {
	workspace := ids.NewV7()
	ctx := principal.WithWorkspaceID(context.Background(), workspace)
	enqueuer := &syncInsertRecorder{}

	if err := (financeSyncQueue{enqueue: enqueuer}).QueueSync(ctx); err != nil {
		t.Fatal(err)
	}
	args, ok := enqueuer.args.(FinanceSyncArgs)
	if !ok || args.Workspace != workspace {
		t.Fatalf("queued %#v, want FinanceSyncArgs for workspace %s", enqueuer.args, workspace)
	}
	spec, declared := jobs.SpecFor(FinanceSyncArgs{}.Kind())
	if !declared {
		t.Fatal("finance_sync is not declared in api/jobs.yaml")
	}
	if enqueuer.opts.Queue != spec.Queue || enqueuer.opts.MaxAttempts != oneOffJobMaxAttempts {
		t.Errorf("queued on %q with %d attempts, want %q with %d",
			enqueuer.opts.Queue, enqueuer.opts.MaxAttempts, spec.Queue, oneOffJobMaxAttempts)
	}
	unique := enqueuer.opts.UniqueOpts
	if !unique.ByArgs || !slices.Equal(unique.ByState, activeSweepStates) {
		t.Errorf("unique opts = %+v, want unique by args while queued or running", unique)
	}

	enqueuer.refuse = errors.New("the queue is unreachable")
	if err := (financeSyncQueue{enqueue: enqueuer}).QueueSync(ctx); !errors.Is(err, enqueuer.refuse) {
		t.Errorf("a refused insert answered %v, want the refusal", err)
	}
}
