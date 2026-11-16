// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package consent

// The records Decree 91/2020/ND-CP Art. 11 asks a sender of advertising email
// to keep for a year and show an inspector. They are what the contact agreed
// to, what they refused, and the receipt each refusal was sent. Retention keeps
// them until their floor ends, so this read reaches back as far as they go.

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5"
	openapi_types "github.com/oapi-codegen/runtime/types"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// OptOutRecords answers one contact's registrations, refusals and receipts,
// newest first in each list. An archived contact is answered too.
func (s *Store) OptOutRecords(ctx context.Context, contactID ids.ContactID) (crmcontracts.OptOutRecords, error) {
	if err := auth.Require(ctx, "contact", principal.ActionRead); err != nil {
		return crmcontracts.OptOutRecords{}, err
	}
	sub := subject{entityType: entityContact, column: subjectColumnContact, id: contactID.UUID}
	out := crmcontracts.OptOutRecords{ContactId: openapi_types.UUID(contactID.UUID), GeneratedAt: s.now().UTC()}
	err := s.db.Tx(ctx, func(tx pgx.Tx) error {
		_, events, err := subjectConsentInTx(ctx, tx, sub)
		if err != nil {
			return err
		}
		out.ConsentEvents = make([]crmcontracts.ConsentEvent, 0, len(events))
		for _, ev := range events {
			out.ConsentEvents = append(out.ConsentEvents, wireEvent(ev))
		}
		if out.Stops, err = recordedStopsTx(ctx, tx, contactID); err != nil {
			return err
		}
		out.Receipts, err = optOutReceiptsTx(ctx, tx, contactID)
		return err
	})
	return out, err
}

// recordedStopsTx reads every stop on the contact, lifted ones included. A stop
// written before received_at existed arrived as it was recorded.
func recordedStopsTx(ctx context.Context, tx pgx.Tx, contactID ids.ContactID) ([]crmcontracts.OptOutRecordStop, error) {
	rows, err := tx.Query(ctx, `
		SELECT s.id, s.kind, p.key, coalesce(s.received_at, s.recorded_at), s.recorded_at, s.revoked_at
		  FROM communication_suppression s
		  LEFT JOIN consent_purpose p ON p.id = s.purpose_id
		 WHERE s.contact_id = $1
		 ORDER BY s.recorded_at DESC, s.id DESC`, contactID)
	if err != nil {
		return nil, fmt.Errorf("consent: reading the contact's recorded stops: %w", err)
	}
	return pgx.CollectRows(rows, func(row pgx.CollectableRow) (crmcontracts.OptOutRecordStop, error) {
		var stop crmcontracts.OptOutRecordStop
		var id ids.UUID
		err := row.Scan(&id, &stop.Kind, &stop.PurposeKey, &stop.ReceivedAt, &stop.StoppedAt, &stop.RevokedAt)
		stop.Id = openapi_types.UUID(id)
		return stop, err
	})
}

// optOutReceiptsTx reads every opt-out receipt of the contact's refusals. A
// receipt is found by the message id its refusal derives, never by the
// timeline link, which a relink can move. A merged record's receipts are found
// by the stop each copy came from and by that record's withdrawal prefix.
// An erasure blanks the words and keeps the row, so a blank reads as null.
func optOutReceiptsTx(ctx context.Context, tx pgx.Tx, contactID ids.ContactID) ([]crmcontracts.OptOutReceiptRecord, error) {
	stops, err := refusedStopsTx(ctx, tx, contactID)
	if err != nil {
		return nil, err
	}
	stopReceipts := make([]string, 0, len(stops))
	for _, stop := range stops {
		stopReceipts = append(stopReceipts, acknowledgementMessageID(stop))
	}
	identities, err := idsTx(ctx, tx, `SELECT id FROM contact WHERE id = $1 OR merged_into_id = $1`, contactID.UUID)
	if err != nil {
		return nil, fmt.Errorf("consent: reading the records merged into the contact: %w", err)
	}
	prefixes := make([]string, 0, len(identities))
	for _, identity := range identities {
		prefixes = append(prefixes, withdrawalMessagePrefix(identity))
	}
	rows, err := tx.Query(ctx, `
		SELECT o.message_id, o.status, o.template_version, o.created_at, o.sent_at,
		       nullif(o.subject, ''), nullif(o.body, '')
		  FROM comms_outbound o
		 WHERE o.sender_kind = 'controller' AND o.template_key = $1
		   AND (o.message_id = ANY($2) OR EXISTS (SELECT 1 FROM unnest($3::text[]) p WHERE starts_with(o.message_id, p)))
		 ORDER BY o.created_at DESC, o.id DESC`,
		TemplateOptOutAcknowledgement, stopReceipts, prefixes)
	if err != nil {
		return nil, fmt.Errorf("consent: reading the contact's opt-out receipts: %w", err)
	}
	return pgx.CollectRows(rows, func(row pgx.CollectableRow) (crmcontracts.OptOutReceiptRecord, error) {
		var receipt crmcontracts.OptOutReceiptRecord
		err := row.Scan(&receipt.MessageId, &receipt.Status, &receipt.TemplateVersion, &receipt.QueuedAt,
			&receipt.SentAt, &receipt.Subject, &receipt.Body)
		return receipt, err
	})
}

// refusedStopsTx answers every stop on the contact and every stop it was
// carried from, through each merge in turn.
func refusedStopsTx(ctx context.Context, tx pgx.Tx, contactID ids.ContactID) ([]ids.UUID, error) {
	stops, err := idsTx(ctx, tx, `
		WITH RECURSIVE carried AS (
		  SELECT id, carried_from FROM communication_suppression WHERE contact_id = $1
		  UNION
		  SELECT s.id, s.carried_from FROM communication_suppression s JOIN carried c ON s.id = c.carried_from)
		SELECT id FROM carried`, contactID.UUID)
	if err != nil {
		return nil, fmt.Errorf("consent: reading the stops the contact's refusals came from: %w", err)
	}
	return stops, nil
}

func idsTx(ctx context.Context, tx pgx.Tx, query string, id ids.UUID) ([]ids.UUID, error) {
	rows, err := tx.Query(ctx, query, id)
	if err != nil {
		return nil, err
	}
	return pgx.CollectRows(rows, pgx.RowTo[ids.UUID])
}
