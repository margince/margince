// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package consent

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/platform/database/storekit"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/noticekeys"
)

// Why a required opt-out receipt did not go out on its own.
const (
	// MissedReceiptNotPrepared is a receipt whose preparation failed.
	MissedReceiptNotPrepared = "not_prepared"
	// MissedReceiptNoAddress is a subject with no address to send it to.
	MissedReceiptNoAddress = "no_address"
	// MissedReceiptOverdue is a receipt staged and not delivered by its deadline.
	MissedReceiptOverdue = "overdue"
	// MissedReceiptNoMailLane is an installation with no lane to send it on.
	MissedReceiptNoMailLane = "no_mail_lane"
)

// MissedReceipt is one required receipt the controller has to see to.
//
// MessageID names the stop act, so one act raises one alarm however often it
// is reported. SubjectType is contact or lead.
type MissedReceipt struct {
	SubjectType string
	SubjectID   ids.UUID
	MessageID   string
	Reason      string
	// DueAt and DeliveryStatus describe an overdue receipt; zero otherwise.
	DueAt          time.Time
	DeliveryStatus string
}

// MissedReceipts raises the alarm for a missed receipt, on the caller's
// transaction. compose writes it as a notice to the privacy operators.
type MissedReceipts interface {
	RaiseMissedReceiptTx(ctx context.Context, tx pgx.Tx, missed MissedReceipt) error
}

// WithMissedReceipts wires where a missed required receipt is raised.
func (s *Store) WithMissedReceipts(m MissedReceipts) *Store {
	s.missedReceipts = m
	return s
}

// WithMissedReceipts wires where the handlers' store raises a missed receipt.
func (h Handlers) WithMissedReceipts(m MissedReceipts) Handlers {
	h.store = h.store.WithMissedReceipts(m)
	return h
}

// missedReceiptOwedAction is the system_log row that keeps a missed receipt
// whose alarm could not be raised. No other row records that the receipt is
// owed, so a sweep raises it again from here.
const missedReceiptOwedAction = "optout_receipt_alarm_owed"

// raiseMissedReceiptTx raises the alarm in a savepoint of its own. A failure
// to raise it never undoes the refusal it is about, and leaves the receipt
// marked as owed for the next sweep.
func (s *Store) raiseMissedReceiptTx(ctx context.Context, tx pgx.Tx, missed MissedReceipt) {
	slog.ErrorContext(ctx, "a required opt-out receipt was not sent; the refusal remains recorded",
		"subject_type", missed.SubjectType, "subject_id", missed.SubjectID, "reason", missed.Reason)
	if s.missedReceipts == nil {
		return
	}
	if err := s.raiseInSavepointTx(ctx, tx, missed); err != nil {
		slog.ErrorContext(ctx, "the missed receipt could not be raised; the next sweep raises it", "cause", err)
		keepMissedReceiptOwedTx(ctx, tx, missed)
	}
}

func (s *Store) raiseInSavepointTx(ctx context.Context, tx pgx.Tx, missed MissedReceipt) error {
	nested, err := tx.Begin(ctx)
	if err != nil {
		return err
	}
	if err := s.missedReceipts.RaiseMissedReceiptTx(ctx, nested, missed); err != nil {
		return errors.Join(err, nested.Rollback(ctx))
	}
	return nested.Commit(ctx)
}

// keepMissedReceiptOwedTx writes the owed marker in a savepoint of its own, so
// a marker that cannot be written still leaves the refusal standing.
func keepMissedReceiptOwedTx(ctx context.Context, tx pgx.Tx, missed MissedReceipt) {
	nested, err := tx.Begin(ctx)
	if err == nil {
		_, err = storekit.LogSystem(ctx, nested, missedReceiptOwedAction, map[string]any{
			"subject_type": missed.SubjectType, "subject_id": missed.SubjectID,
			"message_id": missed.MessageID, fieldReason: missed.Reason,
		})
		if err != nil {
			err = errors.Join(err, nested.Rollback(ctx))
		} else {
			err = nested.Commit(ctx)
		}
	}
	if err != nil {
		slog.ErrorContext(ctx, "the missed receipt could not be kept for the next sweep", "cause", err)
	}
}

// OwedMissedReceipt is a missed receipt kept as owed, and the row that keeps it.
type OwedMissedReceipt struct {
	ID ids.UUID
	MissedReceipt
}

// NamedSubject builds a predicate over a subject's type and id that holds while
// that subject still names somebody. compose supplies privacy's.
type NamedSubject func(typeExpr, idExpr string) string

// OwedMissedReceiptsTx answers the missed receipts kept as owed after since
// that no notice has raised yet. It reads past the row after, in row order, at
// most limit of them. A notice in any state settles one. A subject that names nobody any
// more is left out, so it can never hold a page.
func OwedMissedReceiptsTx(
	ctx context.Context, tx pgx.Tx, since time.Time, after ids.UUID, limit int, named NamedSubject,
) ([]OwedMissedReceipt, error) {
	rows, err := tx.Query(ctx, `
		SELECT l.id, l.detail->>'subject_type', (l.detail->>'subject_id')::uuid,
		       l.detail->>'message_id', l.detail->>'reason'
		  FROM system_log l
		 WHERE l.action = $1 AND l.occurred_at > $2 AND l.id > $3
		   AND NOT EXISTS (SELECT 1 FROM notice n WHERE n.dedupe_key = $4 || (l.detail->>'message_id'))
		   AND `+named("l.detail->>'subject_type'", "(l.detail->>'subject_id')::uuid")+`
		 ORDER BY l.id LIMIT $5`,
		missedReceiptOwedAction, since, after, noticekeys.OptOutReceiptPrefix, limit)
	if err != nil {
		return nil, fmt.Errorf("consent: reading the missed receipts still owed: %w", err)
	}
	return pgx.CollectRows(rows, func(row pgx.CollectableRow) (OwedMissedReceipt, error) {
		var owed OwedMissedReceipt
		err := row.Scan(&owed.ID, &owed.SubjectType, &owed.SubjectID, &owed.MessageID, &owed.Reason)
		return owed, err
	})
}

// AgedOutMissedReceiptsTx counts the receipts kept as owed in [from, to) about a
// subject still named, that no notice ever raised. The sweep stops raising them
// once they leave its window.
func AgedOutMissedReceiptsTx(ctx context.Context, tx pgx.Tx, from, to time.Time, named NamedSubject) (int, error) {
	var n int
	err := tx.QueryRow(ctx, `
		SELECT count(DISTINCT l.detail->>'message_id') FROM system_log l
		 WHERE l.action = $1 AND l.occurred_at >= $2 AND l.occurred_at < $3
		   AND NOT EXISTS (SELECT 1 FROM notice n WHERE n.dedupe_key = $4 || (l.detail->>'message_id'))
		   AND `+named("l.detail->>'subject_type'", "(l.detail->>'subject_id')::uuid"),
		missedReceiptOwedAction, from, to, noticekeys.OptOutReceiptPrefix).Scan(&n)
	if err != nil {
		return 0, fmt.Errorf("consent: counting the missed receipts no sweep raised: %w", err)
	}
	return n, nil
}
