// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package people

// Naming the counterparty among an activity's participants (ACT-DDL-3 /
// ADR-0078).
//
// Capture stamps a participant row for the counterparty as an ADDRESS, because
// at that moment there is no person: the tiered creation gate has not run, and
// for a suppressed or deferred sender it never will. When a person does
// resolve, the row that already names that address is the same party — so it
// is UPDATED to carry the person id rather than joined by a second row. One
// party, one row.
//
// This runs at linkActivityToPerson, the one point every ensure path reaches
// and the one that has already settled the person against a merge, so the id
// written here is the canonical one for the same reason the link is.

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// namePersonAmongParticipants attaches a resolved person to the participant
// rows that named them only by address.
//
// The address set comes from the person's own emails, so an alias resolves as
// readily as the primary — capture recorded whichever address the message
// actually used, which need not be the one the record was created from.
//
// It is deliberately forgiving about finding nothing. A manually logged
// activity has no address-only rows; a channel message has one only when its
// provider knew an address and its source vouched for it, so most carry none;
// a replayed capture already promoted its row on the first pass. None of those
// is an error, and none should fail an ensure that has otherwise succeeded.
func namePersonAmongParticipants(ctx context.Context, tx pgx.Tx, activityID ids.ActivityID, personID ids.PersonID) error {
	_, err := promoteParticipantsToPerson(ctx, tx, &activityID, personID, nil)
	return err
}

// promoteParticipantsToPerson is that update at either width: one activity when
// activityID is set, every activity this person's addresses reach when it is
// nil. ONE statement rather than two, because the capture path and the cohort
// repair are the same promotion asked at different scopes — two spellings would
// drift on the guards below, and each of those is why the write is safe.
// batch bounds a cohort-wide pass; nil means unbounded, which is what the
// single-activity pin uses because one activity's rows are already bounded.
func promoteParticipantsToPerson(
	ctx context.Context, tx pgx.Tx, activityID *ids.ActivityID, personID ids.PersonID, batch *int,
) ([]ids.UUID, error) {
	var pin *ids.UUID
	if activityID != nil {
		id := activityID.UUID
		pin = &id
	}
	// The WHERE clause carries the idempotence: a row that already names a
	// person is left alone, so a second pass cannot repoint a participant that
	// a merge or a human has since settled differently.
	//
	// The NOT EXISTS guard is what keeps the update from colliding with the
	// ACT-DDL-3 uniqueness index. If a person row for the same (activity, role)
	// already exists — capture knew the person up front, say, from a reply
	// lookup — then promoting the address row would duplicate it. Leaving the
	// address row unpromoted in that case is correct: it is a second address
	// for a party already recorded, not a second party. The cohort scan that
	// looks for work carries this same guard, which is what lets it terminate:
	// a row this refuses must not be offered again on every pass.
	//
	// Live addresses only. An abandoned alias names a party who is no longer
	// reachable there, and naming a person by it claims a correspondence the
	// address no longer carries.
	rows, err := tx.Query(ctx, `
		UPDATE activity_participant ap
		   SET person_id = $2
		 WHERE ($1::uuid IS NULL OR ap.activity_id = $1)
		   AND ap.person_id IS NULL
		   AND ap.user_id IS NULL
		   AND ap.address IS NOT NULL
		   AND EXISTS (
		       SELECT 1 FROM person_email pe
		        WHERE pe.person_id = $2 AND pe.archived_at IS NULL
		          AND lower(pe.email) = ap.address)
		   AND NOT EXISTS (
		       SELECT 1 FROM activity_participant other
		        WHERE other.activity_id = ap.activity_id
		          AND other.role = ap.role
		          AND other.person_id = $2)
		   AND ($3::int IS NULL OR ap.id IN (
		       SELECT sub.id FROM activity_participant sub
		        WHERE ($1::uuid IS NULL OR sub.activity_id = $1)
		          AND sub.person_id IS NULL AND sub.user_id IS NULL
		          AND sub.address IS NOT NULL
		          -- Correlated to THIS person. A bound that picked the first N
		          -- address-only rows in the workspace would let somebody else's
		          -- backlog fill it: this contact stays selected by the sweep,
		          -- is offered every tick, and is never reached.
		          AND EXISTS (
		              SELECT 1 FROM person_email pe
		               WHERE pe.person_id = $2 AND pe.archived_at IS NULL
		                 AND lower(pe.email) = sub.address)
		        ORDER BY sub.id
		        LIMIT $3))
		RETURNING ap.activity_id`,
		pin, personID, batch)
	if err != nil {
		return nil, fmt.Errorf("people: naming the person among an activity's participants: %w", err)
	}
	defer rows.Close()
	var touched []ids.UUID
	for rows.Next() {
		var id ids.UUID
		if err := rows.Scan(&id); err != nil {
			return nil, fmt.Errorf("people: naming the person among an activity's participants: %w", err)
		}
		touched = append(touched, id)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("people: naming the person among an activity's participants: %w", err)
	}
	return touched, nil
}
