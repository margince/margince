// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package storekit

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// taskClockWhere selects a record whose stored clock sits on the instant of a
// live task and is not what its fold function now gives. The task half narrows
// the scan to clocks a task can have set. The fold half keeps a corrected
// record out, even when a task and a mail share the second.
func taskClockWhere(table ClockTable) string {
	ident := clockTableIdent(table)
	return `t.last_activity_at IS NOT NULL
		   AND EXISTS (SELECT 1 FROM activity task
		                WHERE task.kind = 'task' AND task.archived_at IS NULL
		                  AND task.occurred_at = t.last_activity_at)
		   AND t.last_activity_at IS DISTINCT FROM last_activity_of_` + ident + `(t.id)`
}

// TaskClocks answers the next records of table after `after`, in id order,
// whose stored clock a task set under a rule that no longer counts one.
func TaskClocks(ctx context.Context, q ClockQuerier, table ClockTable, after ids.UUID, limit int) ([]ids.UUID, error) {
	rows, err := q.Query(ctx, `SELECT t.id FROM `+clockTableIdent(table)+` t
		 WHERE t.id > $1 AND `+taskClockWhere(table)+` ORDER BY t.id LIMIT $2`, after, limit)
	if err != nil {
		return nil, fmt.Errorf("reading %s clocks a task set: %w", table, err)
	}
	return pgx.CollectRows(rows, pgx.RowTo[ids.UUID])
}

// CountTaskClocks counts up to maximum+1 records of table whose stored clock a
// task set.
func CountTaskClocks(ctx context.Context, q ClockQuerier, table ClockTable, maximum int64) (int64, error) {
	var count int64
	if err := q.QueryRow(ctx, `SELECT count(*) FROM (SELECT 1 FROM `+clockTableIdent(table)+` t
		 WHERE `+taskClockWhere(table)+` LIMIT $1) set_by_task`, maximum+1).Scan(&count); err != nil {
		return 0, fmt.Errorf("counting %s clocks a task set: %w", table, err)
	}
	return count, nil
}
