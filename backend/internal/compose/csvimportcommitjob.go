// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

import (
	"context"
	"errors"
	"fmt"

	"github.com/riverqueue/river"

	"github.com/margince/margince/backend/internal/modules/migration"
	"github.com/margince/margince/backend/internal/platform/blobstore"
	"github.com/margince/margince/backend/internal/platform/jobs"
	"github.com/margince/margince/backend/internal/shared/apperrors"
)

// importTagsFailedCause is the error a run carries when the tags its file names
// could not be created before its first row.
const importTagsFailedCause = "the import could not create the tags its file names; approve it again to retry"

// importCommitWorker works one slice of an approved run's commit.
type importCommitWorker struct {
	slicer importSlicer
	// blobs holds the uploads; nil is a worker role that stores no objects.
	blobs blobstore.Store
	// uploadCap is the cap the upload applied, so the walk accepts what it accepted.
	uploadCap int64
}

// Work answers River for one slice: done, snoozed for the next, or failed on
// the run, which the run row then says.
func (w *importCommitWorker) Work(ctx context.Context, job *river.Job[ImportCommitArgs]) error {
	result, err := w.slicer.slice(ctx, job.Args, w.commit)
	if err != nil {
		return jobs.FaultContext(ctx, fmt.Errorf("import_commit: run %s attempt %d: %w", job.Args.RunID, job.Args.Attempt, err))
	}
	if result.stopped != nil {
		w.slicer.log.ErrorContext(ctx, "import_commit: the attempt stopped, and its run says why",
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

// commit builds the run's source and writers under the current principal, so
// every slice re-checks the owner and tag grants, then walks from the cursor.
func (w *importCommitWorker) commit(ctx context.Context, slice importSlice) (bool, error) {
	if w.blobs == nil {
		return false, migration.RunStop{Cause: migration.CauseStorageUnavailable, Err: errNoObjectStore}
	}
	run := slice.run
	if run.Mapping == nil {
		return false, fmt.Errorf("import run %s carries no mapping to commit: %w", run.ID, apperrors.ErrConflict)
	}
	source, _, err := bundleSourceFor(w.blobs, *run.Mapping, run.SourceRef, w.uploadCap)
	if err != nil {
		return false, err
	}
	writers, err := newBundleWriters(ctx, slice.db, run.ID, run.Mapping, source)
	if err != nil {
		return false, err
	}
	if err := writers.bindReviewPlan(run.Report); err != nil {
		return false, err
	}
	if err := writers.validateReviewSource(ctx, source); err != nil {
		return false, err
	}
	if err := createImportTags(ctx, slice, writers.tags); err != nil {
		return false, err
	}
	return migration.NewLeasedEngine(slice.runs, slice.lease, writers).RunSlice(ctx, run.ID, source, slice.budget)
}

// createImportTags creates the tags the file names before any row that
// carries them lands. It runs in the job, so an approval that was refused
// creates none. A run whose tag phase did not finish runs it again.
func createImportTags(ctx context.Context, slice importSlice, tags *tagResolver) error {
	if phase := slice.run.Phase; phase != "" && phase != migration.PhaseCreatingTags {
		return nil
	}
	if err := flushPhase(ctx, slice, migration.PhaseCreatingTags); err != nil {
		return err
	}
	if err := tags.createMissing(ctx); err != nil {
		if errors.Is(err, migration.ErrSuperseded) {
			return err
		}
		return migration.RunStop{Cause: importTagsFailedCause, Err: err}
	}
	return flushPhase(ctx, slice, migration.PhaseImporting)
}

// flushPhase records the phase the run has reached, with its cursor and
// progress as they stand.
func flushPhase(ctx context.Context, slice importSlice, phase string) error {
	progress := migration.Progress{}
	if slice.run.Progress != nil {
		progress = *slice.run.Progress
	}
	return slice.runs.Flush(ctx, slice.lease, migration.Flush{Checkpoint: slice.run.Checkpoint, Progress: progress, Phase: phase})
}
