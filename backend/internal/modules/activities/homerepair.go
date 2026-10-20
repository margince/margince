// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package activities

import (
	"context"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/platform/database/storekit"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// HomeRepairTask carries no source content; its reader must authorize the source separately.
type HomeRepairTask struct {
	ID           ids.ActivityID
	Version      int64
	SourceID     *ids.UUID
	SourceSystem string
	Locator      string
	AssigneeID   *ids.UUID
	DueAt        *time.Time
	Pristine     bool
}

// HomeRepairTasksTx keeps preview and repair on the same bounded, stable task order.
func HomeRepairTasksTx(ctx context.Context, tx pgx.Tx, after ids.UUID, limit int) ([]HomeRepairTask, error) {
	if err := auth.Require(ctx, "activity", principal.ActionRead); err != nil {
		return nil, err
	}
	args := []any{}
	arg := func(value any) int { args = append(args, value); return len(args) }
	scope, err := auth.ActivityContentClause(ctx, "a", arg)
	if err != nil {
		return nil, err
	}
	query := storekit.SQLf(`SELECT a.id,a.version,a.source_activity_id,coalesce(a.source_system,''),coalesce(a.source_id,''),a.assignee_id,a.due_at,
 %s FROM activity a WHERE a.kind='task' AND NOT a.is_done AND a.archived_at IS NULL
 AND a.id > $%d AND (%s) AND (a.source_system=ANY($%d) OR %s)
 ORDER BY a.id LIMIT $%d`, pristineRepairTaskSQL("a"), arg(after), scope,
		arg([]string{CommitmentTaskSource, EmailRequestTaskSource}), SystemMintedExpr("a"), arg(storekit.ClampLimit(&limit)))
	rows, err := tx.Query(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	return pgx.CollectRows(rows, func(row pgx.CollectableRow) (HomeRepairTask, error) {
		var task HomeRepairTask
		err := row.Scan(&task.ID, &task.Version, &task.SourceID, &task.SourceSystem, &task.Locator, &task.AssigneeID, &task.DueAt, &task.Pristine)
		return task, err
	})
}

// A missing audit trail cannot prove non-adoption. Version one and its machine creation
// must both survive; any later write or pin leaves the task for a person to review.
func pristineRepairTaskSQL(alias string) string {
	return storekit.SQLf(`(%[1]s.kind='task' AND NOT %[1]s.is_done AND %[1]s.archived_at IS NULL AND %[1]s.version=1 AND EXISTS (
 SELECT 1 FROM audit_log creation WHERE creation.entity_type='activity' AND creation.entity_id=%[1]s.id
 AND creation.action='create' AND creation.actor_type IN ('system','agent'))
 AND NOT %s AND NOT EXISTS (SELECT 1 FROM worklist_pin pin WHERE pin.source='task' AND pin.row_id=%[1]s.id::text))`, alias, humanWroteReminderSQL(alias))
}
