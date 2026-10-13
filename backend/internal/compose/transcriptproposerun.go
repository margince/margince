// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

// What one reading of one transcript does, from claiming the run record to
// closing it.
//
// The division of labour with transcriptpropose.go is: that file owns the
// question put to the model and what may come back, this one owns the run — the
// claim, the staging, and the three outcomes a rep can be shown. Keeping the
// outcomes here is deliberate, because the difference between them is the
// product: "still reading", "read it and it stated nothing", and "could not
// read it" must never collapse into one another.

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/modules/activities"
	"github.com/margince/margince/backend/internal/modules/ai"
	"github.com/margince/margince/backend/internal/platform/database"
	"github.com/margince/margince/backend/internal/platform/database/storekit"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// transcriptTargetType is what the proposal is filed against: the transcript it
// was read from. The proposal does not MODIFY that activity — it proposes a new
// one — which is why the kind is a context target in approvals rather than a
// version-pinned one.
const transcriptTargetType = string(recordTypeActivity)

// Read performs one reading: claim the run, put the transcript to the model,
// stage what it says was promised, and close the run with what happened.
//
// It returns an error only for a fault the JOB should retry — the model lane
// being down, the database being unreachable. A reading that legitimately
// could not be used closes the run as failed and returns nil, because retrying
// would ask the same question of the same text and get the same answer.
func (p *TranscriptProposer) Read(ctx context.Context, store transcriptReadStore, readID ids.UUID, activityID ids.ActivityID) error {
	if _, err := store.BeginTranscriptRead(ctx, readID, activities.TranscriptReadLease); err != nil {
		return err
	}
	reading, err := store.ReadTranscript(ctx, activityID)
	if err != nil {
		if detail, terminal := unreadableTranscript(err); terminal {
			return p.fail(ctx, store, readID, activityID, detail)
		}
		// Anything else — the database unreachable, a scoped transient fault —
		// is the JOB's to retry. Closing the reading here would turn a blip
		// into a permanent verdict the rep has to notice and undo.
		return err
	}
	// Checked again even though the door checked it: the body can be edited
	// between the request and the reading, and a re-normalized transcript is a
	// different size than the one that was queued.
	if err := activities.WithinReadingBounds(reading.Lines); err != nil {
		return p.fail(ctx, store, readID, activityID, err.Error())
	}
	// The day the activity is FILED under, which is the best available answer
	// to "when was this conversation". The composer offers it as an editable
	// date capped at today, so a rep pasting a three-week-old transcript can
	// set the day it happened — and it defaults to today, so one who does not
	// leaves the paste day standing.
	//
	// Said plainly because the difference matters: a relative deadline resolves
	// against whatever this says, so "by Friday" on a backdated transcript
	// whose date was left at today resolves to the wrong week. The reviewer
	// sees the resulting date on the card before any task exists, which is
	// where that is caught.
	// The reading is ABOUT this meeting, and the request it makes IS the
	// transcript — the largest copy of what was said in that room. Naming the
	// activity is what lets an erasure of anyone quoted in it destroy the
	// captured payload by citation: a transcript names its speakers rather than
	// addressing them, and may
	// never spell an address, so the content match alone leaves it standing.
	// The label is the rail's, and a transcript has no subject line of its own
	// to give it — the meeting's own summary is what the reader recognises, so
	// an unnamed reading draws the rail's unnamed sentence rather than a
	// quotation from somebody's transcript.
	steps, err := p.ask(ai.WithSubject(ctx, activityID.Ref(), ""),
		reading.Lines, reading.OccurredAt.Format(time.DateOnly))
	if err != nil {
		if errors.Is(err, errRefusedTranscript) {
			p.log.WarnContext(ctx, "transcript reading refused",
				"transcript_read_id", readID, "activity_id", activityID, "reason", err)
			return p.fail(ctx, store, readID, activityID,
				"the model's reading of this transcript could not be used; the transcript is unchanged and can be read again")
		}
		return err
	}
	return p.stageAndFinish(ctx, store, readID, aboveFloor(steps), reading, activityID)
}

// stageAndFinish commits the quotations and the record that produced them as
// ONE fact, under the lock the engines that destroy a transcript take.
//
// They were separate transactions, and nothing ordered either against those
// engines. So an erasure could land while the model call was out: it nulled the
// body, found no proposals to scrub because none were staged yet, deleted the
// reading, and committed certifying the words destroyed — and this worker then
// came back and staged them. Nothing revisits those rows (storekit.
// LockTranscriptBody says why), so the quotations stayed in the approvals inbox
// permanently.
//
// The lock is taken FIRST and the reading re-read under it, so the two
// interleavings both land somewhere honest: an erasure that got here first has
// already deleted the reading and this stages nothing, and one that arrives
// second waits, then finds the proposals and scrubs them with everything else.
//
// A re-check without the lock only narrows the window while reading as
// complete, which is worse than the honest gap.
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
			return activities.TranscriptReadOutcome{
				Status:      activities.TranscriptReadDone,
				ProposalIDs: staged.proposals,
				LineCount:   len(reading.Lines),
				Detail:      transcriptReadDetail(len(kept), staged),
			}, nil
		})
}

// finishUnderLock closes a run inside the interlock: the lock, the probe, the
// caller's own way of producing the outcome, and the close — one transaction.
//
// EVERY close goes through it, including the failures and the readings that
// found nothing. A close outside the lock answers ErrConflict for a reading an
// erasure has deleted, and the job reads that as a fault to retry — against a
// reading that will never come back, on a transcript that no longer exists.
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
		// Under the lock, before anything is produced: a reading whose row is
		// gone is a reading whose transcript was destroyed, and the work of
		// quoting it is work that must not happen rather than work to roll back.
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

// unreadableTranscript separates a refusal a rep can act on from a fault the
// job should retry, and answers with the message the rep is shown.
//
// Only the typed refusals reach status_detail. A raw err.Error() there would
// put a driver string ("failed to connect to host=…") in front of a rep on the
// one field this feature exists to make readable, and would settle a transient
// blip as a permanent failure.
func unreadableTranscript(err error) (detail string, terminal bool) {
	var notTranscript *activities.NotATranscriptError
	var tooLong *activities.TranscriptTooLongError
	switch {
	case errors.As(err, &notTranscript), errors.As(err, &tooLong):
		return err.Error(), true
	case errors.Is(err, activities.ErrBlankTranscript):
		return "this transcript is empty, so there is nothing to read", true
	case errors.Is(err, apperrors.ErrNotFound):
		return "this transcript is no longer available to read", true
	}
	return "", false
}

// fail closes the run with a reason a rep can act on. A failure to record the
// failure is returned, so a run cannot be left claimed and silent.
func (p *TranscriptProposer) fail(
	ctx context.Context, store transcriptReadStore, readID ids.UUID, activityID ids.ActivityID, detail string,
) error {
	return p.finishUnderLock(ctx, store, readID, activityID,
		func(pgx.Tx) (activities.TranscriptReadOutcome, error) {
			return activities.TranscriptReadOutcome{
				Status: activities.TranscriptReadFailed,
				Detail: detail,
			}, nil
		})
}

// transcriptStaging is what one reading did with the promises it found.
type transcriptStaging struct {
	proposals []ids.UUID
	tasks     int
	watched   int
}

// stage hands each promise to the commitment rule, under one bundle id.
//
// One bundle because they were asked together: a meeting that produced three
// commitments is one act of reading, and an inbox showing them as three
// unrelated questions makes the rep reconstruct that themselves (0200). Each
// still keeps its own diff hash, expiry and verdict.
func (p *TranscriptProposer) stage(
	ctx context.Context, tx pgx.Tx, steps []proposedStep,
	reading activities.TranscriptReading, activityID ids.ActivityID,
) (transcriptStaging, error) {
	ctx, err := transcriptSeat(ctx, tx, reading.Links)
	if err != nil {
		return transcriptStaging{}, err
	}
	bundleID := ids.NewV7()
	var out transcriptStaging
	for _, step := range steps {
		commitment, err := p.commitmentFrom(ctx, tx, step, reading, activityID)
		if err != nil {
			return transcriptStaging{}, err
		}
		outcome, id, err := p.dispatch.DispatchTx(ctx, tx, commitment, bundleID)
		if err != nil {
			return transcriptStaging{}, err
		}
		switch outcome {
		case CommitmentProposed:
			out.proposals = append(out.proposals, id)
		case CommitmentTaskWritten:
			out.tasks++
		case CommitmentWatched:
			out.watched++
		case CommitmentRemembered:
			// Already a task, already waiting, refused or dismissed: this
			// reading adds nothing new.
		}
	}
	return out, nil
}
