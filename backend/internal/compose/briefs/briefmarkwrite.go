// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package briefs

import (
	"context"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

func applyBriefMark(ctx context.Context, tx pgx.Tx, itemID ids.UUID, state string, at time.Time, held snooze) error {
	var on, wall, zone *string
	if held.on != "" {
		on = new(string(held.on))
	}
	if held.intent != nil {
		wall, zone = &held.intent.WallClock, &held.intent.Timezone
	}
	args := []any{itemID, state, at, held.until, on, held.ref}
	wallHolder := fmt.Sprintf("$%d::text::timestamp", len(args)+1)
	zoneHolder := fmt.Sprintf("$%d", len(args)+2)
	args = append(args, wall, zone)
	_, err := tx.Exec(ctx, `UPDATE brief_item SET state = $2, state_at = $3, snoozed_until = $4,
 reopen_on = $5, reopen_ref = $6, snoozed_wall = `+wallHolder+`, snoozed_zone = `+zoneHolder+` WHERE id = $1`, args...)
	return err
}
