// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

// The queued half of a stage evidence reading: the job an activity's landing
// enqueues, and the worker that runs it.

import (
	"context"
	"log/slog"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/riverqueue/river"

	"github.com/margince/margince/backend/internal/platform/jobs"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
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
}

// Kind is the string River persists for this job.
func (StageEvidenceReadArgs) Kind() string { return "stage_evidence_read" }

// WorkspaceID declares the one tenant this job's work belongs to, and is what
// binds the worker's context — a worker that picked its own could claim one
// workspace and work in another.
func (a StageEvidenceReadArgs) WorkspaceID() ids.UUID { return a.Workspace }

// stageEvidenceReadInsertOpts routes the reading to the model-call queue and
// deduplicates by args.
//
// ByArgs is what makes the at-least-once bus safe here: the same activity
// delivered twice collapses to one reading, and a reading of a DIFFERENT
// activity on the same deal still queues — which is right, because each new
// message is new text that may settle a criterion the last one did not.
func stageEvidenceReadInsertOpts() *river.InsertOpts {
	return &river.InsertOpts{
		Queue: transcriptReadQueue,
		// One-off: nothing re-asks for this reading. An activity lands once,
		// and a message that could not be read this time will not read
		// differently on a third attempt.
		MaxAttempts: oneOffJobMaxAttempts,
		UniqueOpts:  river.UniqueOpts{ByArgs: true},
	}
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
		return jobs.FaultContext(ctx, err)
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
			StageEvidenceDomains(pool), brain,
			StageProgressionProposals(pool, time.Now, log), time.Now, log),
		log: log,
	}
}
