// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package comms

// Reading one staged delivery back, for the worker about to send it.

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// Load reads one delivery and counts the attempt about to be made — durably,
// before anything can reach the provider, so a crash mid-send can never leave
// the retry looking like a first send. A dispatch that turns out to transmit
// nothing usually keeps the rung anyway: the restore is the PACING deferral's
// alone (RecordDeferral), because only there did one of this installation's
// own rules hold the message with no provider ever asked. A park, and a fault
// raised before the send call, both spend theirs — so the count errs HIGH,
// which is the conservative direction: an early park, never a retry that skips
// its prior-send lookup and mails a real recipient twice.
//
// It returns ErrTerminal for a delivery that already finished — or was never
// staged in this workspace — which is how a redelivered job stops without
// transmitting, rather than dereferencing a row that is not there.
//
// Every mail column is COALESCED because a channel-shaped row carries none of
// them (comms_outbound_shape, 0155): the identity, the subject and the three
// address/reference lists are all NULL there. Without the coalesces the very
// first channel delivery fails at load time — a NULL into a Go string, and a
// NULL jsonb into a decode — which is a delivery that can never leave and a
// fault that names the scan rather than the shape. channel_user_id and
// inflight_at are the two columns NOT coalesced: one is the shape discriminator
// and the other says whether a prior attempt reached the provider, and for both
// of them NULL is the answer this scan needs rather than a value to substitute.
func (s *Store) Load(ctx context.Context, id ids.UUID) (Delivery, error) {
	var d Delivery
	var recipients, cc, bcc, refs, files []byte
	// Both nullable since the installation became a sender: a controller row
	// names no user and borrows no consent purpose. Scanned through pointers so
	// a NULL reads as the zero id rather than failing the load.
	var userID *ids.UserID
	var linkID *ids.UUID
	var instructionID *ids.UUID
	err := s.db.Tx(ctx, func(tx pgx.Tx) error {
		return tx.QueryRow(ctx, `
			UPDATE comms_outbound
			   SET attempts = attempts + 1
			 WHERE id = $1 AND status = 'pending'
			RETURNING id, activity_id, user_id, provider, coalesce(message_id, ''),
			          coalesce(recipients, '[]'::jsonb), coalesce(cc, '[]'::jsonb),
			          coalesce(bcc, '[]'::jsonb),
			          coalesce(subject, ''), body, coalesce(html_body, ''), coalesce(from_name, ''),
			          channel_user_id, coalesce(consent_purpose, ''),
			          coalesce(in_reply_to, ''), coalesce(references_chain, '[]'::jsonb),
			          coalesce(list_unsubscribe, ''), inflight_at, status, attempts, created_at,
			          attachments, sender_kind, coalesce(template_key, ''),
			          coalesce(template_version, 0), coalesce(payload_ref, ''),
			          payload_expires_at, link_id, execution_authority, instruction_id`,
			id).Scan(&d.ID, &d.ActivityID, &userID, &d.Provider, &d.MessageID,
			&recipients, &cc, &bcc, &d.Subject, &d.Body, &d.HTMLBody, &d.FromName, &d.ChannelUserID, &d.ConsentPurpose,
			&d.InReplyTo, &refs, &d.ListUnsubscribe, &d.InFlightAt, &d.Status, &d.Attempts, &d.CreatedAt,
			&files, &d.SenderKind, &d.TemplateKey, &d.TemplateVersion, &d.PayloadRef,
			&d.PayloadExpiresAt, &linkID, &d.ExecutionAuthority, &instructionID)
	})
	if errors.Is(err, pgx.ErrNoRows) {
		return Delivery{}, ErrTerminal
	}
	if err != nil {
		return Delivery{}, fmt.Errorf("comms: loading delivery: %w", err)
	}
	if userID != nil {
		d.UserID = *userID
	}
	if linkID != nil {
		d.LinkID = *linkID
	}
	if instructionID != nil {
		d.InstructionID = *instructionID
	}
	if err := json.Unmarshal(recipients, &d.Recipients); err != nil {
		return Delivery{}, fmt.Errorf("comms: decoding recipients: %w", err)
	}
	// A malformed snapshot fails the load rather than defaulting to "no files".
	// Reading it as empty would let a message whose channel cannot carry
	// attachments sail through the carriage gate and go out stripped, which is
	// the one outcome ADR-0086 exists to prevent.
	if err := json.Unmarshal(files, &d.Attachments); err != nil {
		return Delivery{}, fmt.Errorf("comms: decoding the delivery's attachment snapshot: %w", err)
	}
	if err := json.Unmarshal(cc, &d.Cc); err != nil {
		return Delivery{}, fmt.Errorf("comms: decoding cc: %w", err)
	}
	if err := json.Unmarshal(bcc, &d.Bcc); err != nil {
		return Delivery{}, fmt.Errorf("comms: decoding bcc: %w", err)
	}
	if err := json.Unmarshal(refs, &d.References); err != nil {
		return Delivery{}, fmt.Errorf("comms: decoding references: %w", err)
	}
	return d, nil
}
