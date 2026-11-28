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
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/relstrength"
)

// The contact is resolved after capture stamps its parties. Bind the existing
// row instead of adding a second participant for the same contact.
func nameContactAmongParticipants(ctx context.Context, tx pgx.Tx, activityID ids.ActivityID, contactID ids.ContactID) error {
	_, err := promoteParticipantsToContact(ctx, tx, []ids.UUID{activityID.UUID}, contactID, nil)
	return err
}

// errSenderStampUnwired refuses a link written by a store that cannot apply the
// proven-sender rule. A composition that forgot it fails instead of filing mail
// with no sender.
var errSenderStampUnwired = errors.New("contacts: this store has no proven-sender rule; compose must wire WithSenderStamp")

// stampProvenSenders names the contact the sender of the held activities whose
// stated address proves them. It runs after the links and promotions it reads.
func (s *Store) stampProvenSenders(ctx context.Context, tx pgx.Tx, activities []ids.UUID, contactID ids.ContactID) ([]ids.UUID, error) {
	if s.stampSenders == nil {
		return nil, errSenderStampUnwired
	}
	if len(activities) == 0 {
		return nil, nil
	}
	return s.stampSenders(ctx, tx, activities, contactID.UUID)
}

// A bound party stays settled. Duplicate roles and mixed-case address sets
// wait for their own repair before another participant can be bound.
func promotableParticipantSQL(alias, contact string) string {
	return alias + `.contact_id IS NULL
		   AND ` + alias + `.user_id IS NULL
		   AND NOT ` + alias + `.past_party_cap
		   AND NOT EXISTS (SELECT 1 FROM activity_participant older
		        WHERE older.activity_id = ` + alias + `.activity_id AND older.address <> lower(btrim(older.address)))
		   AND NOT EXISTS (SELECT 1 FROM activity_participant other
		        WHERE other.activity_id = ` + alias + `.activity_id AND other.role = ` + alias + `.role
		          AND other.contact_id = ` + contact + `)`
}

func promotableAddressParticipantSQL(alias, contact string) string {
	return promotableParticipantSQL(alias, contact) + " AND " + alias + ".address IS NOT NULL AND " +
		relstrength.AddressProvesSQL(contact, alias+".address")
}

func promotableChannelParticipantSQL(alias, contact string) string {
	return promotableParticipantSQL(alias, contact) + " AND " + linkedChannelParticipantSQL(alias, contact)
}

// Keep this proof correlated to the unresolved party. Pulling the joins into
// its caller walks filed messages before the partial index can narrow them.
func linkedChannelParticipantSQL(alias, contact string) string {
	return `(` + alias + `.channel_user_id IS NOT NULL AND EXISTS (SELECT 1 FROM activity a
		JOIN activity_link l ON l.activity_id = a.id AND l.entity_type = 'contact'
		JOIN contact_channel_identity ci ON ci.contact_id = l.contact_id
		 WHERE a.id = ` + alias + `.activity_id AND l.contact_id = ` + contact + `
		   AND a.archived_at IS NULL AND a.restricted_at IS NULL
		   AND ci.provider = a.channel_provider AND ci.channel_user_id = ` + alias + `.channel_user_id
		   AND ci.archived_at IS NULL
		   AND NOT EXISTS (SELECT 1 FROM contact_channel_identity rival
		        WHERE rival.provider = ci.provider AND rival.channel_user_id = ci.channel_user_id
		          AND rival.archived_at IS NULL AND rival.contact_id <> ci.contact_id)
		 OFFSET 0))`
}

// Separate identity lookups prevent an address proof from scanning channel
// history. Both use one update, pinned to the captured row or bounded by batch.
func promoteParticipantsToContact(
	ctx context.Context, tx pgx.Tx, activities []ids.UUID, contactID ids.ContactID, batch *int,
) ([]ids.UUID, error) {
	addressed, err := promoteMatchingParticipantsToContact(ctx, tx, activities, contactID, batch, promotableAddressParticipantSQL)
	if err != nil {
		return nil, err
	}
	channeled, err := promoteMatchingParticipantsToContact(ctx, tx, activities, contactID, batch, promotableChannelParticipantSQL)
	if err != nil {
		return nil, err
	}
	return append(addressed, channeled...), nil
}

func promoteMatchingParticipantsToContact(
	ctx context.Context, tx pgx.Tx, activities []ids.UUID, contactID ids.ContactID, batch *int,
	match func(string, string) string,
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
		   AND `+match("ap", "@cohort_contact")+`
		   AND (@batch::int IS NULL OR ap.id IN (
		       SELECT sub.id FROM activity_participant sub
		        WHERE sub.activity_id = ANY(@activities)
		          -- Correlated to THIS contact. A bound that picked the first N
		          -- address-only rows in the workspace would let somebody else's
		          -- backlog fill it: this contact stays selected by the sweep,
		          -- is offered every tick, and is never reached.
		          AND `+match("sub", "@cohort_contact")+`
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
