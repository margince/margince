// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

// A required opt-out receipt that did not go out reaches the controller as a
// notice to the privacy operators. It is keyed by the stop act's message id,
// so one act raises one notice, and it is taken back once that receipt is sent.

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/modules/comms"
	"github.com/margince/margince/backend/internal/modules/consent"
	"github.com/margince/margince/backend/internal/modules/identity"
	"github.com/margince/margince/backend/internal/modules/notices"
	"github.com/margince/margince/backend/internal/platform/database"
	"github.com/margince/margince/backend/internal/shared/kernel/noticekeys"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

const (
	// receiptAlarmActor is the system actor that raises and settles the alarm.
	receiptAlarmActor = "system:optout_receipt"
	// receiptAlarmObject is what a recipient must read: the rights cases.
	receiptAlarmObject = "privacy_request"
)

// errNoReceiptOperator is a missed receipt nobody could be told about.
var errNoReceiptOperator = errors.New("compose: no live privacy operator can be told about a missed opt-out receipt")

// receiptAlarm raises and settles missed-receipt notices.
type receiptAlarm struct {
	db *database.DB
}

var _ consent.MissedReceipts = receiptAlarm{}

// RaiseMissedReceiptTx writes one notice per privacy operator, on the caller's
// transaction.
func (a receiptAlarm) RaiseMissedReceiptTx(ctx context.Context, tx pgx.Tx, missed consent.MissedReceipt) error {
	system := principal.SystemActing(ctx, receiptAlarmActor)
	recipients, err := identity.PrivacyReviewOperatorsTx(system, tx, receiptAlarmObject)
	if err != nil {
		return err
	}
	if len(recipients) == 0 {
		return errNoReceiptOperator
	}
	var target notices.Target
	if missed.SubjectType != "" && !missed.SubjectID.IsZero() {
		target = notices.Target{Type: missed.SubjectType, ID: missed.SubjectID}
	}
	store := notices.NewStore(a.db)
	for _, recipient := range recipients {
		if _, err := store.CreateTx(system, tx, notices.NewNotice{
			Recipient: recipient, Kind: notices.KindOptOutReceiptMissed,
			Subject: "A required opt-out receipt was not sent", Body: missedReceiptText(missed),
			Target: target, DedupeKey: noticekeys.OptOutReceipt(missed.MessageID),
		}); err != nil {
			return fmt.Errorf("compose: raising a missed opt-out receipt: %w", err)
		}
	}
	return nil
}

// missedReceiptText says why the receipt is missing and that the refusal stands.
func missedReceiptText(missed consent.MissedReceipt) string {
	const stands = " The refusal itself is recorded and stays in force."
	switch missed.Reason {
	case consent.MissedReceiptNoAddress:
		return "This refusal requires a receipt, and the subject has no email address on record, so none was sent." + stands
	case consent.MissedReceiptNoMailLane:
		return "This refusal requires a receipt, and this installation has no mail lane to send it on, so none was sent." + stands
	case consent.MissedReceiptNoLeadReceipt:
		return "This refusal by a lead requires a receipt, and receipts are not yet sent to leads, so none was sent." + stands
	case consent.MissedReceiptOverdue:
		return fmt.Sprintf("This refusal's receipt was due by %s and has not been delivered. Delivery status: %s.",
			missed.DueAt.UTC().Format(time.RFC3339), missed.DeliveryStatus) + stands
	default:
		return "This refusal requires a receipt, and preparing it failed, so none was sent. The cause is in the log." + stands
	}
}

// raiseOverdue raises a notice for each receipt past its deadline, each in a
// transaction of its own, and answers how many it could not raise. A receipt an
// erasure scrubbed since the read is skipped.
func (a receiptAlarm) raiseOverdue(ctx context.Context, store *comms.Store, now time.Time, overdue []comms.OverdueReceipt) (failed int) {
	for _, receipt := range overdue {
		err := a.db.Tx(ctx, func(tx pgx.Tx) error {
			still, err := store.LockOverdueReceiptTx(ctx, tx, receipt.DeliveryID, now)
			if err != nil || !still {
				return err
			}
			return a.RaiseMissedReceiptTx(ctx, tx, consent.MissedReceipt{
				SubjectType: receipt.SubjectType, SubjectID: receipt.SubjectID, MessageID: receipt.MessageID,
				Reason: consent.MissedReceiptOverdue, DueAt: receipt.DueAt, DeliveryStatus: receipt.Status,
			})
		})
		if err != nil {
			failed++
		}
	}
	return failed
}

// settleSent takes back the notices whose receipt has since been sent.
func (a receiptAlarm) settleSent(ctx context.Context, store *comms.Store) error {
	sent, err := store.SentReceiptsWithOpenNotice(ctx, noticekeys.OptOutReceiptPrefix)
	if err != nil || len(sent) == 0 {
		return err
	}
	system := principal.SystemActing(ctx, receiptAlarmActor)
	return a.db.Tx(system, func(tx pgx.Tx) error {
		for _, messageID := range sent {
			if _, err := notices.NewStore(a.db).Retract(system, tx, noticekeys.OptOutReceipt(messageID)); err != nil {
				return err
			}
		}
		return nil
	})
}
