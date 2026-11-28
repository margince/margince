// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

// The worker half of reading a transcript: the job the api queued, and the
// principal it runs as.

import (
	"context"
	"errors"
	"log/slog"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/riverqueue/river"

	"github.com/margince/margince/backend/internal/compose/modelfailure"
	"github.com/margince/margince/backend/internal/modules/activities"
	"github.com/margince/margince/backend/internal/modules/identity"
	"github.com/margince/margince/backend/internal/platform/jobs"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
	"github.com/margince/margince/backend/internal/shared/ports/baselanguage"
)

// The reading gets its own bounded pool for the deep-read reason: it is one
// long model call, and a fan-out of them must not evict the short maintenance
// jobs from the default queue.
const (
	transcriptReadQueue      = "transcript_read"
	transcriptReadMaxWorkers = 2
)

// TranscriptProposeArgs is one queued reading.
type TranscriptProposeArgs struct {
	ActivityID       ids.UUID `json:"activity_id"`
	TranscriptReadID ids.UUID `json:"transcript_read_id"`
	// RequestedBy is the human who asked, so what the reading stages carries
	// their name rather than the worker's. Provenance is written once and
	// never re-derived.
	RequestedBy string `json:"requested_by"`
}

// Kind is the string River persists for this job.
func (TranscriptProposeArgs) Kind() string { return "transcript_propose" }

// transcriptProposeInsertOpts deduplicates the job by args, only while a job
// is still active: the read id is unique per
// reading, so a re-submitted enqueue of the SAME reading collapses while a
// fresh reading always queues — and a reading rearmIfAbandoned hands back
// after a dead worker carries the same id as the job that finished without
// it, for the reason documentExtractInsertOpts gives.
func transcriptProposeInsertOpts() *river.InsertOpts {
	return jobs.QueuedAs[TranscriptProposeArgs](&river.InsertOpts{
		// One-off: a rep asked for this recording to be read and nothing
		// re-asks, so the ladder carries the blob-store blip and the unreadable
		// transcript alone — the same reading document_extract is sized for.
		MaxAttempts: oneOffJobMaxAttempts,
		UniqueOpts:  river.UniqueOpts{ByArgs: true, ByState: activeSweepStates},
	})
}

// withTranscriptReader binds the reading's bookkeeping: system:transcript_read,
// on behalf of the human who asked. The principals are the reading's own,
// never deep read's. Borrowing those once told the colleague deciding a proposal
// that the site crawler had read their meeting.
func withTranscriptReader(ctx context.Context, requestedBy string, readID ids.UUID) context.Context {
	requester := requestedByUserID(requestedBy)
	ctx = principal.WithActor(ctx, principal.Principal{
		Type:       principal.PrincipalSystem,
		ID:         transcriptReadSystemID,
		UserID:     requester,
		OnBehalfOf: requester,
	})
	return principal.WithCorrelationID(ctx, readID)
}

// readerFor binds the reading's work onto its bookkeeping. The reading, and
// what it files for the requester, runs as agent:transcript-proposer under the
// requester's live grants. The run row, and what it files for other
// colleagues, stays the system's (runBooks). live is false when the requester
// left or lost their seat; the reading is then closed as the system's.
func (w *transcriptProposeWorker) readerFor(ctx context.Context, requestedBy string, readID ids.UUID) (reader context.Context, live bool, err error) {
	books := withTranscriptReader(ctx, requestedBy, readID)
	work, err := agentFor(books, w.users, requestedByUserID(requestedBy), transcriptProposalActor)
	if workRefused(err) {
		return books, false, nil
	}
	if err != nil {
		return nil, false, err
	}
	return withRunBooks(work, books), true, nil
}

// transcriptReadSystemID keeps the reading's run row, and files what the
// reading found for colleagues other than the one who asked.
const transcriptReadSystemID = "system:transcript_read"

// transcriptProposeWorker runs one queued reading. It is always registered on
// the worker role — with no model lane it FAILS the reading with a reason the
// rep can see, rather than leaving it queued forever behind a worker that
// cannot read.
type transcriptProposeWorker struct {
	// activities is the run record's three movements, named as the seam the
	// engine already takes rather than the concrete store: the worker's whole
	// relationship with the module is claim, read, finish.
	language   baselanguage.Resolver
	activities transcriptReadStore
	proposer   *TranscriptProposer
	users      *identity.Service
	bind       func(context.Context) (context.Context, error)
	log        *slog.Logger
}

func (w *transcriptProposeWorker) Work(ctx context.Context, job *river.Job[TranscriptProposeArgs]) error {
	args := job.Args
	wsCtx, err := w.bind(ctx)
	if err != nil {
		return jobs.FaultContext(ctx, err)
	}
	if w.proposer == nil {
		return jobs.FaultContext(ctx, w.declineUnread(withTranscriptReader(wsCtx, args.RequestedBy, args.TranscriptReadID), args))
	}
	wsCtx, live, err := w.readerFor(wsCtx, args.RequestedBy, args.TranscriptReadID)
	if err != nil {
		return jobs.FaultContext(ctx, err)
	}
	if !live {
		_, err := w.closeUnread(wsCtx, args, activities.TranscriptFailureDenied)
		return jobs.FaultContext(ctx, err)
	}
	err = w.proposer.Read(wsCtx, w.activities, args.TranscriptReadID, ids.From[ids.ActivityKind](args.ActivityID))
	if errors.Is(err, apperrors.ErrConflict) {
		// The CAS miss: the reading is no longer queued — a rival replica took
		// it, or it was already closed. Nothing to do and nothing wrong.
		w.log.InfoContext(wsCtx, "transcript reading already claimed",
			"transcript_read_id", args.TranscriptReadID)
		return nil
	}
	err = snoozeWhileRestricted(err)
	if jobs.Exhausted(job.JobRow, err) {
		// The reading was handed back for a try that will not come, and a
		// queued reading with no job is one the door joins forever. Detached,
		// because a try that ran out of time has a cancelled context.
		closeCtx, cancel := terminalCtx(wsCtx)
		defer cancel()
		code := activities.TranscriptFailureUnfinished
		if modelfailure.Unanswered(err) {
			code = activities.TranscriptFailureUnanswered
		}
		closed, closeErr := w.closeUnread(closeCtx, args, code)
		if wait := waitToClose(wsCtx, w.log, args.TranscriptReadID, closed, closeErr,
			activities.TranscriptReadLease); wait != nil {
			return jobs.FaultContext(ctx, wait)
		}
	}
	return jobs.FaultContext(ctx, err)
}

// declineUnread closes a reading no configured lane can perform.
func (w *transcriptProposeWorker) declineUnread(ctx context.Context, args TranscriptProposeArgs) error {
	w.log.WarnContext(ctx, "transcript reading declined: no model lane configured",
		"transcript_read_id", args.TranscriptReadID)
	_, err := w.closeUnread(ctx, args, activities.TranscriptFailureNoModel)
	return err
}

// closeUnread fails a reading no worker will read, with the reason why, and
// reports whether it did. To the rep, a queued row nobody picks up looks the
// same as one being worked. A claim refused means another claim holds it.
func (w *transcriptProposeWorker) closeUnread(ctx context.Context, args TranscriptProposeArgs, code string) (bool, error) {
	ctx = runBooks(ctx)
	if _, err := w.activities.BeginTranscriptRead(ctx, args.TranscriptReadID, activities.TranscriptReadLease); err != nil {
		if errors.Is(err, apperrors.ErrConflict) {
			return false, nil
		}
		return false, err
	}
	return true, w.activities.FinishTranscriptRead(ctx, args.TranscriptReadID,
		transcriptCopyFor(string(w.language.Resolve(ctx))).failedOutcome(
			activities.TranscriptFailure{Code: code}))
}

// waitToClose keeps a live job behind a reading its last try could not close.
// The release failed and a claim still holds it, or the close did not write.
// The job then sleeps one lease, which River does not count as a try, and the
// next run reclaims the reading or finds it settled. It answers nil once the
// reading is closed.
func waitToClose(ctx context.Context, log *slog.Logger, readID ids.UUID, closed bool, closeErr error, lease time.Duration) error {
	if closed && closeErr == nil {
		return nil
	}
	log.WarnContext(ctx, "a reading's last try could not close it; the job waits one lease and tries again",
		"read_id", readID, "error", closeErr)
	return river.JobSnooze(lease)
}

// newTranscriptProposeWorker assembles the worker-role reading. brain may be
// nil — a picked-up reading then finishes failed with an actionable message
// rather than sitting queued behind a worker that cannot read it.
func newTranscriptProposeWorker(pool *pgxpool.Pool, brain completer, log *slog.Logger) *transcriptProposeWorker {
	worker := &transcriptProposeWorker{
		language:   installationLanguage(pool),
		activities: activities.NewStore(InstallationDB(pool)),
		users:      identity.NewService(pool),
		bind:       installationBinder(pool),
		log:        log,
	}
	if brain != nil {
		worker.proposer = NewTranscriptProposer(
			pool, brain, NewApprovalsServiceWithEffects(pool), time.Now, log)
	}
	return worker
}
