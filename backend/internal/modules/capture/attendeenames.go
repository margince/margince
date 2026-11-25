// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package capture

import (
	"context"
	"fmt"
	"strings"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/platform/database/storekit"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/ports/connector"
)

// RecordAttendeeNames settles the display_name of every participant row on one
// activity from what the invitation said, and marks the rest as unnamed.
//
// It lives here because capture already writes activity_participant. A second
// spelling in compose would let the column set drift. The compose recovery pass calls it.
//
// Every name-less row on the activity is settled, named or not. A row left NULL
// would be offered to the recovery pass again on its next tick. The pass would
// then never drain.
func RecordAttendeeNames(
	ctx context.Context,
	tx pgx.Tx,
	activityID ids.ActivityID,
	participants []connector.MessageParticipant,
) error {
	if err := storekit.LockMeetingAttendeeWrites(ctx, tx, activityID.UUID); err != nil {
		return err
	}
	addresses := make([]string, 0, len(participants))
	names := make([]string, 0, len(participants))
	for _, p := range participants {
		address := strings.ToLower(strings.TrimSpace(p.Email))
		name := strings.TrimSpace(p.DisplayName)
		if address == "" || name == "" {
			continue
		}
		addresses = append(addresses, address)
		names = append(names, name)
	}
	if len(addresses) > 0 {
		if _, err := tx.Exec(ctx, `
			UPDATE activity_participant ap
			   SET display_name = inp.display_name
			  FROM unnest($2::text[], $3::text[]) AS inp(address, display_name)
			 WHERE ap.activity_id = $1
			   AND ap.address = inp.address
			   AND ap.display_name IS NULL`,
			activityID, addresses, names); err != nil {
			return fmt.Errorf("capture: recording the name an invitation gave: %w", err)
		}
	}
	if _, err := tx.Exec(ctx, `
		UPDATE activity_participant SET display_name = ''
		 WHERE activity_id = $1 AND display_name IS NULL`, activityID); err != nil {
		return fmt.Errorf("capture: settling the attendees an invitation did not name: %w", err)
	}
	return nil
}

// namesSomebody reports whether any of these parties arrived carrying a name.
//
// It keeps the naming pass off ordinary mail capture. Mail brings no display
// name through this path, so without the guard every message would run a query
// that finds nobody.
func namesSomebody(participants []connector.MessageParticipant) bool {
	for _, p := range participants {
		if strings.TrimSpace(p.DisplayName) != "" {
			return true
		}
	}
	return false
}
