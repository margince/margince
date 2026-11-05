// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package capture

import (
	"context"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// liveConnectionPredicate is a connection a sync still selects or a human
// still has to act on: not withdrawn, not archived.
const liveConnectionPredicate = `c.status <> 'disconnected' AND c.archived_at IS NULL`

// LiveConnectionsTx counts each member's live mail and calendar connections.
// A member with none is absent from the map.
func LiveConnectionsTx(ctx context.Context, tx pgx.Tx, userIDs []ids.UUID) (map[ids.UUID]int, error) {
	out := make(map[ids.UUID]int, len(userIDs))
	if len(userIDs) == 0 {
		return out, nil
	}
	rows, err := tx.Query(ctx, `
		SELECT c.user_id, count(*) FROM capture_connection c
		 WHERE c.user_id = ANY($1) AND `+liveConnectionPredicate+`
		 GROUP BY c.user_id`, userIDs)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	for rows.Next() {
		var user ids.UUID
		var count int
		if err := rows.Scan(&user, &count); err != nil {
			return nil, err
		}
		out[user] = count
	}
	return out, rows.Err()
}

// RecheckMembersTx makes these members' live connections due now, so the next
// sweep asks CaptureRefusal again after their role changed. It moves only the
// clock: the sync itself parks or resumes the connection.
func RecheckMembersTx(ctx context.Context, tx pgx.Tx, userIDs []ids.UUID) error {
	if len(userIDs) == 0 {
		return nil
	}
	_, err := tx.Exec(ctx, `
		UPDATE capture_sync_state s SET next_sync_at = now()
		  FROM capture_connection c
		 WHERE c.id = s.connection_id AND c.user_id = ANY($1) AND `+liveConnectionPredicate+`
		   AND s.next_sync_at > now()`, userIDs)
	return err
}
