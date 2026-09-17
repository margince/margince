// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package notices

// One notice's ONE attempt at leaving the product by mail.
//
// The SAME shape the morning brief and the weekly retrospective take
// (compose/briefs/briefmail.go): claim the attempt before dialling the relay,
// never release it, record why it failed. There is no delivery ledger and no
// receipt behind any of them, so a claim that could be cleared is a retry loop,
// and a retry loop on a synchronous SMTP call is how one decision becomes three
// messages in a colleague's inbox.
//
// What differs is the trigger. A digest is claimed by a pass that runs on a
// clock, so a duplicate needs two ticks; this is claimed by a River job, and
// River redelivers a job whose completion it could not confirm — which makes
// the duplicate the ordinary case rather than the unlucky one.

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/platform/database/storekit"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// maxEmailErrorRunes bounds the stored cause.
//
// A driver's error text is unbounded, and a row the product cannot render helps
// nobody — the point of storing the cause is that a colleague asking "where is
// the mail about my approval" gets an answer from the row rather than from a
// log nobody kept.
const maxEmailErrorRunes = 500

// EmailAttempt is one claimed send: the notice to render, and the seat it is
// addressed to.
//
// The recipient travels beside the notice rather than inside it because a
// Notice is what a READER is shown, and a reader is never told who else holds
// one. The sender needs the seat in order to resolve an address for it, which
// is a different question from anything the centre displays.
type EmailAttempt struct {
	Notice    Notice
	Recipient ids.UserID
}

// ClaimEmailAttempt takes this notice's ONE mail attempt, or reports that there
// is nothing to take.
//
// The claim is a conditional UPDATE exactly one transaction can win. Everything
// the caller does after it is allowed to fail and lose the message; nothing
// after it is allowed to produce a second one.
//
// FALSE covers three different worlds on purpose, because they are one answer
// to a sender: the attempt is already spent, the reader has already answered
// the card on screen, or the notice is gone. `read_at IS NULL` is the half that
// is not about duplicates — a job staged when the notice was written may be
// worked seconds later, and by then the colleague may be looking at the card.
// Mailing them about something they just answered is the most avoidable message
// this lane can send, and it costs one predicate.
//
// The row is read in the SAME statement that claims it. Two statements would
// leave a window in which the row could be settled between them, and the sender
// would render a notice whose state it never actually held.
func (s *Store) ClaimEmailAttempt(ctx context.Context, id ids.UUID) (EmailAttempt, bool, error) {
	var attempt EmailAttempt
	var claimed bool
	err := s.db.Tx(ctx, func(tx pgx.Tx) error {
		var recipient ids.UUID
		var claimedAt time.Time
		// Both halves are nullable and the table pairs them, so either arriving
		// alone is a row the constraint should have refused.
		var targetType *string
		var targetID *ids.UUID
		row := tx.QueryRow(ctx, `
			UPDATE notice
			   SET email_attempted_at = now()
			 WHERE id = $1 AND email_attempted_at IS NULL AND read_at IS NULL
			 RETURNING recipient_user_id, kind, subject, body,
			           target_type, target_id, created_at, origin, email_attempted_at`, id)
		switch scanErr := row.Scan(&recipient, &attempt.Notice.Kind, &attempt.Notice.Subject,
			&attempt.Notice.Body, &targetType, &targetID, &attempt.Notice.CreatedAt,
			&attempt.Notice.Origin, &claimedAt); {
		case errors.Is(scanErr, pgx.ErrNoRows):
			return nil
		case scanErr != nil:
			return fmt.Errorf("notices: claiming the notice's mail attempt: %w", scanErr)
		}
		attempt.Notice.ID = id
		attempt.Recipient = ids.From[ids.UserKind](recipient)
		if targetType != nil && targetID != nil {
			attempt.Notice.Target = Target{Type: *targetType, ID: *targetID}
		}
		claimed = true
		_, err := storekit.Audit(ctx, tx, "update", "notice", id,
			map[string]any{"email_attempted_at": nil},
			map[string]any{"email_attempted_at": claimedAt})
		return err
	})
	if err != nil {
		return EmailAttempt{}, false, err
	}
	return attempt, claimed, nil
}

// EmailFailed records why the claimed attempt produced no message.
//
// It does NOT release the claim, and that is the point: the attempt is spent
// either way, and a caller that could clear the stamp would have rebuilt the
// retry loop this design refuses.
//
// A SKIP is not a failure and never reaches here. A seat who moved the class
// off email between the notice and the send, or one who is no longer live, is a
// decision the product took correctly — writing it into the column an operator
// reads to find out why a message did not arrive would bury the relay failures
// that column exists for.
func (s *Store) EmailFailed(ctx context.Context, id ids.UUID, cause string) error {
	cause = truncate(cause, maxEmailErrorRunes)
	return s.db.Tx(ctx, func(tx pgx.Tx) error {
		// Predicated on the claim, which is what the table's own CHECK says
		// too: a cause beside no attempt describes a send that never happened.
		tag, err := tx.Exec(ctx, `
			UPDATE notice SET email_error = $2
			 WHERE id = $1 AND email_attempted_at IS NOT NULL`, id, cause)
		if err != nil {
			return fmt.Errorf("notices: recording why the notice was not mailed: %w", err)
		}
		if tag.RowsAffected() == 0 {
			return apperrors.ErrNotFound
		}
		_, err = storekit.Audit(ctx, tx, "update", "notice", id,
			map[string]any{"email_error": nil}, map[string]any{"email_error": cause})
		return err
	})
}
