// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package activities

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/platform/database/storekit"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// ownAddressClaimedReason is the evidence an unlink records when a member's
// claim of an address as their own took the contact off the mail.
const ownAddressClaimedReason = "own_address_claimed"

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
// It acts only when every live message linked to the contact is the member's
// own capture, not held, with no other member as a participant. Anything else
// makes the contact foreign and nothing is changed. More than limit messages
// is refused too, so one request never carries an unbounded rewrite.
//
// A rerun finds no link left to take off and changes nothing.
func RefileOwnAddressMailTx(ctx context.Context, tx pgx.Tx, contact ids.ContactID, member ids.UUID, limit int) (OwnAddressRefile, error) {
	rows, err := tx.Query(ctx, `
		SELECT a.id,
		       split_part(a.captured_by, ':', 1) = 'connector'
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
		return OwnAddressRefile{}, fmt.Errorf("activities: reading the mail of a contact whose address was claimed: %w", err)
	}
	defer rows.Close()
	var mail []ids.ActivityID
	for rows.Next() {
		var id ids.ActivityID
		var own bool
		if err := rows.Scan(&id, &own); err != nil {
			return OwnAddressRefile{}, fmt.Errorf("activities: reading the mail of a contact whose address was claimed: %w", err)
		}
		if !own {
			return OwnAddressRefile{Foreign: true}, nil
		}
		mail = append(mail, id)
	}
	if err := rows.Err(); err != nil {
		return OwnAddressRefile{}, fmt.Errorf("activities: reading the mail of a contact whose address was claimed: %w", err)
	}
	rows.Close()
	if len(mail) > limit {
		return OwnAddressRefile{TooMany: true}, nil
	}
	evidence := map[string]any{"reason": ownAddressClaimedReason, "decided_as": member}
	out := OwnAddressRefile{}
	for _, id := range mail {
		moved, err := unlinkOwnAddressContactTx(ctx, tx, id, contact, evidence)
		if err != nil {
			return OwnAddressRefile{}, err
		}
		if moved {
			out.Refiled++
		}
	}
	return out, nil
}

// unlinkOwnAddressContactTx drops one message's link and participant rows for
// the contact under the message's row lock, with the audit row and event
// every unlink writes. The attendee lock comes before any participant write.
func unlinkOwnAddressContactTx(ctx context.Context, tx pgx.Tx, id ids.ActivityID, contact ids.ContactID, evidence map[string]any) (bool, error) {
	if _, err := storekit.LockRow(ctx, tx, "activity", id.UUID, storekit.LiveOnly); err != nil {
		return false, err
	}
	if err := storekit.LockMeetingAttendeeWrites(ctx, tx, id.UUID); err != nil {
		return false, err
	}
	if _, err := tx.Exec(ctx, `DELETE FROM activity_participant WHERE activity_id = $1 AND contact_id = $2`,
		id, contact.UUID); err != nil {
		return false, fmt.Errorf("activities: taking a claimed address off a message's participants: %w", err)
	}
	return unlinkRecordedTx(ctx, tx, id, linkEntityContact, contact.UUID, nil, evidence)
}
