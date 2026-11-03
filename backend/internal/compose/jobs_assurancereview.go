// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

// Delivering a review episode as its task, through a job of its own.
//
// The check queues the job in the transaction that records the finding, so a
// review that exists has a delivery that will run. A failed delivery retries on
// the run and episode it was queued with; it never starts a new check. Once
// River gives up, the episode reads failed, and the next check queues it again.

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"slices"
	"sync"

	"github.com/jackc/pgx/v5"
	"github.com/riverqueue/river"

	"github.com/margince/margince/backend/internal/modules/assurance"
	"github.com/margince/margince/backend/internal/platform/jobs"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// AssuranceReviewDeliveryArgs delivers one review episode. Unique on the
// review and its episode; the run is where its findings are read from.
type AssuranceReviewDeliveryArgs struct {
	Workspace ids.UUID `json:"workspace_id" river:"unique"`
	Exception ids.UUID `json:"exception_id" river:"unique"`
	Episode   int      `json:"episode" river:"unique"`
	Run       ids.UUID `json:"run_id"`
}

// Kind is the stable job identifier River persists in river_job.
func (AssuranceReviewDeliveryArgs) Kind() string { return "assurance_review_delivery" }

// WorkspaceID binds the delivery to its tenant (jobs.WorkspaceScoped).
func (a AssuranceReviewDeliveryArgs) WorkspaceID() ids.UUID { return a.Workspace }

func reviewDeliveryInsertOpts() *river.InsertOpts {
	return jobs.QueuedAs[AssuranceReviewDeliveryArgs](&river.InsertOpts{
		MaxAttempts: oneOffJobMaxAttempts,
		UniqueOpts:  river.UniqueOpts{ByArgs: true, ByState: activeSweepStates},
	})
}

// reviewDeliveryQueue inserts the delivery jobs through the check's own
// transaction, so it holds no pool.
var reviewDeliveryQueue = sync.OnceValues(func() (*jobs.Runner, error) {
	return jobs.NewTxInserter(slog.Default())
})

// queueReviewDeliveries queues a delivery for every review the run observed
// that has no task yet. It reads the run's findings through the same read the
// bundling pass uses, so a deferred review is not queued.
func queueReviewDeliveries(store *assurance.Store) assurance.ReviewDeliveryFunc {
	return func(ctx context.Context, tx pgx.Tx, runID ids.UUID) error {
		workspace, ok := principal.WorkspaceID(ctx)
		if !ok {
			return errors.New("compose: a review delivery was queued outside a bound workspace")
		}
		found, err := bundleableFindings(ctx, tx, runID)
		if err != nil {
			return err
		}
		queue, err := reviewDeliveryQueue()
		if err != nil {
			return err
		}
		for _, finding := range found {
			if !slices.Contains(assurance.ReviewTypes(), finding.Type) {
				continue
			}
			episode, err := store.EpisodeTask(ctx, tx, finding.ID)
			if err != nil || episode.Task != nil {
				return err
			}
			if err := store.QueueDelivery(ctx, tx, finding.ID, episode.Episode); err != nil {
				return err
			}
			args := AssuranceReviewDeliveryArgs{
				Workspace: workspace, Exception: finding.ID, Episode: episode.Episode, Run: runID,
			}
			if err := queue.EnqueueTx(ctx, tx, args, reviewDeliveryInsertOpts()); err != nil {
				return err
			}
		}
		return nil
	}
}

// assuranceReviewDeliveryWorker shares the nightly worker's pool, clock and
// task door, so a delivered review reads like one the pass would mint.
type assuranceReviewDeliveryWorker struct {
	sweep *assuranceSweepWorker
	// deliver is deliverReview unless a test wraps it.
	deliver func(ctx context.Context, deps bundleDeps, args AssuranceReviewDeliveryArgs) error
}

func (w *assuranceReviewDeliveryWorker) Work(ctx context.Context, job *river.Job[AssuranceReviewDeliveryArgs]) error {
	wsCtx, err := workspaceJobCtx(ctx, job.Args)
	if err != nil {
		return jobs.FaultContext(ctx, err)
	}
	wsCtx = principal.SystemActing(wsCtx, assuranceActor)
	store := assurance.NewStore(InstallationDB(w.sweep.pool))
	deps := bundleDeps{bundles: store, activities: w.sweep.activities, exceptions: bundleableFindings, now: w.sweep.now}
	deliver := w.deliver
	if deliver == nil {
		deliver = deliverReview
	}
	err = deliver(wsCtx, deps, job.Args)
	last := err != nil && job.JobRow != nil && job.Attempt >= job.MaxAttempts
	if recordErr := store.InTx(wsCtx, func(ctx context.Context, tx pgx.Tx) error {
		return store.RecordDeliveryAttempt(ctx, tx, job.Args.Exception, job.Args.Episode, last)
	}); recordErr != nil {
		err = errors.Join(err, recordErr)
	}
	return jobs.FaultContext(ctx, err)
}

// deliverReview delivers the episode the job was queued for. A job for an
// episode that has ended, or for a review the run's findings no longer hold
// (answered, cleared or deferred since), does nothing.
func deliverReview(ctx context.Context, deps bundleDeps, args AssuranceReviewDeliveryArgs) (err error) {
	episode, err := readEpisodeTask(ctx, deps, args.Exception)
	if errors.Is(err, apperrors.ErrNotFound) {
		return nil
	}
	if err != nil || episode.Episode != args.Episode {
		return err
	}
	subject, review, found, err := runSubjectOf(ctx, deps, args.Run, args.Exception)
	if err != nil || !found {
		return err
	}
	// Each attempt opens its own cycle. A reused scope would let an attempt that
	// died with its cycle open block every later one.
	cycle, err := deps.bundles.OpenCycle(ctx, fmt.Sprintf("assurance_review:%s:%d:%s",
		args.Exception, args.Episode, ids.NewV7()))
	if err != nil {
		return fmt.Errorf("compose: opening the review's delivery cycle: %w", err)
	}
	defer func() {
		if closeErr := deps.bundles.CloseCycle(ctx, cycle); closeErr != nil {
			err = errors.Join(err, fmt.Errorf("compose: closing the review's delivery cycle: %w", closeErr))
		}
	}()
	_, err = bundleReview(ctx, deps, cycle, subject, review)
	return err
}

// runSubjectOf is the review and every finding on its deal that the run
// observed, as the bundling pass groups them.
func runSubjectOf(
	ctx context.Context, deps bundleDeps, run, review ids.UUID,
) (subjectFindings, assurance.Exception, bool, error) {
	var open []assurance.Exception
	if err := deps.bundles.InTx(ctx, func(ctx context.Context, tx pgx.Tx) error {
		var err error
		open, err = deps.exceptions(ctx, tx, run)
		return err
	}); err != nil {
		return subjectFindings{}, assurance.Exception{}, false,
			fmt.Errorf("compose: reading the findings run %s observed: %w", run, err)
	}
	for _, subject := range groupBySubject(open) {
		for _, finding := range subject.findings {
			if finding.ID == review {
				return subject, finding, true, nil
			}
		}
	}
	return subjectFindings{}, assurance.Exception{}, false, nil
}

// settleReviewTasks completes each open review task whose findings are all
// answered, cleared or gone. Only system-minted tasks complete: the activities
// writer refuses a task somebody wrote by hand. A review still open keeps its
// task.
func settleReviewTasks(ctx context.Context, deps bundleDeps) error {
	var settled []ids.UUID
	if err := deps.bundles.InTx(ctx, func(ctx context.Context, tx pgx.Tx) error {
		rows, err := tx.Query(ctx, `
			SELECT DISTINCT e.task_activity_id
			  FROM assurance_exception e
			  JOIN activity a ON a.id = e.task_activity_id AND NOT a.is_done AND a.archived_at IS NULL
			 WHERE e.type = ANY($1) AND e.status <> 'open'
			   AND NOT EXISTS (SELECT 1 FROM assurance_task_item i
			                     JOIN assurance_exception o ON o.id = i.exception_id
			                    WHERE i.task_activity_id = e.task_activity_id AND o.status = 'open')`,
			assurance.ReviewTypes())
		if err != nil {
			return err
		}
		settled, err = pgx.CollectRows(rows, pgx.RowTo[ids.UUID])
		return err
	}); err != nil {
		return fmt.Errorf("compose: reading the settled review tasks: %w", err)
	}
	if _, err := deps.activities.CompleteCarriedSystemTasks(ctx, settled); err != nil {
		return fmt.Errorf("compose: completing the settled review tasks: %w", err)
	}
	return nil
}
