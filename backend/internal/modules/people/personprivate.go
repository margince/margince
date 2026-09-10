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
//   - A MACHINE created it. Capture (`connector:`) and the sender verdict
//     (`agent:`) both mint contacts nobody asked for; a record a person typed
//     is theirs, and a classifier's opinion does not overrule it. Both prefixes
//     count, because the verdict engine's own records were the ones a
//     connector-only test left standing.
//   - No human has PUBLISHED or edited it. Visibility is deliberately NOT a
//     guard here. The sender verdict promotes a contact to the workspace with
//     nobody behind it, so treating `workspace` as protection let one machine's
//     guess outrank another machine's verdict — and the guess was the earlier
//     of the two. A human's publish audits as a human and still protects the
//     record; a machine's does not.
//   - Nobody has EDITED it since. An edit is a human saying this record is
//     wanted, whatever it was born from.
//   - It has no other correspondence. A person who also writes about business is
//     a business contact who happens to have a private thread too, and the
//     caller establishes that before calling.
//   - The owner has not marked any of its addresses `business`. That override
//     is the owner saying this sender is a counterparty, and no classifier
//     verdict outranks it.
//
// Returns whether a row was actually archived, so the caller can tell a
// retraction from a no-op rather than assuming one happened.
func (s *Store) RetractCaptureOnlyPersonTx(
	ctx context.Context, tx pgx.Tx, id ids.PersonID, ownerID ids.UUID,
) (bool, error) {
	// The row lock comes FIRST, so the eligibility below reads committed truth
	// that cannot change before the archive: checked-then-locked, a human edit
	// or a promotion could commit in between and be archived over.
	if _, err := tx.Exec(ctx, `
		SELECT 1 FROM person WHERE id = $1 FOR UPDATE`, id.UUID); err != nil {
		return false, fmt.Errorf("people: locking a captured contact for retraction: %w", err)
	}
	// The owner's own standing `business` decision protects the record: they
	// have told the product this sender is a counterparty, and no classifier
	// verdict — about a thread or about the sender — outranks that.
	var eligible bool
	if err := tx.QueryRow(ctx, `
		SELECT EXISTS (
		  SELECT 1 FROM person p
		   WHERE p.id = $1
		     AND p.archived_at IS NULL
		     AND p.owner_id = $2
		     AND (p.captured_by LIKE 'connector:%' OR p.captured_by LIKE 'agent:%')
		     AND NOT EXISTS (
		           SELECT 1 FROM audit_log a
		            WHERE a.entity_type = 'person' AND a.entity_id = p.id
		              AND a.actor_type = 'human')
		     AND NOT EXISTS (
		           SELECT 1 FROM capture_sender_override o
		            JOIN person_email pe ON pe.person_id = p.id AND pe.archived_at IS NULL
		           WHERE o.user_id = p.owner_id AND o.decision = 'business'
		             AND o.address = pe.email))`, id.UUID, ownerID).Scan(&eligible); err != nil {
		return false, fmt.Errorf("people: reading whether a captured contact may be retracted: %w", err)
	}
	if !eligible {
		return false, nil
	}
	// NARROW BEFORE ARCHIVING, and it is not cosmetic. The people list honours
	// `include_archived` under a row scope that admits any workspace-visible
	// record, so an archived contact still on `workspace` stays listable by
	// every colleague — the private correspondent would be hidden from nobody.
	// Narrowing first puts the row behind the owner scope even when somebody
	// asks to see archived records.
	if err := shiftVisibilityTx(ctx, tx, id, visibilityWorkspace, visibilityOwner); err != nil {
		return false, err
	}
	// The one spelling of archiving a person inside a transaction: it lands the
	// write shape — the audit row and the satellites — so a retraction is
	// recoverable and auditable exactly like a human's archive.
	if err := archivePersonRows(ctx, tx, id, time.Now().UTC(), nil); err != nil {
		return false, fmt.Errorf("people: retracting a captured contact: %w", err)
	}
	return true, nil
}

// CaptureOnlyHolder names one capture-created record and the mailbox owner it
// was minted for — the pair RetractCaptureOnlyPersonTx is called with.
type CaptureOnlyHolder struct {
	PersonID ids.PersonID
	OwnerID  ids.UUID
}

// CaptureOnlyHoldersOfAddressTx lists the records a sender verdict may be
// entitled to retract: machine-created people holding the address, whatever
// their visibility.
//
// Both machine prefixes, and no visibility filter, for the reason
// RetractCaptureOnlyPersonTx gives: the sender verdict mints contacts under
// `agent:` and promotes them to the workspace itself, so a scan restricted to
// owner-scoped connector records could not see the records that engine made.
//
// It is a candidate scan, not the eligibility ruling. The full predicate — the
// human-audit check included — has exactly one spelling, inside
// RetractCaptureOnlyPersonTx, which re-reads it on the same transaction; a
// candidate listed here that fails it there is a no-op, never an archive.
func (s *Store) CaptureOnlyHoldersOfAddressTx(ctx context.Context, tx pgx.Tx, email string) ([]CaptureOnlyHolder, error) {
	rows, err := tx.Query(ctx, `
		SELECT p.id, p.owner_id
		  FROM person p
		  JOIN person_email pe ON pe.person_id = p.id AND pe.archived_at IS NULL
		 WHERE pe.email = lower(btrim($1))
		   AND p.archived_at IS NULL
		   AND p.merged_into_id IS NULL
		   AND p.owner_id IS NOT NULL
		   AND (p.captured_by LIKE 'connector:%' OR p.captured_by LIKE 'agent:%')`, email)
	if err != nil {
		return nil, fmt.Errorf("people: listing the captured holders of an address: %w", err)
	}
	defer rows.Close()
	var out []CaptureOnlyHolder
	for rows.Next() {
		var h CaptureOnlyHolder
		if err := rows.Scan(&h.PersonID, &h.OwnerID); err != nil {
			return nil, fmt.Errorf("people: reading a captured holder of an address: %w", err)
		}
		out = append(out, h)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("people: listing the captured holders of an address: %w", err)
	}
	return out, nil
}
