// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package people

import (
	"context"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// RetractCaptureOnlyPersonTx archives a contact capture created, once the
// confidentiality classifier has judged the conversation it came from to be the
// mailbox owner's private life.
//
// The verdict almost always arrives AFTER the contact. Capture creates on
// commit; classification is a background pass that reads the thread later. In
// one real mailbox every single contact on a personal thread — all forty-six —
// predated the verdict about it, so a gate at creation time alone would have
// prevented none of them. This is where the answer actually lands.
//
// It is deliberately narrow, and each condition is a way the retraction could
// otherwise destroy something somebody meant to keep:
//
//   - CAPTURE created it, and no human. A record a person typed is theirs, and
//     a classifier's opinion about one thread does not overrule it.
//   - It is still OWNER-SCOPED. A record promoted to the workspace has been
//     judged a business contact by the sender verdict, which is a decision about
//     the PERSON rather than about one conversation.
//   - Nobody has EDITED it since. An edit is a human saying this record is
//     wanted, whatever it was born from.
//   - It has no other correspondence. A person who also writes about business is
//     a business contact who happens to have a private thread too, and the
//     caller establishes that before calling.
//
// Returns whether a row was actually archived, so the caller can tell a
// retraction from a no-op rather than assuming one happened.
func (s *Store) RetractCaptureOnlyPersonTx(
	ctx context.Context, tx pgx.Tx, id ids.PersonID, ownerID ids.UUID,
) (bool, error) {
	var eligible bool
	if err := tx.QueryRow(ctx, `
		SELECT EXISTS (
		  SELECT 1 FROM person p
		   WHERE p.id = $1
		     AND p.archived_at IS NULL
		     AND p.owner_id = $2
		     AND p.visibility = 'owner'
		     AND p.captured_by LIKE 'connector:%'
		     AND NOT EXISTS (
		           SELECT 1 FROM audit_log a
		            WHERE a.entity_type = 'person' AND a.entity_id = p.id
		              AND a.actor_type = 'human'))`, id.UUID, ownerID).Scan(&eligible); err != nil {
		return false, fmt.Errorf("people: reading whether a captured contact may be retracted: %w", err)
	}
	if !eligible {
		return false, nil
	}
	// The one spelling of archiving a person inside a transaction: it lands the
	// write shape — the audit row and the satellites — so a retraction is
	// recoverable and auditable exactly like a human's archive.
	if err := archivePersonRows(ctx, tx, id, time.Now().UTC(), nil); err != nil {
		return false, fmt.Errorf("people: retracting a captured contact: %w", err)
	}
	return true, nil
}
