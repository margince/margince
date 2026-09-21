// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package activities

// Closing a transcript reading, through two doors.
//
// Split from transcriptread.go for size, along the seam that was already there:
// the file next door starts readings and reads them back, and this one records
// what a reading produced. The two doors here differ only in who owns the
// transaction, which is the whole reason the second exists — a reading's
// proposals and the record that it finished are one fact.

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/platform/database/storekit"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// FinishTranscriptRead records what the reading produced and closes it, in a
// transaction of its own.
func (s *Store) FinishTranscriptRead(ctx context.Context, readID ids.UUID, outcome TranscriptReadOutcome) error {
	if err := s.checkTranscriptOutcome(ctx, outcome); err != nil {
		return err
	}
	return s.tx(ctx, func(tx pgx.Tx) error {
		return finishTranscriptReadTx(ctx, tx, readID, outcome)
	})
}

// FinishTranscriptReadTx closes the reading inside a transaction the CALLER
// owns, so the proposals a reading staged and the record that it finished can
// commit or roll back together.
//
// That is the whole reason it exists. The staging and the close were two
// transactions, so an erasure landing between them left quotations of a
// destroyed body standing in the approvals inbox with no reading to explain
// them — and the close's own `status = 'running'` guard reported the conflict
// AFTER the approvals were already committed.
func (s *Store) FinishTranscriptReadTx(
	ctx context.Context, tx pgx.Tx, readID ids.UUID, outcome TranscriptReadOutcome,
) error {
	if err := s.checkTranscriptOutcome(ctx, outcome); err != nil {
		return err
	}
	return finishTranscriptReadTx(ctx, tx, readID, outcome)
}

// checkTranscriptOutcome is the authority and shape gate both doors owe.
func (s *Store) checkTranscriptOutcome(ctx context.Context, outcome TranscriptReadOutcome) error {
	if err := auth.Require(ctx, "activity", principal.ActionCreate); err != nil {
		return err
	}
	if outcome.Status != TranscriptReadDone && outcome.Status != TranscriptReadFailed {
		return fmt.Errorf("activities: a transcript read finishes done or failed, not %q", outcome.Status)
	}
	if outcome.Detail == "" && (outcome.Status == TranscriptReadFailed || len(outcome.ProposalIDs) == 0) {
		return errors.New("activities: a failed or empty transcript read must say why, or its result cannot be told from a broken one")
	}
	return nil
}

func finishTranscriptReadTx(ctx context.Context, tx pgx.Tx, readID ids.UUID, outcome TranscriptReadOutcome) error {
	proposals := outcome.ProposalIDs
	if proposals == nil {
		proposals = []ids.UUID{}
	}
	var detail *string
	if outcome.Detail != "" {
		detail = &outcome.Detail
	}
	// RETURNING because the rail is announced from the SETTLED row rather than
	// from the outcome the caller handed in: the two could disagree about the
	// line count, and the row is what every other reader sees.
	settled, err := scanTranscriptRead(tx.QueryRow(ctx, `
			UPDATE transcript_read
			   SET status = $2, status_detail = $3, proposal_ids = $4, finished_at = now(),
			       line_count = COALESCE($5, line_count)
			 WHERE id = $1 AND status = 'running'
			RETURNING `+transcriptReadColumns,
		readID, outcome.Status, detail, proposals, readLineCount(outcome.LineCount)))
	if errors.Is(err, pgx.ErrNoRows) {
		return fmt.Errorf("%w: transcript read %s is not running", apperrors.ErrConflict, readID)
	}
	if err != nil {
		return fmt.Errorf("finish transcript read: %w", err)
	}
	// AuditEvent, not Audit: the compare-and-set above proves the row was
	// running, so a prior state exists — it is simply a run record's own
	// progress rather than a field a contact edited, and nothing would ever
	// be restored to it.
	if _, err := storekit.AuditEvent(ctx, tx, "update", "transcript_read", readID, map[string]any{
		"status": outcome.Status, "proposals": len(proposals),
	}); err != nil {
		return fmt.Errorf("audit transcript read finish: %w", err)
	}
	return logTranscriptActivity(ctx, tx, settled)
}

// readLineCount keeps the door's own count when the outcome names none — a
// reading that failed before it split the body has nothing truer to say.
func readLineCount(count int) *int {
	if count <= 0 {
		return nil
	}
	return &count
}
