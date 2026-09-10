// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package consent

// A proposal sent through a confirm link becomes a case somebody owns.
//
// What the subject sends is already recorded: stageSubmission files it in
// person_confirm_submission with an audit row. That table answers "what did
// this contact send us" and nothing more. It has no deadline, it is not in the
// queue the DPO works through, and it hands the subject no reference to quote.
//
// The statutory answer is owed from the moment the request arrives, so the
// arrival is what opens the case. Same transaction as the submission it came
// from: a case without its submission names work whose evidence is missing,
// and a submission without its case is the defect this file closes.
//
// NOT A WRITE TO THE PERSON. Opening a case records that somebody asked. What
// the record should now say is a human's decision, made through the queue —
// the subject holds a bearer token and sits outside every row-scope probe, so
// their say-so is evidence of a request and never an edit.

import (
	"context"
	"crypto/rand"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgerrcode"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"

	"github.com/margince/margince/backend/internal/platform/database/storekit"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// The channels a case can arrive through. Constants rather than literals for
// dsr.go's reason: a typo would store a channel no reader branches on, and the
// CHECK would refuse it at a depth that reports badly.
const (
	channelConfirmLink = "confirm_link"
)

// receiptAlphabet excludes the characters that are read back wrong over a
// phone: no O or 0, no I or 1, no S or 5. A receipt exists to be quoted aloud,
// and a reference the subject cannot dictate is one they cannot chase.
const receiptAlphabet = "ABCDEFGHJKLMNPQRTUVWXYZ2346789"

// receiptLength is 10 symbols over that 30-character alphabet, which is about
// 49 bits. Guessing one reveals nothing on its own — the case queue is admin
// gated and the receipt is not a credential — so the bound that matters is
// collision, not attack.
const receiptLength = 10

// caseKindFor maps a submission to the right the subject exercised.
//
// A correction is Art. 16 and an erasure request is Art. 17, and the two carry
// different work: one proposes a value for a human to accept, the other asks
// for the record to go. The existing queue already knows both kinds, so this
// adds no vocabulary.
func caseKindFor(submissionKind string) (string, bool) {
	switch submissionKind {
	case submissionCorrection:
		return "rectify", true
	case submissionErasure:
		return dsrKindErasure, true
	}
	return "", false
}

// oneCalendarMonthAfter is the Art. 12(3) deadline: one month from receipt,
// not thirty days.
//
// CLAMPED TO THE LAST DAY, which AddDate alone does not do. Go normalizes an
// impossible date FORWARD — 31 January plus one month is 31 February, which
// AddDate returns as 3 March — so a request arriving on the last day of a long
// month would silently earn three extra days. The statute reads the other way:
// with no 31st in February, the month ends on the 28th and so does the
// deadline. Held by TestOneCalendarMonthFollowsTheShortMonths, which caught
// exactly this.
func oneCalendarMonthAfter(received time.Time) time.Time {
	candidate := received.AddDate(0, 1, 0)
	// OVERSHOT IS MEASURED BY THE MONTH, never by the day number.
	//
	// Comparing days looks equivalent and is not: AddDate normalizes across a
	// DST boundary too, and a clock that goes back an hour moves the result
	// into the previous day without any month having overflowed. A request
	// received 00:30 on 6 August in Santiago lands at 23:30 on 5 September,
	// and a day-number test read that as overflow and clamped the deadline to
	// 31 August — five days off the month the subject is owed.
	//
	// The month is the fact the clamp is about. Only a target month too short
	// to hold the day rolls the result into the month AFTER the one asked for.
	wanted := received.Month() + 1
	if wanted > time.December {
		wanted -= 12
	}
	if candidate.Month() == wanted {
		return candidate
	}
	// Rolled past it: step back to the last day of the month that was wanted,
	// keeping the clock time the request arrived at.
	return candidate.AddDate(0, 0, -candidate.Day())
}

// mintReceiptReference draws a reference the subject can quote.
func mintReceiptReference() (string, error) {
	buf := make([]byte, receiptLength)
	if _, err := rand.Read(buf); err != nil {
		return "", fmt.Errorf("consent: drawing a receipt reference: %w", err)
	}
	out := make([]byte, receiptLength)
	for i, b := range buf {
		out[i] = receiptAlphabet[int(b)%len(receiptAlphabet)]
	}
	return "DSR-" + string(out), nil
}

// openRightsCaseTx files one case for one submission and answers its receipt.
//
// NO AUTH GATE, and that is the point rather than an omission. The caller is
// the public confirm edge running as system:public_confirm, which holds no
// person grant and must not: a bearer token proving a mailbox is exactly the
// authority to say "this is what I am asking for", and nothing more. What
// bounds this call is that it is reachable only from inside SubmitConfirmation,
// after spendConfirmTokenTx consumed the link that proves the mailbox.
//
// IDEMPOTENT ON THE SUBMISSION. A replayed submit is the ordinary case here — a
// double press, a prefetching mail client, a retry after a timeout — and two
// cases for one request would queue the same work twice and give the subject
// two references for one answer. The unique index decides it; ON CONFLICT reads
// the standing case back so the replay answers the receipt it already earned.
func openRightsCaseTx(ctx context.Context, tx pgx.Tx, personID ids.PersonID,
	submissionID ids.UUID, submissionKind string, receivedAt time.Time,
) (string, error) {
	kind, ok := caseKindFor(submissionKind)
	if !ok {
		return "", nil
	}
	// RETRIED ON A COLLIDING RECEIPT, because losing that draw must not cost
	// the subject their request. Two references cannot both be quotable, so the
	// unique index refuses the second — and inside the submit transaction that
	// refusal rolls back the proposal, the case AND the spent token, answering
	// a legitimate erasure request with a server error and no link left to
	// retry it on. At roughly 49 bits a collision is vanishingly unlikely; what
	// makes the retry worth its lines is what one costs, not how often it comes.
	//
	// SAVEPOINT per attempt, because a failed statement aborts the surrounding
	// transaction: without one the retry runs against a transaction Postgres has
	// already given up on, and every attempt after the first fails identically.
	for attempt := range receiptAttempts {
		caseID, stored, created, err := attemptRightsCase(ctx, tx, kind, personID,
			submissionID, receivedAt)
		if err == nil {
			return finishRightsCase(ctx, tx, caseID, stored, created, kind, submissionID)
		}
		if !isReceiptCollision(err) || attempt == receiptAttempts-1 {
			return "", err
		}
	}
	// Unreachable: the loop above either returns or exhausts its attempts and
	// returns the last error. Go cannot see that, so this states it.
	return "", fmt.Errorf("consent: opening the rights case this request owes an answer to: no attempt ran")
}

// receiptAttempts bounds the redraw. Three, because a second collision after a
// fresh draw is not bad luck any more — it is the alphabet or the generator
// being wrong, and looping on that would hold the transaction open rather than
// report it.
const receiptAttempts = 3

// attemptRightsCase makes one attempt at the insert, inside its own savepoint.
func attemptRightsCase(ctx context.Context, tx pgx.Tx, kind string, personID ids.PersonID,
	submissionID ids.UUID, receivedAt time.Time,
) (caseID ids.UUID, stored string, created bool, err error) {
	receipt, err := mintReceiptReference()
	if err != nil {
		return ids.UUID{}, "", false, err
	}
	nested, err := tx.Begin(ctx)
	if err != nil {
		return ids.UUID{}, "", false, fmt.Errorf("consent: opening the rights case: %w", err)
	}
	// RETURNING with xmax = 0 says whether THIS statement inserted the row:
	// ON CONFLICT DO UPDATE returns the row either way, and without the test a
	// replay would write a second audit entry saying a case was opened that
	// already existed. The update is a no-op assignment for that reason — it
	// exists to make the row returnable, not to change it.
	err = nested.QueryRow(ctx, `
		INSERT INTO data_subject_request
		  (kind, subject_ref, person_id, received_at, channel, due_at,
		   source_submission_id, receipt_reference)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8)
		ON CONFLICT (source_submission_id) WHERE source_submission_id IS NOT NULL
		  DO UPDATE SET source_submission_id = EXCLUDED.source_submission_id
		RETURNING id, receipt_reference, (xmax = 0)`,
		kind, personID.String(), personID.UUID, receivedAt, channelConfirmLink,
		oneCalendarMonthAfter(receivedAt), submissionID, receipt,
	).Scan(&caseID, &stored, &created)
	if err != nil {
		// The insert's failure is what the caller needs, and the savepoint's
		// rollback rides with it rather than being dropped: a rollback that
		// fails usually means the connection is already gone, which turns a
		// legible constraint violation into a confusing one further up. Joined
		// rather than logged because this store carries no logger, and errors.Is
		// still reaches the insert's own sentinel through the join.
		if rbErr := nested.Rollback(ctx); rbErr != nil {
			err = errors.Join(err, fmt.Errorf("rolling the savepoint back: %w", rbErr))
		}
		return ids.UUID{}, "", false, fmt.Errorf(
			"consent: opening the rights case this request owes an answer to: %w", err)
	}
	if err := nested.Commit(ctx); err != nil {
		return ids.UUID{}, "", false, fmt.Errorf("consent: opening the rights case: %w", err)
	}
	return caseID, stored, created, nil
}

// isReceiptCollision reports the one failure worth redrawing for: two cases
// drew the same quotable reference. Every other refusal is a real fault and
// must reach the caller rather than being retried into a different message.
func isReceiptCollision(err error) bool {
	var pgErr *pgconn.PgError
	if !errors.As(err, &pgErr) {
		return false
	}
	return pgErr.Code == pgerrcode.UniqueViolation &&
		pgErr.ConstraintName == "data_subject_request_receipt_reference"
}

// finishRightsCase audits a case this statement actually opened and answers the
// reference either way.
func finishRightsCase(ctx context.Context, tx pgx.Tx, caseID ids.UUID, stored string,
	created bool, kind string, submissionID ids.UUID,
) (string, error) {
	if !created {
		return stored, nil
	}
	// Audited against the CASE, not the person: this row is the work, and a
	// reader asking why the queue holds it wants the act that opened it. The
	// person is named on the row itself.
	//
	// AuditEvent rather than Audit, for stageSubmission's reason — a case being
	// opened is an occurrence with no prior state, and an update audit would
	// demand a before-image that does not exist.
	if _, err := storekit.AuditEvent(ctx, tx, "create", "data_subject_request", caseID, map[string]any{
		fieldKind:       kind,
		"channel":       channelConfirmLink,
		"submission_id": submissionID,
	}); err != nil {
		return "", err
	}
	return stored, nil
}

// RightsCaseReceipt is what the subject is told to quote when they ask after
// their request. It carries the reference and the right it was opened under,
// and never the case id — an id is a handle to a row, and handing one out
// invites it to be typed back in somewhere that trusts it.
type RightsCaseReceipt struct {
	// Kind is the queue's own vocabulary: rectify or erasure.
	Kind string
	// Reference is the quotable string, unique across the installation.
	Reference string
	// Field names the record field a correction proposes, empty for an
	// erasure. A subject who corrected two fields gets two receipts, and
	// without this they cannot tell which answer is about which.
	Field string
}
