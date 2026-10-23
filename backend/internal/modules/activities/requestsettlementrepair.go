// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package activities

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/platform/database/storekit"
)

// Superseded model edits are restored only while no human has taken over the task.
func legacySettlementPatch(ctx context.Context, tx pgx.Tx, in RequestSettlementInput) (UpdateActivityInput, error) {
	if in.Request.TaskID == nil {
		return UpdateActivityInput{}, nil
	}
	var args []any
	bind := func(value any) string { args = append(args, value); return fmt.Sprintf("$%d", len(args)) }
	var title *string
	var restoreTitle, restoreDate bool
	err := tx.QueryRow(ctx, `SELECT request.subject, coalesce(task.subject=old.remaining,false),
 old.due_at IS NOT NULL AND task.due_at IS NOT DISTINCT FROM old.due_at
 FROM activity task JOIN activity request ON request.id=task.source_activity_id
 JOIN activity_request_settlement old ON old.request_activity_id=request.id
 WHERE task.id=`+bind(in.Request.TaskID)+` AND task.version=`+bind(in.Request.TaskVersion)+`
 AND task.captured_by=`+bind(OwedVerdictCapturedBy)+` AND task.source_system='email_request'
 AND old.rule_version<>`+bind(storekit.RequestSettlementRule)+` AND NOT (`+storekit.HeldTaskClause("task")+`)`, args...).Scan(&title, &restoreTitle, &restoreDate)
	if errors.Is(err, pgx.ErrNoRows) {
		return UpdateActivityInput{}, nil
	}
	if err != nil {
		return UpdateActivityInput{}, fmt.Errorf("activities: reading the original request title: %w", err)
	}
	patch := UpdateActivityInput{}
	if restoreTitle {
		patch.Subject = new(emailRequestTaskSubject(title))
	}
	if restoreDate {
		patch.Clear = []string{settlementDueCol}
	}
	return patch, nil
}
