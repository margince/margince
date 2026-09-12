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
	"strings"

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
//     (user_id = the connector principal's user — the contact whose mailbox
//     the report actually arrived in), OR it is a CONTROLLER delivery, which
//     belongs to no seat at all: the installation's own mail — a disclosure, a
//     confirmation link — is staged with user_id NULL, so a check on user_id
//     alone can never match one. Before this arm a bounced Art. 14 disclosure
//     recorded nothing: the ledger never marked it, and the notice case it was
//     carrying sat in `queued` forever claiming a message was on its way. The
//     forgery surface does not widen, because the third check still holds —
//     the report must name an address that message actually went to, and the
//     message-id of a controller mail is derived from the token row rather
//     than guessable,
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
			   AND (user_id = $5 OR sender_kind = 'controller')
			   AND EXISTS (
				SELECT 1 FROM jsonb_array_elements_text(
					recipients || coalesce(cc, '[]'::jsonb) || coalesce(bcc, '[]'::jsonb)
				) AS went(addr) WHERE lower(went.addr) = lower($6))
			RETURNING id, activity_id`,
			report.MessageID, s.now().UTC(), string(report.Kind), reason,
			actor.UserID, report.Recipient).Scan(&id, &activityID)
		if errors.Is(err, pgx.ErrNoRows) {
			// Nothing to mark. Usually a report about mail this installation
			// never sent, or a redelivery of one already marked.
			//
			// It is ALSO how a SECOND RECIPIENT of the same message arrives: the
			// row carries one bounced_at, so once the first recipient's report
			// has set it, the second matches nothing above. Their address is
			// just as dead, and tying the stop to whether the ledger still had a
			// mark to set would leave the second dead mailbox live.
			//
			// So the stop is still considered — but only behind the SAME THREE
			// CHECKS the mark is, re-asked without the bounced_at clause. Every
			// field on a report is attacker-writable, and a fallback that
			// skipped them would let anyone post a report-shaped message naming
			// any address and have this installation stop writing to it.
			return s.stopVerifiedHard(ctx, tx, report, actor.UserID)
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
		if err := storekit.EmitEvent(ctx, tx, auditID, activityID.UUID, crmcontracts.PublicEventCommsDeliveryBounced{
			MessageId: report.MessageID,
			Kind:      crmcontracts.PublicEventCommsDeliveryBouncedKind(report.Kind),
			Reason:    reasonPtr(reason),
		}); err != nil {
			return err
		}
		// ONLY A HARD BOUNCE stops the address. A soft one is a full mailbox or
		// a greylisting server, and the next message may well arrive — stopping
		// on one would silence a live address because its owner went on holiday
		// with a full inbox.
		//
		// In the SAME transaction as the mark, so the stop and the failure that
		// earned it commit together, and the error is returned rather than
		// swallowed: a marked bounce whose stop silently did not land leaves the
		// address dead on the record and live to the send path, which is exactly
		// the state this exists to end. The provider redelivers reports, so
		// failing costs a retry.
		return s.stopIfHard(ctx, tx, report, id)
	})
	return marked, err
}

// stopVerifiedHard stops an address whose report named a message this
// installation really sent, when the ledger had no mark left to set.
//
// It re-asks the two checks that make a report trustworthy — the message is a
// row this store sent to the mailbox owner who is reporting, and the address is
// one that message actually went to — and drops only `bounced_at IS NULL`,
// which is the clause that makes a second recipient's report look like nothing.
// Without the checks this would be an open door: anyone can post a
// report-shaped message into a captured mailbox.
func (s *Store) stopVerifiedHard(
	ctx context.Context, tx pgx.Tx, report connector.BounceReport, owner ids.UUID,
) error {
	if s.bounce == nil || report.Kind != connector.BounceHard {
		return nil
	}
	var deliveryID ids.UUID
	err := tx.QueryRow(ctx, `
		SELECT id FROM comms_outbound
		 WHERE message_id = $1 AND status IN ('pending', 'sent')
		   AND (user_id = $2 OR sender_kind = 'controller')
		   AND EXISTS (
			SELECT 1 FROM jsonb_array_elements_text(
				recipients || coalesce(cc, '[]'::jsonb) || coalesce(bcc, '[]'::jsonb)
			) AS went(addr) WHERE lower(went.addr) = lower($3))
		 LIMIT 1`, report.MessageID, owner, report.Recipient).Scan(&deliveryID)
	if errors.Is(err, pgx.ErrNoRows) {
		// The report names no message this installation sent to this address.
		// Nothing is stopped, which is the same silence an unverifiable report
		// has always earned.
		return nil
	}
	if err != nil {
		return fmt.Errorf("comms: verifying the bounce that named no markable row: %w", err)
	}
	return s.stopIfHard(ctx, tx, report, deliveryID)
}

// stopIfHard tells the observer that this address is permanently gone.
//
// ONLY A HARD BOUNCE stops an address. A soft one is a full mailbox or a
// greylisting server, and the next message may well arrive — stopping on one
// would silence a live address because its owner went away with a full inbox.
//
// Reached from BOTH arms of RecordBounce, which is the point. The marked arm is
// the ordinary case; the unmarked arm is a second recipient of a message whose
// row is already marked, or a report for a message this ledger cannot mark. The
// address is dead either way, and tying the stop to whether the LEDGER had a
// row left to mark would leave the second dead mailbox live.
//
// Both callers hand it a real delivery id: the marked arm the row it just
// marked, the unmarked arm the row stopVerifiedHard found and verified.
func (s *Store) stopIfHard(
	ctx context.Context, tx pgx.Tx, report connector.BounceReport, deliveryID ids.UUID,
) error {
	if s.bounce == nil || report.Kind != connector.BounceHard {
		return nil
	}
	return s.bounce.HardBounceTx(ctx, tx, HardBounceFact{
		Address:    strings.ToLower(report.Recipient),
		DeliveryID: deliveryID,
	})
}

// reasonPtr keeps an empty reason ABSENT from the payload rather than
// present-and-empty, matching the contract's optional field.
func reasonPtr(reason string) *string {
	if reason == "" {
		return nil
	}
	return &reason
}
