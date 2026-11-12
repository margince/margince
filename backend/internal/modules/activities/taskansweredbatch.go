// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package activities

import (
	"context"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/platform/datatask"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

const openContactTaskSQL = `FROM activity task WHERE task.kind='task' AND NOT task.is_done AND task.archived_at IS NULL AND task.assignee_id IS NOT NULL
 AND EXISTS (SELECT 1 FROM activity_link target WHERE target.activity_id=task.id AND target.contact_id IS NOT NULL)`

// CompleteAnsweredTasksTx applies the answered-task rule to mail stored before
// the rule ran. Each open contact task closes on the newest mail that answers
// it, with the evidence and Undo a live completion writes.
func (s *Store) CompleteAnsweredTasksTx(ctx context.Context, tx pgx.Tx, after ids.UUID, limit int) ([]ids.UUID, int, error) {
	if err := auth.RequireSystem(ctx); err != nil {
		return nil, 0, err
	}
	args := schedulingArgs{}
	rows, err := tx.Query(ctx, `SELECT task.id,task.assignee_id::text `+openContactTaskSQL+` AND task.id>`+args.add(after)+` ORDER BY task.id LIMIT `+args.add(limit), args...)
	if err != nil {
		return nil, 0, err
	}
	type openTask struct {
		id       ids.UUID
		assignee string
	}
	page, err := pgx.CollectRows(rows, func(row pgx.CollectableRow) (openTask, error) {
		var one openTask
		err := row.Scan(&one.id, &one.assignee)
		return one, err
	})
	if err != nil {
		return nil, 0, err
	}
	scanned := make([]ids.UUID, 0, len(page))
	completed := 0
	for _, task := range page {
		scanned = append(scanned, task.id)
		email, found, err := newestAnswerTx(ctx, tx, task.id, task.assignee)
		if err != nil {
			return nil, 0, err
		}
		if !found {
			continue
		}
		tasks, err := tasksAnsweredByTx(ctx, tx, email, task.assignee, &task.id)
		if err != nil {
			return nil, 0, err
		}
		n, err := s.closeAnsweredTasksTx(ctx, tx, email, tasks)
		if err != nil {
			return nil, 0, err
		}
		completed += n
	}
	return scanned, completed, nil
}

// newestAnswerTx is the newest mail the rule admits for the task, so the
// receipt cites the latest answer.
func newestAnswerTx(ctx context.Context, tx pgx.Tx, task ids.UUID, assignee string) (ids.ActivityID, bool, error) {
	args := schedulingArgs{}
	human := args.add(assignee)
	// The sender filter only narrows the scan; answeringHuman below decides.
	rows, err := tx.Query(ctx, `SELECT sent.id,sent.captured_by,who.seat FROM activity task JOIN activity sent ON `+answeringMailSQL("sent")+`
 CROSS JOIN LATERAL (SELECT `+answeringSenderSQL("sent")+` AS seat) who
 WHERE task.id=`+args.add(task)+` AND `+answeredTaskSQL("task", "sent", human)+`
 AND (who.seat::text=`+human+` OR (who.seat IS NULL AND strpos(sent.captured_by,`+human+`)>0))
 ORDER BY sent.occurred_at DESC,sent.id DESC`, args...)
	if err != nil {
		return ids.ActivityID{}, false, err
	}
	defer rows.Close()
	for rows.Next() {
		var email ids.UUID
		var stamp string
		var seat *ids.UUID
		if err := rows.Scan(&email, &stamp, &seat); err != nil {
			return ids.ActivityID{}, false, err
		}
		if answeringHuman(stamp, seat) == assignee {
			return ids.From[ids.ActivityKind](email), true, nil
		}
	}
	return ids.ActivityID{}, false, rows.Err()
}

// CountAnsweredTaskCandidates caps the estimate at the open contact tasks the
// batch would inspect.
func CountAnsweredTaskCandidates(ctx context.Context, q datatask.Querier, maximum int64) (int64, bool, error) {
	var count int64
	args := schedulingArgs{}
	err := q.QueryRow(ctx, `SELECT count(*) FROM (SELECT task.id `+openContactTaskSQL+` LIMIT `+args.add(maximum+1)+`) candidates`, args...).Scan(&count)
	return min(count, maximum), count > maximum, err
}
