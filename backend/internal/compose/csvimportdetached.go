// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

// A commit or an undo runs after its request has answered. The response
// deadline is far shorter than the work. Approve and undo answer with the run
// as it stands, and the screen follows the run.

import (
	"context"
	"log/slog"
	"time"

	"github.com/margince/margince/backend/internal/modules/migration"
	"github.com/margince/margince/backend/internal/platform/httpserver"
)

// importCommitTimeout bounds a detached commit or undo. The work scales with
// the upload, whose cap the deployment sets, so this sits far above any cap.
// Too short turns a slow import into a half-written run. The stale-run sweep
// reads the same budget.
const importCommitTimeout = 30 * time.Minute

// importTestRunBudget bounds a test run, which answers inside its request. It
// ends a quarter before the response deadline, so the refusal and the failed run
// are both written while the caller still waits.
const importTestRunBudget = httpserver.ResponseDeadline * 3 / 4

// importRecordStopTimeout bounds writing one failure row. The task's own budget
// may be what ran out, so the write cannot borrow it.
const importRecordStopTimeout = time.Minute

// importTagsFailedCause is the error a run carries when the tags its file names
// could not be created before its first row.
const importTagsFailedCause = "the import could not create the tags its file names; approve it again to retry"

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
				h.recordStop(taskCtx, id, migration.CauseStopped)
			}
		}()
		if err := work(taskCtx); err != nil {
			slog.ErrorContext(taskCtx, "import "+name+" failed", "run", id, "err", err)
			if taskCtx.Err() != nil {
				h.recordStop(taskCtx, id, migration.CauseOverBudget)
			}
		}
	})
}

// recordStop fails a running run with its cause. An undoing run is left
// undoing, which undo already resumes.
func (h importHandlers) recordStop(ctx context.Context, id migration.RunID, cause string) {
	ctx, release := context.WithTimeout(context.WithoutCancel(ctx), importRecordStopTimeout)
	defer release()
	fail := h.failRunning
	if fail == nil {
		fail = migration.NewRunStore(h.db).FailRunning
	}
	if err := fail(ctx, id, cause); err != nil {
		slog.ErrorContext(ctx, "recording a stopped import", "run", id, "err", err)
	}
}
