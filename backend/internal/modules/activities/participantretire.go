// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package activities

// Retiring the participant rows a later, better-resolved row replaced.

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// RetireSupersededAttendeesTx deletes the address-only participant rows of one
// activity that a resolved row for the same address and role has replaced.
//
// It exists because the participant uniqueness index keys on the identity
// columns as well as the address: `uq_activity_participant` is over
// (activity_id, role, user_id, person_id, address), so writing a row that names
// the SAME address with a resolved user_id is a different key rather than a
// conflict. A pass that resolves an attendee therefore leaves TWO rows
// describing one person — the old unresolved one and the new bound one — and
// every reader that folds participants (the interaction graph, the attendee
// list on a meeting) then reports that colleague as an unresolved external
// party while a resolved row sits beside it.
//
// Exported for the meeting attendee repair in compose, which is where the
// resolution happens: the parsers it needs span two modules, so the pass cannot
// live in one. The WRITE lives here because activities owns this table.
//
// Narrow in three ways, each deliberate:
//
//   - only where a resolved twin EXISTS, so an attendee nobody has a record for
//     keeps their address-only row. That row is a fact about the meeting and
//     nothing has superseded it;
//   - matched on role as well as address, so somebody who appears both as
//     organizer and attendee keeps both rows;
//   - never a row that already carries an id, so nothing a human or another pass
//     established is removed.
//
// No display name is lost: the stamp that resolves an attendee carries the
// invitation's own name onto the row it writes, so what is deleted here holds no
// name the surviving row lacks.
func RetireSupersededAttendeesTx(ctx context.Context, tx pgx.Tx, activityID ids.ActivityID) error {
	if _, err := tx.Exec(ctx, `
		DELETE FROM activity_participant old
		 WHERE old.activity_id = $1
		   AND old.user_id IS NULL
		   AND old.person_id IS NULL
		   AND old.address IS NOT NULL
		   AND EXISTS (
		       SELECT 1 FROM activity_participant resolved
		        WHERE resolved.activity_id = old.activity_id
		          AND resolved.role = old.role
		          AND resolved.address = old.address
		          AND (resolved.user_id IS NOT NULL OR resolved.person_id IS NOT NULL))`,
		activityID); err != nil {
		return fmt.Errorf("activities: retiring the attendee rows a resolved one replaced: %w", err)
	}
	return nil
}
