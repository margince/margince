// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package consent

import (
	"context"
	"log/slog"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/shared/kernel/ids"
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
	// MissedReceiptNoLeadReceipt is a lead's refusal: receipts go to contacts only.
	MissedReceiptNoLeadReceipt = "no_lead_receipt"
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

// raiseMissedReceiptTx raises the alarm in a savepoint of its own. A failure
// to raise it is logged and never undoes the refusal it is about.
func (s *Store) raiseMissedReceiptTx(ctx context.Context, tx pgx.Tx, missed MissedReceipt) {
	slog.ErrorContext(ctx, "a required opt-out receipt was not sent; the refusal remains recorded",
		"subject_type", missed.SubjectType, "subject_id", missed.SubjectID, "reason", missed.Reason)
	if s.missedReceipts == nil {
		return
	}
	nested, err := tx.Begin(ctx)
	if err != nil {
		slog.ErrorContext(ctx, "the missed receipt could not be raised", "cause", err)
		return
	}
	if err := s.missedReceipts.RaiseMissedReceiptTx(ctx, nested, missed); err != nil {
		slog.ErrorContext(ctx, "the missed receipt could not be raised", "cause", err)
		if rollbackErr := nested.Rollback(ctx); rollbackErr != nil {
			slog.ErrorContext(ctx, "rolling back the missed receipt alarm", "cause", rollbackErr)
		}
		return
	}
	if err := nested.Commit(ctx); err != nil {
		slog.ErrorContext(ctx, "the missed receipt alarm could not be kept", "cause", err)
	}
}
