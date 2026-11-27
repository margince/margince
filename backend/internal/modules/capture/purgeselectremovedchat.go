// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package capture

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/ports/connector"
)

// SelectRemovedChatPurgeTx is SelectRemovedPurgeTx for a chat message the
// provider deleted. A chat message has no mailbox alias, so it is found by its
// natural key. The seat's own import row must name it, and the deleting
// connector must have filed it. A colleague's copy and the statutory floor
// shield it as they do for mail.
func SelectRemovedChatPurgeTx(
	ctx context.Context, tx pgx.Tx, seat ids.UUID, connectorID string, key connector.NaturalKey, floor StatutoryFloor,
) (PurgeSubject, error) {
	var subject PurgeSubject
	if seat == ids.Nil || key.SourceSystem == "" || key.SourceID == "" {
		return subject, nil
	}
	args := []any{seat, key.SourceSystem, key.SourceID, connectorID}
	shielded, args := floor.column(len(args), args)
	rows, err := tx.Query(ctx, `
        SELECT a.id,
               `+withheldReason(shielded, floor.shieldedAs())+` AS withheld,
               `+importersOfActivity+` AS importers
          FROM activity a
          JOIN capture_import i ON i.activity_id = a.id AND i.user_id = $1
         WHERE a.source_system = $2 AND a.source_id = $3
           AND a.archived_at IS NULL AND a.sent_by_colleague IS NULL
           AND (a.captured_by = $4 OR starts_with(a.captured_by, $4 || ':'))
         ORDER BY a.id`, args...)
	if err != nil {
		return subject, fmt.Errorf("capture: selecting what a chat deletion would destroy: %w", err)
	}
	if err := collectPurgeRows(rows, &subject, "selecting what a chat deletion would destroy"); err != nil {
		return subject, err
	}
	return subject, nil
}
