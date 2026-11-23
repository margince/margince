// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package activities

// Retiring the participant rows a later, better-resolved row replaced.

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/platform/database/storekit"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// RetireSupersededAttendeesTx deletes the address-only participant rows of one
// activity that a resolved row for the same address and role has replaced.
//
// `uq_activity_participant_identity` digests user_id and contact_id beside the address.
// A resolved row is a different key from the unresolved one, so both would stay.
// Only a row with no id and a resolved twin in its role is deleted.
func RetireSupersededAttendeesTx(ctx context.Context, tx pgx.Tx, activityID ids.ActivityID) error {
	if err := storekit.LockMeetingAttendeeWrites(ctx, tx, activityID.UUID); err != nil {
		return err
	}
	if _, err := tx.Exec(ctx, `
		DELETE FROM activity_participant old
		 WHERE old.activity_id = $1
		   AND old.user_id IS NULL
		   AND old.contact_id IS NULL
		   AND old.address IS NOT NULL
		   AND EXISTS (
		       SELECT 1 FROM activity_participant resolved
		        WHERE resolved.activity_id = old.activity_id
		          AND resolved.role = old.role
		          AND resolved.address = old.address
		          AND (resolved.user_id IS NOT NULL OR resolved.contact_id IS NOT NULL))`,
		activityID); err != nil {
		return fmt.Errorf("activities: retiring the attendee rows a resolved one replaced: %w", err)
	}
	return nil
}
