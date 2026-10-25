// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package capture

// The one answer to "does this workspace correspond with the address".
// The sender check asks it when it sets the vendor mark. A later pass asks it
// again to take the mark off.

import (
	"context"
	"fmt"
	"strings"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/platform/database/storekit"
	"github.com/margince/margince/backend/internal/shared/kernel/correspondence"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// WorkspaceCorrespondsTx reports whether the workspace has written to the
// address in a way that counts. It also reports an address that wrote back
// on a thread the workspace started.
//
// The identity lock is taken first, for the reason CorrespondsWith gives.
// The caller acts on the answer inside this transaction.
func WorkspaceCorrespondsTx(ctx context.Context, tx pgx.Tx, email string) (bool, error) {
	if err := storekit.LockWriteIdentity(ctx, tx,
		correspondence.LockEntity, correspondence.LockIdentity(email)); err != nil {
		return false, err
	}
	wrote, err := correspondencePositiveTx(ctx, tx, email)
	if err != nil || wrote {
		return wrote, err
	}
	return wroteBackTx(ctx, tx, email)
}

// MarkedContact names one contact carrying a mark the sender check set, and an
// address of it the workspace has mail with.
type MarkedContact struct {
	ContactID ids.UUID
	Email     string
}

// MarkedContactsWithMailTx lists up to limit contacts whose mark setBy set.
// They also have an attested outbound mail or a reply on a thread the workspace
// started. The list is in id order after the given contact.
//
// This is a cheap prefilter. WorkspaceCorrespondsTx still decides.
// A single declining outbound does not make a correspondence.
func MarkedContactsWithMailTx(ctx context.Context, tx pgx.Tx, setBy string, after ids.UUID, limit int) ([]MarkedContact, error) {
	rows, err := tx.Query(ctx, `
		SELECT DISTINCT ON (p.id) p.id, pe.email
		  FROM contact p
		  JOIN contact_email pe ON pe.contact_id = p.id AND pe.archived_at IS NULL
		 WHERE p.relationship_mark IS NOT NULL AND p.relationship_mark_set_by = $1
		   AND p.archived_at IS NULL AND p.merged_into_id IS NULL
		   AND p.id > $2
		   AND (EXISTS (SELECT 1 FROM activity sent
		                 WHERE sent.counterparty_email = lower(pe.email)
		                   AND sent.counterparty_outbound_attested
		                   AND `+auth.ActivityAvailableClause("sent")+`)
		        OR `+strings.ReplaceAll(wroteBack(), "$1", "lower(pe.email)")+`)
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
