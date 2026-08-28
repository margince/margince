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
// marked. Once-only over status = 'sent': the first report's kind and reason
// are the ones kept, and a message that never reached 'sent' has nothing to
// bounce.
func (s *Store) RecordBounce(ctx context.Context, messageID string, kind connector.BounceKind, reason string) (bool, error) {
	if messageID == "" {
		return false, errors.New("comms: a bounce names no message")
	}
	if kind != connector.BounceHard && kind != connector.BounceSoft {
		return false, fmt.Errorf("comms: unknown bounce kind %q", kind)
	}
	if runes := []rune(reason); len(runes) > bounceReasonCap {
		reason = string(runes[:bounceReasonCap])
	}

	var marked bool
	err := s.db.Tx(ctx, func(tx pgx.Tx) error {
		var id ids.UUID
		var activityID ids.ActivityID
		err := tx.QueryRow(ctx, `
			UPDATE comms_outbound
			   SET bounced_at = $2, bounce_kind = $3, bounce_reason = nullif($4, '')
			 WHERE message_id = $1 AND status = 'sent' AND bounced_at IS NULL
			RETURNING id, activity_id`,
			messageID, s.now().UTC(), string(kind), reason).Scan(&id, &activityID)
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
			map[string]any{"bounce": map[string]any{"message_id": messageID, "kind": string(kind), "reason": reason}})
		if err != nil {
			return err
		}
		return storekit.EmitEvent(ctx, tx, auditID, activityID.UUID, crmcontracts.PublicEventCommsDeliveryBounced{
			MessageId: messageID,
			Kind:      crmcontracts.PublicEventCommsDeliveryBouncedKind(kind),
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
