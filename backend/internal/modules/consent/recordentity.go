// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package consent

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/platform/database/storekit"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/ports/jurisdiction"
)

// recordEntityTx is the legal entity that answers for a contact or lead when
// nobody is acting on it: the entity its owner works for, else the default.
// Data-protection law follows the company that controls the data, and the
// owner's company is the one that holds this record.
func recordEntityTx(ctx context.Context, tx pgx.Tx, who ruleSubject) (ids.UUID, error) {
	owner, err := recordOwnerTx(ctx, tx, who)
	if err != nil {
		return ids.UUID{}, err
	}
	return storekit.LegalEntityOf(ctx, tx, owner)
}

// recordCountryTx is the data-protection country of recordEntityTx.
func (s *Store) recordCountryTx(ctx context.Context, tx pgx.Tx, who ruleSubject) (jurisdiction.Code, error) {
	entity, err := recordEntityTx(ctx, tx, who)
	if err != nil {
		return "", err
	}
	return s.senderCountryOf(ctx, tx, entity)
}

// recordOwnerTx reads the owner of a contact or lead, zero for none or for a
// subject that names no record. Each kind's column is a compile-time literal.
func recordOwnerTx(ctx context.Context, tx pgx.Tx, who ruleSubject) (ids.UUID, error) {
	var query string
	switch who.Kind {
	case entityContact:
		query = `SELECT owner_id FROM contact WHERE id = $1::uuid`
	case entityLead:
		query = `SELECT owner_id FROM lead WHERE id = $1::uuid`
	default:
		return ids.UUID{}, nil
	}
	var owner *ids.UUID
	err := tx.QueryRow(ctx, query, who.ID).Scan(&owner)
	if errors.Is(err, pgx.ErrNoRows) || (err == nil && owner == nil) {
		return ids.UUID{}, nil
	}
	if err != nil {
		return ids.UUID{}, fmt.Errorf("consent: reading who owns the record: %w", err)
	}
	return *owner, nil
}

// withRecordEntity names the entity a controller mail goes out for, from the
// record it is filed under. Every call into confirmSender passes here.
func withRecordEntity(ctx context.Context, tx pgx.Tx, send ConfirmationSend) (ConfirmationSend, error) {
	who := ruleSubject{Kind: entityContact, ID: send.ContactID.String()}
	if !send.LeadID.IsZero() {
		who = ruleSubject{Kind: entityLead, ID: send.LeadID.String()}
	} else if send.ContactID.IsZero() {
		who = ruleSubject{}
	}
	var err error
	send.Entity, err = recordEntityTx(ctx, tx, who)
	return send, err
}

// queueControllerMailTx queues a controller mail for the entity that answers
// for its record.
func (s *Store) queueControllerMailTx(ctx context.Context, tx pgx.Tx, send ConfirmationSend) (ids.UUID, error) {
	send, err := withRecordEntity(ctx, tx, send)
	if err != nil {
		return ids.UUID{}, err
	}
	return s.confirmSender.QueueConfirmationTx(ctx, tx, send)
}

// presentControllerMailTx previews a controller mail under the entity the
// queue would send it for, so the preview and the send judge alike.
func (s *Store) presentControllerMailTx(ctx context.Context, tx pgx.Tx, send ConfirmationSend) (PresentedMail, error) {
	send, err := withRecordEntity(ctx, tx, send)
	if err != nil {
		return PresentedMail{}, err
	}
	return s.confirmSender.PresentConfirmationTx(ctx, tx, send)
}
