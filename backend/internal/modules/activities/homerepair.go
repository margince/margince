// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package activities

import (
	"context"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/platform/database/storekit"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// HomeRepairTask carries no source content; its reader must authorize the source separately.
type HomeRepairTask struct {
	ID           ids.ActivityID
	Version      int64
	SourceID     *ids.UUID
	SourceSystem string
	CapturedBy   string
	Locator      string
	AssigneeID   *ids.UUID
	DueAt        *time.Time
	Pristine     bool
}

const paramRepairSource = "source"

// HomeRepairTasksTx keeps preview and repair on the same bounded, stable task order.
func HomeRepairTasksTx(ctx context.Context, tx pgx.Tx, after ids.UUID, limit int) ([]HomeRepairTask, error) {
	return readHomeRepairTasksTx(ctx, tx, after, nil, limit)
}

// HomeRepairTaskTx applies the same content and eligibility scope to a reviewed identity.
func HomeRepairTaskTx(ctx context.Context, tx pgx.Tx, id ids.UUID) (HomeRepairTask, error) {
	rows, err := readHomeRepairTasksTx(ctx, tx, ids.Nil, &id, 1)
	if err != nil {
		return HomeRepairTask{}, err
	}
	if len(rows) == 0 {
		return HomeRepairTask{}, apperrors.ErrNotFound
	}
	return rows[0], nil
}

func readHomeRepairTasksTx(ctx context.Context, tx pgx.Tx, after ids.UUID, exact *ids.UUID, limit int) ([]HomeRepairTask, error) {
	if err := auth.Require(ctx, "activity", principal.ActionRead); err != nil {
		return nil, err
	}
	args := []any{}
	arg := func(value any) int { args = append(args, value); return len(args) }
	scope, err := auth.ActivityContentClause(ctx, "a", arg)
	if err != nil {
		return nil, err
	}
	exactArg := arg(exact)
	query := storekit.SQLf(`SELECT a.id,a.version,a.source_activity_id,coalesce(a.source_system,''),coalesce(a.source_id,''),a.captured_by,a.assignee_id,a.due_at,
 %s FROM activity a WHERE a.kind='task' AND NOT a.is_done AND a.archived_at IS NULL
 AND ($%d::uuid IS NULL OR a.id=$%d) AND a.id > $%d AND (%s) AND (a.source_system=ANY($%d) OR %s)
 ORDER BY a.id LIMIT $%d`, pristineRepairTaskSQL("a"), exactArg, exactArg, arg(after), scope,
		arg([]string{CommitmentTaskSource, EmailRequestTaskSource}), SystemMintedExpr("a"), arg(storekit.ClampLimit(&limit)))
	rows, err := tx.Query(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	return pgx.CollectRows(rows, func(row pgx.CollectableRow) (HomeRepairTask, error) {
		var task HomeRepairTask
		err := row.Scan(&task.ID, &task.Version, &task.SourceID, &task.SourceSystem, &task.Locator, &task.CapturedBy, &task.AssigneeID, &task.DueAt, &task.Pristine)
		return task, err
	})
}

// Initial selected-audience setup increments the row before its creation audit.
// Only creation and traced metadata corrections prove non-adoption; other writes preserve work.
func pristineRepairTaskSQL(alias string) string {
	return storekit.SQLf(`(%[1]s.kind='task' AND NOT %[1]s.is_done AND %[1]s.archived_at IS NULL AND %[1]s.remind_at IS NULL AND EXISTS (
 SELECT 1 FROM audit_log creation WHERE creation.entity_type='activity' AND creation.entity_id=%[1]s.id
 AND creation.action='create' AND creation.actor_type IN ('system','agent')
 AND %[1]s.version=CASE WHEN creation.after->>'audience'='selected' THEN 2 ELSE 1 END
 + (SELECT count(*) FROM audit_log correction WHERE correction.entity_type='activity'
 AND correction.entity_id=%[1]s.id AND %s))
 AND NOT %s)`, alias, storekit.AutomaticTaskRepairAuditClause("correction"), storekit.HeldTaskClause(alias))
}

// RequestReminderPastHorizonTx uses the same age boundary as routine retirement.
// Decision reads lock the source and recheck visibility before disclosing eligibility.
func RequestReminderPastHorizonTx(ctx context.Context, tx pgx.Tx, task HomeRepairTask, asOf time.Time, forDecision bool) (bool, error) {
	if err := auth.Require(ctx, "activity", principal.ActionRead); err != nil {
		return false, err
	}
	if task.SourceID == nil || task.SourceSystem != EmailRequestTaskSource || task.CapturedBy != OwedVerdictCapturedBy {
		return false, nil
	}
	if err := auth.EnsureActivityContentVisibleLive(ctx, tx, *task.SourceID); err != nil {
		return false, err
	}
	cutoff, err := requestReminderCutoff(ctx, tx, asOf)
	if err != nil {
		return false, err
	}
	var occurredAt time.Time
	query := `SELECT occurred_at FROM activity WHERE id=@source`
	if forDecision {
		query += " FOR UPDATE"
	}
	if err := tx.QueryRow(ctx, query, pgx.NamedArgs{paramRepairSource: *task.SourceID}).Scan(&occurredAt); err != nil {
		return false, fmt.Errorf("activities: reading reminder source age: %w", err)
	}
	if err := auth.EnsureActivityContentVisibleLive(ctx, tx, *task.SourceID); err != nil {
		return false, err
	}
	return occurredAt.Before(cutoff), nil
}

func requestReminderCutoff(ctx context.Context, tx pgx.Tx, asOf time.Time) (time.Time, error) {
	horizon, err := waitingHorizonAt(ctx, tx, asOf)
	return asOf.AddDate(0, 0, -horizon), err
}
