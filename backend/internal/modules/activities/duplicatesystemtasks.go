// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package activities

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/platform/datatask"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

const (
	repairAfterArg              = "after"
	automaticTaskRepairEvidence = "automatic_task_repair"
)

func duplicateLeadTaskSQL(alias string) string {
	return `EXISTS(SELECT 1 FROM activity_link original
 JOIN activity_link next_link ON next_link.lead_id=original.lead_id
 JOIN activity newest ON newest.id=next_link.activity_id
 WHERE original.activity_id=` + alias + `.id AND newest.kind='task' AND NOT newest.is_done AND newest.archived_at IS NULL
 AND newest.source_system=` + alias + `.source_system AND (newest.created_at,newest.id)>(` + alias + `.created_at,` + alias + `.id))`
}

// RetireDuplicateLeadTasksTx returns inspected task IDs, bounding both reads and writes by limit.
// Human work is preserved; the newest open task on each lead remains.
func RetireDuplicateLeadTasksTx(ctx context.Context, tx pgx.Tx, source string, after ids.UUID, limit int, now time.Time) ([]ids.UUID, int, error) {
	if err := auth.RequireSystem(ctx); err != nil {
		return nil, 0, err
	}
	rows, err := tx.Query(ctx, `SELECT t.id FROM activity t WHERE t.source_system=@source AND t.id>@after AND `+pristineRepairTaskSQL("t")+` AND `+duplicateLeadTaskSQL("t")+` ORDER BY t.id LIMIT @limit`, pgx.NamedArgs{ledgerSourceKey: source, repairAfterArg: after, argLimit: limit})
	if err != nil {
		return nil, 0, fmt.Errorf("activities: reading duplicate tasks: %w", err)
	}
	scanned, err := pgx.CollectRows(rows, pgx.RowTo[ids.UUID])
	if err != nil {
		return nil, 0, err
	}
	// The writer locks the lead before its task; repair takes those locks in the same order.
	if _, err := tx.Exec(ctx, `SELECT id FROM lead WHERE id IN (SELECT lead_id FROM activity_link WHERE activity_id=ANY(@tasks)) ORDER BY id FOR UPDATE`, pgx.NamedArgs{"tasks": scanned}); err != nil {
		return nil, 0, err
	}
	retired := 0
	for _, id := range scanned {
		var duplicate bool
		if err := tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM activity t WHERE t.id=@id AND t.source_system=@source AND `+pristineRepairTaskSQL("t")+` AND `+duplicateLeadTaskSQL("t")+`)`, pgx.NamedArgs{"id": id, ledgerSourceKey: source}).Scan(&duplicate); err != nil {
			return nil, 0, err
		}
		if !duplicate {
			continue
		}
		if _, err := archiveActivityInTx(ctx, tx, ids.From[ids.ActivityKind](id), nil, archiveOptions{pristineTask: true}, now); err != nil {
			if errors.Is(err, apperrors.ErrConflict) || errors.Is(err, apperrors.ErrNotFound) {
				continue
			}
			return nil, 0, fmt.Errorf("activities: retiring duplicate task: %w", err)
		}
		retired++
	}
	return scanned, retired, nil
}

// CountDuplicateLeadTasks caps the estimate at the operator's requested ceiling.
func CountDuplicateLeadTasks(ctx context.Context, q datatask.Querier, source string, maximum int64) (int64, bool, error) {
	var count int64
	err := q.QueryRow(ctx, `SELECT count(*) FROM (SELECT t.id FROM activity t WHERE t.source_system=@source AND `+pristineRepairTaskSQL("t")+` AND `+duplicateLeadTaskSQL("t")+` LIMIT @limit) candidates`, pgx.NamedArgs{ledgerSourceKey: source, argLimit: maximum + 1}).Scan(&count)
	return min(count, maximum), count > maximum, err
}
