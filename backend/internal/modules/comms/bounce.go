// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package comms

// A sent message the receiving system returned, recorded on its own row.
//
// The bounce is a later fact ABOUT a send, not a different outcome of it:
// status stays 'sent', because the provider did accept and dispatch the
// message and every status reader depends on that. What changes is that the
// row now says the mail did not arrive — the one thing 'sent' alone can
// never say.

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/platform/database/storekit"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/ports/connector"
)

// bounceReasonCap bounds the stored reason. The text comes from the delivery
// report — external input — and is shown to operators, so it is kept short
// enough to read and too short to smuggle a transcript in.
const bounceReasonCap = 500

// RecordBounce marks the sent message the delivery report named. It reports
// whether a row was marked: false is a normal answer, not a fault — the
// report may name mail this installation never sent (the owner's own mail
// client shares the mailbox), or a redelivered report may name a row already
// marked. Once-only, first report wins. The row may still read 'pending':
// a delivery report proves the wire carried the message, and the receipt
// write can lose a race against the report that answers it — refusing the
// pending row would consume the report off the wire and lose the bounce for
// good. A parked delivery stays unmarkable: nothing was handed to a provider.
//
// A report is written only when THREE facts line up, because everything on it
// is attacker-writable — a Message-ID is known to every recipient of the
// mail, and anyone can post a report-shaped message into a captured mailbox:
//   - the named message is a row this store sent (message_id + status),
//   - the row belongs to the mailbox owner whose capture is reporting
//     (user_id = the connector principal's user — the person whose mailbox
//     the report actually arrived in),
//   - the address the report says failed is one the message actually went to.
//
// A forged report failing any of the three records nothing, which reduces
// the forgery surface to a genuine recipient lying about their own mail.
func (s *Store) RecordBounce(ctx context.Context, report connector.BounceReport) (bool, error) {
	if report.MessageID == "" || report.Recipient == "" {
		return false, errors.New("comms: a bounce names no message or no recipient")
	}
	if report.Kind != connector.BounceHard && report.Kind != connector.BounceSoft {
		return false, fmt.Errorf("comms: unknown bounce kind %q", report.Kind)
	}
	actor, err := storekit.Actor(ctx)
	if err != nil {
		return false, err
	}
	if actor.UserID.IsZero() {
		return false, errors.New("comms: a bounce report carries no capturing mailbox owner")
	}
	reason := report.Reason
	if runes := []rune(reason); len(runes) > bounceReasonCap {
		reason = string(runes[:bounceReasonCap])
	}

	var marked bool
	err = s.db.Tx(ctx, func(tx pgx.Tx) error {
		var id ids.UUID
		var activityID ids.ActivityID
		err := tx.QueryRow(ctx, `
			UPDATE comms_outbound
			   SET bounced_at = $2, bounce_kind = $3, bounce_reason = nullif($4, ''),
			       bounce_recipient = lower($6)
			 WHERE message_id = $1 AND status IN ('pending', 'sent') AND bounced_at IS NULL
			   AND user_id = $5
			   AND EXISTS (
				SELECT 1 FROM jsonb_array_elements_text(
					recipients || coalesce(cc, '[]'::jsonb) || coalesce(bcc, '[]'::jsonb)
				) AS went(addr) WHERE lower(went.addr) = lower($6))
			RETURNING id, activity_id`,
			report.MessageID, s.now().UTC(), string(report.Kind), reason,
			actor.UserID, report.Recipient).Scan(&id, &activityID)
		if errors.Is(err, pgx.ErrNoRows) {
			return nil
		}
		if err != nil {
			return fmt.Errorf("comms: recording the bounce: %w", err)
		}
		marked = true

		// action "update": the ledger's verb list is closed, and a bounce IS an
		// update to the delivery's record — the evidence carries what changed.
		auditID, err := storekit.AuditEvent(ctx, tx, "update", "activity", activityID.UUID,
			map[string]any{"bounce": map[string]any{"message_id": report.MessageID, "kind": string(report.Kind), "reason": reason}})
		if err != nil {
			return err
		}
		return storekit.EmitEvent(ctx, tx, auditID, activityID.UUID, crmcontracts.PublicEventCommsDeliveryBounced{
			MessageId: report.MessageID,
			Kind:      crmcontracts.PublicEventCommsDeliveryBouncedKind(report.Kind),
			Reason:    reasonPtr(reason),
		})
	})
	return marked, err
}

// reasonPtr keeps an empty reason ABSENT from the payload rather than
// present-and-empty, matching the contract's optional field.
func reasonPtr(reason string) *string {
	if reason == "" {
		return nil
	}
	return &reason
}
