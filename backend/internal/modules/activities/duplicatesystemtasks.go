// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package activities

import (
	"context"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/platform/datatask"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// duplicateLeadTasksSQL names the leads holding more than one open system task
// from source $1, the shape KeepOneSystemTaskOnTx now prevents.
const duplicateLeadTasksSQL = `FROM activity_link l JOIN activity t ON t.id = l.activity_id
 WHERE l.lead_id IS NOT NULL AND t.kind = 'task' AND t.source_system = $1
   AND NOT t.is_done AND t.archived_at IS NULL
 GROUP BY l.lead_id HAVING count(DISTINCT t.id) > 1`

// RetireDuplicateLeadTasksTx archives every open system task from source on a
// lead but the newest, for the page of leads after `after`. A task a human
// worked stays. It returns the leads it inspected and the tasks it archived.
func RetireDuplicateLeadTasksTx(ctx context.Context, tx pgx.Tx, source string, after ids.UUID, limit int, now time.Time) ([]ids.UUID, int, error) {
	if err := auth.RequireSystem(ctx); err != nil {
		return nil, 0, err
	}
	rows, err := tx.Query(ctx, `SELECT l.lead_id `+duplicateLeadTasksSQL+` AND l.lead_id > $2 ORDER BY l.lead_id LIMIT $3`, source, after, limit)
	if err != nil {
		return nil, 0, fmt.Errorf("activities: reading leads with duplicate %s tasks: %w", source, err)
	}
	leads, err := pgx.CollectRows(rows, pgx.RowTo[ids.UUID])
	if err != nil {
		return nil, 0, fmt.Errorf("activities: reading leads with duplicate %s tasks: %w", source, err)
	}
	retired := 0
	for _, lead := range leads {
		rows, err := tx.Query(ctx, `SELECT t.id FROM activity t JOIN activity_link l ON l.activity_id = t.id
			 WHERE l.lead_id = $1 AND t.source_system = $2 AND `+pristineRepairTaskSQL("t")+`
			   AND t.id <> (SELECT newest.id FROM activity newest JOIN activity_link nl ON nl.activity_id = newest.id
			                 WHERE nl.lead_id = $1 AND newest.kind = 'task' AND newest.source_system = $2
			                   AND NOT newest.is_done AND newest.archived_at IS NULL
			                 ORDER BY newest.created_at DESC, newest.id DESC LIMIT 1)
			 FOR UPDATE OF t`, lead, source)
		if err != nil {
			return nil, 0, fmt.Errorf("activities: reading the duplicate %s tasks on a lead: %w", source, err)
		}
		older, err := pgx.CollectRows(rows, pgx.RowTo[ids.UUID])
		if err != nil {
			return nil, 0, fmt.Errorf("activities: reading the duplicate %s tasks on a lead: %w", source, err)
		}
		for _, id := range older {
			if _, err := archiveActivityInTx(ctx, tx, ids.From[ids.ActivityKind](id), nil, archiveOptions{}, now); err != nil {
				return nil, 0, fmt.Errorf("activities: retiring the duplicate task %s: %w", id, err)
			}
			retired++
		}
	}
	return leads, retired, nil
}

// CountLeadsWithDuplicateTasks caps the estimate at the leads the batch would
// inspect.
func CountLeadsWithDuplicateTasks(ctx context.Context, q datatask.Querier, source string, maximum int64) (int64, bool, error) {
	var count int64
	err := q.QueryRow(ctx, `SELECT count(*) FROM (SELECT l.lead_id `+duplicateLeadTasksSQL+` LIMIT $2) candidates`, source, maximum+1).Scan(&count)
	return min(count, maximum), count > maximum, err
}
