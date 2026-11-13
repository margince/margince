// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

// One reading of one transcript, from claiming the run record to closing it.
// transcriptpropose.go owns the question and the answer; this file owns the
// run. "Still reading", "it stated nothing" and "could not read it" stay apart.

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/modules/activities"
	"github.com/margince/margince/backend/internal/modules/ai"
	"github.com/margince/margince/backend/internal/modules/identity"
	"github.com/margince/margince/backend/internal/platform/database"
	"github.com/margince/margince/backend/internal/platform/database/storekit"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// transcriptTargetType is the transcript a proposal is filed against. The
// proposal adds an activity and changes none, so the target is not pinned.
const transcriptTargetType = string(recordTypeActivity)

// Read performs one reading: claim the run, put the transcript to the model,
// stage what it says was promised, and close the run with what happened.
//
// It returns an error only for a fault the job should retry. A reading that
// could not be used closes as failed: a retry would get the same answer.
func (p *TranscriptProposer) Read(ctx context.Context, store transcriptReadStore, readID ids.UUID, activityID ids.ActivityID) error {
	reader := *p
	reader.said = transcriptCopyFor(identity.BaseLanguageForPrompt(ctx, p.pool))
	return reader.readTranscript(ctx, store, readID, activityID)
}

func (p *TranscriptProposer) readTranscript(ctx context.Context, store transcriptReadStore, readID ids.UUID, activityID ids.ActivityID) error {
	if _, err := store.BeginTranscriptRead(ctx, readID, activities.TranscriptReadLease); err != nil {
		return err
	}
	reading, err := store.ReadTranscript(ctx, activityID)
	if err != nil {
		if failure, terminal := unreadableTranscript(err); terminal {
			return p.fail(ctx, store, readID, activityID, *failure)
		}
		// A transient fault is the job's to retry, not a verdict for the rep.
		return err
	}
	// Checked again: the body can be edited between the request and the reading.
	if err := activities.WithinReadingBounds(reading.Lines); err != nil {
		if failure, terminal := unreadableTranscript(err); terminal {
			return p.fail(ctx, store, readID, activityID, *failure)
		}
		return err
	}
	// "By Friday" resolves against the day the meeting is filed under, which
	// the rep may backdate; the card shows the resulting day before any task.
	// The call names the meeting as its subject, so an erasure of anyone
	// quoted in it reaches the captured request by citation.
	parties, err := p.readCommitmentParties(ctx, activityID)
	if err != nil {
		return err
	}
	steps, err := p.ask(ai.WithSubject(ctx, activityID.Ref(), ""), activityID.UUID,
		reading.Lines, reading.OccurredAt.Format(time.DateOnly), parties)
	if err != nil {
		if errors.Is(err, errRefusedTranscript) {
			p.log.WarnContext(ctx, "transcript reading refused",
				"transcript_read_id", readID, "activity_id", activityID, "reason", err)
			return p.fail(ctx, store, readID, activityID,
				activities.TranscriptFailure{Code: activities.TranscriptFailureRefused})
		}
		return err
	}
	return p.stageAndFinish(ctx, store, readID, aboveFloor(steps), reading, activityID)
}

// stageAndFinish commits the quotations and the run record as one fact, under
// the lock the engines that destroy a transcript take (storekit.
// LockTranscriptBody). The reading is re-read under it: an erasure that came
// first leaves nothing to stage, and one that comes second scrubs what was.
func (p *TranscriptProposer) stageAndFinish(
	ctx context.Context,
	store transcriptReadStore,
	readID ids.UUID,
	kept []proposedStep,
	reading activities.TranscriptReading,
	activityID ids.ActivityID,
) error {
	return p.finishUnderLock(ctx, store, readID, activityID,
		func(tx pgx.Tx) (activities.TranscriptReadOutcome, error) {
			staged, err := p.stage(ctx, tx, kept, reading, activityID)
			if err != nil {
				return activities.TranscriptReadOutcome{}, err
			}
			facts := staged.facts(len(kept))
			return activities.TranscriptReadOutcome{
				Status:      activities.TranscriptReadDone,
				ProposalIDs: staged.approvalIDs(),
				LineCount:   len(reading.Lines),
				Detail:      p.said.outcomeSentence(*facts, transcriptAudience{}),
				Facts:       facts,
			}, nil
		})
}

// finishUnderLock closes a run in one transaction with the lock and the probe.
// Every close goes through it: one outside the lock would retry, forever, a
// reading an erasure deleted.
func (p *TranscriptProposer) finishUnderLock(
	ctx context.Context,
	store transcriptReadStore,
	readID ids.UUID,
	activityID ids.ActivityID,
	produce func(tx pgx.Tx) (activities.TranscriptReadOutcome, error),
) error {
	return database.WithWorkspaceTx(ctx, p.pool, func(tx pgx.Tx) error {
		if err := storekit.LockTranscriptBody(ctx, tx, []ids.UUID{activityID.UUID}); err != nil {
			return err
		}
		// A reading whose row is gone had its transcript destroyed: quote nothing.
		live, err := readingIsRunning(ctx, tx, readID)
		if err != nil {
			return err
		}
		if !live {
			p.log.WarnContext(ctx, "transcript reading abandoned: its record is gone",
				"transcript_read_id", readID, "activity_id", activityID)
			return nil
		}
		outcome, err := produce(tx)
		if err != nil {
			return err
		}
		return store.FinishTranscriptReadTx(ctx, tx, readID, outcome)
	})
}

// readingIsRunning asks whether the reading this worker holds still exists and
// is still the running one, with the row locked so a destructive pass queues
// behind this transaction rather than inside it.
func readingIsRunning(ctx context.Context, tx pgx.Tx, readID ids.UUID) (bool, error) {
	var one int
	err := tx.QueryRow(ctx,
		`SELECT 1 FROM transcript_read WHERE id = $1 AND status = 'running' FOR UPDATE`, readID).Scan(&one)
	if errors.Is(err, pgx.ErrNoRows) {
		return false, nil
	}
	if err != nil {
		return false, fmt.Errorf("compose: re-reading the transcript reading under its lock: %w", err)
	}
	return true, nil
}

// fail closes the run with a reason a rep can act on. A failure to record the
// failure is returned, so a run cannot be left claimed and silent.
func (p *TranscriptProposer) fail(
	ctx context.Context, store transcriptReadStore, readID ids.UUID, activityID ids.ActivityID,
	failure activities.TranscriptFailure,
) error {
	return p.finishUnderLock(ctx, store, readID, activityID,
		func(pgx.Tx) (activities.TranscriptReadOutcome, error) {
			return p.said.failedOutcome(failure), nil
		})
}

// stage hands each promise to the commitment rule under one bundle id, since
// they were read together. Each keeps its own diff hash, expiry and verdict.
func (p *TranscriptProposer) stage(
	ctx context.Context, tx pgx.Tx, steps []proposedStep,
	reading activities.TranscriptReading, activityID ids.ActivityID,
) (transcriptStaging, error) {
	bundleID := ids.NewV7()
	asOf := p.now()
	out := newTranscriptStaging()
	if err := lockPromiseSources(ctx, tx, []ids.UUID{activityID.UUID}); err != nil {
		return transcriptStaging{}, err
	}
	for _, step := range steps {
		commitment, err := p.commitmentFrom(ctx, tx, step, reading, activityID, asOf)
		if err != nil {
			return transcriptStaging{}, err
		}
		outcome, id, err := p.dispatch.DispatchTx(ctx, tx, commitment, bundleID)
		if err != nil {
			return transcriptStaging{}, err
		}
		if outcome.withheld() {
			out.withheld[outcome]++
			continue
		}
		switch outcome {
		case CommitmentWatched:
			// The customer's promise, and the follow-up when somebody chases it.
			out.watched++
			if id.IsZero() {
				continue
			}
			fallthrough
		case CommitmentProposed, CommitmentHistoricalProposed:
			seat, err := proposalDecider(ctx, tx, id)
			if err != nil {
				return transcriptStaging{}, err
			}
			out.proposals = append(out.proposals, activities.TranscriptProposal{ApprovalID: id, SeatID: seat})
		case CommitmentTaskWritten:
			seat, err := transcriptTaskAssignee(ctx, tx, id)
			if err != nil {
				return transcriptStaging{}, err
			}
			out.tasks[seat]++
		case CommitmentRemembered, CommitmentRetained:
			// Already a task, already waiting, refused or dismissed: this
			// reading adds nothing new, and says whom it was put to.
			out.known[derefSeat(commitment.Seat)]++
		}
	}
	return out, nil
}
