// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

// An import's commit and undo run in the worker as slices of one attempt, and
// its test run as one unsliced attempt. The run row is the fence and the
// checkpoint. Every slice claims the attempt its job names, works until its
// budget is spent, and snoozes for the next one.

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/riverqueue/river"

	"github.com/margince/margince/backend/internal/modules/identity"
	"github.com/margince/margince/backend/internal/modules/migration"
	"github.com/margince/margince/backend/internal/platform/database"
	"github.com/margince/margince/backend/internal/platform/database/storekit"
	"github.com/margince/margince/backend/internal/platform/jobs"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// importWorkerActor records what the worker itself writes: reading the run
// before its actor is rebuilt, and the cause of an attempt that stopped.
const importWorkerActor = "system:import-worker"

// ImportCommitArgs names one attempt at committing a run.
type ImportCommitArgs struct {
	Workspace ids.UUID `json:"workspace_id"`
	RunID     ids.UUID `json:"run_id"`
	Attempt   int      `json:"attempt"`
}

// Kind is the stable job identifier River persists in river_job.
func (ImportCommitArgs) Kind() string { return "import_commit" }

// WorkspaceID binds the slice to its tenant (jobs.WorkspaceScoped).
func (a ImportCommitArgs) WorkspaceID() ids.UUID { return a.Workspace }

func (a ImportCommitArgs) lease() migration.Lease {
	return migration.Lease{Run: a.RunID, Status: migration.StatusRunning, Attempt: a.Attempt}
}

func (ImportCommitArgs) insertOpts() *river.InsertOpts {
	return jobs.QueuedAs[ImportCommitArgs](importInsertOpts())
}

// ImportUndoArgs names one attempt at undoing a run.
type ImportUndoArgs struct {
	Workspace ids.UUID `json:"workspace_id"`
	RunID     ids.UUID `json:"run_id"`
	Attempt   int      `json:"attempt"`
}

// Kind is the stable job identifier River persists in river_job.
func (ImportUndoArgs) Kind() string { return "import_undo" }

// WorkspaceID binds the slice to its tenant (jobs.WorkspaceScoped).
func (a ImportUndoArgs) WorkspaceID() ids.UUID { return a.Workspace }

func (a ImportUndoArgs) lease() migration.Lease {
	return migration.Lease{Run: a.RunID, Status: migration.StatusUndoing, Attempt: a.Attempt}
}

func (ImportUndoArgs) insertOpts() *river.InsertOpts {
	return jobs.QueuedAs[ImportUndoArgs](importInsertOpts())
}

// importJobArgs is an attempt's job: test, commit or undo.
type importJobArgs interface {
	jobs.WorkspaceScoped
	lease() migration.Lease
	insertOpts() *river.InsertOpts
}

// importInsertOpts dedupes on every arg. A new attempt is a new job, and the
// same attempt queued twice is one; the run row, not River, retries.
func importInsertOpts() *river.InsertOpts {
	return &river.InsertOpts{
		MaxAttempts: rowOwnedMaxAttempts,
		UniqueOpts:  river.UniqueOpts{ByArgs: true, ByState: activeSweepStates},
	}
}

// importEnqueuer queues an attempt's job in the transaction that moved its run.
type importEnqueuer struct {
	runner *jobs.Runner
}

// enqueue inserts the job of a fresh attempt. A dedupe here means two moves
// claimed one attempt number, which the fence makes impossible.
func (e importEnqueuer) enqueue(ctx context.Context, tx pgx.Tx, args importJobArgs) error {
	inserted, err := e.runner.EnqueueTxUnique(ctx, tx, args, args.insertOpts())
	if err != nil {
		return err
	}
	if !inserted {
		lease := args.lease()
		return fmt.Errorf("%s: attempt %d of import run %s was already queued", args.Kind(), lease.Attempt, lease.Run)
	}
	return nil
}

// validates queues the test-run job of the attempt a move into validating opened.
func (e *importEnqueuer) validates(ws ids.UUID) migration.Enqueue {
	return func(ctx context.Context, tx pgx.Tx, run migration.Run) error {
		return e.enqueue(ctx, tx, ImportValidateArgs{Workspace: ws, RunID: run.ID, Attempt: run.JobAttempt})
	}
}

// commits queues the commit job of the attempt a move into running opened.
func (e *importEnqueuer) commits(ws ids.UUID) migration.Enqueue {
	return func(ctx context.Context, tx pgx.Tx, run migration.Run) error {
		return e.enqueue(ctx, tx, ImportCommitArgs{Workspace: ws, RunID: run.ID, Attempt: run.JobAttempt})
	}
}

// undos queues the undo job of the attempt a move into undoing opened.
func (e *importEnqueuer) undos(ws ids.UUID) migration.Enqueue {
	return func(ctx context.Context, tx pgx.Tx, run migration.Run) error {
		return e.enqueue(ctx, tx, ImportUndoArgs{Workspace: ws, RunID: run.ID, Attempt: run.JobAttempt})
	}
}

// importSliceEnd is how one slice ended, which Work turns into River's answer.
type importSliceEnd int

const (
	// importSliceSettled: the run row says what happened, done, failed or
	// taken over by a newer attempt.
	importSliceSettled importSliceEnd = iota
	// importSliceContinues: rows are left, so the next slice starts at once.
	importSliceContinues
	// importSliceLockHeld: another slice holds the run's write lock.
	importSliceLockHeld
)

// importSliceResult is how a slice ended and, when it failed the attempt, the
// error that did. The run carries a sentence for it; the error goes to the log.
type importSliceResult struct {
	end     importSliceEnd
	stopped error
}

// importSlice is one claimed slice, handed to the part commit and undo differ in.
type importSlice struct {
	db     *database.DB
	runs   *migration.RunStore
	lease  migration.Lease
	run    migration.Run
	budget migration.Budget
}

// importSliceWork works one claimed slice and answers whether the run is done.
type importSliceWork func(ctx context.Context, slice importSlice) (bool, error)

// importSlicer is what a commit slice and an undo slice share. That is the
// actor, the run's write lock, the claim, and the record of a stopped slice.
type importSlicer struct {
	pool   *pgxpool.Pool
	actors storedActor
	log    *slog.Logger
	now    func() time.Time
}

func newImportSlicer(pool *pgxpool.Pool, log *slog.Logger) importSlicer {
	return importSlicer{pool: pool, actors: storedActor{authority: identity.NewService(pool)}, log: log, now: time.Now}
}

// slice runs one slice of the attempt args names. A panic fails the attempt
// with its cause, because the engine records only the failures it returns.
func (s importSlicer) slice(ctx context.Context, args importJobArgs, work importSliceWork) (result importSliceResult, err error) {
	db, err := workspaceJobDB(s.pool, args)
	if err != nil {
		return importSliceResult{}, err
	}
	ctx, err = workspaceJobCtx(ctx, args)
	if err != nil {
		return importSliceResult{}, err
	}
	lease, runs := args.lease(), migration.NewRunStore(db)
	system := principal.SystemActing(ctx, importWorkerActor)
	defer func() {
		if recovered := recover(); recovered != nil {
			panicked := fmt.Errorf("%s panicked: %v", args.Kind(), recovered)
			result, err = s.stop(system, runs, lease, panicked, migration.StopCause(lease.Status, panicked))
		}
	}()
	actorCtx, trace, err := s.actAs(ctx, system, runs, lease)
	if trace != nil {
		system = principal.WithCorrelationID(system, *trace)
	}
	if err != nil {
		return s.settle(ctx, system, runs, lease, err)
	}
	// A test run writes nothing to the estate, so it takes no run lock.
	if lease.Status != migration.StatusValidating {
		release, err := runs.LockRunWrites(actorCtx, lease.Run)
		if errors.Is(err, migration.ErrRunWritesHeld) {
			return importSliceResult{end: importSliceLockHeld}, nil
		}
		if err != nil {
			return s.settle(ctx, system, runs, lease, err)
		}
		defer release()
	}
	run, err := runs.Claim(actorCtx, lease)
	if err != nil {
		return s.settle(ctx, system, runs, lease, err)
	}
	done, err := work(actorCtx, importSlice{db: db, runs: runs, lease: lease, run: run, budget: importSliceBudgetOn(s.now)})
	if err == nil && !done {
		return s.yield(system, runs, lease)
	}
	return s.settle(ctx, system, runs, lease, err)
}

// yield hands the attempt back to its queue before the job snoozes, on a
// context of its own: a stopping worker's has ended.
func (s importSlicer) yield(system context.Context, runs *migration.RunStore, lease migration.Lease) (importSliceResult, error) {
	yieldCtx, cancel := context.WithTimeout(context.WithoutCancel(system), importStopFlushTimeout)
	defer cancel()
	err := runs.Yield(yieldCtx, lease)
	switch {
	case errors.Is(err, migration.ErrSuperseded):
		return importSliceResult{}, nil
	case err != nil:
		return importSliceResult{}, err
	}
	return importSliceResult{end: importSliceContinues}, nil
}

// actAs reads the run and binds the principal that queued its attempt. The
// attempt's correlation and the run's batch go on every audit row. The
// correlation is answered even when the principal is refused, so the record of
// that stop joins its attempt's trace. A run gone or moved past this attempt
// answers ErrSuperseded.
func (s importSlicer) actAs(
	ctx, system context.Context, runs *migration.RunStore, lease migration.Lease,
) (context.Context, *ids.UUID, error) {
	run, err := runs.GetStaged(system, lease.Run)
	if errors.Is(err, apperrors.ErrNotFound) || (err == nil && (run.Status != lease.Status || run.JobAttempt != lease.Attempt)) {
		return nil, nil, fmt.Errorf("import run %s attempt %d: %w", lease.Run, lease.Attempt, migration.ErrSuperseded)
	}
	if err != nil {
		return nil, nil, err
	}
	actorCtx, err := s.actors.actAsRun(ctx, run)
	if err != nil {
		return nil, run.AttemptCorrelationID, err
	}
	return storekit.WithImportRun(storekit.WithBatch(actorCtx, lease.Run), lease.Run), run.AttemptCorrelationID, nil
}

// settle records how a slice that will not continue ended.
//
// A stopped worker cancels the slice, which has flushed: the next slice
// resumes it. A slice past its wall clock has flushed too, and fails over
// budget. A stop the engine recorded is only logged. Any other error fails the
// attempt with its cause.
func (s importSlicer) settle(
	ctx, system context.Context, runs *migration.RunStore, lease migration.Lease, err error,
) (importSliceResult, error) {
	switch {
	case err == nil, errors.Is(err, migration.ErrSuperseded):
		return importSliceResult{}, nil
	case errors.Is(err, migration.ErrStopRecorded):
		return importSliceResult{stopped: err}, nil
	case errors.Is(ctx.Err(), context.Canceled):
		return s.yield(system, runs, lease)
	case errors.Is(ctx.Err(), context.DeadlineExceeded):
		return s.stop(system, runs, lease, err, migration.StopCause(lease.Status, ctx.Err()))
	}
	return s.stop(system, runs, lease, err, migration.StopCause(lease.Status, err))
}

// stop writes why the attempt stopped, on a context of its own: the slice's may
// be what ended. A newer attempt owning the run is no stop to record or log.
func (s importSlicer) stop(
	system context.Context, runs *migration.RunStore, lease migration.Lease, stopped error, cause string,
) (importSliceResult, error) {
	stopCtx, cancel := context.WithTimeout(context.WithoutCancel(system), importStopFlushTimeout)
	defer cancel()
	err := runs.FailLease(stopCtx, lease, cause)
	if errors.Is(err, migration.ErrSuperseded) {
		return importSliceResult{}, nil
	}
	return importSliceResult{stopped: stopped}, err
}

// addImportJobs registers the test run and the commit and undo slices. They
// always register: a worker with no object store stops a run with that cause
// rather than leaving it queued.
func addImportJobs(reg *jobRegistry, pool *pgxpool.Pool, cfg JobRunnerConfig, log *slog.Logger) {
	slicer := newImportSlicer(pool, log)
	addDeclaredWorker[ImportValidateArgs](reg, &importValidateWorker{slicer: slicer, blobs: cfg.Blobstore, uploadCap: cfg.ImportUploadCap})
	addDeclaredWorker[ImportCommitArgs](reg, &importCommitWorker{slicer: slicer, blobs: cfg.Blobstore, uploadCap: cfg.ImportUploadCap})
	addDeclaredWorker[ImportUndoArgs](reg, &importUndoWorker{slicer: slicer})
}
