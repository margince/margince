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
	"github.com/margince/margince/backend/internal/shared/kernel/systemtext"
	"github.com/margince/margince/backend/internal/shared/kernel/textlang"
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
			SystemText: &systemtext.Message{Subject: systemtext.Sentence("receipt.missed.subject"), Body: missedReceiptComponent(missed)},
			Subject:    systemtext.Literal("receipt.missed.subject"), Body: missedReceiptText(missed),
			Target: target, DedupeKey: noticekeys.OptOutReceipt(missed.MessageID),
		}); err != nil {
			return fmt.Errorf("compose: raising a missed opt-out receipt: %w", err)
		}
	}
	return nil
}

// missedReceiptText says why the receipt is missing and that the refusal stands.
func missedReceiptText(missed consent.MissedReceipt) string {
	if missed.Reason == consent.MissedReceiptOverdue {
		return fmt.Sprintf(systemtext.Copy("receipt.overdue", textlang.English), missed.DueAt.UTC().Format(time.RFC3339), missed.DeliveryStatus)
	}
	return systemtext.Literal(missedReceiptComponent(missed).Code)
}

func missedReceiptComponent(missed consent.MissedReceipt) *systemtext.Component {
	switch missed.Reason {
	case consent.MissedReceiptNoAddress:
		return systemtext.Sentence("receipt.no_address")
	case consent.MissedReceiptNoMailLane:
		return systemtext.Sentence("receipt.no_mail_lane")
	case consent.MissedReceiptOverdue:
		return systemtext.Sentence("receipt.overdue", systemtext.Date(missed.DueAt.UTC().Format(time.RFC3339)), systemtext.Text(missed.DeliveryStatus))
	default:
		return systemtext.Sentence("receipt.failed")
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
