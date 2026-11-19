// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package privacy

// The one-time link material behind a controller delivery and a meeting
// invitation: a live capability sitting in the key vault, referenced by a row
// Art. 17 is about to scrub.
//
// Erasure never touches the vault. The transaction owns the rows, and a
// rollback would bring back rows naming a secret that was already destroyed.
// So the rows keep their reference and carry the mark each owner's retrying
// pass already reads: a delivery's payload is due now, an invitation is erased
// and an erased proposal's token is the tombstone. The destroying pass deletes
// the secret after this transaction commits, and clears the reference only once
// the vault has answered, so a refusing vault is retried.

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/shared/kernel/erasedmark"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// erasePayloads marks the link material behind the subject's deliveries and
// meetings for destruction after commit, and scrubs what names the subject.
//
// A subject is reached through their DELIVERIES rather than through a contact
// column, because comms_outbound carries no subject id: the tie is the activity,
// which is exactly how the rest of the delivery scrub finds these rows.
func erasePayloads(ctx context.Context, tx pgx.Tx, activityIDs []ids.UUID) error {
	if len(activityIDs) == 0 {
		return nil
	}
	if _, err := tx.Exec(ctx, `
		UPDATE comms_outbound SET payload_expires_at = now()
		 WHERE activity_id = ANY($1) AND payload_ref IS NOT NULL`, activityIDs); err != nil {
		return fmt.Errorf("privacy: marking the subject's link material due: %w", err)
	}
	if _, err := tx.Exec(ctx, `
		UPDATE meeting_proposal SET request = '{}'::jsonb, used_at = coalesce(used_at, now()),
		       token_hash = $2 || activity_id::text
		 WHERE activity_id = ANY($1)`, activityIDs, erasedmark.Prefix); err != nil {
		return fmt.Errorf("privacy: retiring the subject's scheduling links: %w", err)
	}
	if _, err := tx.Exec(ctx, `UPDATE meeting_invitation SET
 appointment=jsonb_build_object('CalendarID',calendar_id,'RequestID',appointment->>'RequestID'),
 receipt=CASE WHEN receipt IS NULL THEN NULL ELSE jsonb_build_object('event_id',receipt->>'event_id') END,
 public_intent=NULL,status='erased',command='cancel',reminder_status='off',management_hash=$2||activity_id::text,
 version=version+1,next_attempt_at=now(),updated_at=now()
 WHERE activity_id=ANY($1) AND status<>'erased'`, activityIDs, erasedmark.Prefix); err != nil {
		return err
	}
	return nil
}
