// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

import (
	"context"
	"errors"
	"io"
	"sync/atomic"
	"time"

	"github.com/riverqueue/river"

	"github.com/margince/margince/backend/internal/platform/database"
	"github.com/margince/margince/backend/internal/platform/httpserver"
	"github.com/margince/margince/backend/internal/platform/jobs"
)

type sessionBudgetKey struct{}

var (
	pinnedSessions       atomic.Int64
	sessionBudgetSnoozes atomic.Uint64
)

// WriteJobProcessMetrics reports this process's session-lock resource pressure.
func WriteJobProcessMetrics(w io.Writer) {
	httpserver.WriteLine(w, "# HELP margince_worker_pinned_sessions Session-lock connection reservations.\n# TYPE margince_worker_pinned_sessions gauge\nmargince_worker_pinned_sessions %d\n", pinnedSessions.Load())
	httpserver.WriteLine(w, "# HELP margince_worker_session_budget_snoozes_total Jobs yielding before borrowing a session-lock connection.\n# TYPE margince_worker_session_budget_snoozes_total counter\nmargince_worker_session_budget_snoozes_total %d\n", sessionBudgetSnoozes.Load())
}

type sessionBudgetWorker[T river.JobArgs] struct {
	work        jobs.WorkOnly[T]
	slots       chan struct{}
	withContext func(context.Context) context.Context
}

func (w sessionBudgetWorker[T]) Work(ctx context.Context, job *river.Job[T]) error {
	if w.withContext != nil {
		ctx = w.withContext(ctx)
	}
	if w.slots != nil {
		ctx = context.WithValue(ctx, sessionBudgetKey{}, w.slots)
	}
	ctx = database.WithSessionBudget(ctx, takeSessionSlot)
	return jobs.FaultForKind(ctx, job.Args.Kind(), w.work.Work(ctx, job))
}

// Limit pinned connections before borrowing, so lock holders cannot exhaust the pool.
func takeSessionSlot(ctx context.Context) (func(), error) {
	slots, bounded := ctx.Value(sessionBudgetKey{}).(chan struct{})
	if !bounded {
		return func() {}, nil
	}
	select {
	case slots <- struct{}{}:
		pinnedSessions.Add(1)
		return func() { <-slots; pinnedSessions.Add(-1) }, nil
	case <-ctx.Done():
		return nil, ctx.Err()
	default:
		sessionBudgetSnoozes.Add(1)
		return nil, errors.Join(database.ErrSessionBudgetBusy, river.JobSnooze(5*time.Second))
	}
}
