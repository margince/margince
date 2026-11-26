// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package capture

// What a personal verdict on a chat burst withdraws, and what it keeps from
// being made again. Mail answers by the thread key and the counterparty's
// address. A chat message has no address. A burst answers by its messages,
// found through capture_import.burst_id, and by the contacts those messages name
// as participants, channel-identity-only contacts included.

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/threadverdict"
)

// chatThreadIsPrivateTx says whether one of this seat's bursts on the chat
// thread was judged personal and is still held. Later messages of that chat
// create no contact, as mail on a personal thread creates none. They are still
// filed under the contacts the sender already has. The seat shares the burst or
// holds it for another reason to lift this.
func chatThreadIsPrivateTx(ctx context.Context, tx pgx.Tx, user ids.UUID, threadKey string) (bool, string, error) {
	var status string
	err := tx.QueryRow(ctx, `
		SELECT tv.status
		  FROM activity a
		  JOIN capture_import b ON b.activity_id = a.id AND b.user_id = $2 AND b.burst_id IS NOT NULL
		  JOIN capture_thread_verdict tv ON tv.user_id = $2 AND tv.thread_key = `+threadverdict.BurstKeySQL("b.burst_id")+`
		 WHERE a.thread_key = $1 AND a.archived_at IS NULL AND a.restricted_at IS NULL AND tv.kind = $3 AND tv.status IN ($4, $5)
		 LIMIT 1`, threadKey, user, ThreadKindPersonal, VerdictHeld, VerdictHeldByOwner).Scan(&status)
	if err == pgx.ErrNoRows {
		return false, "", nil
	}
	if err != nil {
		return false, "", fmt.Errorf("capture: reading whether this chat holds a private burst: %w", err)
	}
	return true, status, nil
}

// chatBurstIsPrivateTx says whether the burst one stored chat message belongs to
// was judged personal and is still held.
func chatBurstIsPrivateTx(ctx context.Context, tx pgx.Tx, activity ids.ActivityID, user ids.UUID) (bool, error) {
	var private bool
	if err := tx.QueryRow(ctx, `
		SELECT EXISTS (
		  SELECT 1
		    FROM capture_import b
		    JOIN capture_thread_verdict tv ON tv.user_id = b.user_id AND tv.thread_key = `+threadverdict.BurstKeySQL("b.burst_id")+`
		   WHERE b.activity_id = $1 AND b.user_id = $2 AND tv.kind = $3 AND tv.status IN ($4, $5))`,
		activity, user, ThreadKindPersonal, VerdictHeld, VerdictHeldByOwner).Scan(&private); err != nil {
		return false, fmt.Errorf("capture: reading whether this chat message's burst is private: %w", err)
	}
	return private, nil
}

// contactsOrphanedByBurstTx answers which contacts a personal burst left with no
// business reason to exist. They are the contacts its messages are filed under,
// the seat's own or none's, since a chat channel creates its contact ownerless.
// Any other message of theirs that was judged business, opened to the workspace
// or imported by a colleague keeps them. A message still waiting for its verdict
// is silence, not evidence, as for mail (ContactsOrphanedByPrivacyTx).
func contactsOrphanedByBurstTx(ctx context.Context, tx pgx.Tx, burst, user ids.UUID) ([]PrivateThreadContact, error) {
	rows, err := tx.Query(ctx, `
		SELECT DISTINCT p.id, $1::uuid
		  FROM capture_import b
		  JOIN activity a ON a.id = b.activity_id AND a.archived_at IS NULL AND a.restricted_at IS NULL
		  JOIN activity_link l ON l.activity_id = a.id AND l.entity_type = 'contact'
		  JOIN contact p ON p.id = l.contact_id AND p.archived_at IS NULL
		 WHERE b.user_id = $1 AND b.burst_id = $2 AND (p.owner_id = $1 OR p.owner_id IS NULL)
		   AND NOT EXISTS (
		         SELECT 1
		           FROM activity_link ol
		           JOIN activity other ON other.id = ol.activity_id
		           LEFT JOIN capture_import oi ON oi.activity_id = other.id AND oi.user_id = $1
		           LEFT JOIN capture_thread_verdict tv ON tv.user_id = $1
		            AND tv.thread_key = coalesce(`+threadverdict.BurstKeySQL("oi.burst_id")+`, other.thread_key)
		          WHERE ol.entity_type = 'contact' AND ol.contact_id = p.id
		            AND oi.burst_id IS DISTINCT FROM $2
		            AND other.archived_at IS NULL AND other.restricted_at IS NULL
		            AND ((tv.id IS NOT NULL AND tv.status <> $3
		                  AND NOT (tv.kind IS NOT DISTINCT FROM $4 AND tv.status IN ($5, $6)))
		                 OR other.audience = $7
		                 OR EXISTS (SELECT 1 FROM capture_import x
		                             WHERE x.activity_id = other.id AND x.user_id IS DISTINCT FROM $1)))`,
		user, burst, VerdictPending, ThreadKindPersonal, VerdictHeld, VerdictHeldByOwner, audienceWorkspace)
	if err != nil {
		return nil, fmt.Errorf("capture: reading the contacts a private chat burst orphaned: %w", err)
	}
	defer rows.Close()
	var out []PrivateThreadContact
	for rows.Next() {
		var c PrivateThreadContact
		if err := rows.Scan(&c.ContactID, &c.OwnerID); err != nil {
			return nil, fmt.Errorf("capture: reading the contacts a private chat burst orphaned: %w", err)
		}
		out = append(out, c)
	}
	return out, rows.Err()
}
