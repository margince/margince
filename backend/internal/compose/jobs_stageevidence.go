// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

// The queued half of a stage evidence reading: the job an activity's landing
// enqueues, and the worker that runs it.

import (
	"context"
	"errors"
	"log/slog"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/riverqueue/river"

	"github.com/margince/margince/backend/internal/compose/evidencereread"
	"github.com/margince/margince/backend/internal/modules/capture"
	"github.com/margince/margince/backend/internal/platform/jobs"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// StageEvidenceReadArgs is one queued reading of one activity against one
// deal's criteria.
type StageEvidenceReadArgs struct {
	Workspace ids.UUID `json:"workspace_id"`
	DealID    ids.UUID `json:"deal_id"`
	// ActivityID is the text to read. One activity rather than the deal's
	// whole history: it is what just landed, so it is the only text that can
	// have changed the answer.
	ActivityID ids.UUID `json:"activity_id"`
	// RereadFor is the own-domain removal a second reading answers, and nil
	// on a first one. It keeps the insert from coalescing with the first.
	RereadFor *ids.UUID `json:"reread_for,omitempty"`
}

// Kind is the string River persists for this job.
func (StageEvidenceReadArgs) Kind() string { return "stage_evidence_read" }

// WorkspaceID declares the one tenant this job's work belongs to, and is what
// binds the worker's context — a worker that picked its own could claim one
// workspace and work in another.
func (a StageEvidenceReadArgs) WorkspaceID() ids.UUID { return a.Workspace }

// stageEvidenceReadInsertOpts deduplicates the reading by args.
//
// ByArgs is what makes the at-least-once bus safe here: the same activity
// delivered twice collapses to one reading, and a reading of a DIFFERENT
// activity on the same deal still queues — which is right, because each new
// message is new text that may settle a criterion the last one did not.
func stageEvidenceReadInsertOpts() *river.InsertOpts {
	return jobs.QueuedAs[StageEvidenceReadArgs](&river.InsertOpts{
		// One-off: nothing re-asks for this reading. An activity lands once,
		// and a message that could not be read this time will not read
		// differently on a third attempt.
		MaxAttempts: oneOffJobMaxAttempts,
		UniqueOpts:  river.UniqueOpts{ByArgs: true},
	})
}

// stageEvidenceReadWorker runs one queued reading.
type stageEvidenceReadWorker struct {
	reader *StageEvidenceReader
	log    *slog.Logger
}

// Work reads the activity against the deal's criteria and records what it
// settles.
func (w *stageEvidenceReadWorker) Work(ctx context.Context, job *river.Job[StageEvidenceReadArgs]) error {
	args := job.Args
	wsCtx, err := workspaceJobCtx(ctx, args)
	if err != nil {
		return jobs.FaultContext(ctx, err)
	}
	written, err := w.reader.Read(wsCtx,
		ids.From[ids.DealKind](args.DealID), args.ActivityID)
	if err != nil {
		return jobs.FaultContext(ctx, snoozeWhileRestricted(err))
	}
	if written == 0 {
		// The common outcome, and worth a line at debug rather than silence:
		// most conversations settle no criterion, and an operator asking why a
		// stage never moves should be able to see that it was read.
		w.log.DebugContext(wsCtx, "stage evidence read settled nothing",
			"deal", args.DealID.String(), "activity", args.ActivityID.String())
	}
	return nil
}

// newStageEvidenceReadWorker assembles the worker-role reading.
//
// Unlike the transcript reading beside it, this one is NOT registered without
// a model lane: nobody asked for it, so there is no queued row a rep is
// watching, and an installation with no model configured simply keeps the
// deterministic evidence the ledger already writes.
func newStageEvidenceReadWorker(
	pool *pgxpool.Pool, brain completer, log *slog.Logger,
) *stageEvidenceReadWorker {
	return &stageEvidenceReadWorker{
		reader: NewStageEvidenceReader(pool, StageEvidenceDeals(pool),
			StageEvidenceDomains(pool), brain, time.Now, log),
		log: log,
	}
}

// StageEvidenceRereadArgs is one page of the mail a removed own domain sent,
// to be read against its deals' criteria again.
type StageEvidenceRereadArgs struct {
	Workspace ids.UUID `json:"workspace_id"`
	Domain    string   `json:"domain"`
	// Removal is the audit row of the removal, so a later removal of the same
	// domain is its own sweep.
	Removal ids.UUID `json:"removal_id"`
	// After is the last activity the page before this one judged.
	After ids.UUID `json:"after_activity_id"`
}

// Kind is the string River persists for this job.
func (StageEvidenceRereadArgs) Kind() string { return "stage_evidence_reread" }

// WorkspaceID declares the one tenant this job's work belongs to.
func (a StageEvidenceRereadArgs) WorkspaceID() ids.UUID { return a.Workspace }

// SentFrom is the removed domain whose mail the page reads.
func (a StageEvidenceRereadArgs) SentFrom() string { return a.Domain }

// PageAfter is the cursor the page starts after.
func (a StageEvidenceRereadArgs) PageAfter() ids.UUID { return a.After }

// stageEvidenceRereadPriority is River's lowest. A whole domain's mail waits
// behind the first readings and transcript reads that share its queue.
const stageEvidenceRereadPriority = 4

// stageEvidenceRereadOpts coalesces a page River queues twice. The cursor is
// in the args, so each page of a sweep is its own job.
func stageEvidenceRereadOpts() *river.InsertOpts {
	return jobs.QueuedAs[StageEvidenceRereadArgs](&river.InsertOpts{
		MaxAttempts: oneOffJobMaxAttempts,
		UniqueOpts:  river.UniqueOpts{ByArgs: true},
	})
}

// queueStageEvidenceReread queues the sweep in the transaction that removes an
// own domain, so a committed removal has its sweep.
func queueStageEvidenceReread(ctx context.Context, tx pgx.Tx, domain string, removal ids.UUID) error {
	ws, ok := principal.WorkspaceID(ctx)
	if !ok {
		return errors.New("compose: an own domain was removed outside a bound workspace")
	}
	queue, err := callerTxJobs()
	if err != nil {
		return err
	}
	return queue.EnqueueTx(ctx, tx,
		StageEvidenceRereadArgs{Workspace: ws, Domain: domain, Removal: removal}, stageEvidenceRereadOpts())
}

// newStageEvidenceRereadWorker assembles the sweep. Without a model lane it
// queues nothing, because no worker on this role would run the readings.
func newStageEvidenceRereadWorker(
	pool *pgxpool.Pool, lane bool, log *slog.Logger,
) *evidencereread.Worker[StageEvidenceRereadArgs] {
	return &evidencereread.Worker[StageEvidenceRereadArgs]{
		Bind: workspaceJobCtx, Pool: pool, Log: log, Reads: lane,
		Queue: func() (evidencereread.Enqueuer, error) { return callerTxJobs() },
		Own:   ownDomainReader{store: capture.NewOwnDomainStore(InstallationDB(pool))},
		Reading: func(job StageEvidenceRereadArgs, deal ids.DealID, activity ids.UUID) (river.JobArgs, *river.InsertOpts) {
			opts := stageEvidenceReadInsertOpts()
			opts.Priority = stageEvidenceRereadPriority
			return StageEvidenceReadArgs{
				Workspace: job.Workspace, DealID: deal.UUID, ActivityID: activity, RereadFor: &job.Removal,
			}, opts
		},
		Next: func(job StageEvidenceRereadArgs, after ids.UUID) (river.JobArgs, *river.InsertOpts) {
			job.After = after
			return job, stageEvidenceRereadOpts()
		},
	}
}
