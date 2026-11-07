// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package contacts

// Naming the counterparty among an activity's participants (ACT-DDL-3 /
// ADR-0078).
//
// Capture stamps a participant row for the counterparty as an ADDRESS, because
// at that moment there is no contact: the tiered creation gate has not run, and
// for a suppressed or deferred sender it never will. When a contact does
// resolve, the row that already names that address is the same party — so it
// is UPDATED to carry the contact id rather than joined by a second row. One
// party, one row.
//
// This runs at linkActivityToContact, the one point every ensure path reaches
// and the one that has already settled the contact against a merge, so the id
// written here is the canonical one for the same reason the link is.

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// nameContactAmongParticipants attaches a resolved contact to the participant
// rows that named them only by address.
//
// The address set comes from the contact's own emails, so an alias resolves as
// readily as the primary — capture recorded whichever address the message
// actually used, which need not be the one the record was created from.
//
// It is deliberately forgiving about finding nothing. A manually logged
// activity has no address-only rows; a channel message has one only when its
// provider knew an address and its source vouched for it, so most carry none;
// a replayed capture already promoted its row on the first pass. None of those
// is an error, and none should fail an ensure that has otherwise succeeded.
func nameContactAmongParticipants(ctx context.Context, tx pgx.Tx, activityID ids.ActivityID, contactID ids.ContactID) error {
	_, err := promoteParticipantsToContact(ctx, tx, []ids.UUID{activityID.UUID}, contactID, nil)
	return err
}

// promotableParticipantSQL is the predicate, on the participant row alias, for
// an address-only row the promotion names @cohort_contact on. Live addresses
// only: an abandoned alias names a party who is no longer reachable there.
func promotableParticipantSQL(alias string) string {
	return alias + `.contact_id IS NULL
		   AND ` + alias + `.user_id IS NULL
		   AND ` + alias + `.address IS NOT NULL
		   AND NOT ` + alias + `.past_party_cap
		   AND EXISTS (
		       SELECT 1 FROM contact_email pe
		        WHERE pe.contact_id = @cohort_contact AND pe.archived_at IS NULL
		          AND lower(pe.email) = ` + alias + `.address)`
}

// promoteParticipantsToContact is that update over the given activities: the
// one the ensure ran for, or the ones a cohort repair holds. Both paths share
// this one statement, so both apply the same guards.
// batch bounds a cohort pass; nil means unbounded, for the single activity.
func promoteParticipantsToContact(
	ctx context.Context, tx pgx.Tx, activities []ids.UUID, contactID ids.ContactID, batch *int,
) ([]ids.UUID, error) {
	// The WHERE clause carries the idempotence: a row that already names a
	// contact is left alone, so a second pass cannot repoint a participant that
	// a merge or a human has since settled differently.
	//
	// The NOT EXISTS guard is what keeps the update from colliding with the
	// ACT-DDL-3 uniqueness index. If a contact row for the same (activity, role)
	// already exists — capture knew the contact up front, say, from a reply
	// lookup — then promoting the address row would duplicate it. Leaving the
	// address row unpromoted in that case is correct: it is a second address
	// for a party already recorded, not a second party. The cohort scan that
	// looks for work carries this same guard, which is what lets it terminate:
	// a row this refuses must not be offered again on every pass.
	rows, err := tx.Query(ctx, `
		UPDATE activity_participant ap
		   SET contact_id = @cohort_contact
		 WHERE ap.activity_id = ANY(@activities)
		   AND `+promotableParticipantSQL("ap")+`
		   AND NOT EXISTS (
		       SELECT 1 FROM activity_participant other
		        WHERE other.activity_id = ap.activity_id
		          AND other.role = ap.role
		          AND other.contact_id = @cohort_contact)
		   AND (@batch::int IS NULL OR ap.id IN (
		       SELECT sub.id FROM activity_participant sub
		        WHERE sub.activity_id = ANY(@activities)
		          -- Correlated to THIS contact. A bound that picked the first N
		          -- address-only rows in the workspace would let somebody else's
		          -- backlog fill it: this contact stays selected by the sweep,
		          -- is offered every tick, and is never reached.
		          AND `+promotableParticipantSQL("sub")+`
		        ORDER BY sub.id
		        LIMIT @batch))
		RETURNING ap.activity_id`,
		pgx.NamedArgs{"cohort_contact": contactID, "activities": activities, "batch": batch})
	if err != nil {
		return nil, fmt.Errorf("contacts: naming the contact among an activity's participants: %w", err)
	}
	defer rows.Close()
	var touched []ids.UUID
	for rows.Next() {
		var id ids.UUID
		if err := rows.Scan(&id); err != nil {
			return nil, fmt.Errorf("contacts: naming the contact among an activity's participants: %w", err)
		}
		touched = append(touched, id)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("contacts: naming the contact among an activity's participants: %w", err)
	}
	return touched, nil
}
