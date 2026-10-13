// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

import (
	"context"
	"fmt"

	"github.com/riverqueue/river"

	"github.com/margince/margince/backend/internal/platform/jobs"
)

// importUndoWorker works one slice of a run's undo.
type importUndoWorker struct {
	slicer importSlicer
}

// Work answers River for one slice: done, snoozed for the next, or stopped on
// the run, which stays undoing with its cause.
func (w *importUndoWorker) Work(ctx context.Context, job *river.Job[ImportUndoArgs]) error {
	result, err := w.slicer.slice(ctx, job.Args, undoImportSlice)
	if err != nil {
		return jobs.FaultContext(ctx, fmt.Errorf("import_undo: run %s attempt %d: %w", job.Args.RunID, job.Args.Attempt, err))
	}
	if result.stopped != nil {
		w.slicer.log.ErrorContext(ctx, "import_undo: the attempt stopped, and its run says why",
			"run", job.Args.RunID, "attempt", job.Args.Attempt, "err", result.stopped)
	}
	switch result.end {
	case importSliceContinues:
		return river.JobSnooze(0)
	case importSliceLockHeld:
		return river.JobSnooze(importLockWait)
	default:
		return nil
	}
}

// undoImportSlice reverses the run's created rows from its cursor. An undo
// reads the identity map, never the upload, so it needs no source.
func undoImportSlice(ctx context.Context, slice importSlice) (bool, error) {
	writers, err := newBundleWriters(ctx, slice.db, slice.run.ID, slice.run.Mapping, nil)
	if err != nil {
		return false, err
	}
	return slice.runs.UndoSlice(ctx, slice.lease, writers, slice.budget)
}
