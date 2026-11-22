// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package activities

import (
	"context"
	"encoding/json"
	"fmt"
	"slices"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/platform/database/storekit"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// The evidence keys an own-address unlink records. The claim's removal reads
// them to put the link and the participants back.
const (
	ownAddressClaimedReason  = "own_address_claimed"
	ownAddressReleasedReason = "own_address_released"
	evidenceReason           = "reason"
	evidenceClaim            = "claim"
	evidenceLink             = "link"
	evidenceParticipants     = "participants"
	evidenceUndoOf           = "undo_of"
)

// OwnAddressRefile is what one pass over a contact's mail found and did.
type OwnAddressRefile struct {
	// Refiled counts the messages taken off the contact.
	Refiled int
	// Foreign is true when the contact is linked to something that is not
	// only the member's own captured mail. Nothing is refiled then.
	Foreign bool
	// TooMany is true when the contact has more mail than one call moves.
	// Nothing is refiled then.
	TooMany bool
}

// RefileOwnAddressMailTx takes a contact off the member's own captured mail,
// once the contact's address turned out to be the member's. Live capture files
// no contact and stamps no participant for an own address. Stored mail gets the
// same shape: the contact's link and participant rows go, and every other link
// and participant stays.
//
// It acts only when every live message linked to the contact is an email.
// The member's own connector must have captured it. It must not be held, and
// no other member may be a participant. Anything else makes the contact
// foreign and nothing is changed.
// More than limit messages is refused too.
//
// The first read picks candidates. The attendee locks and then the row locks
// are taken, and the same read runs again under them. A message that changed
// in between makes the contact foreign.
//
// Each unlink records the claim, the link and the participants it removed, and
// recomputes the audience, so the mail stays the member's own.
func RefileOwnAddressMailTx(ctx context.Context, tx pgx.Tx, contact ids.ContactID, member, claim ids.UUID, limit int) (OwnAddressRefile, error) {
	mail, foreign, err := ownAddressMailTx(ctx, tx, contact, member, limit)
	if err != nil || foreign || len(mail) > limit {
		return OwnAddressRefile{Foreign: foreign, TooMany: len(mail) > limit}, err
	}
	if err := storekit.LockMeetingAttendeePage(ctx, tx, uuids(mail)); err != nil {
		return OwnAddressRefile{}, err
	}
	again, foreign, err := ownAddressMailTx(ctx, tx, contact, member, limit)
	if err != nil {
		return OwnAddressRefile{}, err
	}
	if foreign || !slices.Equal(mail, again) {
		return OwnAddressRefile{Foreign: true}, nil
	}
	out := OwnAddressRefile{}
	for _, id := range mail {
		moved, err := unlinkOwnAddressContactTx(ctx, tx, id, contact, member, claim)
		if err != nil {
			return OwnAddressRefile{}, err
		}
		if moved {
			out.Refiled++
		}
	}
	return out, nil
}

func uuids(mail []ids.ActivityID) []ids.UUID {
	out := make([]ids.UUID, len(mail))
	for i, id := range mail {
		out[i] = id.UUID
	}
	return out
}

// ownAddressMailTx reads the live messages linked to the contact, up to one
// past limit. It reports whether any of them is not the member's own email.
func ownAddressMailTx(ctx context.Context, tx pgx.Tx, contact ids.ContactID, member ids.UUID, limit int) ([]ids.ActivityID, bool, error) {
	rows, err := tx.Query(ctx, `
		SELECT a.id,
		       a.kind = 'email'
		   AND split_part(a.captured_by, ':', 1) = 'connector'
		   AND split_part(a.captured_by, ':', 3) = $2::uuid::text
		   AND a.restricted_at IS NULL
		   AND NOT EXISTS (SELECT 1 FROM activity_participant p
		                    WHERE p.activity_id = a.id AND p.user_id IS NOT NULL AND p.user_id <> $2::uuid)
		  FROM activity_link l
		  JOIN activity a ON a.id = l.activity_id
		 WHERE l.contact_id = $1 AND a.archived_at IS NULL
		 ORDER BY a.id
		 LIMIT $3`, contact.UUID, member, limit+1)
	if err != nil {
		return nil, false, fmt.Errorf("activities: reading the mail of a contact whose address was claimed: %w", err)
	}
	defer rows.Close()
	var mail []ids.ActivityID
	for rows.Next() {
		var id ids.ActivityID
		var own bool
		if err := rows.Scan(&id, &own); err != nil {
			return nil, false, fmt.Errorf("activities: reading the mail of a contact whose address was claimed: %w", err)
		}
		if !own {
			return nil, true, nil
		}
		mail = append(mail, id)
	}
	return mail, false, rows.Err()
}

// unlinkOwnAddressContactTx drops one message's link and participant rows for
// the contact, with the audit row and event every unlink writes. The caller
// holds the attendee and row locks.
//
// The audit evidence keeps the removed rows, so the claim's removal can put
// them back. The participant's stated address is left out of it. An audit row
// outlives an erasure, and the address is the contact's own.
func unlinkOwnAddressContactTx(ctx context.Context, tx pgx.Tx, id ids.ActivityID, contact ids.ContactID, member, claim ids.UUID) (bool, error) {
	var link json.RawMessage
	if err := tx.QueryRow(ctx, `SELECT coalesce((SELECT to_jsonb(l) FROM activity_link l WHERE l.activity_id = $1 AND l.contact_id = $2), 'null'::jsonb)`,
		id, contact.UUID).Scan(&link); err != nil {
		return false, fmt.Errorf("activities: reading a link before taking a claimed address off it: %w", err)
	}
	rows, err := tx.Query(ctx, `
		DELETE FROM activity_participant ap WHERE ap.activity_id = $1 AND ap.contact_id = $2
		RETURNING to_jsonb(ap) - 'address' - 'address_domain_folded'`, id, contact.UUID)
	if err != nil {
		return false, fmt.Errorf("activities: taking a claimed address off a message's participants: %w", err)
	}
	participants, err := pgx.CollectRows(rows, pgx.RowTo[json.RawMessage])
	if err != nil {
		return false, fmt.Errorf("activities: taking a claimed address off a message's participants: %w", err)
	}
	evidence := map[string]any{
		evidenceReason: ownAddressClaimedReason, "decided_as": member, evidenceClaim: claim,
		evidenceLink: link, evidenceParticipants: participants,
	}
	removed, err := unlinkRecordedTx(ctx, tx, id, linkEntityContact, contact.UUID, nil, evidence)
	if err != nil || !removed {
		return removed, err
	}
	return true, RecomputeAudienceTx(ctx, tx, id)
}
