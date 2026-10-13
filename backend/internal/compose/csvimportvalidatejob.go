// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

// A test run is one attempt in the worker. It reads the whole file in one
// snapshot, because the report describes one moment of the estate, so it is not
// sliced. Its heartbeat and progress go on their own connection beside the
// snapshot, where a reader polling the run sees them.

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"time"

	"github.com/riverqueue/river"

	"github.com/margince/margince/backend/internal/modules/migration"
	"github.com/margince/margince/backend/internal/platform/blobstore"
	"github.com/margince/margince/backend/internal/platform/httperr"
	"github.com/margince/margince/backend/internal/platform/jobs"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// ImportValidateArgs names one attempt at testing a run.
type ImportValidateArgs struct {
	Workspace ids.UUID `json:"workspace_id"`
	RunID     ids.UUID `json:"run_id"`
	Attempt   int      `json:"attempt"`
}

// Kind is the stable job identifier River persists in river_job.
func (ImportValidateArgs) Kind() string { return "import_validate" }

// WorkspaceID binds the test run to its tenant (jobs.WorkspaceScoped).
func (a ImportValidateArgs) WorkspaceID() ids.UUID { return a.Workspace }

func (a ImportValidateArgs) lease() migration.Lease {
	return migration.Lease{Run: a.RunID, Status: migration.StatusValidating, Attempt: a.Attempt}
}

func (ImportValidateArgs) insertOpts() *river.InsertOpts {
	return jobs.QueuedAs[ImportValidateArgs](importInsertOpts())
}

// importValidateWorker works one attempt at a run's test run.
type importValidateWorker struct {
	slicer importSlicer
	// blobs holds the uploads; nil is a worker role that stores no objects.
	blobs blobstore.Store
	// uploadCap is the cap the upload applied, so the read accepts what it accepted.
	uploadCap int64
}

// Work answers River for one attempt: done, stopped on the run, which then says
// why, or snoozed by a stopping worker. The next claim then tests from the top.
func (w *importValidateWorker) Work(ctx context.Context, job *river.Job[ImportValidateArgs]) error {
	result, err := w.slicer.slice(ctx, job.Args, w.validate)
	if err != nil {
		return jobs.FaultContext(ctx, fmt.Errorf("import_validate: run %s attempt %d: %w", job.Args.RunID, job.Args.Attempt, err))
	}
	if result.stopped != nil {
		w.slicer.log.ErrorContext(ctx, "import_validate: the test run stopped, and its run says why",
			"run", job.Args.RunID, "attempt", job.Args.Attempt, "err", result.stopped)
	}
	if result.end == importSliceContinues {
		return river.JobSnooze(0)
	}
	return nil
}

// validate tests the claimed run and parks it with its report. A heartbeat
// that finds the attempt moved on ends the read within one beat.
func (w *importValidateWorker) validate(ctx context.Context, slice importSlice) (bool, error) {
	if w.blobs == nil {
		return false, migration.RunStop{Cause: migration.CauseTestStorageUnavailable, Err: errNoObjectStore}
	}
	run := slice.run
	if run.Mapping == nil {
		return false, fmt.Errorf("import run %s carries no mapping to test: %w", run.ID, apperrors.ErrConflict)
	}
	source, skipped, err := bundleSourceFor(w.blobs, *run.Mapping, run.SourceRef, w.uploadCap)
	if err != nil {
		return false, testRunRefusal(err)
	}
	progress, err := watchTestRun(ctx, slice, source)
	if err != nil {
		return false, testRunRefusal(err)
	}
	reading, stop := context.WithCancelCause(ctx)
	beating := w.heartbeat(reading, stop, slice)
	report, err := validateParts(reading, slice.db, run.ID, source, skipped, run.Mapping, progress)
	stop(nil)
	<-beating
	if lost := context.Cause(reading); errors.Is(lost, migration.ErrSuperseded) {
		return false, lost
	}
	if err != nil {
		return false, testRunRefusal(err)
	}
	return true, slice.runs.FinishValidation(ctx, slice.lease, report)
}

// heartbeat beats until ctx ends, and stops the read with ErrSuperseded once
// the attempt has moved on. The channel closes when it has returned.
func (w *importValidateWorker) heartbeat(ctx context.Context, stop context.CancelCauseFunc, slice importSlice) <-chan struct{} {
	done := make(chan struct{})
	go func() {
		defer close(done)
		tick := time.NewTicker(importHeartbeatEvery)
		defer tick.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-tick.C:
			}
			err := slice.runs.Heartbeat(ctx, slice.lease)
			switch {
			case errors.Is(err, migration.ErrSuperseded):
				stop(err)
				return
			case err != nil && ctx.Err() == nil:
				w.slicer.log.WarnContext(ctx, "import_validate: a heartbeat was not written", "run", slice.lease.Run, "err", err)
			}
		}
	}()
	return done
}

// testRunRefusal keeps a refusal the importer can act on, such as a row the
// file cannot hold, as the sentence its draft carries. Any other error may hold
// a driver message, so migration.StopCause words it instead.
func testRunRefusal(err error) error {
	for _, own := range []error{apperrors.ErrPermissionDenied, migration.ErrSuperseded, context.Canceled, context.DeadlineExceeded} {
		if errors.Is(err, own) {
			return err
		}
	}
	fault, ok := httperr.Classify(importProblem(err))
	if !ok || fault.Status >= http.StatusInternalServerError {
		return err
	}
	return migration.RunStop{Cause: fault.Detail, Err: err}
}

// testRunProgress records how far a test run has matched its rows against the
// estate, on its own connection, every importFlushEvery or importFlushRows.
type testRunProgress struct {
	runs   *migration.RunStore
	lease  migration.Lease
	budget migration.Budget
	// objects is the source's order, and totals each object's rows.
	objects []string
	totals  map[string]int
	phase   string
	object  string
	done    map[string]int
	// sinceFlush counts rows since the last write, which flushedAt dates.
	sinceFlush int
	flushedAt  time.Time
}

// watchTestRun counts the source's rows and hears every page of every part,
// so the progress moves while one part is still being read.
func watchTestRun(ctx context.Context, slice importSlice, source migration.Source) (*testRunProgress, error) {
	totals, err := source.Counts(ctx)
	if err != nil {
		return nil, err
	}
	p := &testRunProgress{
		runs: slice.runs, lease: slice.lease, budget: slice.budget,
		objects: source.Objects(), totals: totals, done: map[string]int{},
	}
	for _, object := range p.objects {
		part, err := partSourceOf(source, object)
		if err != nil {
			return nil, err
		}
		part.ObserveRows(p.read)
	}
	return p, nil
}

// enter starts a pass over the rows and writes that it has.
func (p *testRunProgress) enter(ctx context.Context, phase string) error {
	p.phase, p.object, p.done = phase, "", map[string]int{}
	return p.flush(ctx)
}

// read hears a page end. Reads before the first pass, which gather owners and
// tags, are not matched against the estate and move nothing.
func (p *testRunProgress) read(ctx context.Context, object string, delivered int) error {
	moved := delivered - p.done[object]
	if p.phase == "" || moved <= 0 {
		return nil
	}
	p.done[object], p.object = delivered, object
	p.sinceFlush += moved
	if p.sinceFlush < p.budget.FlushRows && p.now().Sub(p.flushedAt) < p.budget.FlushEvery {
		return nil
	}
	return p.flush(ctx)
}

func (p *testRunProgress) flush(ctx context.Context) error {
	progress := migration.Progress{Attempt: p.lease.Attempt}
	for _, object := range p.objects {
		total := p.totals[object]
		progress.Objects = append(progress.Objects, migration.ObjectProgress{
			Object: object, RowsDone: min(p.done[object], total), RowsTotal: total,
		})
	}
	p.sinceFlush, p.flushedAt = 0, p.now()
	return p.runs.FlushTestProgress(ctx, p.lease, progress, p.phase, p.object)
}

func (p *testRunProgress) now() time.Time {
	if p.budget.Clock == nil {
		return time.Now()
	}
	return p.budget.Clock()
}
