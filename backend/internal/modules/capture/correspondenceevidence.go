// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package capture

// The answers to "does this workspace correspond with the address" that the
// vendor mark reads. The sender check asks it when it sets the mark. A later
// pass asks it again, about one contact, to take the mark off.

import (
	"context"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/platform/database/storekit"
	"github.com/margince/margince/backend/internal/shared/kernel/correspondence"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// lockCorrespondence takes the identity lock for the reason CorrespondsWith
// gives. The caller acts on the answer inside this transaction.
func lockCorrespondence(ctx context.Context, tx pgx.Tx, email string) error {
	return storekit.LockWriteIdentity(ctx, tx,
		correspondence.LockEntity, correspondence.LockIdentity(email))
}

// WorkspaceCorrespondsTx reports whether the workspace has written to the
// address in a way that counts, whenever it did so.
func WorkspaceCorrespondsTx(ctx context.Context, tx pgx.Tx, email string) (bool, error) {
	if err := lockCorrespondence(ctx, tx, email); err != nil {
		return false, err
	}
	return correspondencePositiveTx(ctx, tx, email)
}

// CorrespondsUnderContactTx reports whether the workspace has written to the
// address since the given time, on mail filed under this contact.
//
// A reply from the address is never evidence by itself. Our own outbound must
// pass the decline-aware rule (writesTo), or a vendor could lift its own mark
// by answering one "not interested". Older mail and mail filed under another
// contact say nothing about this contact's present relationship.
func CorrespondsUnderContactTx(
	ctx context.Context, tx pgx.Tx, contactID ids.UUID, email string, since time.Time,
) (bool, error) {
	normalized := normalizeEmail(email)
	if normalized == "" {
		return false, nil
	}
	if err := lockCorrespondence(ctx, tx, email); err != nil {
		return false, err
	}
	sent, err := outboundToTx(ctx, tx, `
		  AND occurred_at > $2
		  AND EXISTS (SELECT 1 FROM activity_link filed
		               WHERE filed.activity_id = activity.id AND filed.contact_id = $3)
		 LIMIT 2`, []string{normalized}, since, contactID)
	if err != nil {
		return false, err
	}
	return writesTo(sent), nil
}

// MarkedContact names one contact carrying a mark the sender check set, and an
// address of it the workspace has mail with.
type MarkedContact struct {
	ContactID ids.UUID
	Email     string
}

// MarkedContactsWithMailTx lists up to limit contacts whose mark setBy set.
// They also have attested outbound mail filed under them since they were made.
// A contact is made when its mark is set. The list is in id order after the
// given contact.
//
// This is a cheap prefilter. CorrespondsUnderContactTx still decides.
func MarkedContactsWithMailTx(ctx context.Context, tx pgx.Tx, setBy string, after ids.UUID, limit int) ([]MarkedContact, error) {
	rows, err := tx.Query(ctx, `
		SELECT DISTINCT ON (p.id) p.id, pe.email
		  FROM contact p
		  JOIN contact_email pe ON pe.contact_id = p.id AND pe.archived_at IS NULL
		 WHERE p.relationship_mark IS NOT NULL AND p.relationship_mark_set_by = $1
		   AND p.archived_at IS NULL AND p.merged_into_id IS NULL
		   AND p.id > $2
		   AND EXISTS (SELECT 1 FROM activity sent
		                 JOIN activity_link filed ON filed.activity_id = sent.id AND filed.contact_id = p.id
		                WHERE sent.counterparty_email = lower(pe.email)
		                  AND sent.counterparty_outbound_attested
		                  AND sent.occurred_at > p.created_at
		                  AND `+auth.ActivityAvailableClause("sent")+`)
		 ORDER BY p.id, pe.email
		 LIMIT $3`, setBy, after, limit)
	if err != nil {
		return nil, fmt.Errorf("capture: listing marked contacts with mail: %w", err)
	}
	defer rows.Close()
	var out []MarkedContact
	for rows.Next() {
		var c MarkedContact
		if err := rows.Scan(&c.ContactID, &c.Email); err != nil {
			return nil, fmt.Errorf("capture: reading a marked contact with mail: %w", err)
		}
		out = append(out, c)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("capture: listing marked contacts with mail: %w", err)
	}
	return out, nil
}
