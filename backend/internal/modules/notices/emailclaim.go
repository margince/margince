// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package notices

// One notice's one message by mail.
//
// The morning brief and the weekly retrospective take the same shape
// (compose/briefs/briefmail.go): claim the attempt before dialling the relay
// and record why it failed. No receipt stands behind any of them. A claim
// cleared after the relay may have taken the message sends a second copy.
// Only a relay that never took it (mailer.NotTaken) is released.
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
	"github.com/margince/margince/backend/internal/shared/kernel/textcut"
)

// maxMailErrorRunes bounds the stored cause, matching the CHECK on both
// columns that hold one: notice.email_error here, and
// notification_digest_run.mail_error beside it.
//
// A driver's error text is unbounded, and a row the product cannot render helps
// nobody — the point of storing the cause is that a colleague asking "where is
// the mail about my approval" gets an answer from the row rather than from a
// log nobody kept.
const maxMailErrorRunes = 500

// EmailAttempt is one claimed send: the notice to render, and the seat it is
// addressed to.
//
// The recipient travels beside the notice rather than inside it because a
// Notice is what a READER is shown, and a reader is never told who else holds
// one. The sender needs the seat in order to resolve an address for it, which
// is a different question from anything the centre displays.
type EmailAttempt struct {
	ApprovalID *ids.ApprovalID
	Notice     Notice
	Recipient  ids.UserID
	// ClaimedAt is the stamp this claim wrote. A release matches it, so a
	// claim another attempt took since is never freed.
	ClaimedAt time.Time
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
		row := tx.QueryRow(ctx, `
			UPDATE notice
			   SET email_attempted_at = now()
			 WHERE id = $1 AND email_attempted_at IS NULL AND read_at IS NULL
			 RETURNING `+claimedNoticeColumns, id)
		var claimedAt time.Time
		var scanErr error
		attempt, claimedAt, scanErr = scanClaimedNotice(row)
		switch {
		case errors.Is(scanErr, pgx.ErrNoRows):
			return nil
		case scanErr != nil:
			return fmt.Errorf("notices: claiming the notice's mail attempt: %w", scanErr)
		}
		claimed = true
		return auditEmailClaim(ctx, tx, attempt.Notice.ID, claimedAt)
	})
	if err != nil {
		return EmailAttempt{}, false, err
	}
	return attempt, claimed, nil
}

// claimedNoticeColumns is what a claim returns about each notice it took.
const claimedNoticeColumns = `id, recipient_user_id, kind, subject, body,
	target_type, target_id, created_at, origin, email_attempted_at, coalesce(dedupe_key,''), system_text`

// scanClaimedNotice reads one row of claimedNoticeColumns.
func scanClaimedNotice(row pgx.Row) (EmailAttempt, time.Time, error) {
	var attempt EmailAttempt
	var id, recipient ids.UUID
	var claimedAt time.Time
	var noticeKey string
	// Both halves are nullable and the table pairs them, so either arriving
	// alone is a row the constraint should have refused.
	var targetType *string
	var targetID *ids.UUID
	if err := row.Scan(&id, &recipient, &attempt.Notice.Kind, &attempt.Notice.Subject,
		&attempt.Notice.Body, &targetType, &targetID, &attempt.Notice.CreatedAt,
		&attempt.Notice.Origin, &claimedAt, &noticeKey, &attempt.Notice.SystemText); err != nil {
		return attempt, claimedAt, err
	}
	if approval, ok := approvalOfNoticeKey(attempt.Notice.Kind, noticeKey); ok {
		typed := ids.From[ids.ApprovalKind](approval)
		attempt.ApprovalID = &typed
	}
	attempt.Notice.ID = id
	attempt.Recipient = ids.From[ids.UserKind](recipient)
	attempt.ClaimedAt = claimedAt
	if targetType != nil && targetID != nil {
		attempt.Notice.Target = Target{Type: *targetType, ID: *targetID}
	}
	return attempt, claimedAt, nil
}

func auditEmailClaim(ctx context.Context, tx pgx.Tx, id ids.UUID, claimedAt time.Time) error {
	_, err := storekit.Audit(ctx, tx, "update", "notice", id,
		map[string]any{"email_attempted_at": nil},
		map[string]any{"email_attempted_at": claimedAt})
	return err
}

// EmailFailed records why the claimed attempt produced no message.
//
// It does not release the claim: the attempt is spent. Only a relay that
// never took the message is released, by ReleaseEmailAttempts.
//
// A SKIP is not a failure and never reaches here. A seat who moved the class
// off email between the notice and the send, or one who is no longer live, is a
// decision the product took correctly — writing it into the column an operator
// reads to find out why a message did not arrive would bury the relay failures
// that column exists for.
func (s *Store) EmailFailed(ctx context.Context, id ids.UUID, cause string) error {
	cause = textcut.Runes(cause, maxMailErrorRunes)
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

// ReleaseEmailAttempts frees the claims of one send the relay never took, so
// the job's next attempt can claim and send them. Its caller holds the proof
// (mailer.NotTaken); a send the relay may have taken is never released.
//
// All of them or none. A claim whose stamp is not the one this attempt wrote
// answers ErrReleaseChanged and rolls the transaction back. A retry never
// finds half its message spent.
func (s *Store) ReleaseEmailAttempts(ctx context.Context, attempts []EmailAttempt) error {
	claims := map[ids.UUID]time.Time{}
	for _, attempt := range attempts {
		claims[attempt.Notice.ID] = attempt.ClaimedAt
	}
	noticeIDs := make([]ids.UUID, 0, len(claims))
	stamps := make([]time.Time, 0, len(claims))
	for id, at := range claims {
		noticeIDs, stamps = append(noticeIDs, id), append(stamps, at)
	}
	return s.db.Tx(ctx, func(tx pgx.Tx) error {
		rows, err := tx.Query(ctx, `
			WITH held AS (
			       SELECT n.id, n.email_attempted_at, n.email_error FROM notice n
			         JOIN unnest($1::uuid[], $2::timestamptz[]) AS c(id, claimed_at)
			           ON c.id = n.id AND c.claimed_at = n.email_attempted_at
			          FOR UPDATE OF n)
			UPDATE notice SET email_attempted_at = NULL, email_error = NULL
			  FROM held WHERE notice.id = held.id
			RETURNING notice.id, held.email_attempted_at, held.email_error`, noticeIDs, stamps)
		if err != nil {
			return fmt.Errorf("notices: releasing the mail attempts the relay never took: %w", err)
		}
		released, err := scanReleases(rows)
		if err != nil {
			return err
		}
		if len(released) != len(claims) {
			return ErrReleaseChanged
		}
		for _, r := range released {
			if _, err := storekit.Audit(ctx, tx, "update", "notice", r.id,
				map[string]any{"email_attempted_at": r.at, "email_error": r.cause},
				map[string]any{"email_attempted_at": nil, "email_error": nil}); err != nil {
				return err
			}
		}
		return nil
	})
}

// ErrReleaseChanged refuses a release whose claims are not all as this
// attempt left them. Nothing was released.
var ErrReleaseChanged = errors.New("notices: a claim changed since it was taken, so none was released")

type release struct {
	id    ids.UUID
	at    time.Time
	cause *string
}

func scanReleases(rows pgx.Rows) ([]release, error) {
	defer rows.Close()
	var released []release
	for rows.Next() {
		var r release
		if err := rows.Scan(&r.id, &r.at, &r.cause); err != nil {
			return nil, fmt.Errorf("notices: reading a released mail attempt: %w", err)
		}
		released = append(released, r)
	}
	return released, rows.Err()
}
