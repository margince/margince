// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

// A commit or an undo runs after its request has answered. The response
// deadline is far shorter than the work. Approve and undo answer with the run
// as it stands, and the screen follows the run.

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"github.com/margince/margince/backend/internal/modules/migration"
)

// importCommitTimeout bounds a detached commit or undo. The work scales with
// the upload, whose cap the deployment sets, so this sits far above any cap.
// Too short turns a slow import into a half-written run. The stale-run sweep
// reads the same budget.
const importCommitTimeout = 30 * time.Minute

// importOverBudgetCause is the error a run carries when its commit or undo ran
// out of time; the engine could not record it on the expired context.
const importOverBudgetCause = "the import ran past its time budget; approve it again to continue from its checkpoint"

// RunImportInline runs a detached task where it is handed, so a test sees the
// finished run in the response that started it.
func RunImportInline(task func()) { task() }

// runDetached hands work to the import runner on a context that outlives the
// request and carries the commit budget. A panic fails the run with its cause,
// because the engine records only the failures it sees. A returned error is
// already on the run and is logged here.
func (h importHandlers) runDetached(ctx context.Context, id migration.RunID, name string, work func(context.Context) error) {
	detach := h.detach
	if detach == nil {
		detach = func(task func()) { go task() }
	}
	budget := h.budget
	if budget == 0 {
		budget = importCommitTimeout
	}
	detach(func() {
		taskCtx, release := context.WithTimeout(context.WithoutCancel(ctx), budget)
		defer release()
		defer func() {
			if recovered := recover(); recovered != nil {
				slog.ErrorContext(taskCtx, "import "+name+" panicked", "run", id, "panic", recovered)
				h.recordStop(taskCtx, id, fmt.Sprintf("the import stopped unexpectedly: %v", recovered))
			}
		}()
		if err := work(taskCtx); err != nil {
			slog.ErrorContext(taskCtx, "import "+name+" failed", "run", id, "err", err)
			if taskCtx.Err() != nil {
				h.recordStop(context.WithoutCancel(taskCtx), id, importOverBudgetCause)
			}
		}
	})
}

// recordStop fails a running run with its cause. An undoing run is left
// undoing, which undo already resumes.
func (h importHandlers) recordStop(ctx context.Context, id migration.RunID, cause string) {
	fail := h.failRunning
	if fail == nil {
		fail = migration.NewRunStore(h.db).FailRunning
	}
	if err := fail(ctx, id, cause); err != nil {
		slog.ErrorContext(ctx, "recording a stopped import", "run", id, "err", err)
	}
}
