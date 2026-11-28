// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package activities

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/margince/margince/backend/internal/shared/kernel/correspondence"

	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/platform/database/storekit"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// NameBirthRosterTx names seat as the roster of the guest rows a calendar wrote
// when it made the meeting, before rows recorded their roster. The caller
// proved that seat's calendar made the meeting and listed these addresses.
//
// A row counts only when it was written with the meeting, which is what a
// calendar did then. A row that names a contact must still hold an address of
// that contact. Otherwise a member may have moved it, and no roster retires that.
func NameBirthRosterTx(ctx context.Context, tx pgx.Tx, activityID ids.ActivityID, seat ids.UUID, listed []string) (int, error) {
	if err := auth.RequireSystem(ctx); err != nil {
		return 0, err
	}
	addresses := make([]string, 0, len(listed))
	for _, address := range listed {
		if folded := correspondence.Fold(address); folded != "" {
			addresses = append(addresses, folded)
		}
	}
	if seat == ids.Nil || len(addresses) == 0 {
		return 0, nil
	}
	if err := storekit.LockMeetingAttendeeWrites(ctx, tx, activityID.UUID); err != nil {
		return 0, err
	}
	tag, err := tx.Exec(ctx, `
		UPDATE activity_participant ap SET `+storekit.StoreRosterSeatsSQL("ARRAY[$2::uuid]")+`
		  FROM activity a
		 WHERE a.id = ap.activity_id AND ap.activity_id = $1 AND a.kind = 'meeting'
		   AND a.archived_at IS NULL AND a.restricted_at IS NULL
		   AND `+BirthRosterRowSQL("ap", "a")+` AND ap.address = ANY($3)
		   AND (ap.contact_id IS NULL OR EXISTS (
		         SELECT 1 FROM contact_email held
		          WHERE held.contact_id = ap.contact_id AND held.email = ap.address))`,
		activityID, seat, addresses)
	if err != nil {
		return 0, fmt.Errorf("activities: naming the roster of a meeting's first guests: %w", err)
	}
	return int(tag.RowsAffected()), nil
}

// BirthRosterRowSQL selects a guest row written with its meeting that names no
// roster and that no member relinked.
func BirthRosterRowSQL(participant, activity string) string {
	return fmt.Sprintf(`(NOT `+storekit.OnACalendarRosterSQL(participant)+` AND %[1]s.relinked_at IS NULL
		AND %[1]s.address IS NOT NULL AND %[1]s.created_at = %[2]s.created_at)`, participant, activity)
}
