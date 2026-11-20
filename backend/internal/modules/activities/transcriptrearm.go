// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package activities

// Handing a dead reading back to a worker — a different concern from starting
// one, and split out when transcriptread.go crossed the line cap.

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// rearmIfAbandoned hands a dead reading back to a worker.
//
// A row still `running` past its lease is not a live reading: the worker that
// claimed it was killed, timed out, or exhausted its retries. Nothing else
// would ever pick it up — a finished job is not re-enqueued, and
// uq_transcript_read_inflight makes the corpse block every new reading of that
// transcript — so without this the transcript is unreadable for good.
//
// Pressing the button again is therefore the recovery path, which is also the
// thing a rep would try unprompted.
func rearmIfAbandoned(
	ctx context.Context, tx pgx.Tx, read *TranscriptRead, enqueue TranscriptReadEnqueue,
) error {
	if read.Status != TranscriptReadRunning {
		return nil
	}
	rearmed, requeued, err := requeueRunning(ctx, tx, read.ID, requeueGuard{deadFor: TranscriptReadLease})
	if err != nil || !requeued {
		// Inside its lease: a real worker holds it, and joining is correct.
		return err
	}
	*read = rearmed
	if enqueue == nil {
		return nil
	}
	return enqueue(ctx, tx, rearmed)
}

// ReleaseTranscriptRead hands a claimed reading back to the queue when the
// worker's try failed, so the job's next try can claim it. Left running, the
// retry inside the lease takes the reading for somebody else's and ends as
// success, with no job behind it. Scoped to the claim that began at claimedAt:
// a worker whose lease ran out must not re-queue a reading another one holds.
func (s *Store) ReleaseTranscriptRead(ctx context.Context, readID ids.UUID, claimedAt time.Time) error {
	if err := auth.Require(ctx, "activity", principal.ActionCreate); err != nil {
		return err
	}
	return s.tx(ctx, func(tx pgx.Tx) error {
		_, _, err := requeueRunning(ctx, tx, readID, requeueGuard{claimedAt: &claimedAt})
		return err
	})
}

// requeueGuard names which running claim may be handed back: the one that
// began at claimedAt, or any whose lease has been dead for deadFor.
type requeueGuard struct {
	claimedAt *time.Time
	deadFor   time.Duration
}

// requeueRunning hands a running reading back to the queue as a new attempt.
// The higher attempt it announces is what lets the projection prefer the
// queued row over the claim it replaces.
func requeueRunning(ctx context.Context, tx pgx.Tx, readID ids.UUID, guard requeueGuard) (TranscriptRead, bool, error) {
	var deadFor *int64
	if guard.claimedAt == nil {
		micros := guard.deadFor.Microseconds()
		deadFor = &micros
	}
	requeued, err := scanTranscriptRead(tx.QueryRow(ctx, `
		UPDATE transcript_read
		   SET status = 'queued', started_at = NULL, status_detail = NULL,
		       attempt = attempt + 1, attempt_at = now()
		 WHERE id = $1 AND status = 'running'
		   AND (started_at = $2 OR started_at < now() - ($3 * interval '1 microsecond'))
		RETURNING `+transcriptReadColumns, readID, guard.claimedAt, deadFor))
	if errors.Is(err, pgx.ErrNoRows) {
		return TranscriptRead{}, false, nil
	}
	if err != nil {
		return TranscriptRead{}, false, fmt.Errorf("requeue transcript read: %w", err)
	}
	return requeued, true, logTranscriptActivity(ctx, tx, requeued)
}
